// keys_test.go validates keyboard shortcut mappings for all registered actions.
// Test Strategy: Table-driven unit tests verifying positive and negative key identifier matching.
package keys

import (
	"testing"
)

func TestIsKeyMatch(t *testing.T) {
	tests := []struct {
		action   string
		keyID    string
		expected bool
	}{
		{"up", "<Up>", true},
		{"up", "k", true},
		{"up", "j", false},
		{"down", "<Down>", true},
		{"down", "j", true},
		{"pgup", "<PageUp>", true},
		{"pgup", "<C-u>", true},
		{"pgdown", "<PageDown>", true},
		{"pgdown", "<C-d>", true},
		{"exit", "q", true},
		{"exit", "<Escape>", true},
		{"exit", "<C-c>", true},
		{"help", "h", true},
		{"help", "?", true},
		{"enter", "<Enter>", true},
		{"unknownAction", "x", false},
		{"up", "unknownKey", false},
	}

	for _, tt := range tests {
		t.Run(tt.action+"_"+tt.keyID, func(t *testing.T) {
			result := IsKeyMatch(tt.action, tt.keyID)
			if result != tt.expected {
				t.Fatalf("IsKeyMatch(%q, %q) = %v; expected %v", tt.action, tt.keyID, result, tt.expected)
			}
		})
	}
}
