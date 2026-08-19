// sanitize_test.go validates ANSI and OSC escape sequence removal from log strings.
// Test Strategy: Table-driven unit tests verifying color codes, cursor sequences, OSC titles, and edge cases.
package sanitize

import (
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
