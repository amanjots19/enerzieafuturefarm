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
// including all required production credentials.
func validProduction() map[string]string {
	env := valid()
	env["APP_ENV"] = "production"
	env["RAZORPAY_KEY_ID"] = "rzp_live_testkey"
	env["RAZORPAY_KEY_SECRET"] = "live-key-secret"
	env["RAZORPAY_WEBHOOK_SECRET"] = "live-webhook-secret"
	env["MSG91_AUTH_KEY"] = "msg91-auth-key-value"
	env["ADMIN_EMAIL"] = "ops@enerzia.in"
	env["ADMIN_PASSWORD_HASH"] = "$2a$10$dummyhashfortest"
	env["CLOUDINARY_CLOUD_NAME"] = "testcloud"
	env["CLOUDINARY_API_KEY"] = "cloudinary-api-key"
	env["CLOUDINARY_API_SECRET"] = "cloudinary-api-secret"
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

func TestLoadAdminRequiredInProduction(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantMsg string
	}{
		{
			name:    "missing admin email",
			mutate:  func(e map[string]string) { delete(e, "ADMIN_EMAIL") },
			wantMsg: "ADMIN_EMAIL is required in production",
		},
		{
			name:    "blank admin email",
			mutate:  func(e map[string]string) { e["ADMIN_EMAIL"] = "   " },
			wantMsg: "ADMIN_EMAIL is required in production",
		},
		{
			name:    "missing admin password hash",
			mutate:  func(e map[string]string) { delete(e, "ADMIN_PASSWORD_HASH") },
			wantMsg: "ADMIN_PASSWORD_HASH is required in production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validProduction()
			tt.mutate(env)

			_, err := config.Load(stub(env))
			if err == nil {
				t.Fatalf("Load() error = nil, want one containing %q", tt.wantMsg)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("Load() error = %q, want it to contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestLoadAdminOptionalOutsideProduction(t *testing.T) {
	cfg, err := config.Load(stub(valid()))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.AdminEmail != "" || cfg.AdminPasswordHash != "" {
		t.Errorf("admin fields should be empty when not provided: email=%q hash=%q",
			cfg.AdminEmail, cfg.AdminPasswordHash)
	}
}

func TestLoadAdminEmailIsTrimmed(t *testing.T) {
	env := valid()
	env["ADMIN_EMAIL"] = "  ops@enerzia.in  "
	cfg, err := config.Load(stub(env))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AdminEmail != "ops@enerzia.in" {
		t.Errorf("AdminEmail = %q, want %q", cfg.AdminEmail, "ops@enerzia.in")
	}
}

func TestLoadAdminPasswordHashIsNotTrimmed(t *testing.T) {
	// A bcrypt hash has no surrounding whitespace; silently trimming one hides
	// a bad paste. The raw value must be stored as-is.
	env := valid()
	env["ADMIN_PASSWORD_HASH"] = "$2a$10$abcdef"
	cfg, err := config.Load(stub(env))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AdminPasswordHash != "$2a$10$abcdef" {
		t.Errorf("AdminPasswordHash = %q, want verbatim hash", cfg.AdminPasswordHash)
	}
}

func TestLoadDoesNotLeakAdminPasswordHash(t *testing.T) {
	env := validProduction()
	delete(env, "ADMIN_EMAIL") // force a production error
	env["ADMIN_PASSWORD_HASH"] = "$2a$10$supersecretbcrypthash"

	_, err := config.Load(stub(env))
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
	if strings.Contains(err.Error(), "supersecretbcrypthash") {
		t.Errorf("error message leaks admin password hash: %q", err)
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

func TestLoadCloudinaryRequiredInProduction(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantMsg string
	}{
		{
			name:    "missing cloud name",
			mutate:  func(e map[string]string) { delete(e, "CLOUDINARY_CLOUD_NAME") },
			wantMsg: "CLOUDINARY_CLOUD_NAME is required in production",
		},
		{
			name:    "blank cloud name",
			mutate:  func(e map[string]string) { e["CLOUDINARY_CLOUD_NAME"] = "  " },
			wantMsg: "CLOUDINARY_CLOUD_NAME is required in production",
		},
		{
			name:    "missing api key",
			mutate:  func(e map[string]string) { delete(e, "CLOUDINARY_API_KEY") },
			wantMsg: "CLOUDINARY_API_KEY is required in production",
		},
		{
			name:    "missing api secret",
			mutate:  func(e map[string]string) { delete(e, "CLOUDINARY_API_SECRET") },
			wantMsg: "CLOUDINARY_API_SECRET is required in production",
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
				t.Errorf("Load() must return zero Config alongside an error, got %+v", cfg)
			}
		})
	}
}

func TestLoadCloudinaryOptionalOutsideProduction(t *testing.T) {
	cfg, err := config.Load(stub(valid()))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for dev without Cloudinary creds", err)
	}
	if cfg.CloudinaryCloudName != "" || cfg.CloudinaryAPIKey != "" || cfg.CloudinaryAPISecret != "" {
		t.Errorf("Cloudinary fields should be empty when not provided: cloud=%q key=%q secret=(redacted)",
			cfg.CloudinaryCloudName, cfg.CloudinaryAPIKey)
	}
}

func TestLoadCloudinaryFolderDefault(t *testing.T) {
	// CloudinaryFolder is not set — handler defaults it to "enerzia/products".
	// Config stores it empty; the cloudinary.NewClient call applies the default.
	cfg, err := config.Load(stub(valid()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.CloudinaryFolder != "" {
		t.Errorf("CloudinaryFolder = %q, want empty (default applied by NewClient)", cfg.CloudinaryFolder)
	}
}

func TestLoadCloudinaryFolderIsRead(t *testing.T) {
	env := valid()
	env["CLOUDINARY_FOLDER"] = "  custom/folder  "
	cfg, err := config.Load(stub(env))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.CloudinaryFolder != "custom/folder" {
		t.Errorf("CloudinaryFolder = %q, want %q (trimmed)", cfg.CloudinaryFolder, "custom/folder")
	}
}

func TestLoadDoesNotLeakCloudinaryAPISecret(t *testing.T) {
	env := validProduction()
	delete(env, "CLOUDINARY_CLOUD_NAME") // force a production error
	env["CLOUDINARY_API_SECRET"] = "super-cloudinary-api-secret-value"

	_, err := config.Load(stub(env))
	if err == nil {
		t.Fatal("Load() error = nil, want an error (missing CLOUDINARY_CLOUD_NAME in production)")
	}
	if strings.Contains(err.Error(), "super-cloudinary-api-secret-value") {
		t.Errorf("error message leaks Cloudinary API secret: %q", err)
	}
}

/* ------------------------------------------------------------- ship origin */

// withShip merges origin variables into the smallest valid development env, so
// these tests exercise the origin rules and nothing else.
func withShip(ship map[string]string) map[string]string {
	env := valid()
	for k, v := range ship {
		env[k] = v
	}
	return env
}

func TestShipFromIsAllOrNothing(t *testing.T) {
	full := map[string]string{
		"SHIP_FROM_NAME":  "Enerzeia Future Farm",
		"SHIP_FROM_LINE1": "Plot 14, Sector 58",
		"SHIP_FROM_CITY":  "Faridabad",
		"SHIP_FROM_STATE": "Haryana",
		"SHIP_FROM_PIN":   "121004",
		"SHIP_FROM_PHONE": "9812345678",
	}

	t.Run("none set is not an error", func(t *testing.T) {
		cfg, err := config.Load(stub(valid()))
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.ShipFrom.Configured() {
			t.Error("an unset origin reported itself configured")
		}
	})

	t.Run("all six set is configured", func(t *testing.T) {
		cfg, err := config.Load(stub(withShip(full)))
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if !cfg.ShipFrom.Configured() {
			t.Error("a complete origin reported itself unconfigured")
		}
		if cfg.ShipFrom.Pin != "121004" || cfg.ShipFrom.Phone != "9812345678" {
			t.Errorf("origin = %+v", cfg.ShipFrom)
		}
	})

	// Some-but-not-all is a typo or a half-done edit. It must fail at boot
	// rather than print a label missing the one line that gets an undelivered
	// parcel home.
	for missing := range full {
		t.Run("missing "+missing, func(t *testing.T) {
			env := map[string]string{}
			for k, v := range full {
				if k != missing {
					env[k] = v
				}
			}
			_, err := config.Load(stub(withShip(env)))
			if err == nil {
				t.Fatal("Load() accepted a partially configured origin")
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error = %v, want it to name %s", err, missing)
			}
		})
	}
}

func TestShipFromRejectsMalformedPinAndPhone(t *testing.T) {
	// A malformed PIN is worse than a missing one: it prints, and the parcel
	// goes nowhere.
	base := map[string]string{
		"SHIP_FROM_NAME":  "Farm",
		"SHIP_FROM_LINE1": "Plot 14",
		"SHIP_FROM_CITY":  "Faridabad",
		"SHIP_FROM_STATE": "Haryana",
		"SHIP_FROM_PIN":   "121004",
		"SHIP_FROM_PHONE": "9812345678",
	}
	tests := []struct{ name, key, val, want string }{
		{"short pin", "SHIP_FROM_PIN", "12100", "SHIP_FROM_PIN"},
		{"long pin", "SHIP_FROM_PIN", "1210045", "SHIP_FROM_PIN"},
		{"pin with letters", "SHIP_FROM_PIN", "12100a", "SHIP_FROM_PIN"},
		{"short phone", "SHIP_FROM_PHONE", "981234567", "SHIP_FROM_PHONE"},
		{"phone with country code", "SHIP_FROM_PHONE", "+919812345678", "SHIP_FROM_PHONE"},
		{"phone with spaces", "SHIP_FROM_PHONE", "98123 45678", "SHIP_FROM_PHONE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{}
			for k, v := range base {
				env[k] = v
			}
			env[tt.key] = tt.val

			_, err := config.Load(stub(withShip(env)))
			if err == nil {
				t.Fatalf("Load() accepted %s=%q", tt.key, tt.val)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to name %s", err, tt.want)
			}
		})
	}
}

/* --------------------------------------------------------------------- SMTP */

func TestSMTPIsAllOrNothing(t *testing.T) {
	full := map[string]string{
		"SMTP_HOST":       "smtp.zoho.in",
		"SMTP_PORT":       "465",
		"SMTP_USERNAME":   "orders@enerzeiafuturefarm.com",
		"SMTP_PASSWORD":   "secret",
		"SMTP_FROM_EMAIL": "orders@enerzeiafuturefarm.com",
		"SMTP_FROM_NAME":  "Enerzeia Future Farm",
	}

	t.Run("none set leaves mail switched off", func(t *testing.T) {
		cfg, err := config.Load(stub(valid()))
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.SMTPConfigured() {
			t.Error("mail reported configured with no SMTP_* set")
		}
	})

	t.Run("all set is configured", func(t *testing.T) {
		cfg, err := config.Load(stub(withShip(full)))
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if !cfg.SMTPConfigured() {
			t.Error("a complete SMTP config reported itself unconfigured")
		}
		if cfg.SMTPPort != 465 {
			t.Errorf("SMTPPort = %d, want 465", cfg.SMTPPort)
		}
	})

	t.Run("port defaults to 587", func(t *testing.T) {
		env := map[string]string{}
		for k, v := range full {
			if k != "SMTP_PORT" {
				env[k] = v
			}
		}
		cfg, err := config.Load(stub(withShip(env)))
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.SMTPPort != 587 {
			t.Errorf("SMTPPort = %d, want the 587 default", cfg.SMTPPort)
		}
	})

	// A from-address with no host sends nothing at all, silently — the worst
	// possible failure for an order confirmation.
	t.Run("from without a host is an error", func(t *testing.T) {
		_, err := config.Load(stub(withShip(map[string]string{
			"SMTP_FROM_EMAIL": "orders@enerzeiafuturefarm.com",
		})))
		if err == nil || !strings.Contains(err.Error(), "SMTP_HOST") {
			t.Errorf("error = %v, want it to name SMTP_HOST", err)
		}
	})

	// A host with no from-address sends mail nobody can reply to.
	t.Run("host without a from is an error", func(t *testing.T) {
		_, err := config.Load(stub(withShip(map[string]string{"SMTP_HOST": "smtp.zoho.in"})))
		if err == nil || !strings.Contains(err.Error(), "SMTP_FROM_EMAIL") {
			t.Errorf("error = %v, want it to name SMTP_FROM_EMAIL", err)
		}
	})

	t.Run("a bad port is an error", func(t *testing.T) {
		env := map[string]string{}
		for k, v := range full {
			env[k] = v
		}
		env["SMTP_PORT"] = "not-a-port"
		_, err := config.Load(stub(withShip(env)))
		if err == nil || !strings.Contains(err.Error(), "SMTP_PORT") {
			t.Errorf("error = %v, want it to name SMTP_PORT", err)
		}
	})
}
