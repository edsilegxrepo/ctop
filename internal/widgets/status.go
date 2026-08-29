package widgets

import (
	"image"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

var statusHeight = 1

type StatusLine struct {
	ui.Block
	Message string
	Style   ui.Style
	isErr   bool
}

func NewStatusLine() *StatusLine {
	sl := &StatusLine{
		Block: *ui.NewBlock(),
		Style: theme.Style2("header.fg", "header.bg"),
	}
	sl.Border = false
	sl.Align()
	return sl
}

func (sl *StatusLine) Show(s string) {
	sl.isErr = false
	sl.Style = theme.Style2("header.fg", "header.bg")
	sl.Message = s
	sl.display()
}

func (sl *StatusLine) ShowErr(s string) {
	sl.isErr = true
	sl.Style = theme.Style2("status.danger", "header.bg")
	sl.Message = s
	sl.display()
}

func (sl *StatusLine) display() {
	sl.Align()
}

func (sl *StatusLine) Draw(buf *ui.Buffer) {
	bgStyle := theme.Style2("header.fg", "header.bg")
	buf.Fill(ui.NewCell(' ', bgStyle), sl.Rectangle)
	if sl.Message != "" {
		buf.SetString(sl.Message, sl.Style, image.Pt(sl.Min.X+2, sl.Min.Y))
	}
}

func (sl *StatusLine) Align() {
	w, h := theme.TermDimensions()
	sl.SetRect(0, h-statusHeight, w, h)
}

func (sl *StatusLine) Height() int { return statusHeight }
