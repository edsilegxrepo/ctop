package single

import (
	"fmt"
	"image"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

const (
	TabMetrics   = 0
	TabLogs      = 1
	TabVolumes   = 2
	TabNetwork   = 3
	TabProcess   = 4
	TabImage     = 5
	TabTop       = 6
	TabDiff      = 7
	TabGenerator = 8
	TabLabels    = 9
	TabFiles     = 10
	TotalTabs    = 11
)

var TabTitles = []string{
	"1: Overview",
	"2: Logs",
	"3: Mounts",
	"4: Network",
	"5: Env/Proc",
	"6: Image",
	"7: Top",
	"8: Diff",
	"9: Recreate",
	"0: Labels",
	"F: Files",
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

	activeStyle := ui.NewStyle(theme.Color("status.warn"), theme.Color("bg"), ui.ModifierBold)
	inactiveStyle := theme.Style("grid.header.fg")
	hintStyle := theme.Style("grid.header.fg")

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
