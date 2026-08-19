package single

import (
	"fmt"
	"image"
	"math"

	"github.com/edsilegx/ctop/cwidgets"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

type Mem struct {
	ui.Block
	valHist *IntHist
	limit   int
	val     int
}

func NewMem() *Mem {
	mem := &Mem{
		Block:   *ui.NewBlock(),
		valHist: NewIntHist(9),
	}
	mem.Title = "MEM"
	mem.BorderStyle = theme.Style("border.fg")
	mem.TitleStyle = theme.Style("label.fg")
	mem.SetRect(0, 0, colWidth[0], 12)
	return mem
}

func (w *Mem) Align() {
	innerW := (w.Max.X - w.Min.X) - 4
	if innerW <= 0 {
		return
	}
	barWidth := 6
	barGap := 2
	numBars := (innerW + barGap) / (barWidth + barGap)
	if numBars < 5 {
		numBars = 5
	}
	w.valHist.SetLimit(numBars)
}

func (w *Mem) Update(val int, limit int) {
	w.val = val
	w.limit = limit
	w.valHist.Append(val)
}

func (w *Mem) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	// Top Label: e.g. "864K / 7G"
	label := fmt.Sprintf("%v / %v", cwidgets.ByteFormatShort(w.val), cwidgets.ByteFormatShort(w.limit))
	buf.SetString(label, theme.Style("par.text.fg"), image.Pt(w.Min.X+2, w.Min.Y+1))

	innerW := (w.Max.X - w.Min.X) - 4
	innerH := (w.Max.Y - w.Min.Y) - 2
	if innerW <= 10 || innerH <= 3 {
		return
	}

	// Calculate scale: container memory limit or max in history
	maxVal := w.limit
	if maxVal <= 0 {
		for _, v := range w.valHist.Data {
			if v > maxVal {
				maxVal = v
			}
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	labelY := w.Max.Y - 2
	chartBottomY := labelY - 1
	chartTopY := w.Min.Y + 3
	chartH := chartBottomY - chartTopY + 1
	if chartH <= 0 {
		return
	}

	numBars := len(w.valHist.Data)
	if numBars == 0 {
		return
	}

	barWidth := 6
	barGap := 2
	totalNeeded := numBars*barWidth + (numBars-1)*barGap
	if totalNeeded > innerW {
		barGap = 1
		totalNeeded = numBars*barWidth + (numBars-1)*barGap
		if totalNeeded > innerW {
			barWidth = (innerW - (numBars-1)*barGap) / numBars
			if barWidth < 3 {
				barWidth = 3
			}
		}
	}

	startX := w.Min.X + 2

	barColor := theme.Color("mbarchart.bar.bg")
	if w.limit > 0 && float64(w.val)/float64(w.limit) > 0.9 {
		barColor = theme.Color("status.danger")
	} else if w.limit > 0 && float64(w.val)/float64(w.limit) > 0.75 {
		barColor = theme.Color("status.warn")
	}

	barStyle := ui.NewStyle(barColor, theme.Color("bg"))
	labelStyle := theme.Style("par.text.fg")

	for i, val := range w.valHist.Data {
		bx := startX + i*(barWidth+barGap)
		if bx+barWidth > w.Max.X-1 {
			break
		}

		var bHeight int
		if maxVal > 0 && val > 0 {
			bHeight = int(math.Round(float64(val) / float64(maxVal) * float64(chartH)))
			if bHeight < 1 {
				bHeight = 1
			}
			if bHeight > chartH {
				bHeight = chartH
			}
		}

		for by := chartBottomY; by >= chartBottomY-bHeight+1 && by >= chartTopY; by-- {
			for x := bx; x < bx+barWidth; x++ {
				buf.SetCell(ui.NewCell('█', barStyle), image.Pt(x, by))
			}
		}

		lbl := cwidgets.ByteFormatShort(val)
		if len(lbl) > barWidth {
			lbl = lbl[:barWidth]
		}
		buf.SetString(lbl, labelStyle, image.Pt(bx, labelY))
	}
}
