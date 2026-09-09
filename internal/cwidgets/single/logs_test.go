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
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/models"
	ui "github.com/gizak/termui/v3"
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

func TestLogLinesMaxCapacityInPlaceRotation(t *testing.T) {
	logsWidget := NewLogs()

	// Add more than maxLogEntries (8192) to force in-place rotation
	totalLogs := maxLogEntries + 100
	for i := 1; i <= totalLogs; i++ {
		logsWidget.Add(models.Log{
			Timestamp: time.Unix(int64(i), 0),
			Message:   fmt.Sprintf("log-line-%05d", i),
		})
	}

	if len(logsWidget.Entries) != maxLogEntries {
		t.Fatalf("expected strictly %d entries, got %d", maxLogEntries, len(logsWidget.Entries))
	}

	// First entry should be index 101 ("log-line-00101")
	expectedFirst := "log-line-00101"
	if logsWidget.Entries[0].Message != expectedFirst {
		t.Fatalf("expected first entry %q, got %q", expectedFirst, logsWidget.Entries[0].Message)
	}

	// Last entry should be totalLogs ("log-line-08292")
	expectedLast := fmt.Sprintf("log-line-%05d", totalLogs)
	if logsWidget.Entries[maxLogEntries-1].Message != expectedLast {
		t.Fatalf("expected last entry %q, got %q", expectedLast, logsWidget.Entries[maxLogEntries-1].Message)
	}

	// Verify TUI Draw displays upper limit counter (8192/8192)
	buf := ui.NewBuffer(image.Rect(0, 0, 120, 30))
	logsWidget.SetRect(0, 0, 120, 30)
	logsWidget.Draw(buf)

	expectedLimitTag := fmt.Sprintf("(%d/%d)", maxLogEntries, maxLogEntries)
	if !strings.Contains(logsWidget.Title, expectedLimitTag) {
		t.Fatalf("expected title to contain %q for limit awareness, got: %s", expectedLimitTag, logsWidget.Title)
	}
}

func TestLogsWidgetFilterAndSave(t *testing.T) {
	logsWidget := NewLogs()
	logsWidget.SetContainerName("worker-svc")

	logsWidget.Add(models.Log{Timestamp: time.Now(), Message: "[INFO] Service healthy"})
	logsWidget.Add(models.Log{Timestamp: time.Now(), Message: "[WARN] Memory threshold 80%"})
	logsWidget.Add(models.Log{Timestamp: time.Now(), Message: "[ERROR] Database connection refused"})

	// Test filter
	logsWidget.SetFilter("ERROR")
	if logsWidget.Filter != "ERROR" {
		t.Fatalf("expected filter ERROR, got %s", logsWidget.Filter)
	}

	// Test SaveLogs
	tmpDir := t.TempDir()
	savedPath, err := logsWidget.SaveLogs(tmpDir)
	if err != nil {
		t.Fatalf("failed to save logs: %v", err)
	}
	if !strings.Contains(savedPath, "ctop_logs_worker-svc_") {
		t.Fatalf("unexpected save path filename: %s", savedPath)
	}
}
