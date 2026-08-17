// Package email sends transactional mail over SMTP.
//
// It carries no templates and knows nothing about orders: callers build a
// Message and hand it over. That keeps the wire format in one place and lets
// tests assert on a Message rather than on bytes.
package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
)

// ErrNotConfigured is returned by Unconfigured on every call. Callers treat it
// as "mail is switched off", not as a failure worth retrying.
var ErrNotConfigured = errors.New("email: sender is not configured")

// Attachment is one file on a message.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Message is one email.
//
// Both bodies are sent when both are present: a text/plain alternative is not
// decoration. Some clients render it, spam filters weigh its absence, and a
// mail with no plain part is a common reason a transactional email is scored
// down.
type Message struct {
	To       string
	Subject  string
	HTMLBody string
	TextBody string

	Attachments []Attachment
}

// Validate checks a message can be sent at all.
func (m Message) Validate() error {
	if strings.TrimSpace(m.To) == "" {
		return errors.New("email: recipient is required")
	}
	if _, err := mail.ParseAddress(m.To); err != nil {
		return fmt.Errorf("email: recipient %q is not a valid address: %w", m.To, err)
	}
	if strings.TrimSpace(m.Subject) == "" {
		return errors.New("email: subject is required")
	}
	if m.HTMLBody == "" && m.TextBody == "" {
		return errors.New("email: a message needs a body")
	}
	// A header cannot contain a newline. Without this check a crafted subject
	// could inject arbitrary headers — a Bcc, a different From — into the
	// message. Subjects here are built from order data, which is partly
	// shopper-supplied.
	if strings.ContainsAny(m.Subject, "\r\n") || strings.ContainsAny(m.To, "\r\n") {
		return errors.New("email: header fields must not contain newlines")
	}
	return nil
}

// Sender delivers a message. *SMTPSender satisfies it; Unconfigured refuses
// every call, and tests substitute a recorder.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Unconfigured is a Sender that refuses every call, selected when SMTP_HOST is
// empty. It exists so the rest of the system wires up identically whether or
// not mail is set up — the same shape Razorpay, Cloudinary and MSG91 use.
type Unconfigured struct{}

// Send always returns ErrNotConfigured.
func (Unconfigured) Send(context.Context, Message) error { return ErrNotConfigured }

// build renders a message as RFC 5322 bytes.
//
// Layout depends on what the message carries, because a client picks its
// renderer from the top-level content type:
//
//	multipart/mixed          → attachments present
//	  multipart/alternative  → both bodies
//	    text/plain, text/html
//
// With no attachments the mixed wrapper is dropped, and with one body the
// alternative wrapper is dropped too. A single-part message wrapped in two
// pointless layers renders as an empty mail in some clients.
func build(from, fromName string, m Message, boundarySeed func() string) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	fromHeader := from
	if fromName != "" {
		// EncodeWord handles a non-ASCII display name; a bare UTF-8 name in a
		// header is not legal and some servers reject the whole message.
		fromHeader = mime.QEncoding.Encode("utf-8", fromName) + " <" + from + ">"
	}

	writeHeader(&buf, "From", fromHeader)
	writeHeader(&buf, "To", m.To)
	writeHeader(&buf, "Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	writeHeader(&buf, "MIME-Version", "1.0")

	body, contentType, err := buildBody(m, boundarySeed)
	if err != nil {
		return nil, err
	}
	writeHeader(&buf, "Content-Type", contentType)
	buf.WriteString("\r\n")
	buf.Write(body)

	return buf.Bytes(), nil
}

func writeHeader(buf *bytes.Buffer, key, value string) {
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

// buildBody returns the message body and the Content-Type header describing it.
func buildBody(m Message, boundarySeed func() string) ([]byte, string, error) {
	if len(m.Attachments) == 0 {
		return buildBodyOnly(m, boundarySeed)
	}

	var buf bytes.Buffer
	mixed := multipart.NewWriter(&buf)
	if boundarySeed != nil {
		if err := mixed.SetBoundary(boundarySeed()); err != nil {
			return nil, "", fmt.Errorf("email: set boundary: %w", err)
		}
	}

	inner, innerType, err := buildBodyOnly(m, nil)
	if err != nil {
		return nil, "", err
	}
	part, err := mixed.CreatePart(textproto.MIMEHeader{"Content-Type": {innerType}})
	if err != nil {
		return nil, "", fmt.Errorf("email: body part: %w", err)
	}
	if _, err := part.Write(inner); err != nil {
		return nil, "", fmt.Errorf("email: write body: %w", err)
	}

	for _, a := range m.Attachments {
		if err := writeAttachment(mixed, a); err != nil {
			return nil, "", err
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, "", fmt.Errorf("email: close mixed: %w", err)
	}
	return buf.Bytes(), "multipart/mixed; boundary=" + mixed.Boundary(), nil
}

// buildBodyOnly renders just the text and/or HTML parts.
func buildBodyOnly(m Message, boundarySeed func() string) ([]byte, string, error) {
	switch {
	case m.TextBody != "" && m.HTMLBody != "":
		var buf bytes.Buffer
		alt := multipart.NewWriter(&buf)
		if boundarySeed != nil {
			if err := alt.SetBoundary(boundarySeed()); err != nil {
				return nil, "", fmt.Errorf("email: set boundary: %w", err)
			}
		}
		// Plain first: a client that understands both picks the LAST part it
		// can render, so HTML must come second or everyone reads the text
		// version.
		if err := writeTextPart(alt, "text/plain; charset=utf-8", m.TextBody); err != nil {
			return nil, "", err
		}
		if err := writeTextPart(alt, "text/html; charset=utf-8", m.HTMLBody); err != nil {
			return nil, "", err
		}
		if err := alt.Close(); err != nil {
			return nil, "", fmt.Errorf("email: close alternative: %w", err)
		}
		return buf.Bytes(), "multipart/alternative; boundary=" + alt.Boundary(), nil

	case m.HTMLBody != "":
		return encodeQP(m.HTMLBody), "text/html; charset=utf-8", nil
	default:
		return encodeQP(m.TextBody), "text/plain; charset=utf-8", nil
	}
}

func writeTextPart(w *multipart.Writer, contentType, body string) error {
	part, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {contentType},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return fmt.Errorf("email: create part: %w", err)
	}
	if _, err := part.Write(encodeQP(body)); err != nil {
		return fmt.Errorf("email: write part: %w", err)
	}
	return nil
}

// writeAttachment adds one file part, base64 encoded and wrapped at 76 columns
// as RFC 2045 requires — an unwrapped base64 blob exceeds the 998-octet line
// limit and some servers reject or truncate the message.
func writeAttachment(w *multipart.Writer, a Attachment) error {
	ct := a.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	part, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {ct},
		"Content-Transfer-Encoding": {"base64"},
		// The filename is encoded because an order id is ASCII but a future
		// caller's might not be.
		"Content-Disposition": {mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename})},
	})
	if err != nil {
		return fmt.Errorf("email: create attachment part: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(a.Data)
	for i := 0; i < len(encoded); i += 76 {
		end := min(i+76, len(encoded))
		if _, err := part.Write([]byte(encoded[i:end] + "\r\n")); err != nil {
			return fmt.Errorf("email: write attachment: %w", err)
		}
	}
	return nil
}

// encodeQP quoted-printable encodes a body. Without it a line longer than 998
// octets is illegal, and a rupee sign or an en dash arrives mangled.
func encodeQP(s string) []byte {
	var buf bytes.Buffer
	w := quotedprintable.NewWriter(&buf)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return buf.Bytes()
}
