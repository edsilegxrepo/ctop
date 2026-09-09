// Package sanitize provides text sanitation, ANSI escape sequence stripping, and secret masking utilities.
//
// Objective:
//
//	Clean log messages, telemetry fields, and environment variables to prevent terminal rendering glitches and protect sensitive credentials.
//
// Core Components:
//   - StripANSI: Strips CSI, OSC, and control escape codes.
//   - IsSensitiveKey: Regex pattern matcher identifying API keys, tokens, passwords, and secrets.
//   - SanitizeEnv: Masks secret values before dashboard or telemetry serialization.
package sanitize

import (
	"regexp"
	"strings"
)

// ansiRegex matches ANSI escape sequences (CSI, OSC, etc.)
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[PX^_].*?\x1b\\|\x1b[()][A-Za-z0-9]|\x1b[@-Z\\-_]`)

// sensitiveKeyPattern matches sensitive environment variable or credential keys.
var sensitiveKeyPattern = regexp.MustCompile(`(?i)(PASSW|SECRET|TOKEN|CRED|PRIVATE|DATABASE_URL|DB_URL|DSN|SIGNATURE|BEARER|AWS_|ACCESS_KEY|SESSION_TOKEN|COOKIE|SALT|HMAC|WEBHOOK|PRIVATE_KEY|API_KEY|APIKEY|AUTH_HEADER|BASIC_AUTH|_KEY|KEY_|CERT)`)

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

// MaskValue is the standard visual obfuscation placeholder for sensitive credentials.
const MaskValue = "•••••••••••• [masked]"

// MaskSecret returns the obfuscated placeholder for sensitive values.
func MaskSecret(val string) string {
	if val == "" {
		return ""
	}
	return MaskValue
}

// MaskEnv masks sensitive environment variable definitions in "KEY=VALUE" form.
// If the key indicates sensitive data (via IsSensitiveKey), the value is replaced with MaskValue.
func MaskEnv(env string) string {
	env = strings.TrimSpace(env)
	if env == "" {
		return ""
	}
	eqIdx := strings.Index(env, "=")
	if eqIdx == -1 {
		if IsSensitiveKey(env) {
			return env + "=" + MaskValue
		}
		return env
	}
	key := env[:eqIdx]
	if IsSensitiveKey(key) {
		return key + "=" + MaskValue
	}
	return env
}

// MaskEnvList returns a copy of envs with all sensitive variable values masked.
func MaskEnvList(envs []string) []string {
	if len(envs) == 0 {
		return nil
	}
	var res []string
	for _, env := range envs {
		env = strings.TrimSpace(env)
		if env == "" {
			continue
		}
		res = append(res, MaskEnv(env))
	}
	return res
}

// MaskLabels returns a copy of the label map with all sensitive keys obfuscated with MaskValue.
func MaskLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	clean := make(map[string]string, len(labels))
	for k, v := range labels {
		if IsSensitiveKey(k) {
			clean[k] = MaskValue
		} else {
			clean[k] = v
		}
	}
	return clean
}

// cmdEnvRunRegex matches docker run -e flags: -e "KEY=VALUE" or -e 'KEY=VALUE' or -e KEY=VALUE
var cmdEnvRunRegex = regexp.MustCompile(`(?i)(-e\s+["']?)([a-zA-Z0-9_.-]+)=([^"'\n\\]*)(["']?)`)

// composeEnvRegex matches docker-compose environment list items: - KEY=VALUE
var composeEnvRegex = regexp.MustCompile(`(?m)^(\s*-\s+)([a-zA-Z0-9_.-]+)=([^\r\n]*)$`)

// composeEnvKeyValRegex matches docker-compose dictionary entries: KEY: VALUE or KEY: "VALUE"
var composeEnvKeyValRegex = regexp.MustCompile(`(?m)^(\s*)([a-zA-Z0-9_.-]+):\s*["']?([^"'\r\n]+)["']?$`)

// SanitizeCommandString searches a generated command string or compose specification and masks all sensitive keys.
func SanitizeCommandString(cmd string) string {
	if cmd == "" {
		return ""
	}
	// 1. Docker run -e "KEY=VALUE"
	out := cmdEnvRunRegex.ReplaceAllStringFunc(cmd, func(m string) string {
		sub := cmdEnvRunRegex.FindStringSubmatch(m)
		if len(sub) == 5 && IsSensitiveKey(sub[2]) {
			return sub[1] + sub[2] + "=" + MaskValue + sub[4]
		}
		return m
	})
	// 2. Docker compose list: - KEY=VALUE
	out = composeEnvRegex.ReplaceAllStringFunc(out, func(m string) string {
		sub := composeEnvRegex.FindStringSubmatch(m)
		if len(sub) == 4 && IsSensitiveKey(sub[2]) {
			return sub[1] + sub[2] + "=" + MaskValue
		}
		return m
	})
	// 3. Docker compose dict: KEY: VALUE
	out = composeEnvKeyValRegex.ReplaceAllStringFunc(out, func(m string) string {
		sub := composeEnvKeyValRegex.FindStringSubmatch(m)
		if len(sub) == 4 && IsSensitiveKey(sub[2]) {
			return sub[1] + sub[2] + ": " + MaskValue
		}
		return m
	})
	return out
}
