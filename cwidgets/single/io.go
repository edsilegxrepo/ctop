package single

import (
	"fmt"
	"image"
	"strings"

	"github.com/edsilegx/ctop/cwidgets"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

type IO struct {
	ui.Block
	readTitle  string
	writeTitle string
	readHist   *DiffHist
	writeHist  *DiffHist
}

func NewIO() *IO {
	io := &IO{
		Block:      *ui.NewBlock(),
		readTitle:  "READ [0b/s]",
		writeTitle: "WRITE [0b/s]",
		readHist:   NewDiffHist(60),
		writeHist:  NewDiffHist(60),
	}
	io.Title = "IO"
	io.BorderStyle = theme.Style("border.fg")
	io.TitleStyle = theme.Style("label.fg")
	io.SetRect(0, 0, colWidth[0], 12)
	return io
}

func (w *IO) Align() {
	dx := (w.Max.X - w.Min.X) - 4
	if dx > 10 {
		w.readHist.SetLimit(dx)
		w.writeHist.SetLimit(dx)
	}
}

func (w *IO) Update(read int64, write int64) {
	w.readHist.Append(int(read))
	rateRead := strings.ToLower(cwidgets.ByteFormatShort(w.readHist.Val))
	w.readTitle = fmt.Sprintf("READ [%s/s]", rateRead)

	w.writeHist.Append(int(write))
	rateWrite := strings.ToLower(cwidgets.ByteFormatShort(w.writeHist.Val))
	w.writeTitle = fmt.Sprintf("WRITE [%s/s]", rateWrite)
}

func (w *IO) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	readColor := theme.Color("sparkline.line.fg")
	writeColor := theme.Color("status.warn")

	innerH := w.Inner.Dy()
	sparkH := (innerH - 2) / 2
	if sparkH < 1 {
		sparkH = 1
	}

	minX := w.Inner.Min.X + 1
	maxX := w.Inner.Max.X - 1

	// Section 1: READ
	y1 := w.Inner.Min.Y
	buf.SetString(w.readTitle, theme.Style("sparkline.title.fg"), image.Pt(minX, y1))
	drawSparkline(buf, w.readHist.Data, readColor, minX, y1+1, sparkH, maxX)

	// Section 2: WRITE
	y2 := y1 + 1 + sparkH
	if y2 < w.Inner.Max.Y {
		buf.SetString(w.writeTitle, theme.Style("sparkline.title.fg"), image.Pt(minX, y2))
		drawSparkline(buf, w.writeHist.Data, writeColor, minX, y2+1, sparkH, maxX)
	}
}
