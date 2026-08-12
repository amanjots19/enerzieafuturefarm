// Package config loads and validates application configuration from the
// environment exactly once at startup.
package config

import (
	"errors"
	"fmt"
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
