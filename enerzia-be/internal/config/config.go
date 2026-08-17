// Package config loads and validates application configuration from the
// environment exactly once at startup.
package config

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Environment names the deployment the process is running in.
type Environment string

// Recognised environments.
const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// minJWTSecretLen is the shortest signing secret accepted. Anything shorter is
// brute-forceable and is treated as a configuration error rather than a
// warning.
const minJWTSecretLen = 32

// Config is the fully validated configuration for the API process.
type Config struct {
	AppEnv          Environment
	Port            int
	MongoURI        string
	MongoDB         string
	JWTSecret       string
	AllowedOrigins  []string
	MongoTimeout    time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownGrace   time.Duration
	SweeperInterval time.Duration

	// Razorpay credentials. All three are required in production; outside
	// production they are optional and the gateway falls back to Unconfigured.
	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string

	// MSG91AuthKey is the server-only key for the widget token verification API.
	// Required in production; outside production an empty value selects
	// msg91.Unconfigured, which rejects every call.
	MSG91AuthKey string

	// AdminEmail is the single administrator's email address. Compared
	// case-insensitively at sign-in time.
	AdminEmail string
	// AdminPasswordHash is the bcrypt hash of the administrator's password.
	// NOT trimmed — a bcrypt hash has no surrounding whitespace, and silently
	// trimming one hides a bad paste. Generate with `make admin-password`.
	// Never logged, never returned in a response.
	AdminPasswordHash string

	// Cloudinary credentials for the signed-upload endpoint. All three are
	// required in production; outside production they default to the
	// Unconfigured signer, which returns 500 on every request.
	CloudinaryCloudName string
	CloudinaryAPIKey    string
	// CloudinaryAPISecret is NOT trimmed — same reasoning as AdminPasswordHash.
	// Never logged, never returned in a response, never leaves the process.
	CloudinaryAPISecret string
	// SMTP delivers transactional mail. Optional everywhere: an empty SMTPHost
	// selects email.Unconfigured, which refuses every send, and order
	// confirmations are simply not sent. Mail is never allowed to be the reason
	// a payment fails to confirm.
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	// SMTPPassword is NOT trimmed — same reasoning as the other secrets.
	SMTPPassword string
	SMTPFromMail string
	SMTPFromName string

	// ShipFrom is the pickup address printed as the return address on every
	// shipping label. It is configuration, not data: it changes when the farm
	// moves, not when an order is placed.
	//
	// Optional everywhere. Unset, the label endpoint answers 503 rather than
	// printing a label with blanks where the return address belongs — a parcel
	// that can be neither delivered nor returned is simply gone.
	ShipFrom ShipFromAddress

	// CloudinaryFolder is the upload folder prefix. Defaults to
	// "enerzia/products" when empty.
	CloudinaryFolder string
}

// ShipFromAddress is the shipping origin (roadmap.md §GET
// /api/v1/admin/orders/{orderId}/label).
type ShipFromAddress struct {
	Name  string
	Line1 string
	City  string
	State string
	Pin   string
	Phone string
}

// SMTPConfigured reports whether transactional mail can be sent at all.
func (c Config) SMTPConfigured() bool { return c.SMTPHost != "" && c.SMTPFromMail != "" }

// Configured reports whether every field is present. It is all-or-nothing on
// purpose: a partially filled origin would print a label missing the very line
// that gets an undelivered parcel home, and would do it silently.
func (s ShipFromAddress) Configured() bool {
	return s.Name != "" && s.Line1 != "" && s.City != "" &&
		s.State != "" && s.Pin != "" && s.Phone != ""
}

// IsProduction reports whether the process runs in production, where
// development conveniences such as returning the OTP in the response body must
// be disabled.
func (c Config) IsProduction() bool { return c.AppEnv == EnvProduction }

// Addr returns the TCP address the HTTP server should listen on.
func (c Config) Addr() string { return fmt.Sprintf(":%d", c.Port) }

// Getenv reads one environment variable. os.Getenv satisfies it; tests pass a
// map-backed stub so no test mutates the real process environment.
type Getenv func(string) string

// Load reads configuration through getenv, applies defaults, and validates the
// result. It returns every problem it finds at once so a misconfigured
// deployment does not have to be fixed one variable per restart.
func Load(getenv Getenv) (Config, error) {
	cfg := Config{
		AppEnv:                Environment(orDefault(getenv("APP_ENV"), string(EnvDevelopment))),
		MongoURI:              strings.TrimSpace(getenv("MONGO_URI")),
		MongoDB:               strings.TrimSpace(getenv("MONGO_DB")),
		JWTSecret:             getenv("JWT_SECRET"),
		AllowedOrigins:        splitOrigins(orDefault(getenv("ALLOWED_ORIGINS"), "http://localhost:3000")),
		MongoTimeout:          5 * time.Second,
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          15 * time.Second,
		ShutdownGrace:         15 * time.Second,
		SweeperInterval:       time.Minute,
		RazorpayKeyID:         strings.TrimSpace(getenv("RAZORPAY_KEY_ID")),
		RazorpayKeySecret:     getenv("RAZORPAY_KEY_SECRET"),
		RazorpayWebhookSecret: getenv("RAZORPAY_WEBHOOK_SECRET"),
		MSG91AuthKey:          getenv("MSG91_AUTH_KEY"),
		AdminEmail:            strings.TrimSpace(getenv("ADMIN_EMAIL")),
		AdminPasswordHash:     getenv("ADMIN_PASSWORD_HASH"), // NOT trimmed — a bcrypt hash has no surrounding whitespace
		CloudinaryCloudName:   strings.TrimSpace(getenv("CLOUDINARY_CLOUD_NAME")),
		CloudinaryAPIKey:      strings.TrimSpace(getenv("CLOUDINARY_API_KEY")),
		CloudinaryAPISecret:   getenv("CLOUDINARY_API_SECRET"), // NOT trimmed — never has surrounding whitespace
		CloudinaryFolder:      strings.TrimSpace(getenv("CLOUDINARY_FOLDER")),
		SMTPHost:              strings.TrimSpace(getenv("SMTP_HOST")),
		SMTPUsername:          strings.TrimSpace(getenv("SMTP_USERNAME")),
		SMTPPassword:          getenv("SMTP_PASSWORD"),
		SMTPFromMail:          strings.TrimSpace(getenv("SMTP_FROM_EMAIL")),
		SMTPFromName:          strings.TrimSpace(getenv("SMTP_FROM_NAME")),
		ShipFrom: ShipFromAddress{
			Name:  strings.TrimSpace(getenv("SHIP_FROM_NAME")),
			Line1: strings.TrimSpace(getenv("SHIP_FROM_LINE1")),
			City:  strings.TrimSpace(getenv("SHIP_FROM_CITY")),
			State: strings.TrimSpace(getenv("SHIP_FROM_STATE")),
			Pin:   strings.TrimSpace(getenv("SHIP_FROM_PIN")),
			Phone: strings.TrimSpace(getenv("SHIP_FROM_PHONE")),
		},
	}

	var problems []string

	switch cfg.AppEnv {
	case EnvDevelopment, EnvStaging, EnvProduction:
	default:
		problems = append(problems, fmt.Sprintf(
			"APP_ENV must be one of development, staging, production (got %q)", cfg.AppEnv))
	}

	port, err := strconv.Atoi(orDefault(getenv("PORT"), "8080"))
	switch {
	case err != nil:
		problems = append(problems, fmt.Sprintf("PORT must be a number (got %q)", getenv("PORT")))
	case port < 1 || port > 65535:
		problems = append(problems, fmt.Sprintf("PORT must be between 1 and 65535 (got %d)", port))
	default:
		cfg.Port = port
	}

	if cfg.MongoURI == "" {
		problems = append(problems, "MONGO_URI is required")
	}
	if cfg.MongoDB == "" {
		problems = append(problems, "MONGO_DB is required")
	}
	switch {
	case cfg.JWTSecret == "":
		problems = append(problems, "JWT_SECRET is required")
	case len(cfg.JWTSecret) < minJWTSecretLen:
		problems = append(problems, fmt.Sprintf(
			"JWT_SECRET must be at least %d characters", minJWTSecretLen))
	}
	if len(cfg.AllowedOrigins) == 0 {
		problems = append(problems, "ALLOWED_ORIGINS must list at least one origin")
	}

	if cfg.IsProduction() {
		if cfg.RazorpayKeyID == "" {
			problems = append(problems, "RAZORPAY_KEY_ID is required in production")
		}
		if cfg.RazorpayKeySecret == "" {
			problems = append(problems, "RAZORPAY_KEY_SECRET is required in production")
		}
		if cfg.RazorpayWebhookSecret == "" {
			problems = append(problems, "RAZORPAY_WEBHOOK_SECRET is required in production")
		}
		if cfg.MSG91AuthKey == "" {
			problems = append(problems, "MSG91_AUTH_KEY is required in production")
		}
		if cfg.AdminEmail == "" {
			problems = append(problems, "ADMIN_EMAIL is required in production")
		}
		if cfg.AdminPasswordHash == "" {
			problems = append(problems, "ADMIN_PASSWORD_HASH is required in production")
		}
		if cfg.CloudinaryCloudName == "" {
			problems = append(problems, "CLOUDINARY_CLOUD_NAME is required in production")
		}
		if cfg.CloudinaryAPIKey == "" {
			problems = append(problems, "CLOUDINARY_API_KEY is required in production")
		}
		if cfg.CloudinaryAPISecret == "" {
			problems = append(problems, "CLOUDINARY_API_SECRET is required in production")
		}
	}

	// The shipping origin is all-or-nothing. Zero fields set means "no label
	// printing yet", which is fine. Some-but-not-all is a typo or a half-done
	// edit, and it must fail at boot rather than print a label missing the one
	// line that gets an undelivered parcel home.
	problems = append(problems, shipFromProblems(cfg.ShipFrom)...)

	// SMTP, like the origin, is all-or-nothing: a host with no from-address
	// sends mail nobody can reply to, and a from-address with no host silently
	// sends nothing at all.
	if cfg.SMTPHost != "" {
		port, portErr := strconv.Atoi(orDefault(getenv("SMTP_PORT"), "587"))
		switch {
		case portErr != nil || port < 1 || port > 65535:
			problems = append(problems, fmt.Sprintf(
				"SMTP_PORT must be a port number (got %q)", getenv("SMTP_PORT")))
		default:
			cfg.SMTPPort = port
		}
		if cfg.SMTPFromMail == "" {
			problems = append(problems, "SMTP_FROM_EMAIL is required once SMTP_HOST is set")
		}
	} else if cfg.SMTPFromMail != "" || cfg.SMTPUsername != "" {
		problems = append(problems, "SMTP_HOST is required once any other SMTP_* is set")
	}

	if len(problems) > 0 {
		// Values are deliberately omitted for secrets; only names are reported.
		return Config{}, fmt.Errorf("invalid configuration: %w",
			errors.New(strings.Join(problems, "; ")))
	}
	return cfg, nil
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func splitOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// shipFromFields pairs each origin field with the variable that sets it, so a
// problem can name the variable the operator has to edit rather than a struct
// field they cannot see.
func shipFromFields(s ShipFromAddress) []struct {
	env, val string
} {
	return []struct{ env, val string }{
		{"SHIP_FROM_NAME", s.Name},
		{"SHIP_FROM_LINE1", s.Line1},
		{"SHIP_FROM_CITY", s.City},
		{"SHIP_FROM_STATE", s.State},
		{"SHIP_FROM_PIN", s.Pin},
		{"SHIP_FROM_PHONE", s.Phone},
	}
}

// shipFromProblems reports configuration errors in the shipping origin.
//
// Nothing set at all is not an error — label printing is simply unconfigured
// and the endpoint answers 503. Anything else must be complete and well-formed.
func shipFromProblems(s ShipFromAddress) []string {
	fields := shipFromFields(s)

	set := 0
	for _, f := range fields {
		if f.val != "" {
			set++
		}
	}
	if set == 0 {
		return nil // unconfigured, deliberately
	}

	var problems []string
	for _, f := range fields {
		if f.val == "" {
			problems = append(problems, f.env+" is required once any SHIP_FROM_* is set")
		}
	}
	// A malformed PIN or phone is worse than a missing one: it prints, and the
	// parcel goes nowhere.
	if s.Pin != "" && !sixDigits.MatchString(s.Pin) {
		problems = append(problems, "SHIP_FROM_PIN must be exactly 6 digits")
	}
	if s.Phone != "" && !tenDigits.MatchString(s.Phone) {
		problems = append(problems, "SHIP_FROM_PHONE must be exactly 10 digits, with no +91 and no spaces")
	}
	return problems
}

var (
	sixDigits = regexp.MustCompile(`^\d{6}$`)
	tenDigits = regexp.MustCompile(`^\d{10}$`)
)
