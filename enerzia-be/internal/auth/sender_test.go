package auth_test

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/enerzia/enerzia-be/internal/auth"
)

func TestLogSenderRecordsTheCodeLoudly(t *testing.T) {
	var buf bytes.Buffer
	sender := auth.NewLogSender(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	if err := sender.Send(t.Context(), testPhone, "123456"); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}

	out := buf.String()
	if !strings.Contains(out, "123456") {
		t.Error("the development sender must record the code, or it is useless")
	}
	// It must be obvious in a log that this is not a real delivery path.
	if !strings.Contains(out, "DEVELOPMENT ONLY") {
		t.Errorf("log line is not clearly marked: %s", out)
	}
	if !strings.Contains(out, `"level":"WARN"`) {
		t.Errorf("expected a WARN so it stands out, got: %s", out)
	}
}

func TestUnconfiguredSenderAlwaysFails(t *testing.T) {
	// The production default: a missing integration must be loud, not silent.
	err := auth.UnconfiguredSender{}.Send(t.Context(), testPhone, "123456")

	if !errors.Is(err, auth.ErrNoSender) {
		t.Errorf("Send() error = %v, want ErrNoSender", err)
	}
}

func TestUnconfiguredSenderSatisfiesTheInterface(t *testing.T) {
	var _ auth.Sender = auth.UnconfiguredSender{}
	var _ auth.Sender = auth.NewLogSender(slog.Default())
}
