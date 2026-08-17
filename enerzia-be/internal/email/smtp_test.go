package email_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/email"
)

// fakeSMTP is an in-process SMTP server over a net.Pipe. It speaks just enough
// of the protocol to drive the client, so the suite runs offline with no socket
// and no real host — the same posture internal/mongotest takes for Mongo.
type fakeSMTP struct {
	mu sync.Mutex
	// offerSTARTTLS controls the EHLO capability list.
	offerSTARTTLS bool
	// failAt is the verb the server rejects with a 5xx, or "" to accept all.
	failAt string
	// upgradeCert, when set, makes STARTTLS actually succeed and the session
	// continue inside TLS — the port-587 path most real hosts use.
	upgradeCert *tls.Certificate

	transcript []string
	data       string
}

func (f *fakeSMTP) record(line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transcript = append(f.transcript, line)
}

func (f *fakeSMTP) saw(verb string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.transcript {
		if strings.HasPrefix(strings.ToUpper(l), verb) {
			return true
		}
	}
	return false
}

// dial returns a transport that hands the client one end of a pipe and runs the
// server on the other.
func (f *fakeSMTP) dial() func(context.Context, string) (net.Conn, error) {
	return func(context.Context, string) (net.Conn, error) {
		client, server := net.Pipe()
		go f.serve(server)
		return client, nil
	}
}

func (f *fakeSMTP) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	br := bufio.NewReader(c)
	write := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }

	write("220 fake ESMTP")
	inData := false
	var body strings.Builder

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				f.mu.Lock()
				f.data = body.String()
				f.mu.Unlock()
				write("250 OK")
				continue
			}
			body.WriteString(line + "\n")
			continue
		}

		f.record(line)
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])

		if f.failAt != "" && verb == f.failAt {
			write("550 refused")
			continue
		}

		switch verb {
		case "EHLO", "HELO":
			if f.offerSTARTTLS {
				write("250-fake")
				write("250-STARTTLS")
				write("250 AUTH PLAIN")
			} else {
				write("250-fake")
				write("250 AUTH PLAIN")
			}
		case "STARTTLS":
			if f.upgradeCert == nil {
				write("454 TLS not available")
				continue
			}
			write("220 go ahead")
			tlsConn := tls.Server(c, &tls.Config{
				Certificates: []tls.Certificate{*f.upgradeCert},
				MinVersion:   tls.VersionTLS12,
			})
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			// Everything after the upgrade is spoken inside TLS, so swap both
			// the reader and the writer over.
			c = tlsConn
			br = bufio.NewReader(tlsConn)
			write = func(s string) { _, _ = tlsConn.Write([]byte(s + "\r\n")) }
		case "AUTH":
			// 235, not 250 — smtp.PlainAuth treats anything else as a failure.
			write("235 authenticated")
		case "MAIL", "RCPT":
			write("250 OK")
		case "DATA":
			inData = true
			write("354 send it")
		case "QUIT":
			write("221 bye")
			// Do NOT close here. net.Pipe is unbuffered, so closing now leaves
			// the client's TLS close-notify with nobody reading it and Quit
			// fails on a write to a dead pipe. Keep looping; the read below
			// errors once the client hangs up, which is the real end.
			continue
		default:
			write("250 OK")
		}
	}
}

func newSender(t *testing.T, f *fakeSMTP, port int) *email.SMTPSender {
	t.Helper()
	return email.NewSMTPSender(email.SMTPConfig{
		Host: "smtp.test", Port: port,
		Username: "user", Password: "pass",
		FromMail: "ops@enerzia.in", FromName: "Enerzeia Future Farm",
	}).WithDial(f.dial()).WithBoundary(fixedBoundary())
}

// TestSendRefusesPlaintextWhenSTARTTLSIsAbsent is the important one. An order
// confirmation carries a customer's name, address and phone; falling back to an
// unencrypted session because the server did not advertise the extension is how
// that leaks, so the client must refuse rather than downgrade.
func TestSendRefusesPlaintextWhenSTARTTLSIsAbsent(t *testing.T) {
	f := &fakeSMTP{offerSTARTTLS: false}
	s := newSender(t, f, 587)

	err := s.Send(t.Context(), email.Message{To: "a@b.co", Subject: "s", TextBody: "t"})

	if err == nil {
		t.Fatal("Send() succeeded over an unencrypted connection")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("error = %v, want it to name STARTTLS", err)
	}
	// It must bail before handing over the recipient or the body.
	if f.saw("MAIL") || f.saw("RCPT") || f.saw("DATA") {
		t.Errorf("the client sent envelope data in clear: %v", f.transcript)
	}
}

func TestSendAttemptsSTARTTLSWhenOffered(t *testing.T) {
	f := &fakeSMTP{offerSTARTTLS: true}
	s := newSender(t, f, 587)

	// The fake refuses the upgrade at the TLS handshake, so this errors — the
	// point is that the client ASKED, and still sent nothing in clear.
	err := s.Send(t.Context(), email.Message{To: "a@b.co", Subject: "s", TextBody: "t"})
	if err == nil {
		t.Fatal("Send() succeeded despite a refused TLS upgrade")
	}
	if !f.saw("STARTTLS") {
		t.Errorf("the client never issued STARTTLS: %v", f.transcript)
	}
	if f.saw("MAIL") || f.saw("DATA") {
		t.Errorf("the client sent data after a failed upgrade: %v", f.transcript)
	}
}

func TestSendValidatesBeforeConnecting(t *testing.T) {
	// A malformed message must not open a socket at all.
	f := &fakeSMTP{offerSTARTTLS: true}
	s := newSender(t, f, 587)

	err := s.Send(t.Context(), email.Message{To: "", Subject: "s", TextBody: "t"})
	if err == nil {
		t.Fatal("Send() accepted a message with no recipient")
	}
	if len(f.transcript) != 0 {
		t.Errorf("the client connected for an invalid message: %v", f.transcript)
	}
}

func TestSendSurfacesDialFailures(t *testing.T) {
	s := email.NewSMTPSender(email.SMTPConfig{
		Host: "smtp.test", Port: 587, FromMail: "ops@enerzia.in",
	}).WithDial(func(context.Context, string) (net.Conn, error) { return nil, net.ErrClosed })

	err := s.Send(t.Context(), email.Message{To: "a@b.co", Subject: "s", TextBody: "t"})
	if err == nil {
		t.Fatal("Send() returned nil on a dial failure")
	}
	if !strings.Contains(err.Error(), "dial") {
		t.Errorf("error = %v, want it to mention the dial", err)
	}
}

/* ------------------------------------------------- the full success path */

// selfSigned mints a certificate for smtp.test so the fake can complete a real
// TLS handshake. Verification stays ON — the test trusts this CA explicitly
// rather than the client skipping the check.
func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "smtp.test"},
		DNSNames:              []string{"smtp.test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// dialTLS serves the fake behind implicit TLS, as port 465 expects.
func (f *fakeSMTP) dialTLS(cert tls.Certificate) func(context.Context, string) (net.Conn, error) {
	return func(context.Context, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			tlsConn := tls.Server(server, &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			})
			f.serve(tlsConn)
		}()
		return client, nil
	}
}

func TestSendDeliversOverImplicitTLS(t *testing.T) {
	cert, pool := selfSigned(t)
	f := &fakeSMTP{offerSTARTTLS: false} // 465 needs no STARTTLS: it is already encrypted
	s := email.NewSMTPSender(email.SMTPConfig{
		Host: "smtp.test", Port: 465,
		Username: "user", Password: "pass",
		FromMail: "ops@enerzia.in", FromName: "Enerzeia Future Farm",
	}).WithDial(f.dialTLS(cert)).WithTLSRoots(pool).WithBoundary(fixedBoundary())

	err := s.Send(t.Context(), email.Message{
		To:      "ananya@example.com",
		Subject: "Your order EFF-483413",
		// Both bodies plus an attachment: the shape a confirmation will take.
		TextBody: "Thanks for your order.",
		HTMLBody: "<p>Thanks for your order.</p>",
		Attachments: []email.Attachment{{
			Filename: "invoice-EFF-483413.pdf", ContentType: "application/pdf",
			Data: []byte("%PDF-1.4 fake"),
		}},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	for _, verb := range []string{"MAIL", "RCPT", "DATA"} {
		if !f.saw(verb) {
			t.Errorf("the server never saw %s: %v", verb, f.transcript)
		}
	}
	// STARTTLS must NOT be issued on an already-encrypted connection.
	if f.saw("STARTTLS") {
		t.Error("the client issued STARTTLS inside an implicit-TLS session")
	}

	f.mu.Lock()
	body := f.data
	f.mu.Unlock()

	for _, want := range []string{
		"To: ananya@example.com",
		"multipart/mixed",
		"invoice-EFF-483413.pdf",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("delivered message is missing %q", want)
		}
	}
	// The password must never appear in what is transmitted as message content.
	if strings.Contains(body, "pass") {
		t.Error("the delivered body contains the SMTP password")
	}
}

func TestSendSurfacesARejectedRecipient(t *testing.T) {
	cert, pool := selfSigned(t)
	f := &fakeSMTP{failAt: "RCPT"}
	s := email.NewSMTPSender(email.SMTPConfig{
		Host: "smtp.test", Port: 465, FromMail: "ops@enerzia.in",
	}).WithDial(f.dialTLS(cert)).WithTLSRoots(pool)

	err := s.Send(t.Context(), email.Message{To: "a@b.co", Subject: "s", TextBody: "t"})
	if err == nil {
		t.Fatal("Send() returned nil for a rejected recipient")
	}
	if !strings.Contains(err.Error(), "rcpt to") {
		t.Errorf("error = %v, want it to name the failing step", err)
	}
}

func TestSendDeliversOverSTARTTLS(t *testing.T) {
	// Port 587 with an upgrade is what most real hosts use, so this is the path
	// production will actually take.
	cert, pool := selfSigned(t)
	f := &fakeSMTP{offerSTARTTLS: true, upgradeCert: &cert}
	s := email.NewSMTPSender(email.SMTPConfig{
		Host: "smtp.test", Port: 587,
		Username: "user", Password: "pass",
		FromMail: "ops@enerzia.in", FromName: "Enerzeia Future Farm",
	}).WithDial(f.dial()).WithTLSRoots(pool).WithBoundary(fixedBoundary())

	err := s.Send(t.Context(), email.Message{
		To: "ananya@example.com", Subject: "Your order EFF-483413",
		TextBody: "Thanks", HTMLBody: "<p>Thanks</p>",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !f.saw("STARTTLS") {
		t.Error("the client never upgraded")
	}
	for _, verb := range []string{"MAIL", "RCPT", "DATA"} {
		if !f.saw(verb) {
			t.Errorf("the server never saw %s: %v", verb, f.transcript)
		}
	}

	f.mu.Lock()
	body := f.data
	f.mu.Unlock()
	if !strings.Contains(body, "To: ananya@example.com") {
		t.Errorf("delivered message missing recipient header:\n%s", body)
	}
	if !strings.Contains(body, "multipart/alternative") {
		t.Error("both bodies were not sent as alternatives")
	}
}

func TestSendUsesARealDialerByDefault(t *testing.T) {
	// No WithDial: exercises the production transport. Port 1 on localhost has
	// nothing listening, so this fails at connect — which is the branch being
	// covered.
	s := email.NewSMTPSender(email.SMTPConfig{
		Host: "127.0.0.1", Port: 1, FromMail: "ops@enerzia.in",
	})
	if err := s.Send(t.Context(), email.Message{To: "a@b.co", Subject: "s", TextBody: "t"}); err == nil {
		t.Fatal("Send() returned nil connecting to a closed port")
	}
}

func TestSendSurfacesEachRejectedStep(t *testing.T) {
	// Real servers refuse at different points: an unverified sender fails MAIL,
	// a size or policy limit fails DATA. Each must name its step so a log says
	// what actually went wrong.
	tests := []struct{ verb, wantIn string }{
		{"MAIL", "mail from"},
		{"DATA", "data"},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			cert, pool := selfSigned(t)
			f := &fakeSMTP{failAt: tt.verb}
			s := email.NewSMTPSender(email.SMTPConfig{
				Host: "smtp.test", Port: 465, FromMail: "ops@enerzia.in",
			}).WithDial(f.dialTLS(cert)).WithTLSRoots(pool)

			err := s.Send(t.Context(), email.Message{To: "a@b.co", Subject: "s", TextBody: "t"})
			if err == nil {
				t.Fatalf("Send() returned nil when %s was rejected", tt.verb)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error = %v, want it to name %q", err, tt.wantIn)
			}
		})
	}
}

func TestBuildRejectsAnIllegalBoundary(t *testing.T) {
	// A boundary over 70 characters is illegal. This is the only way the
	// multipart writer errors in practice, so it is the one branch of that
	// family worth pinning.
	bad := func() string { return strings.Repeat("X", 100) }

	t.Run("alternative", func(t *testing.T) {
		_, err := email.Build("ops@enerzia.in", "", email.Message{
			To: "a@b.co", Subject: "s", TextBody: "t", HTMLBody: "<p>h</p>",
		}, bad)
		if err == nil {
			t.Fatal("Build() accepted an illegal boundary")
		}
	})

	t.Run("mixed", func(t *testing.T) {
		_, err := email.Build("ops@enerzia.in", "", email.Message{
			To: "a@b.co", Subject: "s", TextBody: "t",
			Attachments: []email.Attachment{{Filename: "f.bin", Data: []byte("x")}},
		}, bad)
		if err == nil {
			t.Fatal("Build() accepted an illegal boundary")
		}
	})
}
