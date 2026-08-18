package compact

import (
	"image"

	"github.com/bcicen/ctop/config"
	"github.com/bcicen/ctop/logging"
	"github.com/bcicen/ctop/models"
	"github.com/bcicen/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

const rowPadding = 1

var log = logging.Init()

type RowBufferer interface {
	SetY(int)
	SetWidths(int, []int)
	GetHeight() int
	GetRect() image.Rectangle
	Draw(buf *ui.Buffer)
}

type CompactRow struct {
	ui.Block
	Cols   []CompactCol
	Height int
}

func NewCompactRow() *CompactRow {
	row := &CompactRow{
		Block:  *ui.NewBlock(),
		Cols:   newRowWidgets(),
		Height: 1,
	}
	row.Border = false
	return row
}

func (row *CompactRow) SetMeta(m models.Meta) {
	for _, w := range row.Cols {
		w.SetMeta(m)
	}
}

func (row *CompactRow) SetMetrics(m models.Metrics) {
	for _, w := range row.Cols {
		w.SetMetrics(m)
	}
}

// Reset gauges, counters, etc. to default unread values
func (row *CompactRow) Reset() {
	for _, w := range row.Cols {
		w.Reset()
	}
}

func (row *CompactRow) GetHeight() int { return row.Height }

func (row *CompactRow) GetRect() image.Rectangle { return row.Rectangle }

func (row *CompactRow) SetY(y int) {
	row.SetRect(row.Min.X, y, row.Max.X, y+row.Height)
	for _, w := range row.Cols {
		rect := w.GetRect()
		w.SetRect(rect.Min.X, y, rect.Max.X, y+row.Height)
	}
}

func (row *CompactRow) SetWidths(totalWidth int, widths []int) {
	x := rowPadding
	y := row.Min.Y
	row.SetRect(x, y, x+totalWidth, y+row.Height)

	for n, w := range row.Cols {
		wWidth := 0
		if n < len(widths) {
			wWidth = widths[n]
		}
		w.SetRect(x, y, x+wWidth, y+row.Height)
		x += wWidth + colSpacing
	}
}

func (row *CompactRow) Draw(buf *ui.Buffer) {
	row.Block.Draw(buf)
	for _, w := range row.Cols {
		w.Draw(buf)
	}
}

func (row *CompactRow) Highlight() {
	if config.GetSwitchVal("fullRowCursor") {
		for i := 1; i < len(row.Cols); i++ {
			row.Cols[i].Highlight()
		}
	} else if len(row.Cols) > 1 {
		row.Cols[1].Highlight()
	}
}

func (row *CompactRow) UnHighlight() {
	for i := 1; i < len(row.Cols); i++ {
		row.Cols[i].UnHighlight()
	}
}

type RowBg struct {
	ui.Block
	BgStyle ui.Style
}

func NewRowBg() *RowBg {
	bg := &RowBg{
		Block:   *ui.NewBlock(),
		BgStyle: theme.Style("par.text.bg"),
	}
	bg.Border = false
	return bg
}
