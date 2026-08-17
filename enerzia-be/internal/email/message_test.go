package email_test

import (
	"encoding/base64"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/enerzia/enerzia-be/internal/email"
)

func fixedBoundary() func() string {
	return func() string { return "BOUNDARY0000000000000000" }
}

// parse reads rendered bytes back through net/mail, so the tests assert the
// message is genuinely well-formed rather than that it merely contains some
// substrings.
func parse(t *testing.T, raw []byte) *mail.Message {
	t.Helper()
	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("rendered message does not parse: %v\n%s", err, raw)
	}
	return m
}

func TestValidate(t *testing.T) {
	base := email.Message{To: "a@b.co", Subject: "Order EFF-483413", TextBody: "hi"}

	tests := []struct {
		name    string
		mutate  func(*email.Message)
		wantErr bool
	}{
		{"complete", func(*email.Message) {}, false},
		{"html only", func(m *email.Message) { m.TextBody = ""; m.HTMLBody = "<p>hi</p>" }, false},
		{"no recipient", func(m *email.Message) { m.To = "" }, true},
		{"recipient is not an address", func(m *email.Message) { m.To = "not-an-email" }, true},
		{"no subject", func(m *email.Message) { m.Subject = "" }, true},
		{"no body at all", func(m *email.Message) { m.TextBody = ""; m.HTMLBody = "" }, true},
		// Header injection: a newline in a shopper-influenced field could add a
		// Bcc or rewrite From.
		{"newline in subject", func(m *email.Message) { m.Subject = "Hi\r\nBcc: evil@x.com" }, true},
		{"newline in recipient", func(m *email.Message) { m.To = "a@b.co\nBcc: evil@x.com" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := base
			tt.mutate(&m)
			err := m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildTextOnly(t *testing.T) {
	raw, err := email.Build("ops@enerzia.in", "", email.Message{
		To: "a@b.co", Subject: "Order EFF-483413", TextBody: "Thanks",
	}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	m := parse(t, raw)

	// A single-part message must NOT be wrapped in multipart layers — some
	// clients render an unnecessary wrapper as an empty mail.
	if ct := m.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if got := m.Header.Get("From"); got != "ops@enerzia.in" {
		t.Errorf("From = %q", got)
	}
}

func TestBuildEncodesTheFromNameAndSubject(t *testing.T) {
	raw, err := email.Build("ops@enerzia.in", "Enerzeia Future Farm", email.Message{
		To:       "a@b.co",
		Subject:  "Your order — ₹1,050 paid",
		TextBody: "Thanks",
	}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	m := parse(t, raw)

	// Raw UTF-8 in a header is illegal and some servers reject the message.
	dec := new(mime.WordDecoder)
	subject, err := dec.DecodeHeader(m.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("decoding subject: %v", err)
	}
	if subject != "Your order — ₹1,050 paid" {
		t.Errorf("subject round-trip = %q", subject)
	}
	from, err := mail.ParseAddress(m.Header.Get("From"))
	if err != nil {
		t.Fatalf("From does not parse: %v", err)
	}
	if from.Name != "Enerzeia Future Farm" || from.Address != "ops@enerzia.in" {
		t.Errorf("From = %+v", from)
	}
}

func TestBuildAlternativeOrdersPlainBeforeHTML(t *testing.T) {
	raw, err := email.Build("ops@enerzia.in", "", email.Message{
		To: "a@b.co", Subject: "s", TextBody: "plain version", HTMLBody: "<p>html version</p>",
	}, fixedBoundary())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	m := parse(t, raw)

	mt, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("Content-Type = %q (err %v), want multipart/alternative", m.Header.Get("Content-Type"), err)
	}

	var types []string
	mr := multipart.NewReader(m.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		types = append(types, strings.Split(p.Header.Get("Content-Type"), ";")[0])
	}
	// A client picks the LAST part it can render, so HTML must come second or
	// every recipient reads the plain-text version.
	if len(types) != 2 || types[0] != "text/plain" || types[1] != "text/html" {
		t.Errorf("part order = %v, want [text/plain text/html]", types)
	}
}

func TestBuildWithAttachment(t *testing.T) {
	pdf := []byte(strings.Repeat("PDFDATA", 200)) // long enough to need wrapping
	raw, err := email.Build("ops@enerzia.in", "", email.Message{
		To: "a@b.co", Subject: "s", TextBody: "t", HTMLBody: "<p>h</p>",
		Attachments: []email.Attachment{{
			Filename: "invoice-EFF-483413.pdf", ContentType: "application/pdf", Data: pdf,
		}},
	}, fixedBoundary())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	m := parse(t, raw)

	mt, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/mixed" {
		t.Fatalf("Content-Type = %q, want multipart/mixed", m.Header.Get("Content-Type"))
	}

	var (
		sawBody  bool
		sawFile  bool
		fileData []byte
	)
	mr := multipart.NewReader(m.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		ct := p.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(ct, "multipart/alternative"):
			sawBody = true
		case strings.HasPrefix(ct, "application/pdf"):
			sawFile = true
			if fn := p.FileName(); fn != "invoice-EFF-483413.pdf" {
				t.Errorf("attachment filename = %q", fn)
			}
			// multipart.Reader does not decode base64 for us.
			var sb strings.Builder
			buf := make([]byte, 1024)
			for {
				n, rerr := p.Read(buf)
				sb.Write(buf[:n])
				if rerr != nil {
					break
				}
			}
			decoded, derr := base64.StdEncoding.DecodeString(
				strings.NewReplacer("\r", "", "\n", "").Replace(sb.String()))
			if derr != nil {
				t.Fatalf("attachment is not valid base64: %v", derr)
			}
			fileData = decoded
		}
	}

	if !sawBody {
		t.Error("the message body part is missing")
	}
	if !sawFile {
		t.Fatal("the attachment part is missing")
	}
	if string(fileData) != string(pdf) {
		t.Errorf("attachment round-trip failed: got %d bytes, want %d", len(fileData), len(pdf))
	}
}

func TestBuildWrapsBase64At76Columns(t *testing.T) {
	// An unwrapped base64 blob breaks the 998-octet line limit and servers
	// truncate or reject the message.
	raw, err := email.Build("ops@enerzia.in", "", email.Message{
		To: "a@b.co", Subject: "s", TextBody: "t",
		Attachments: []email.Attachment{{
			Filename: "f.bin", Data: []byte(strings.Repeat("A", 5000)),
		}},
	}, fixedBoundary())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for i, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 998 {
			t.Fatalf("line %d is %d octets, over the RFC limit", i, len(line))
		}
	}
}

func TestUnconfiguredRefusesEverything(t *testing.T) {
	err := email.Unconfigured{}.Send(t.Context(), email.Message{To: "a@b.co", Subject: "s", TextBody: "t"})
	if err == nil {
		t.Fatal("Unconfigured.Send() returned nil")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %v, want it to say the sender is not configured", err)
	}
}
