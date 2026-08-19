package compact

import (
	"image"

	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

type CompactHeader struct {
	ui.Block
	fields    []string
	widths    []int
	TextStyle ui.Style
}

func NewCompactHeader() *CompactHeader {
	ch := &CompactHeader{
		Block:     *ui.NewBlock(),
		TextStyle: theme.Style("fg"),
	}
	ch.Border = false
	ch.SetRect(rowPadding, 0, rowPadding+100, 1)
	return ch
}

func (row *CompactHeader) GetHeight() int {
	return 1
}

func (row *CompactHeader) GetRect() image.Rectangle {
	return row.Rectangle
}

func (row *CompactHeader) SetWidths(totalWidth int, widths []int) {
	row.widths = widths
	row.SetRect(rowPadding, row.Min.Y, rowPadding+totalWidth, row.Min.Y+1)
}

func (row *CompactHeader) SetY(y int) {
	row.SetRect(row.Min.X, y, row.Max.X, y+1)
}

func (row *CompactHeader) Draw(buf *ui.Buffer) {
	row.Block.Draw(buf)
	x := rowPadding
	y := row.Min.Y

	for n, field := range row.fields {
		w := 0
		if n < len(row.widths) {
			w = row.widths[n]
		}
		if w > 0 && len(field) > w {
			field = field[:w]
		}
		buf.SetString(field, row.TextStyle, image.Pt(x, y))
		x += w + colSpacing
	}
}

func (row *CompactHeader) clearFieldPars() {
	row.fields = []string{}
}

func (row *CompactHeader) addFieldPar(s string) {
	row.fields = append(row.fields, s)
}
