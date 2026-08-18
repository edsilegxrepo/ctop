package compact

import (
	"fmt"
	"image"

	"github.com/bcicen/ctop/cwidgets"
	"github.com/bcicen/ctop/models"
	"github.com/bcicen/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

type CPUCol struct {
	*GaugeCol
	scaleCpu bool
}

func NewCPUCol() CompactCol {
	c := &CPUCol{NewGaugeCol("CPU"), false}
	c.rightAlign = true
	return c
}

func NewCpuScaledCol() CompactCol {
	c := &CPUCol{NewGaugeCol("CPUS"), true}
	c.rightAlign = true
	return c
}

func (w *CPUCol) SetMetrics(m models.Metrics) {
	val := m.CPUUtil
	w.BarColor = colorScale(val)
	if !w.scaleCpu {
		val = val * int(m.NCpus)
	}
	w.Label = fmt.Sprintf("%d%%", val)

	if val > 100 {
		val = 100
	}
	w.Percent = val
}

type MemCol struct {
	*GaugeCol
}

func NewMemCol() CompactCol {
	return &MemCol{NewGaugeCol("MEM")}
}

func (w *MemCol) SetMetrics(m models.Metrics) {
	w.BarColor = theme.Color("gauge.bar.bg")
	w.Label = fmt.Sprintf("%s / %s", cwidgets.ByteFormat64Short(m.MemUsage), cwidgets.ByteFormat64Short(m.MemLimit))
	w.Percent = m.MemPercent
}

type GaugeCol struct {
	ui.Block
	Percent     int
	Label       string
	header      string
	fWidth      int
	BarColor    ui.Color
	LabelStyle  ui.Style
	highlighted bool
	rightAlign  bool
}

func NewGaugeCol(header string) *GaugeCol {
	g := &GaugeCol{
		Block:      *ui.NewBlock(),
		header:     header,
		fWidth:     0,
		BarColor:   theme.Color("gauge.bar.bg"),
		LabelStyle: theme.Style2("par.text.fg", "par.text.bg"),
	}
	g.Border = false
	g.Reset()
	return g
}

func (w *GaugeCol) Reset() {
	w.Label = "-"
	w.Percent = 0
}

func (w *GaugeCol) Highlight() {
	w.highlighted = true
	w.LabelStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
}

func (w *GaugeCol) UnHighlight() {
	w.highlighted = false
	w.LabelStyle = theme.Style2("par.text.fg", "par.text.bg")
}

func (w *GaugeCol) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	width := w.Max.X - w.Min.X
	if width <= 0 {
		return
	}

	if w.highlighted {
		hiCell := ui.NewCell(' ', ui.NewStyle(ui.ColorBlack, ui.ColorWhite))
		buf.Fill(hiCell, w.Rectangle)
	}

	label := w.Label
	if len(label) > width {
		label = label[:width]
	}

	pt := image.Pt(w.Min.X, w.Min.Y)
	if w.rightAlign && len(label) < width {
		pt = image.Pt(w.Min.X+(width-len(label)), w.Min.Y)
	}

	buf.SetString(label, w.LabelStyle, pt)
}

func (w *GaugeCol) SetX(x int) {
	w.SetRect(x, w.Min.Y, x+(w.Max.X-w.Min.X), w.Min.Y+1)
}

func (w *GaugeCol) SetY(y int) {
	w.SetRect(w.Min.X, y, w.Max.X, y+1)
}

func (w *GaugeCol) SetWidth(width int) {
	w.SetRect(w.Min.X, w.Min.Y, w.Min.X+width, w.Min.Y+1)
}

// GaugeCol implements CompactCol
func (w *GaugeCol) SetMeta(models.Meta)       {}
func (w *GaugeCol) SetMetrics(models.Metrics) {}
func (w *GaugeCol) Header() string            { return w.header }
func (w *GaugeCol) FixedWidth() int           { return w.fWidth }

func colorScale(n int) ui.Color {
	if n <= 70 {
		return theme.Color("status.ok")
	}
	if n <= 90 {
		return theme.Color("status.warn")
	}
	return theme.Color("status.danger")
}
