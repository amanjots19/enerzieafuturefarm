package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/enerzia/enerzia-be/internal/config"
)

// stub builds a Getenv backed by a map, so no test touches the real
// process environment.
func stub(env map[string]string) config.Getenv {
	return func(k string) string { return env[k] }
}

// valid is the smallest environment that loads successfully in development.
func valid() map[string]string {
	return map[string]string{
		"MONGO_URI":  "mongodb://localhost:27017",
		"MONGO_DB":   "enerzia",
		"JWT_SECRET": strings.Repeat("s", 32),
	}
}

// validProduction returns the smallest env map that loads in production,
// including all three required Razorpay credentials.
func validProduction() map[string]string {
	env := valid()
	env["APP_ENV"] = "production"
	env["RAZORPAY_KEY_ID"] = "rzp_live_testkey"
	env["RAZORPAY_KEY_SECRET"] = "live-key-secret"
	env["RAZORPAY_WEBHOOK_SECRET"] = "live-webhook-secret"
	env["MSG91_AUTH_KEY"] = "msg91-auth-key-value"
	return env
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := config.Load(stub(valid()))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.AppEnv != config.EnvDevelopment {
		t.Errorf("AppEnv = %q, want %q", cfg.AppEnv, config.EnvDevelopment)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if got, want := cfg.Addr(), ":8080"; got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
	if cfg.IsProduction() {
		t.Error("IsProduction() = true, want false for the development default")
	}
	if want := []string{"http://localhost:3000"}; len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != want[0] {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, want)
	}
	if cfg.MongoTimeout == 0 || cfg.ReadTimeout == 0 || cfg.WriteTimeout == 0 || cfg.ShutdownGrace == 0 || cfg.SweeperInterval == 0 {
		t.Error("timeout defaults must all be non-zero")
	}
}

func TestLoadReadsOverrides(t *testing.T) {
	env := validProduction() // production requires Razorpay vars
	env["PORT"] = "9000"
	env["ALLOWED_ORIGINS"] = "https://shop.enerzia.in, https://enerzia.in ,"

	cfg, err := config.Load(stub(env))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false, want true")
	}
	if cfg.Addr() != ":9000" {
		t.Errorf("Addr() = %q, want :9000", cfg.Addr())
	}
	// Blank entries from a trailing comma are dropped, and each origin trimmed.
	want := []string{"https://shop.enerzia.in", "https://enerzia.in"}
	if len(cfg.AllowedOrigins) != len(want) {
		t.Fatalf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, want)
	}
	for i := range want {
		if cfg.AllowedOrigins[i] != want[i] {
			t.Errorf("AllowedOrigins[%d] = %q, want %q", i, cfg.AllowedOrigins[i], want[i])
		}
	}
}

func TestLoadRejectsInvalidEnvironments(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantMsg string
	}{
		{
			name:    "missing mongo uri",
			mutate:  func(e map[string]string) { delete(e, "MONGO_URI") },
			wantMsg: "MONGO_URI is required",
		},
		{
			name:    "blank mongo uri",
			mutate:  func(e map[string]string) { e["MONGO_URI"] = "   " },
			wantMsg: "MONGO_URI is required",
		},
		{
			name:    "missing mongo db",
			mutate:  func(e map[string]string) { delete(e, "MONGO_DB") },
			wantMsg: "MONGO_DB is required",
		},
		{
			name:    "missing jwt secret",
			mutate:  func(e map[string]string) { delete(e, "JWT_SECRET") },
			wantMsg: "JWT_SECRET is required",
		},
		{
			name:    "short jwt secret",
			mutate:  func(e map[string]string) { e["JWT_SECRET"] = "too-short" },
			wantMsg: "at least 32 characters",
		},
		{
			name:    "unknown app env",
			mutate:  func(e map[string]string) { e["APP_ENV"] = "qa" },
			wantMsg: "APP_ENV must be one of",
		},
		{
			name:    "non numeric port",
			mutate:  func(e map[string]string) { e["PORT"] = "http" },
			wantMsg: "PORT must be a number",
		},
		{
			name:    "port out of range",
			mutate:  func(e map[string]string) { e["PORT"] = "70000" },
			wantMsg: "between 1 and 65535",
		},
		{
			name:    "port zero",
			mutate:  func(e map[string]string) { e["PORT"] = "0" },
			wantMsg: "between 1 and 65535",
		},
		{
			name:    "no usable origins",
			mutate:  func(e map[string]string) { e["ALLOWED_ORIGINS"] = " , , " },
			wantMsg: "ALLOWED_ORIGINS must list at least one origin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := valid()
			tt.mutate(env)

			cfg, err := config.Load(stub(env))
			if err == nil {
				t.Fatalf("Load() error = nil, want one containing %q", tt.wantMsg)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("Load() error = %q, want it to contain %q", err, tt.wantMsg)
			}
			if !reflect.DeepEqual(cfg, config.Config{}) {
				t.Errorf("Load() must return the zero Config alongside an error, got %+v", cfg)
			}
		})
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	// A misconfigured deployment should not need one restart per bad variable.
	_, err := config.Load(stub(map[string]string{"PORT": "nope"}))
	if err == nil {
		t.Fatal("Load() error = nil, want an aggregate error")
	}
	for _, want := range []string{"MONGO_URI", "MONGO_DB", "JWT_SECRET", "PORT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("aggregate error %q is missing %q", err, want)
		}
	}
}

func TestLoadDoesNotLeakSecretValues(t *testing.T) {
	env := valid()
	env["JWT_SECRET"] = "short-but-secret"
	env["MONGO_URI"] = ""

	_, err := config.Load(stub(env))
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
	if strings.Contains(err.Error(), "short-but-secret") {
		t.Errorf("error message leaks the secret value: %q", err)
	}
}

func TestLoadRazorpayRequiredInProduction(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantMsg string
	}{
		{
			name:    "missing key id",
			mutate:  func(e map[string]string) { delete(e, "RAZORPAY_KEY_ID") },
			wantMsg: "RAZORPAY_KEY_ID",
		},
		{
			name:    "blank key id",
			mutate:  func(e map[string]string) { e["RAZORPAY_KEY_ID"] = "   " },
			wantMsg: "RAZORPAY_KEY_ID",
		},
		{
			name:    "missing key secret",
			mutate:  func(e map[string]string) { delete(e, "RAZORPAY_KEY_SECRET") },
			wantMsg: "RAZORPAY_KEY_SECRET",
		},
		{
			name:    "missing webhook secret",
			mutate:  func(e map[string]string) { delete(e, "RAZORPAY_WEBHOOK_SECRET") },
			wantMsg: "RAZORPAY_WEBHOOK_SECRET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validProduction()
			tt.mutate(env)

			cfg, err := config.Load(stub(env))
			if err == nil {
				t.Fatalf("Load() error = nil, want one containing %q", tt.wantMsg)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("Load() error = %q, want it to contain %q", err, tt.wantMsg)
			}
			if !reflect.DeepEqual(cfg, config.Config{}) {
				t.Errorf("Load() must return the zero Config alongside an error, got %+v", cfg)
			}
		})
	}
}

func TestLoadRazorpayOptionalOutsideProduction(t *testing.T) {
	// Development without any Razorpay vars must load successfully.
	cfg, err := config.Load(stub(valid()))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for dev without Razorpay creds", err)
	}
	if cfg.RazorpayKeyID != "" || cfg.RazorpayKeySecret != "" || cfg.RazorpayWebhookSecret != "" {
		t.Errorf("Razorpay fields should be empty when not provided: %+v", cfg)
	}
}

func TestLoadProductionWithAllRazorpayVars(t *testing.T) {
	cfg, err := config.Load(stub(validProduction()))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.RazorpayKeyID != "rzp_live_testkey" {
		t.Errorf("RazorpayKeyID = %q, want %q", cfg.RazorpayKeyID, "rzp_live_testkey")
	}
	if cfg.RazorpayKeySecret != "live-key-secret" {
		t.Errorf("RazorpayKeySecret = %q, want %q", cfg.RazorpayKeySecret, "live-key-secret")
	}
	if cfg.RazorpayWebhookSecret != "live-webhook-secret" {
		t.Errorf("RazorpayWebhookSecret = %q, want %q", cfg.RazorpayWebhookSecret, "live-webhook-secret")
	}
}

func TestLoadMSG91AuthKeyRequiredInProduction(t *testing.T) {
	env := validProduction()
	delete(env, "MSG91_AUTH_KEY")

	_, err := config.Load(stub(env))
	if err == nil {
		t.Fatal("Load() error = nil, want an error for missing MSG91_AUTH_KEY in production")
	}
	if !strings.Contains(err.Error(), "MSG91_AUTH_KEY") {
		t.Errorf("error %q does not mention MSG91_AUTH_KEY", err)
	}
}

func TestLoadMSG91AuthKeyOptionalOutsideProduction(t *testing.T) {
	cfg, err := config.Load(stub(valid()))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for dev without MSG91_AUTH_KEY", err)
	}
	if cfg.MSG91AuthKey != "" {
		t.Errorf("MSG91AuthKey = %q, want empty when not provided", cfg.MSG91AuthKey)
	}
}

func TestLoadMSG91AuthKeyIsRead(t *testing.T) {
	cfg, err := config.Load(stub(validProduction()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MSG91AuthKey != "msg91-auth-key-value" {
		t.Errorf("MSG91AuthKey = %q, want msg91-auth-key-value", cfg.MSG91AuthKey)
	}
}

func TestLoadDoesNotLeakRazorpaySecrets(t *testing.T) {
	env := validProduction()
	delete(env, "RAZORPAY_KEY_ID") // force a production error
	env["RAZORPAY_KEY_SECRET"] = "super-secret-razorpay-key"
	env["RAZORPAY_WEBHOOK_SECRET"] = "super-secret-webhook-key"

	_, err := config.Load(stub(env))
	if err == nil {
		t.Fatal("Load() error = nil, want an error (missing KEY_ID in production)")
	}
	if strings.Contains(err.Error(), "super-secret-razorpay-key") {
		t.Errorf("error message leaks Razorpay key secret: %q", err)
	}
	if strings.Contains(err.Error(), "super-secret-webhook-key") {
		t.Errorf("error message leaks Razorpay webhook secret: %q", err)
	}
}
