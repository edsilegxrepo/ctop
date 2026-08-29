// Package sanitize provides text sanitation and ANSI escape sequence stripping utilities.
// Objective: Clean log messages and container output strings to prevent terminal rendering glitches.
package sanitize

import (
	"regexp"
	"strings"
)

// ansiRegex matches ANSI escape sequences (CSI, OSC, etc.)
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[PX^_].*?\x1b\\|\x1b[()][A-Za-z0-9]|\x1b[@-Z\\-_]`)

// sensitiveKeyPattern matches sensitive environment variable or credential keys.
var sensitiveKeyPattern = regexp.MustCompile(`(?i)(PASS|SECRET|KEY|TOKEN|AUTH|CERT|CRED|PRIVATE|DATABASE_URL|DB_URL|DSN|SIGNATURE|BEARER|AWS_|ACCESS_KEY|SESSION_TOKEN|COOKIE|SALT|HMAC|WEBHOOK|PRIVATE_KEY|APIKEY)`)

// StripANSI removes all ANSI escape sequences from a string.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// IsSensitiveKey returns true if the key name indicates sensitive credential or secret data.
func IsSensitiveKey(key string) bool {
	return sensitiveKeyPattern.MatchString(key)
}

// SanitizeEnv filters out all sensitive environment variables to prevent leaking credentials in web dashboards or telemetry.
func SanitizeEnv(envs []string) []string {
	if len(envs) == 0 {
		return nil
	}
	var clean []string
	for _, env := range envs {
		env = strings.TrimSpace(env)
		if env == "" {
			continue
		}
		eqIdx := strings.Index(env, "=")
		key := env
		if eqIdx > -1 {
			key = env[:eqIdx]
		}
		if !IsSensitiveKey(key) {
			clean = append(clean, env)
		}
	}
	return clean
}
