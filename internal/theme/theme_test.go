// theme_test.go validates theme color lookup, style generation, color map inversion, and terminal dimension fallback.
// Test Strategy: Verifies palette mappings, inverted mode toggling, and headless fallback sizing.
package theme

import (
	"testing"

	ui "github.com/gizak/termui/v3"
)

func TestThemeColors(t *testing.T) {
	if c := Color("status.ok"); c != ui.ColorGreen {
		t.Errorf("expected status.ok to be ColorGreen, got %v", c)
	}
	if c := Color("status.danger"); c != ui.ColorRed {
		t.Errorf("expected status.danger to be ColorRed, got %v", c)
	}
	if c := Color("nonexistent"); c != ui.ColorWhite {
		t.Errorf("expected default ColorWhite for nonexistent key, got %v", c)
	}
}

func TestThemeStyles(t *testing.T) {
	s := Style("status.ok")
	if s.Fg != ui.ColorGreen {
		t.Errorf("expected Style Fg to be ColorGreen, got %v", s.Fg)
	}

	s2 := Style2("status.ok", "status.danger")
	if s2.Fg != ui.ColorGreen || s2.Bg != ui.ColorRed {
		t.Errorf("expected Style2 Fg=Green, Bg=Red, got Fg=%v, Bg=%v", s2.Fg, s2.Bg)
	}
}

func TestInvertColorMap(t *testing.T) {
	InvertColorMap()
	if c := Color("fg"); c != ui.ColorBlack {
		t.Errorf("expected inverted fg to be ColorBlack, got %v", c)
	}
	if c := Color("header.bg"); c != ui.ColorBlack {
		t.Errorf("expected inverted header.bg to be ColorBlack, got %v", c)
	}
	// Calling a second time should be a no-op
	InvertColorMap()
}

func TestTermDimensionsAndSync(t *testing.T) {
	w, h := TermDimensions()
	if w <= 0 || h <= 0 {
		t.Errorf("expected positive dimensions, got w=%d, h=%d", w, h)
	}
	SyncTerm()
	SafeClear()
}
