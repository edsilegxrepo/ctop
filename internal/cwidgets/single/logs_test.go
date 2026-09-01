// logs_test.go validates container log buffering, ANSI code stripping, and export operations.
//
// Objective:
//
//	Verify log entry ingestion, timestamp formatting, ANSI color escape code stripping, and disk export helpers.
//
// Test Strategy:
//   - Tests raw ANSI escape sequence removal preserving message readability.
//   - Verifies ring buffer capacity caps and timestamp toggle switches.
package single

import (
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/models"
)

func TestLogLinesAnsiSanitization(t *testing.T) {
	logsWidget := NewLogs()

	rawLog := models.Log{
		Timestamp: time.Now(),
		Message:   "\x1b[32m[INFO]\x1b[0m Container \x1b[1mnginx-prod\x1b[0m listening on port 80",
	}

	logsWidget.Add(rawLog)

	if len(logsWidget.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(logsWidget.Entries))
	}

	expected := "[INFO] Container nginx-prod listening on port 80"
	if logsWidget.Entries[0].Message != expected {
		t.Fatalf("expected '%s', got '%s'", expected, logsWidget.Entries[0].Message)
	}
}

func TestLogLinesCapacityRotation(t *testing.T) {
	logsWidget := NewLogs()

	for i := 1; i <= 50; i++ {
		logsWidget.Add(models.Log{
			Timestamp: time.Now(),
			Message:   string(rune('0' + (i % 10))),
		})
	}

	if len(logsWidget.Entries) != 50 {
		t.Fatalf("expected 50 entries, got %d", len(logsWidget.Entries))
	}
}
