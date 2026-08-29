// sanitize_test.go validates ANSI and OSC escape sequence removal from log strings.
// Test Strategy: Table-driven unit tests verifying color codes, cursor sequences, OSC titles, and edge cases.
package sanitize

import (
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "color codes",
			input:    "\x1b[31;1mError:\x1b[0m Failed to start",
			expected: "Error: Failed to start",
		},
		{
			name:     "cursor movements",
			input:    "Loading\x1b[2K\rDone",
			expected: "Loading\rDone",
		},
		{
			name:     "OSC title sequence",
			input:    "\x1b]0;Title\x07Log message",
			expected: "Log message",
		},
		{
			name:     "complex mixed sequences",
			input:    "\x1b[38;2;255;0;0mRed\x1b[0m \x1b[48;5;16mBlack\x1b[0m text",
			expected: "Red Black text",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripANSI(tt.input)
			if result != tt.expected {
				t.Fatalf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestIsSensitiveKey(t *testing.T) {
	sensitive := []string{
		"DB_PASSWORD",
		"AWS_SECRET_ACCESS_KEY",
		"API_KEY",
		"GITHUB_TOKEN",
		"DATABASE_URL",
		"AUTH_HEADER",
		"SSL_CERT",
		"PRIVATE_KEY",
		"WEBHOOK_SECRET",
		"SESSION_COOKIE",
		"SIGNATURE_HASH",
	}

	for _, key := range sensitive {
		if !IsSensitiveKey(key) {
			t.Errorf("expected key %q to be identified as sensitive", key)
		}
	}

	nonSensitive := []string{
		"PORT",
		"HOST",
		"NODE_ENV",
		"APP_NAME",
		"LOG_LEVEL",
		"MAX_WORKERS",
	}

	for _, key := range nonSensitive {
		if IsSensitiveKey(key) {
			t.Errorf("expected key %q to NOT be identified as sensitive", key)
		}
	}
}

func TestSanitizeEnv(t *testing.T) {
	input := []string{
		"PORT=8080",
		"DB_PASSWORD=super_secret_123",
		"NODE_ENV=production",
		"API_TOKEN=xyz987abc",
		"DATABASE_URL=postgres://user:pass@host/db",
		"APP_DEBUG=false",
	}

	cleaned := SanitizeEnv(input)

	for _, env := range cleaned {
		if strings.Contains(env, "PASSWORD") || strings.Contains(env, "TOKEN") || strings.Contains(env, "DATABASE_URL") {
			t.Fatalf("sensitive variable leaked in sanitized env: %s", env)
		}
	}

	if len(cleaned) != 3 {
		t.Fatalf("expected 3 non-sensitive env variables, got %d: %+v", len(cleaned), cleaned)
	}
}
