package email

import (
	"context"
	"crypto/x509"
	"net"
)

// Build exposes the wire renderer to tests in the external test package.
func Build(from, fromName string, m Message, boundary func() string) ([]byte, error) {
	return build(from, fromName, m, boundary)
}

// WithDial swaps the transport so tests drive a fake server over a pipe.
func (s *SMTPSender) WithDial(d func(ctx context.Context, addr string) (net.Conn, error)) *SMTPSender {
	s.dial = d
	return s
}

// WithBoundary makes MIME boundaries deterministic under test.
func (s *SMTPSender) WithBoundary(f func() string) *SMTPSender {
	s.boundary = f
	return s
}

// WithTLSRoots trusts an extra CA, so a test can run a real TLS handshake
// against a self-signed fake without disabling verification.
func (s *SMTPSender) WithTLSRoots(pool *x509.CertPool) *SMTPSender {
	s.tlsRoots = pool
	return s
}
