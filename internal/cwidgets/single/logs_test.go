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

func TestLogsWidgetIncludeExcludeFilter(t *testing.T) {
	logsWidget := NewLogs()
	logsWidget.SetContainerName("api-server")

	logsWidget.Add(models.Log{Timestamp: time.Now(), Message: "[INFO] Server started on port 8080"})
	logsWidget.Add(models.Log{Timestamp: time.Now(), Message: "[WARN] Cache miss for key session:123"})
	logsWidget.Add(models.Log{Timestamp: time.Now(), Message: "[ERROR] Connection timeout to redis"})

	bufferContains := func(buf *ui.Buffer, needle string) bool {
		for y := buf.Min.Y; y < buf.Max.Y; y++ {
			var sb strings.Builder
			for x := buf.Min.X; x < buf.Max.X; x++ {
				cell := buf.GetCell(image.Pt(x, y))
				if cell.Rune != 0 {
					sb.WriteRune(cell.Rune)
				}
			}
			if strings.Contains(sb.String(), needle) {
				return true
			}
		}
		return false
	}

	// 1. Default filter mode must be inclusive
	if logsWidget.IsFilterExclude() {
		t.Fatalf("expected default filter mode to be inclusive (FilterExclude=false)")
	}

	// 2. Inclusive filter for "ERROR": only ERROR line rendered
	logsWidget.SetFilter("ERROR")
	buf := ui.NewBuffer(image.Rect(0, 0, 120, 20))
	logsWidget.SetRect(0, 0, 120, 20)
	logsWidget.Draw(buf)

	if !strings.Contains(logsWidget.Title, "[/filter: ERROR (include)]") {
		t.Fatalf("expected title to show include filter mode, got: %s", logsWidget.Title)
	}
	if !bufferContains(buf, "Connection timeout to redis") {
		t.Fatalf("expected ERROR log line to be present in include mode")
	}
	if bufferContains(buf, "Server started on port 8080") {
		t.Fatalf("expected INFO log line to be omitted in include mode")
	}
	if bufferContains(buf, "Cache miss for key session:123") {
		t.Fatalf("expected WARN log line to be omitted in include mode")
	}

	// 3. Toggle filter mode to Exclude: ERROR line excluded, INFO and WARN rendered
	mode := logsWidget.ToggleFilterMode()
	if !mode || !logsWidget.IsFilterExclude() {
		t.Fatalf("expected filter mode to be exclude after ToggleFilterMode")
	}

	buf = ui.NewBuffer(image.Rect(0, 0, 120, 20))
	logsWidget.Draw(buf)
	if !strings.Contains(logsWidget.Title, "[/filter: ERROR (exclude)]") {
		t.Fatalf("expected title to show exclude filter mode, got: %s", logsWidget.Title)
	}
	if bufferContains(buf, "Connection timeout to redis") {
		t.Fatalf("expected ERROR log line to be excluded in exclude mode")
	}
	if !bufferContains(buf, "Server started on port 8080") {
		t.Fatalf("expected INFO log line to be present in exclude mode")
	}
	if !bufferContains(buf, "Cache miss for key session:123") {
		t.Fatalf("expected WARN log line to be present in exclude mode")
	}

	// 4. SetFilterExclude explicit setter
	logsWidget.SetFilterExclude(false)
	if logsWidget.IsFilterExclude() {
		t.Fatalf("expected filter mode to be inclusive after SetFilterExclude(false)")
	}
	logsWidget.SetFilterExclude(true)
	if !logsWidget.IsFilterExclude() {
		t.Fatalf("expected filter mode to be exclusive after SetFilterExclude(true)")
	}

	// 5. Empty filter query in exclude mode shows all logs (does not filter anything out)
	logsWidget.SetFilter("")
	buf = ui.NewBuffer(image.Rect(0, 0, 120, 20))
	logsWidget.Draw(buf)
	if !bufferContains(buf, "Server started on port 8080") ||
		!bufferContains(buf, "Cache miss for key session:123") ||
		!bufferContains(buf, "Connection timeout to redis") {
		t.Fatalf("expected all logs to be rendered when filter query is empty in exclude mode")
	}

	// 6. When filter matches all lines in exclude mode, empty state message is shown
	logsWidget.SetFilter("e") // all 3 entries contain letter 'e'
	buf = ui.NewBuffer(image.Rect(0, 0, 120, 20))
	logsWidget.Draw(buf)
	if !bufferContains(buf, "no logs left after excluding \"e\"") {
		t.Fatalf("expected empty exclude state message in buffer")
	}
}
