package single

import (
	"fmt"
	"image"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

const (
	TabMetrics   = 0
	TabVolumes   = 1
	TabNetwork   = 2
	TabProcess   = 3
	TabTop       = 4
	TabDiff      = 5
	TabGenerator = 6
	TabLabels    = 7
	TabFiles     = 8
	TotalTabs    = 9
)

var TabTitles = []string{
	"1: Overview",
	"2: Mounts",
	"3: Network",
	"4: Env/Process",
	"5: Top",
	"6: Diff",
	"7: Recreate",
	"8: Labels",
	"9: Files",
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
	hint := "[Tab/1-9: Switch | ↑/↓: Scroll | u: Unmask | q: Exit]"
	hintX := w.Inner.Max.X - len(hint) - 1
	if hintX > x {
		buf.SetString(hint, hintStyle, image.Pt(hintX, y))
	}
}
