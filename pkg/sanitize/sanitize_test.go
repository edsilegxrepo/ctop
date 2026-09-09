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
		"X_AUTHELIA_CONFIG",
		"AUTHELIA_CONFIG",
		"AUTHELIA_LOG_LEVEL",
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

func TestMaskSecretAndEnv(t *testing.T) {
	if MaskSecret("secret") != MaskValue {
		t.Errorf("expected %q, got %q", MaskValue, MaskSecret("secret"))
	}
	if MaskSecret("") != "" {
		t.Errorf("expected empty string for empty input, got %q", MaskSecret(""))
	}

	// MaskEnv
	passEnv := "AQL_NUXEO_PASSWORD=99uzy9eFaX0YgVF4UbZw8MWBLiXg8KH1"
	maskedPass := MaskEnv(passEnv)
	expectedPass := "AQL_NUXEO_PASSWORD=" + MaskValue
	if maskedPass != expectedPass {
		t.Errorf("expected %q, got %q", expectedPass, maskedPass)
	}

	normalEnv := "PORT=8080"
	if MaskEnv(normalEnv) != normalEnv {
		t.Errorf("expected non-sensitive %q to remain unchanged, got %q", normalEnv, MaskEnv(normalEnv))
	}

	// MaskEnvList
	list := []string{
		"PORT=8080",
		"AQL_NUXEO_PASSWORD=secret123",
		"API_KEY=key_abc",
	}
	maskedList := MaskEnvList(list)
	if len(maskedList) != 3 {
		t.Fatalf("expected 3 items, got %d", len(maskedList))
	}
	if maskedList[0] != "PORT=8080" {
		t.Errorf("unexpected first item: %s", maskedList[0])
	}
	if maskedList[1] != "AQL_NUXEO_PASSWORD="+MaskValue {
		t.Errorf("unexpected second item: %s", maskedList[1])
	}
	if maskedList[2] != "API_KEY="+MaskValue {
		t.Errorf("unexpected third item: %s", maskedList[2])
	}

	// MaskLabels
	labels := map[string]string{
		"version":     "1.0",
		"DB_PASSWORD": "secretpassword",
	}
	cleanLabels := MaskLabels(labels)
	if cleanLabels["version"] != "1.0" {
		t.Errorf("expected version to be 1.0, got %s", cleanLabels["version"])
	}
	if cleanLabels["DB_PASSWORD"] != MaskValue {
		t.Errorf("expected DB_PASSWORD to be masked, got %s", cleanLabels["DB_PASSWORD"])
	}
}

func TestSanitizeCommandString(t *testing.T) {
	runCmd := `docker run -d \
  --name aqilink \
  -e "PORT=8080" \
  -e "AQL_NUXEO_PASSWORD=99uzy9eFaX0YgVF4UbZw8MWBLiXg8KH1" \
  -e "AUTH_TOKEN=supertoken" \
  aqilink:latest`

	sanitizedRun := SanitizeCommandString(runCmd)
	if strings.Contains(sanitizedRun, "99uzy9eFaX0YgVF4UbZw8MWBLiXg8KH1") {
		t.Errorf("secret leaked in sanitized run command: %s", sanitizedRun)
	}
	if strings.Contains(sanitizedRun, "supertoken") {
		t.Errorf("token leaked in sanitized run command: %s", sanitizedRun)
	}
	if !strings.Contains(sanitizedRun, "-e \"AQL_NUXEO_PASSWORD="+MaskValue+"\"") {
		t.Errorf("expected masked password in run command, got: %s", sanitizedRun)
	}
	if !strings.Contains(sanitizedRun, "-e \"PORT=8080\"") {
		t.Errorf("expected non-sensitive port to be preserved in run command, got: %s", sanitizedRun)
	}

	compose := `version: '3.8'
services:
  aqilink:
    environment:
      - PORT=8080
      - AQL_NUXEO_PASSWORD=99uzy9eFaX0YgVF4UbZw8MWBLiXg8KH1
      - DB_SECRET=dbsecret`

	sanitizedCompose := SanitizeCommandString(compose)
	if strings.Contains(sanitizedCompose, "99uzy9eFaX0YgVF4UbZw8MWBLiXg8KH1") {
		t.Errorf("secret leaked in sanitized compose: %s", sanitizedCompose)
	}
	if strings.Contains(sanitizedCompose, "dbsecret") {
		t.Errorf("secret leaked in sanitized compose: %s", sanitizedCompose)
	}
	if !strings.Contains(sanitizedCompose, "- AQL_NUXEO_PASSWORD="+MaskValue) {
		t.Errorf("expected masked password in compose, got: %s", sanitizedCompose)
	}
	if !strings.Contains(sanitizedCompose, "- PORT=8080") {
		t.Errorf("expected non-sensitive port to be preserved in compose, got: %s", sanitizedCompose)
	}
}
