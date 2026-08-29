package widgets

import (
	"fmt"
	"image"
	"time"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

type ErrorView struct {
	ui.Block
	lines     []string
	TextStyle ui.Style
}

func NewErrorView() *ErrorView {
	w, h := theme.TermDimensions()
	ev := &ErrorView{
		Block:     *ui.NewBlock(),
		lines:     make([]string, 0, 50),
		TextStyle: theme.Style("status.warn"),
	}
	ev.Title = " ctop - error "
	ev.BorderStyle = theme.Style("status.warn")
	ev.TitleStyle = theme.Style("status.warn")
	ev.SetRect(2, 1, w-2, h-1)
	return ev
}

func (w *ErrorView) Append(s string) {
	if len(w.lines)+2 >= cap(w.lines) {
		w.lines = append(w.lines[:0], w.lines[2:]...)
	}
	ts := time.Now().Local().Format("15:04:05 MST")
	w.lines = append(w.lines, fmt.Sprintf("[%s] %s", ts, s))
	w.lines = append(w.lines, "")
}

func (w *ErrorView) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	innerHeight := w.Inner.Max.Y - w.Inner.Min.Y
	if innerHeight <= 0 {
		return
	}

	offset := len(w.lines) - innerHeight
	if offset < 0 {
		offset = 0
	}

	visibleLines := w.lines[offset:]
	for i, line := range visibleLines {
		y := w.Inner.Min.Y + i
		if y >= w.Inner.Max.Y {
			break
		}
		buf.SetString(line, w.TextStyle, image.Pt(w.Inner.Min.X+1, y))
	}
}

func (w *ErrorView) Resize() {
	termW, termH := theme.TermDimensions()
	w.SetRect(2, 1, termW-2, termH-1)
}
