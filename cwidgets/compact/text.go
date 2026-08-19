package compact

import (
	"fmt"
	"image"

	"github.com/edsilegx/ctop/cwidgets"
	"github.com/edsilegx/ctop/models"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

// Column that shows container's meta property i.e. name, id, image etc.
type MetaCol struct {
	*TextCol
	metaName string
}

func (w *MetaCol) SetMeta(m models.Meta) {
	w.setText(m.Get(w.metaName))
}

func NewNameCol() CompactCol {
	return &MetaCol{NewTextCol("NAME"), "name"}
}

func NewCIDCol() CompactCol {
	c := &MetaCol{NewTextCol("CID"), "id"}
	c.fWidth = 12
	return c
}

func NewImageCol() CompactCol {
	return &MetaCol{NewTextCol("IMAGE"), "image"}
}

func NewPortsCol() CompactCol {
	return &MetaCol{NewTextCol("PORTS"), "ports"}
}

func NewIpsCol() CompactCol {
	return &MetaCol{NewTextCol("IPs"), "IPs"}
}

func NewCreatedCol() CompactCol {
	c := &MetaCol{NewTextCol("CREATED"), "created"}
	c.fWidth = 19
	return c
}

type NetCol struct {
	*TextCol
}

func NewNetCol() CompactCol {
	return &NetCol{NewTextCol("NET RX/TX")}
}

func (w *NetCol) SetMetrics(m models.Metrics) {
	label := fmt.Sprintf("%s / %s", cwidgets.ByteFormat64Short(m.NetRx), cwidgets.ByteFormat64Short(m.NetTx))
	w.setText(label)
}

type IOCol struct {
	*TextCol
}

func NewIOCol() CompactCol {
	return &IOCol{NewTextCol("IO R/W")}
}

func (w *IOCol) SetMetrics(m models.Metrics) {
	label := fmt.Sprintf("%s / %s", cwidgets.ByteFormat64Short(m.IOBytesRead), cwidgets.ByteFormat64Short(m.IOBytesWrite))
	w.setText(label)
}

type PIDCol struct {
	*TextCol
}

func NewPIDCol() CompactCol {
	w := &PIDCol{NewTextCol("PIDS")}
	w.fWidth = 5
	w.rightAlign = true
	return w
}

func (w *PIDCol) SetMetrics(m models.Metrics) {
	w.setText(fmt.Sprintf("%d", m.Pids))
}

type UptimeCol struct {
	*TextCol
}

func NewUptimeCol() CompactCol {
	return &UptimeCol{NewTextCol("UPTIME")}
}

func (w *UptimeCol) SetMeta(m models.Meta) {
	w.setText(m.Get("uptime"))
}

type TextCol struct {
	ui.Block
	Text        string
	header      string
	fWidth      int
	TextStyle   ui.Style
	highlighted bool
	rightAlign  bool
}

func NewTextCol(header string) *TextCol {
	t := &TextCol{
		Block:     *ui.NewBlock(),
		Text:      "-",
		header:    header,
		fWidth:    0,
		TextStyle: theme.Style2("par.text.fg", "par.text.bg"),
	}
	t.Border = false
	return t
}

func (w *TextCol) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)
	width := w.Max.X - w.Min.X
	if width <= 0 {
		return
	}

	if w.highlighted {
		hiCell := ui.NewCell(' ', ui.NewStyle(ui.ColorBlack, ui.ColorWhite))
		buf.Fill(hiCell, w.Rectangle)
	}

	s := w.Text
	if len(s) > width {
		s = s[:width]
	}

	pt := image.Pt(w.Min.X, w.Min.Y)
	if w.rightAlign && len(s) < width {
		pt = image.Pt(w.Min.X+(width-len(s)), w.Min.Y)
	}

	buf.SetString(s, w.TextStyle, pt)
}

func (w *TextCol) SetX(x int) {
	w.SetRect(x, w.Min.Y, x+(w.Max.X-w.Min.X), w.Min.Y+1)
}

func (w *TextCol) SetY(y int) {
	w.SetRect(w.Min.X, y, w.Max.X, y+1)
}

func (w *TextCol) SetWidth(width int) {
	w.SetRect(w.Min.X, w.Min.Y, w.Min.X+width, w.Min.Y+1)
}

func (w *TextCol) Highlight() {
	w.highlighted = true
	w.TextStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
}

func (w *TextCol) UnHighlight() {
	w.highlighted = false
	w.TextStyle = theme.Style2("par.text.fg", "par.text.bg")
}

// TextCol implements CompactCol
func (w *TextCol) Reset()                    { w.setText("-") }
func (w *TextCol) SetMeta(models.Meta)       {}
func (w *TextCol) SetMetrics(models.Metrics) {}
func (w *TextCol) Header() string            { return w.header }
func (w *TextCol) FixedWidth() int           { return w.fWidth }

func (w *TextCol) setText(s string) {
	if s == "" {
		s = "-"
	}
	if w.fWidth > 0 && len(s) > w.fWidth {
		s = s[0:w.fWidth]
	}
	w.Text = s
}
