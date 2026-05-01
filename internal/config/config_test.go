package config

import (
	"os"
	"testing"
	"time"
)

func clearConfigEnv(t *testing.T) {
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

func TestLoad_MissingSessionSecret(t *testing.T) {
	clearConfigEnv(t)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when SESSION_SECRET is missing, got nil")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SESSION_SECRET", "test-secret-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://kariz:kariz@localhost:5432/kariz?sslmode=disable" {
		t.Errorf("DatabaseURL = %q, want default", cfg.DatabaseURL)
	}
	if cfg.DockerSocketPath != "/var/run/docker.sock" {
		t.Errorf("DockerSocketPath = %q, want /var/run/docker.sock", cfg.DockerSocketPath)
	}
	if cfg.AppPort != "8080" {
		t.Errorf("AppPort = %q, want 8080", cfg.AppPort)
	}
	if cfg.SessionSecret != "test-secret-key" {
		t.Errorf("SessionSecret = %q, want test-secret-key", cfg.SessionSecret)
	}
	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL = %v, want 24h", cfg.SessionTTL)
	}
	if cfg.SMTPHost != "" {
		t.Errorf("SMTPHost = %q, want empty", cfg.SMTPHost)
	}
	if cfg.SMTPPort != "587" {
		t.Errorf("SMTPPort = %q, want 587", cfg.SMTPPort)
	}
	if cfg.SMTPFrom != "" {
		t.Errorf("SMTPFrom = %q, want empty", cfg.SMTPFrom)
	}
	if cfg.ArtifactStorePath != "/data/artifacts" {
		t.Errorf("ArtifactStorePath = %q, want /data/artifacts", cfg.ArtifactStorePath)
	}
	if cfg.AppURL != "http://localhost:8080" {
		t.Errorf("AppURL = %q, want http://localhost:8080", cfg.AppURL)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SESSION_SECRET", "my-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/mydb?sslmode=require")
	t.Setenv("DOCKER_SOCKET_PATH", "/tmp/docker.sock")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("SESSION_TTL", "2h")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_FROM", "noreply@example.com")
	t.Setenv("ARTIFACT_STORE_PATH", "/tmp/artifacts")
	t.Setenv("APP_URL", "https://kariz.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://user:pass@db:5432/mydb?sslmode=require" {
		t.Errorf("DatabaseURL = %q, want custom value", cfg.DatabaseURL)
	}
	if cfg.DockerSocketPath != "/tmp/docker.sock" {
		t.Errorf("DockerSocketPath = %q, want /tmp/docker.sock", cfg.DockerSocketPath)
	}
	if cfg.AppPort != "9090" {
		t.Errorf("AppPort = %q, want 9090", cfg.AppPort)
	}
	if cfg.SessionTTL != 2*time.Hour {
		t.Errorf("SessionTTL = %v, want 2h", cfg.SessionTTL)
	}
	if cfg.SMTPHost != "smtp.example.com" {
		t.Errorf("SMTPHost = %q, want smtp.example.com", cfg.SMTPHost)
	}
	if cfg.SMTPPort != "465" {
		t.Errorf("SMTPPort = %q, want 465", cfg.SMTPPort)
	}
	if cfg.SMTPFrom != "noreply@example.com" {
		t.Errorf("SMTPFrom = %q, want noreply@example.com", cfg.SMTPFrom)
	}
	if cfg.ArtifactStorePath != "/tmp/artifacts" {
		t.Errorf("ArtifactStorePath = %q, want /tmp/artifacts", cfg.ArtifactStorePath)
	}
	if cfg.AppURL != "https://kariz.example.com" {
		t.Errorf("AppURL = %q, want https://kariz.example.com", cfg.AppURL)
	}
}

func TestLoad_InvalidSessionTTL_FallsBackToDefault(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SESSION_SECRET", "test-secret")
	t.Setenv("SESSION_TTL", "not-a-duration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL = %v, want 24h (default) when invalid value provided", cfg.SessionTTL)
	}
}

func TestSMTPConfigured(t *testing.T) {
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
			cfg := &Config{SMTPHost: tt.host, SMTPFrom: tt.from}
			if got := cfg.SMTPConfigured(); got != tt.expected {
				t.Errorf("SMTPConfigured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("TEST_CONFIG_VAR", "hello")
	if got := getEnv("TEST_CONFIG_VAR", "default"); got != "hello" {
		t.Errorf("getEnv() = %q, want hello", got)
	}
	if got := getEnv("TEST_CONFIG_UNSET_VAR", "fallback"); got != "fallback" {
		t.Errorf("getEnv() = %q, want fallback", got)
	}
}

func TestGetEnvDuration(t *testing.T) {
	t.Setenv("TEST_DUR", "30m")
	if got := getEnvDuration("TEST_DUR", time.Hour); got != 30*time.Minute {
		t.Errorf("getEnvDuration() = %v, want 30m", got)
	}

	t.Setenv("TEST_DUR_BAD", "invalid")
	if got := getEnvDuration("TEST_DUR_BAD", time.Hour); got != time.Hour {
		t.Errorf("getEnvDuration() = %v, want 1h (default) for invalid input", got)
	}

	if got := getEnvDuration("TEST_DUR_MISSING", 2*time.Hour); got != 2*time.Hour {
		t.Errorf("getEnvDuration() = %v, want 2h (default) for missing var", got)
	}
}
