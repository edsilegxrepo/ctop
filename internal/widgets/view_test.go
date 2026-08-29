// view_test.go validates TextView line splitting, word wrapping, and ANSI sequence stripping.
// Test Strategy: Tests string slicing boundaries, limit alignments, and ANSI escape filter pipelines.
package widgets

import "testing"

func TestSplitEmptyLine(t *testing.T) {
	result := splitLine("", 5)
	if len(result) != 0 {
		t.Errorf("expected: 0 lines, got: %d", len(result))
	}
}

func TestSplitLineShorterThanLimit(t *testing.T) {
	result := splitLine("hello", 7)
	if len(result) != 1 {
		t.Errorf("expected: 0 lines, got: %d", len(result))
	}
}

func TestSplitLineLongerThanLimit(t *testing.T) {
	result := splitLine("hello", 3)
	if len(result) != 2 {
		t.Errorf("expected: 0 lines, got: %d", len(result))
	}
}

func TestSplitLineSameAsLimit(t *testing.T) {
	result := splitLine("hello", 5)
	if len(result) != 1 {
		t.Errorf("expected: 0 lines, got: %d", len(result))
	}
}

type testToggleText struct {
	text string
}

func (t *testToggleText) Toggle(on bool) string {
	return t.text
}

func TestTextViewAnsiSanitization(t *testing.T) {
	tv := &TextView{
		Text:    []ToggleText{&testToggleText{text: "\x1b[31;1mERROR:\x1b[0m Failed to connect to server"}},
		padding: Padding{0, 0},
	}
	tv.Inner.Min.X = 0
	tv.Inner.Max.X = 80
	tv.Inner.Min.Y = 0
	tv.Inner.Max.Y = 20

	tv.RecomputeTextOut()

	if len(tv.TextOut) != 1 {
		t.Fatalf("expected 1 line, got %d", len(tv.TextOut))
	}
	expected := "ERROR: Failed to connect to server"
	if tv.TextOut[0] != expected {
		t.Fatalf("expected ANSI stripped '%s', got '%s'", expected, tv.TextOut[0])
	}
}
