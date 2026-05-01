package smoke

import (
	"os"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/config"
)

// clearAllConfigEnv unsets all config-related environment variables.
func clearAllConfigEnv(t *testing.T) {
	t.Helper()
	envVars := []string{
		"DATABASE_URL", "DOCKER_SOCKET_PATH", "APP_PORT",
		"SESSION_SECRET", "SESSION_TTL",
		"SMTP_HOST", "SMTP_PORT", "SMTP_FROM",
		"ARTIFACT_STORE_PATH", "APP_URL",
	}
	for _, key := range envVars {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

// TestConfigRequiredVars verifies that required environment variables are enforced.
func TestConfigRequiredVars(t *testing.T) {
	clearAllConfigEnv(t)

	// SESSION_SECRET is required — loading without it should fail.
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when SESSION_SECRET is not set")
	}
}

// TestConfigDefaults verifies that all default values are applied correctly
// when only required variables are set.
func TestConfigDefaults(t *testing.T) {
	clearAllConfigEnv(t)
	t.Setenv("SESSION_SECRET", "smoke-test-secret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"DatabaseURL", cfg.DatabaseURL, "postgres://kariz:kariz@localhost:5432/kariz?sslmode=disable"},
		{"DockerSocketPath", cfg.DockerSocketPath, "/var/run/docker.sock"},
		{"AppPort", cfg.AppPort, "8080"},
		{"SessionSecret", cfg.SessionSecret, "smoke-test-secret"},
		{"SMTPHost", cfg.SMTPHost, ""},
		{"SMTPPort", cfg.SMTPPort, "587"},
		{"SMTPFrom", cfg.SMTPFrom, ""},
		{"ArtifactStorePath", cfg.ArtifactStorePath, "/data/artifacts"},
		{"AppURL", cfg.AppURL, "http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}

	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL = %v, want 24h", cfg.SessionTTL)
	}
}

// TestConfigCustomValues verifies that all environment variables are read correctly
// when explicitly set.
func TestConfigCustomValues(t *testing.T) {
	clearAllConfigEnv(t)

	t.Setenv("SESSION_SECRET", "custom-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@remotehost:5432/proddb?sslmode=require")
	t.Setenv("DOCKER_SOCKET_PATH", "/custom/docker.sock")
	t.Setenv("APP_PORT", "3000")
	t.Setenv("SESSION_TTL", "8h")
	t.Setenv("SMTP_HOST", "mail.example.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_FROM", "kariz@example.com")
	t.Setenv("ARTIFACT_STORE_PATH", "/mnt/artifacts")
	t.Setenv("APP_URL", "https://kariz.prod.example.com")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"DatabaseURL", cfg.DatabaseURL, "postgres://user:pass@remotehost:5432/proddb?sslmode=require"},
		{"DockerSocketPath", cfg.DockerSocketPath, "/custom/docker.sock"},
		{"AppPort", cfg.AppPort, "3000"},
		{"SessionSecret", cfg.SessionSecret, "custom-secret"},
		{"SMTPHost", cfg.SMTPHost, "mail.example.com"},
		{"SMTPPort", cfg.SMTPPort, "465"},
		{"SMTPFrom", cfg.SMTPFrom, "kariz@example.com"},
		{"ArtifactStorePath", cfg.ArtifactStorePath, "/mnt/artifacts"},
		{"AppURL", cfg.AppURL, "https://kariz.prod.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}

	if cfg.SessionTTL != 8*time.Hour {
		t.Errorf("SessionTTL = %v, want 8h", cfg.SessionTTL)
	}
}

// TestConfigSMTPConfigured verifies the SMTPConfigured helper method.
func TestConfigSMTPConfigured(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		from     string
		expected bool
	}{
		{"both set", "smtp.example.com", "noreply@example.com", true},
		{"host only", "smtp.example.com", "", false},
		{"from only", "", "noreply@example.com", false},
		{"neither set", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{SMTPHost: tt.host, SMTPFrom: tt.from}
			if got := cfg.SMTPConfigured(); got != tt.expected {
				t.Errorf("SMTPConfigured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestConfigInvalidSessionTTL verifies that an invalid SESSION_TTL falls back to default.
func TestConfigInvalidSessionTTL(t *testing.T) {
	clearAllConfigEnv(t)
	t.Setenv("SESSION_SECRET", "test-secret")
	t.Setenv("SESSION_TTL", "not-a-duration")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL = %v, want 24h (default) for invalid input", cfg.SessionTTL)
	}
}
