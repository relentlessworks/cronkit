package config

import (
	"flag"
	"os"

	"github.com/relentlessworks/cronkit/internal/auth"
)

// Config holds all configuration for the service.
type Config struct {
	Addr      string
	DataDir   string
	Secret    string
	SMTPHost  string
	SMTPPort  string
	SMTPUser  string
	SMTPPass  string
	FromEmail string
}

// Load parses config from defaults < env < flags.
func Load() *Config {
	cfg := &Config{
		Addr:      ":7777",
		DataDir:   "./data",
		Secret:    "",
		SMTPHost:  "",
		SMTPPort:  "587",
		SMTPUser:  "",
		SMTPPass:  "",
		FromEmail: "noreply@cronkit.local",
	}

	// Env vars
	if v := os.Getenv("CRONKIT_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("CRONKIT_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("CRONKIT_SECRET"); v != "" {
		cfg.Secret = v
	}
	if v := os.Getenv("CRONKIT_SMTP_HOST"); v != "" {
		cfg.SMTPHost = v
	}
	if v := os.Getenv("CRONKIT_SMTP_PORT"); v != "" {
		cfg.SMTPPort = v
	}
	if v := os.Getenv("CRONKIT_SMTP_USER"); v != "" {
		cfg.SMTPUser = v
	}
	if v := os.Getenv("CRONKIT_SMTP_PASS"); v != "" {
		cfg.SMTPPass = v
	}
	if v := os.Getenv("CRONKIT_FROM_EMAIL"); v != "" {
		cfg.FromEmail = v
	}

	// Flags
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	flag.StringVar(&cfg.DataDir, "data", cfg.DataDir, "data directory")
	flag.StringVar(&cfg.Secret, "secret", cfg.Secret, "token signing secret (auto-generated if empty)")
	flag.StringVar(&cfg.SMTPHost, "smtp-host", cfg.SMTPHost, "SMTP server host")
	flag.StringVar(&cfg.SMTPPort, "smtp-port", cfg.SMTPPort, "SMTP server port")
	flag.StringVar(&cfg.SMTPUser, "smtp-user", cfg.SMTPUser, "SMTP username")
	flag.StringVar(&cfg.SMTPPass, "smtp-pass", cfg.SMTPPass, "SMTP password")
	flag.StringVar(&cfg.FromEmail, "from-email", cfg.FromEmail, "from email address")
	flag.Parse()

	// Auto-generate secret if not provided
	if cfg.Secret == "" {
		cfg.Secret = auth.GenerateRandomSecret()
	}

	return cfg
}
