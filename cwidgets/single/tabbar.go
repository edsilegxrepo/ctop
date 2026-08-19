package single

import (
	"fmt"
	"image"

	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

const (
	TabMetrics = 0
	TabVolumes = 1
	TabNetwork = 2
	TabProcess = 3
	TabLabels  = 4
	TotalTabs  = 5
)

var TabTitles = []string{
	"1: Overview & Metrics",
	"2: Volumes & Mounts",
	"3: Networking & Ports",
	"4: Process & Env",
	"5: Labels & Compose",
}

// TabBar widget renders the top navigation header for class-based views.
type TabBar struct {
	ui.Block
	ActiveTab int
}

// NewTabBar creates a new TabBar widget.
func NewTabBar() *TabBar {
	tb := &TabBar{
		Block:     *ui.NewBlock(),
		ActiveTab: TabMetrics,
	}
	tb.Border = false
	return tb
}

// Draw renders the active and inactive tabs with clear hotkey indicators.
func (w *TabBar) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	activeStyle := theme.Style("status.warn")
	inactiveStyle := theme.Style("par.text.fg")
	hintStyle := theme.Style("label.fg")

	x := w.Inner.Min.X + 1
	y := w.Inner.Min.Y

	for i, title := range TabTitles {
		style := inactiveStyle
		bracketOpen := "["
		bracketClose := "]"
		if i == w.ActiveTab {
			style = activeStyle
			bracketOpen = "【"
			bracketClose = "】"
		}

		tabTxt := fmt.Sprintf("%s%s%s", bracketOpen, title, bracketClose)
		buf.SetString(tabTxt, style, image.Pt(x, y))
		x += len(tabTxt) + 2
	}

	// Hotkey hint on the right
	hint := "[Tab/1-5: Switch | ↑/↓: Scroll | q: Exit]"
	hintX := w.Inner.Max.X - len(hint) - 1
	if hintX > x {
		buf.SetString(hint, hintStyle, image.Pt(hintX, y))
	}
}
