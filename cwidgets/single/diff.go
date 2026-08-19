package single

import (
	"fmt"
	"image"
	"strings"

	"github.com/edsilegx/ctop/models"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

// Diff widget displays container filesystem changes (docker diff).
type Diff struct {
	ui.Block
	Changes []models.Change
	empty   bool
}

// NewDiff constructs a new container filesystem changes inspector widget.
func NewDiff() *Diff {
	d := &Diff{
		Block: *ui.NewBlock(),
		empty: true,
	}
	d.Title = "FILESYSTEM CHANGES (DIFF)"
	d.BorderStyle = theme.Style("border.fg")
	d.TitleStyle = theme.Style("label.fg")
	d.SetRect(0, 0, colWidth[0], 6)
	return d
}

// Set updates the changes list.
func (w *Diff) Set(changes []models.Change) {
	w.Changes = changes
	w.empty = len(changes) == 0
}

// GetHeight returns required vertical lines.
func (w *Diff) GetHeight() int {
	if w.empty {
		return 5
	}
	return len(w.Changes) + 4
}

// Draw renders color-coded filesystem changes.
func (w *Diff) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	headerStyle := theme.Style("label.fg")
	valStyle := theme.Style("par.text.fg")
	addStyle := theme.Style("status.ok")
	modStyle := theme.Style("status.warn")
	delStyle := theme.Style("status.danger")

	y := w.Inner.Min.Y

	if w.empty {
		buf.SetString("No filesystem changes detected on writable container layer.", valStyle, image.Pt(w.Inner.Min.X+2, y+1))
		return
	}

	header := fmt.Sprintf("%-6s %s", "FLAG", "CHANGED FILE / DIRECTORY PATH")
	buf.SetString(header, headerStyle, image.Pt(w.Inner.Min.X+1, y))
	y++

	sepLine := strings.Repeat("─", max(10, w.Inner.Max.X-w.Inner.Min.X-2))
	buf.SetString(sepLine, theme.Style("border.fg"), image.Pt(w.Inner.Min.X+1, y))
	y++

	for _, ch := range w.Changes {
		if y >= w.Inner.Max.Y {
			break
		}

		var flagStr string
		var flagStyle ui.Style
		switch ch.Kind {
		case 1:
			flagStr = "[A]"
			flagStyle = addStyle
		case 2:
			flagStr = "[D]"
			flagStyle = delStyle
		default: // 0
			flagStr = "[C]"
			flagStyle = modStyle
		}

		buf.SetString(flagStr, flagStyle, image.Pt(w.Inner.Min.X+1, y))

		pathX := w.Inner.Min.X + 8
		maxW := w.Inner.Max.X - pathX
		path := ch.Path
		if maxW > 0 {
			if len(path) > maxW {
				path = path[:maxW]
			}
			buf.SetString(path, valStyle, image.Pt(pathX, y))
		}

		y++
	}
}
