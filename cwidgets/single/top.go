package single

import (
	"fmt"
	"image"
	"strings"

	"github.com/edsilegx/ctop/models"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

// Top widget displays running processes inside the container (docker top).
type Top struct {
	ui.Block
	Result models.TopResult
	empty  bool
}

// NewTop constructs a new in-container process top inspector widget.
func NewTop() *Top {
	t := &Top{
		Block: *ui.NewBlock(),
		empty: true,
	}
	t.Title = "IN-CONTAINER PROCESSES (TOP)"
	t.BorderStyle = theme.Style("border.fg")
	t.TitleStyle = theme.Style("label.fg")
	t.SetRect(0, 0, colWidth[0], 6)
	return t
}

// Set updates process table data.
func (w *Top) Set(res models.TopResult) {
	w.Result = res
	w.empty = len(res.Processes) == 0
}

// GetHeight returns required vertical space for processes.
func (w *Top) GetHeight() int {
	if w.empty {
		return 5
	}
	return len(w.Result.Processes) + 4
}

// Draw renders the formatted processes table.
func (w *Top) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	headerStyle := theme.Style("label.fg")
	valStyle := theme.Style("par.text.fg")
	pidStyle := theme.Style("status.warn")

	y := w.Inner.Min.Y

	if w.empty {
		buf.SetString("No active in-container processes found (container may be stopped).", valStyle, image.Pt(w.Inner.Min.X+2, y+1))
		return
	}

	// Format header columns
	titles := w.Result.Titles
	if len(titles) == 0 {
		titles = []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"}
	}

	headerStr := ""
	for i, t := range titles {
		if i == len(titles)-1 {
			headerStr += t
		} else {
			headerStr += fmt.Sprintf("%-10s ", t)
		}
	}
	buf.SetString(headerStr, headerStyle, image.Pt(w.Inner.Min.X+1, y))
	y++

	sepLine := strings.Repeat("─", max(10, w.Inner.Max.X-w.Inner.Min.X-2))
	buf.SetString(sepLine, theme.Style("border.fg"), image.Pt(w.Inner.Min.X+1, y))
	y++

	for _, proc := range w.Result.Processes {
		if y >= w.Inner.Max.Y {
			break
		}

		x := w.Inner.Min.X + 1
		for i, col := range proc {
			if x >= w.Inner.Max.X-2 {
				break
			}
			style := valStyle
			if i == 1 { // PID column
				style = pidStyle
			}

			if i == len(proc)-1 {
				maxW := w.Inner.Max.X - x - 1
				if maxW > 0 {
					if len(col) > maxW {
						col = col[:maxW]
					}
					buf.SetString(col, style, image.Pt(x, y))
				}
			} else {
				buf.SetString(fmt.Sprintf("%-10s", col), style, image.Pt(x, y))
				x += 11
			}
		}
		y++
	}
}
