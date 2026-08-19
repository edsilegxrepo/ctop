// Package sanitize provides text sanitation and ANSI escape sequence stripping utilities.
// Objective: Clean log messages and container output strings to prevent terminal rendering glitches.
package sanitize

import (
	"regexp"
)

// ansiRegex matches ANSI escape sequences (CSI, OSC, etc.)
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[PX^_].*?\x1b\\|\x1b[()][A-Za-z0-9]|\x1b[@-Z\\-_]`)

// StripANSI removes all ANSI escape sequences from a string.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}
