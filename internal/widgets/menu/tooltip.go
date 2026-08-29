package menu

import (
	"image"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

type ToolTip struct {
	ui.Block
	Lines     []string
	TextStyle ui.Style
	padding   Padding
}

func NewToolTip(lines ...string) *ToolTip {
	t := &ToolTip{
		Block:     *ui.NewBlock(),
		Lines:     lines,
		TextStyle: theme.Style2("menu.text.fg", "menu.text.bg"),
		padding:   Padding{2, 1},
	}
	t.BorderStyle = theme.Style("menu.border.fg")
	t.TitleStyle = theme.Style("menu.label.fg")
	t.Align()
	return t
}

func (t *ToolTip) Draw(buf *ui.Buffer) {
	t.Block.Draw(buf)

	y := t.Inner.Min.Y + t.padding[1]
	for _, line := range t.Lines {
		x := t.Inner.Min.X + t.padding[0]
		buf.SetString(line, t.TextStyle, image.Pt(x, y))
		y++
	}
}

// Align sets width and height based on screen size
func (t *ToolTip) Align() {
	w, h := theme.TermDimensions()
	width := w - (t.padding[0] * 2) - 2
	height := len(t.Lines) + (t.padding[1] * 2) + 2
	y1 := h - height
	if y1 < 0 {
		y1 = 0
	}
	t.SetRect(1, y1, 1+width, h)
}
