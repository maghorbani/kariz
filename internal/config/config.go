package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all application configuration read from environment variables.
type Config struct {
	// Database
	DatabaseURL string

	// Docker
	DockerSocketPath string

	// Application
	AppPort       string
	SessionSecret string
	SessionTTL    time.Duration

	// SMTP (optional)
	SMTPHost string
	SMTPPort string
	SMTPFrom string

	// Artifact storage
	ArtifactStorePath string

	// App URL for notification links
	AppURL string
}

// Load reads configuration from environment variables with sensible defaults.
// It returns an error if required variables are missing.
func Load() (*Config, error) {
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET environment variable is required")
	}

	cfg := &Config{
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://kariz:kariz@localhost:5432/kariz?sslmode=disable"),
		DockerSocketPath:  getEnv("DOCKER_SOCKET_PATH", "/var/run/docker.sock"),
		AppPort:           getEnv("APP_PORT", "8080"),
		SessionSecret:     sessionSecret,
		SessionTTL:        getEnvDuration("SESSION_TTL", 24*time.Hour),
		SMTPHost:          getEnv("SMTP_HOST", ""),
		SMTPPort:          getEnv("SMTP_PORT", "587"),
		SMTPFrom:          getEnv("SMTP_FROM", ""),
		ArtifactStorePath: getEnv("ARTIFACT_STORE_PATH", "/data/artifacts"),
		AppURL:            getEnv("APP_URL", "http://localhost:8080"),
	}

	return cfg, nil
}

// SMTPConfigured returns true if SMTP settings are provided.
func (c *Config) SMTPConfigured() bool {
	return c.SMTPHost != "" && c.SMTPFrom != ""
}

// getEnv returns the value of the environment variable named by key,
// or defaultValue if the variable is not set or empty.
func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// getEnvDuration returns the environment variable parsed as a time.Duration,
// or defaultValue if the variable is not set, empty, or cannot be parsed.
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultValue
	}
	return d
}
