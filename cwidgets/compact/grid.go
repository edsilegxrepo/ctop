package compact

import (
	"image"
	"sync"

	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

const rowSpacing = 1

type CompactGrid struct {
	ui.Block
	header *CompactHeader
	cols   []CompactCol // reference columns
	Rows   []RowBufferer
	Width  int
	Y      int
	Offset int // starting row offset
	mu     sync.Mutex
}

func NewCompactGrid() *CompactGrid {
	w, h := theme.TermDimensions()
	cg := &CompactGrid{
		Block:  *ui.NewBlock(),
		header: NewCompactHeader(),
		Width:  w,
	}
	cg.Border = false
	cg.SetRect(0, 0, w, h)
	cg.rebuildHeader()
	return cg
}

func (cg *CompactGrid) Align() {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	termW, termH := theme.TermDimensions()
	cg.SetRect(0, cg.Y, termW, termH)
	cg.Width = termW

	y := cg.Y

	if cg.Offset >= len(cg.Rows) || cg.Offset < 0 {
		cg.Offset = 0
	}

	// update row ypos, width recursively
	colWidths := cg.calcWidths()
	cg.header.SetY(y)
	cg.header.SetWidths(cg.Width, colWidths)
	y += cg.header.GetHeight() + 1 // blank separator line between header and container rows

	for _, r := range cg.visibleRows() {
		r.SetY(y)
		r.SetWidths(cg.Width, colWidths)
		y += r.GetHeight() + rowSpacing
	}
}

func (cg *CompactGrid) Clear() {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.Rows = []RowBufferer{}
	cg.rebuildHeader()
}

func (cg *CompactGrid) rowHeight() int {
	if len(cg.Rows) > 0 {
		return cg.Rows[0].GetHeight()
	}
	return 2
}

func (cg *CompactGrid) GetHeight() int {
	rowH := cg.rowHeight()
	if len(cg.Rows) == 0 {
		return cg.header.GetHeight() + 1
	}
	return len(cg.Rows)*(rowH+rowSpacing) + cg.header.GetHeight() + 1
}

func (cg *CompactGrid) SetY(y int)     { cg.Y = y }
func (cg *CompactGrid) SetWidth(w int) { cg.Width = w }

func (cg *CompactGrid) MaxRows() int {
	_, termH := theme.TermDimensions()
	avail := termH - cg.header.GetHeight() - 1 - cg.Y
	if avail <= 0 {
		return 0
	}
	return avail / (cg.rowHeight() + rowSpacing)
}

// calculate and return per-column width
func (cg *CompactGrid) calcWidths() []int {
	var autoCols int
	width := cg.Width
	colWidths := make([]int, len(cg.cols))

	for n, w := range cg.cols {
		colWidths[n] = w.FixedWidth()
		width -= w.FixedWidth()
		if w.FixedWidth() == 0 {
			autoCols++
		}
	}

	spacing := colSpacing * len(cg.cols)
	autoWidth := 10
	if autoCols > 0 && (width-spacing) > 0 {
		autoWidth = (width - spacing) / autoCols
	}
	for n, val := range colWidths {
		if val == 0 {
			colWidths[n] = autoWidth
		}
	}
	return colWidths
}

func (cg *CompactGrid) visibleRows() (rows []RowBufferer) {
	max := cg.MaxRows()
	if max <= 0 {
		return nil
	}
	end := cg.Offset + max
	if end > len(cg.Rows) {
		end = len(cg.Rows)
	}
	if cg.Offset < len(cg.Rows) {
		rows = append(rows, cg.Rows[cg.Offset:end]...)
	}
	return rows
}

func (cg *CompactGrid) Draw(buf *ui.Buffer) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.Block.Draw(buf)
	blank := ui.NewCell(' ', theme.Style("bg"))
	buf.Fill(blank, cg.Rectangle)
	cg.header.Draw(buf)

	divStyle := ui.NewStyle(ui.Color(238), theme.Color("bg"))
	divCell := ui.NewCell('─', divStyle)

	vRows := cg.visibleRows()
	for i, r := range vRows {
		r.Draw(buf)
		divY := r.GetRect().Max.Y
		if divY < cg.Max.Y && i < len(vRows)-1 {
			buf.Fill(divCell, image.Rect(rowPadding, divY, cg.Width-rowPadding, divY+1))
		}
	}
}

func (cg *CompactGrid) AddRows(rows ...RowBufferer) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.Rows = append(cg.Rows, rows...)
}

func (cg *CompactGrid) rebuildHeader() {
	cg.cols = newRowWidgets()
	cg.header.clearFieldPars()
	for _, col := range cg.cols {
		cg.header.addFieldPar(col.Header())
	}
}
