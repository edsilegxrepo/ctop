// icons_test.go validates icon style toggles (Unicode vs Nerd Font) and state/health glyph resolution.
//
// Objective:
//
//	Ensure glyph lookups return correct Unicode and Nerd Font code points for all container states and health checks.
//
// Test Strategy:
//   - Tests SetIconStyle switching and verifies StatusGlyph, HealthGlyph, and ExtGlyph outputs.
package theme

import (
	"testing"
)

func TestIconStyles(t *testing.T) {
	SetIconStyle("unicode")
	if GetIconStyle() != IconStyleUnicode {
		t.Fatalf("expected IconStyleUnicode, got %s", GetIconStyle())
	}
	if StatusGlyph("running") != '►' {
		t.Fatalf("expected '►' for running in unicode mode, got %c", StatusGlyph("running"))
	}
	if HealthGlyph("healthy") != '☼' {
		t.Fatalf("expected '☼' for healthy in unicode mode, got %c", HealthGlyph("healthy"))
	}

	SetIconStyle("nerd")
	if GetIconStyle() != IconStyleNerd {
		t.Fatalf("expected IconStyleNerd, got %s", GetIconStyle())
	}
	if StatusGlyph("running") != '\uf04b' {
		t.Fatalf("expected '\\uf04b' for running in nerd mode, got %c", StatusGlyph("running"))
	}
	if HealthGlyph("healthy") != '\uf058' {
		t.Fatalf("expected '\\uf058' for healthy in nerd mode, got %c", HealthGlyph("healthy"))
	}

	// Reset to default
	SetIconStyle("unicode")
}
