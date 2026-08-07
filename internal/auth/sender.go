package auth

import (
	"context"
	"errors"
	"log/slog"
)

// Sender delivers a one-time code to a mobile number. Implementations wrap an
// SMS or WhatsApp provider; swapping provider must not touch anything else.
type Sender interface {
	Send(ctx context.Context, phone, code string) error
}

// ErrNoSender is returned when the service is asked to deliver a code but no
// provider is configured.
var ErrNoSender = errors.New("auth: no OTP provider configured")

// LogSender writes codes to the log instead of delivering them.
//
// Development only. It is the counterpart of the devCode field in the request
// response: both exist so sign-in can be exercised without a paid provider,
// and both must be off in production. Wiring must never select this when
// APP_ENV is production.
type LogSender struct {
	logger *slog.Logger
}

// NewLogSender builds the development sender.
func NewLogSender(logger *slog.Logger) *LogSender {
	return &LogSender{logger: logger}
}

// Send records the code at warn level, loudly, so it is obvious in a log that
// this is not a real delivery path.
func (s *LogSender) Send(ctx context.Context, phone, code string) error {
	s.logger.WarnContext(ctx, "DEVELOPMENT ONLY: one-time code not actually sent",
		slog.String("phone", phone),
		slog.String("code", code),
	)
	return nil
}

// UnconfiguredSender fails every send. It is the production default until a
// real provider is wired, so a missing integration surfaces as a loud error
// rather than shoppers silently never receiving a code.
type UnconfiguredSender struct{}

// Send always fails with ErrNoSender.
func (UnconfiguredSender) Send(context.Context, string, string) error {
	return ErrNoSender
}
