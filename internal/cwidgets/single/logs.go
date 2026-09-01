package single

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/jsonfmt"
	"github.com/edsilegx/ctop/pkg/models"
	"github.com/edsilegx/ctop/pkg/sanitize"
	ui "github.com/gizak/termui/v3"
	"github.com/mattn/go-runewidth"
)

// LogEntry stores an individual container log line with timestamp
type LogEntry struct {
	Timestamp time.Time
	Message   string
}

// Logs widget provides real-time streaming, filtering, timestamp toggling, and export for container logs.
type Logs struct {
	ui.Block
	Entries       []LogEntry
	ContainerName string
	Offset        int
	AutoTail      bool
	ShowTime      bool
	Filter        string
	StatusMsg     string
	mu            sync.RWMutex
}

// NewLogs constructs a new Logs inspection widget.
func NewLogs() *Logs {
	l := &Logs{
		Block:    *ui.NewBlock(),
		Entries:  make([]LogEntry, 0, 4096),
		Offset:   0,
		AutoTail: true,
		ShowTime: true,
	}
	l.Title = "LOGS [Auto-Tail | t: time | /: filter | s: save | ▲▼: scroll]"
	l.BorderStyle = theme.Style("border.fg")
	l.TitleStyle = theme.Style("label.fg")
	l.SetRect(0, 0, colWidth[0], 6)
	return l
}

// SetContainerName updates container name for title and exports.
func (w *Logs) SetContainerName(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ContainerName = name
}

// Add appends a new log message to the buffer.
func (w *Logs) Add(l models.Log) {
	w.mu.Lock()
	defer w.mu.Unlock()

	cleanMsg := sanitize.StripANSI(l.Message)
	formatted := jsonfmt.FormatLogMessage(cleanMsg)

	if len(w.Entries) >= 4096 {
		w.Entries = append(w.Entries[:0], w.Entries[1:]...)
	}
	w.Entries = append(w.Entries, LogEntry{
		Timestamp: l.Timestamp,
		Message:   formatted,
	})
}

// ToggleTime toggles display of timestamps.
func (w *Logs) ToggleTime() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ShowTime = !w.ShowTime
}

// SetFilter sets substring filter for displayed log lines.
func (w *Logs) SetFilter(f string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Filter = strings.TrimSpace(f)
	w.Offset = 0
}

// Up scrolls logs up by 1 line (pauses auto-tail).
func (w *Logs) Up() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.AutoTail = false
	if w.Offset > 0 {
		w.Offset--
	}
}

// Down scrolls logs down by 1 line.
func (w *Logs) Down() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Offset++
}

// PgUp scrolls logs up by half a page.
func (w *Logs) PgUp() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.AutoTail = false
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	step := visibleH / 2
	if step < 1 {
		step = 1
	}
	w.Offset -= step
	if w.Offset < 0 {
		w.Offset = 0
	}
}

// PgDown scrolls logs down by half a page.
func (w *Logs) PgDown() {
	w.mu.Lock()
	defer w.mu.Unlock()
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	step := visibleH / 2
	if step < 1 {
		step = 1
	}
	w.Offset += step
}

// ScrollTop scrolls to the beginning of logs.
func (w *Logs) ScrollTop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.AutoTail = false
	w.Offset = 0
}

// ScrollBottom scrolls to the latest log line and enables auto-tail.
func (w *Logs) ScrollBottom() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.AutoTail = true
}

// SaveLogs exports all buffered log lines to the destination directory.
func (w *Logs) SaveLogs(destDir string) (string, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if destDir == "" {
		destDir = "."
	}
	_ = os.MkdirAll(filepath.Clean(destDir), 0o750)

	cName := w.ContainerName
	if cName == "" {
		cName = "container"
	}
	filePath := filepath.Join(destDir, fmt.Sprintf("ctop_logs_%s_%s.log", cName, time.Now().Format("20060102_150405")))

	var lines []string
	for _, entry := range w.Entries {
		if w.ShowTime && !entry.Timestamp.IsZero() {
			lines = append(lines, fmt.Sprintf("[%s] %s", entry.Timestamp.Format("2006-01-02 15:04:05"), entry.Message))
		} else {
			lines = append(lines, entry.Message)
		}
	}

	if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		return "", err
	}
	return filePath, nil
}

// GetHeight returns required vertical lines.
func (w *Logs) GetHeight() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.Entries) + 2
}

// Draw renders streaming logs with closed bottom border and scroll position.
func (w *Logs) Draw(buf *ui.Buffer) {
	w.mu.Lock()
	defer w.mu.Unlock()

	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	visibleW := w.Inner.Max.X - w.Inner.Min.X
	if visibleH <= 0 || visibleW <= 0 {
		return
	}

	// Filter and format matching lines
	var renderedLines []string
	filterLower := strings.ToLower(w.Filter)

	for _, entry := range w.Entries {
		line := entry.Message
		if w.ShowTime && !entry.Timestamp.IsZero() {
			line = fmt.Sprintf("%s %s", entry.Timestamp.Format("2006-01-02 15:04:05"), entry.Message)
		}
		if filterLower != "" && !strings.Contains(strings.ToLower(line), filterLower) {
			continue
		}
		renderedLines = append(renderedLines, line)
	}

	maxOffset := len(renderedLines) - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}

	if w.AutoTail {
		w.Offset = maxOffset
	} else if w.Offset > maxOffset {
		w.Offset = maxOffset
		w.AutoTail = true
	}

	// Update Title
	filterInfo := ""
	if w.Filter != "" {
		filterInfo = fmt.Sprintf(" [/filter: %s]", w.Filter)
	}
	statusInfo := ""
	if w.StatusMsg != "" {
		statusInfo = fmt.Sprintf(" [%s]", w.StatusMsg)
	}

	cTag := ""
	if w.ContainerName != "" {
		cTag = fmt.Sprintf(" [%s]", w.ContainerName)
	}

	if w.AutoTail {
		w.Title = fmt.Sprintf("LOGS%s%s%s [🔴 Auto-Tail | t: time | /: filter | s: save | ▲▼: scroll]", cTag, filterInfo, statusInfo)
	} else {
		endLine := w.Offset + visibleH
		if endLine > len(renderedLines) {
			endLine = len(renderedLines)
		}
		w.Title = fmt.Sprintf("LOGS%s%s%s [⏸ PAUSED %d-%d/%d | G: resume tail | t: time | s: save]", cTag, filterInfo, statusInfo, w.Offset+1, endLine, len(renderedLines))
	}

	w.Block.Draw(buf)

	if len(renderedLines) == 0 {
		msg := "(no logs available)"
		if w.Filter != "" {
			msg = "(no logs matching filter)"
		}
		buf.SetString(msg, theme.Style("grid.header.fg"), image.Pt(w.Inner.Min.X+2, w.Inner.Min.Y+1))
		return
	}

	y := w.Inner.Min.Y
	tsStyle := ui.NewStyle(ui.ColorCyan, theme.Color("bg"))
	textStyle := theme.Style("par.text.fg")

	endIdx := w.Offset + visibleH
	if endIdx > len(renderedLines) {
		endIdx = len(renderedLines)
	}

	for idx := w.Offset; idx < endIdx; idx++ {
		if y >= w.Inner.Max.Y {
			break
		}
		line := renderedLines[idx]
		currX := w.Inner.Min.X + 2
		runes := []rune(line)

		isTsLine := w.ShowTime && len(runes) >= 19 && runes[4] == '-' && runes[7] == '-' && runes[10] == ' ' && runes[13] == ':' && runes[16] == ':'
		for charIdx, ch := range runes {
			if currX >= w.Inner.Max.X-2 {
				break
			}
			cellStyle := textStyle
			if isTsLine && charIdx < 19 {
				cellStyle = tsStyle
			}
			buf.SetCell(ui.NewCell(ch, cellStyle), image.Pt(currX, y))
			currX += runewidth.RuneWidth(ch)
		}
		y++
	}
}
