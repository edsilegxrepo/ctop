// logs_test.go validates ring buffer log storage, capacity rotation, and ANSI code stripping.
// Test Strategy: Verifies ring buffer FIFO rotation at boundary capacities and clean ANSI sanitization.
package single

import (
	"testing"
	"time"

	"github.com/edsilegx/ctop/models"
)

func TestLogLinesAnsiSanitization(t *testing.T) {
	ll := NewLogLines(10)

	rawLog := models.Log{
		Timestamp: time.Now(),
		Message:   "\x1b[32m[INFO]\x1b[0m Container \x1b[1mnginx-prod\x1b[0m listening on port 80",
	}

	ll.add(rawLog)

	lines := ll.getLines(0, 1)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	expected := "[INFO] Container nginx-prod listening on port 80"
	if lines[0] != expected {
		t.Fatalf("expected '%s', got '%s'", expected, lines[0])
	}
}

func TestLogLinesCapacityRotation(t *testing.T) {
	ll := NewLogLines(3)

	for i := 1; i <= 5; i++ {
		ll.add(models.Log{
			Timestamp: time.Now(),
			Message:   string(rune('0' + i)),
		})
	}

	lines := ll.getLines(0, 3)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	// Should contain the last 3 items: "3", "4", "5"
	if lines[0] != "3" || lines[1] != "4" || lines[2] != "5" {
		t.Fatalf("unexpected rotated lines: %v", lines)
	}
}
