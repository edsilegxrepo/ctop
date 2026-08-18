package single

import (
	"github.com/bcicen/ctop/theme"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type Cpu struct {
	*widgets.Plot
	hist FloatHist
}

func NewCpu() *Cpu {
	p := widgets.NewPlot()
	p.Title = "CPU"
	p.Marker = widgets.MarkerDot
	p.MaxVal = 100
	p.SetRect(0, 0, colWidth[0], 12)
	p.LineColors = []ui.Color{theme.Color("status.ok")}
	p.BorderStyle = theme.Style("border.fg")
	p.TitleStyle = theme.Style("label.fg")

	cpu := &Cpu{p, NewFloatHist(colWidth[0] - 10)}
	cpu.Data = [][]float64{cpu.hist.Data}
	return cpu
}

func (w *Cpu) Align() {
	plotW := (w.Max.X - w.Min.X) - 10
	if plotW < 10 {
		plotW = 10
	}
	w.hist.SetLimit(plotW)
	w.Data = [][]float64{w.hist.Data}
}

func (w *Cpu) Update(val int) {
	w.hist.Append(float64(val))
	w.Data = [][]float64{w.hist.Data}
}
