package email

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"time"
)

// dialTimeout bounds a connection attempt. Without it a hung SMTP host holds
// the sending goroutine open indefinitely.
const dialTimeout = 15 * time.Second

// SMTPSender delivers mail over SMTP.
//
// net/smtp is stdlib, so this costs no dependency. It is also unmaintained and
// deliberately minimal, which is why the connection is built by hand below
// rather than through smtp.SendMail — that helper cannot do implicit TLS on
// port 465, and it always issues STARTTLS opportunistically without letting the
// caller require it.
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
	fromMail string
	fromName string

	// dial is swappable so tests can drive a fake server without a real socket.
	dial func(ctx context.Context, addr string) (net.Conn, error)
	// tlsRoots lets a test trust a self-signed fake. Nil means the system pool,
	// so certificate verification is never skipped in production — there is no
	// InsecureSkipVerify anywhere in this file, deliberately.
	tlsRoots *x509.CertPool
	// boundary makes MIME boundaries deterministic under test.
	boundary func() string
}

// SMTPConfig groups what SMTPSender needs.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	FromMail string
	FromName string
}

// NewSMTPSender builds a sender for a real SMTP host.
func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	return &SMTPSender{
		host:     cfg.Host,
		port:     cfg.Port,
		username: cfg.Username,
		password: cfg.Password,
		fromMail: cfg.FromMail,
		fromName: cfg.FromName,
	}
}

// Send delivers one message.
//
// Port 465 is implicit TLS: the connection is encrypted before a single byte of
// SMTP is spoken. Everything else connects in clear and upgrades with STARTTLS,
// which is REQUIRED rather than attempted — an order confirmation carries a
// customer's name, address and phone, and silently falling back to plaintext
// because a server did not advertise the extension is how that leaks.
func (s *SMTPSender) Send(ctx context.Context, m Message) error {
	raw, err := build(s.fromMail, s.fromName, m, s.boundary)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	conn, err := s.dialConn(ctx, addr)
	if err != nil {
		return fmt.Errorf("email: dial %s: %w", addr, err)
	}

	if s.port == 465 {
		conn = tls.Client(conn, s.tlsConfig())
	}

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email: smtp handshake: %w", err)
	}
	defer func() { _ = c.Close() }()

	if s.port != 465 {
		ok, _ := c.Extension("STARTTLS")
		if !ok {
			return fmt.Errorf("email: %s does not offer STARTTLS; refusing to send in clear", s.host)
		}
		if tlsErr := c.StartTLS(s.tlsConfig()); tlsErr != nil {
			return fmt.Errorf("email: starttls: %w", tlsErr)
		}
	}

	if s.username != "" {
		if authErr := c.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); authErr != nil {
			// The error is returned but never logged with the password in it —
			// smtp.PlainAuth errors do not carry the credential.
			return fmt.Errorf("email: auth: %w", authErr)
		}
	}

	if mailErr := c.Mail(s.fromMail); mailErr != nil {
		return fmt.Errorf("email: mail from: %w", mailErr)
	}
	if rcptErr := c.Rcpt(m.To); rcptErr != nil {
		return fmt.Errorf("email: rcpt to: %w", rcptErr)
	}
	w, dataErr := c.Data()
	if dataErr != nil {
		return fmt.Errorf("email: data: %w", dataErr)
	}
	if _, writeErr := w.Write(raw); writeErr != nil {
		return fmt.Errorf("email: write body: %w", writeErr)
	}
	if closeErr := w.Close(); closeErr != nil {
		return fmt.Errorf("email: close body: %w", closeErr)
	}
	return c.Quit()
}

// tlsConfig is the only place TLS settings are built, so the two call sites
// cannot drift apart.
func (s *SMTPSender) tlsConfig() *tls.Config {
	return &tls.Config{
		ServerName: s.host,
		MinVersion: tls.VersionTLS12,
		RootCAs:    s.tlsRoots,
	}
}

func (s *SMTPSender) dialConn(ctx context.Context, addr string) (net.Conn, error) {
	if s.dial != nil {
		return s.dial(ctx, addr)
	}
	// DialContext rather than DialTimeout so a shutdown cancels an in-flight
	// connect instead of holding the drain open for the full timeout.
	d := net.Dialer{Timeout: dialTimeout}
	return d.DialContext(ctx, "tcp", addr)
}
