package single

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/edsilegx/ctop/cwidgets"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

type Net struct {
	ui.Block
	rxTitle string
	txTitle string
	rxHist  *DiffHist
	txHist  *DiffHist
}

func NewNet() *Net {
	net := &Net{
		Block:   *ui.NewBlock(),
		rxTitle: "RX [0b/s]",
		txTitle: "TX [0b/s]",
		rxHist:  NewDiffHist(60),
		txHist:  NewDiffHist(60),
	}
	net.Title = "NET"
	net.BorderStyle = theme.Style("border.fg")
	net.TitleStyle = theme.Style("label.fg")
	net.SetRect(0, 0, colWidth[0], 12)
	return net
}

func (w *Net) Align() {
	dx := (w.Max.X - w.Min.X) - 4
	if dx > 10 {
		w.rxHist.SetLimit(dx)
		w.txHist.SetLimit(dx)
	}
}

func (w *Net) Update(rx int64, tx int64) {
	w.rxHist.Append(int(rx))
	rateRx := strings.ToLower(cwidgets.ByteFormat(w.rxHist.Val))
	w.rxTitle = fmt.Sprintf("RX [%s/s]", rateRx)

	w.txHist.Append(int(tx))
	rateTx := strings.ToLower(cwidgets.ByteFormat(w.txHist.Val))
	w.txTitle = fmt.Sprintf("TX [%s/s]", rateTx)
}

func (w *Net) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	rxColor := theme.Color("sparkline.line.fg")
	txColor := theme.Color("status.warn")

	innerH := w.Inner.Dy()
	sparkH := (innerH - 2) / 2
	if sparkH < 1 {
		sparkH = 1
	}

	minX := w.Inner.Min.X + 1
	maxX := w.Inner.Max.X - 1

	// Section 1: RX
	y1 := w.Inner.Min.Y
	buf.SetString(w.rxTitle, theme.Style("sparkline.title.fg"), image.Pt(minX, y1))
	drawSparkline(buf, w.rxHist.Data, rxColor, minX, y1+1, sparkH, maxX)

	// Section 2: TX
	y2 := y1 + 1 + sparkH
	if y2 < w.Inner.Max.Y {
		buf.SetString(w.txTitle, theme.Style("sparkline.title.fg"), image.Pt(minX, y2))
		drawSparkline(buf, w.txHist.Data, txColor, minX, y2+1, sparkH, maxX)
	}
}

func drawSparkline(buf *ui.Buffer, data []int, color ui.Color, minX, topY, height, maxX int) {
	if topY+height > buf.Max.Y || minX >= maxX || height <= 0 {
		return
	}
	dx := maxX - minX
	if dx <= 0 {
		return
	}

	maxVal := 0
	for _, v := range data {
		if v > maxVal {
			maxVal = v
		}
	}

	samples := data
	if len(samples) > dx {
		samples = samples[len(samples)-dx:]
	}

	barStyle := ui.NewStyle(color, theme.Color("bg"))
	bottomY := topY + height - 1

	startX := maxX - len(samples)
	for i, val := range samples {
		x := startX + i
		if x < minX || x >= maxX {
			continue
		}
		if val <= 0 || maxVal <= 0 {
			continue
		}

		ratio := float64(val) / float64(maxVal)
		totalHalfBlocks := int(math.Round(ratio * float64(height*2)))
		if totalHalfBlocks < 1 {
			totalHalfBlocks = 1
		}
		if totalHalfBlocks > height*2 {
			totalHalfBlocks = height * 2
		}

		fullLines := totalHalfBlocks / 2
		hasHalf := (totalHalfBlocks % 2) != 0

		for ly := 0; ly < fullLines && ly < height; ly++ {
			buf.SetCell(ui.NewCell('█', barStyle), image.Pt(x, bottomY-ly))
		}

		if hasHalf && fullLines < height {
			buf.SetCell(ui.NewCell('▄', barStyle), image.Pt(x, bottomY-fullLines))
		}
	}
}

func toFloat64Slice(ints []int) []float64 {
	floats := make([]float64, len(ints))
	for i, v := range ints {
		if v < 0 {
			v = 0
		}
		floats[i] = float64(v)
	}
	return floats
}
