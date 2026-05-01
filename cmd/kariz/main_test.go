package main

import (
	"testing"
)

func TestMaskDatabaseURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard postgres URL",
			input:    "postgres://kariz:kariz@localhost:5432/kariz?sslmode=disable",
			expected: "postgres://kariz:***@localhost:5432/kariz?sslmode=disable",
		},
		{
			name:     "URL with special chars in password",
			input:    "postgres://admin:s3cr3t!@db.example.com:5432/mydb",
			expected: "postgres://admin:***@db.example.com:5432/mydb",
		},
		{
			name:     "URL without password",
			input:    "postgres://admin@localhost:5432/kariz",
			expected: "postgres://admin@localhost:5432/kariz",
		},
		{
			name:     "URL without credentials",
			input:    "postgres://localhost:5432/kariz",
			expected: "postgres://localhost:5432/kariz",
		},
		{
			name:     "no scheme",
			input:    "not-a-url",
			expected: "***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskDatabaseURL(tt.input)
			if got != tt.expected {
				t.Errorf("maskDatabaseURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
