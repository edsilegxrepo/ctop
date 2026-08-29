package compact

import (
	"fmt"
	"image"

	"github.com/edsilegx/ctop/internal/cwidgets"
	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/models"
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

func NewHostCol() CompactCol {
	return &MetaCol{NewTextCol("HOST"), "host"}
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
	return &NetCol{NewTextCol("NET (Rx / Tx)")}
}

func (w *NetCol) Header() string {
	return "NET (Rx / Tx)"
}

func (w *NetCol) SetMetrics(m models.Metrics) {
	if config.GetSwitchVal("rateMode") {
		if m.NetRxRate >= 0 || m.NetTxRate >= 0 {
			rxRate := m.NetRxRate
			if rxRate < 0 {
				rxRate = 0
			}
			txRate := m.NetTxRate
			if txRate < 0 {
				txRate = 0
			}
			label := fmt.Sprintf("%s/s / %s/s", cwidgets.ByteFormat64Short(rxRate), cwidgets.ByteFormat64Short(txRate))
			w.setText(label)
			return
		}
	}
	if m.NetRx >= 0 || m.NetTx >= 0 {
		rx := m.NetRx
		if rx < 0 {
			rx = 0
		}
		tx := m.NetTx
		if tx < 0 {
			tx = 0
		}
		label := fmt.Sprintf("%s / %s", cwidgets.ByteFormat64Short(rx), cwidgets.ByteFormat64Short(tx))
		w.setText(label)
		return
	}
	w.setText("-")
}

type IOCol struct {
	*TextCol
}

func NewIOCol() CompactCol {
	return &IOCol{NewTextCol("IO (Reads / Writes)")}
}

func (w *IOCol) Header() string {
	return "IO (Reads / Writes)"
}

func (w *IOCol) SetMetrics(m models.Metrics) {
	if config.GetSwitchVal("rateMode") {
		if m.IORateRead >= 0 || m.IORateWrite >= 0 {
			readRate := m.IORateRead
			if readRate < 0 {
				readRate = 0
			}
			writeRate := m.IORateWrite
			if writeRate < 0 {
				writeRate = 0
			}
			label := fmt.Sprintf("%s/s / %s/s", cwidgets.ByteFormat64Short(readRate), cwidgets.ByteFormat64Short(writeRate))
			w.setText(label)
			return
		}
	}
	if m.IOBytesRead >= 0 || m.IOBytesWrite >= 0 {
		read := m.IOBytesRead
		if read < 0 {
			read = 0
		}
		write := m.IOBytesWrite
		if write < 0 {
			write = 0
		}
		label := fmt.Sprintf("%s / %s", cwidgets.ByteFormat64Short(read), cwidgets.ByteFormat64Short(write))
		w.setText(label)
		return
	}
	w.setText("-")
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
	height := w.Max.Y - w.Min.Y
	if width <= 0 || height <= 0 {
		return
	}

	if w.highlighted {
		hiCell := ui.NewCell(' ', ui.NewStyle(theme.Color("cursor.fg"), theme.Color("cursor.bg")))
		buf.Fill(hiCell, w.Rectangle)
	}

	s := w.Text
	if len(s) > width {
		s = s[:width]
	}

	y := w.Min.Y + (height-1)/2
	pt := image.Pt(w.Min.X, y)
	if w.rightAlign && len(s) < width {
		pt = image.Pt(w.Min.X+(width-len(s)), y)
	}

	buf.SetString(s, w.TextStyle, pt)
}

func (w *TextCol) SetX(x int) {
	w.SetRect(x, w.Min.Y, x+(w.Max.X-w.Min.X), w.Max.Y)
}

func (w *TextCol) SetY(y int) {
	w.SetRect(w.Min.X, y, w.Max.X, y+(w.Max.Y-w.Min.Y))
}

func (w *TextCol) SetWidth(width int) {
	w.SetRect(w.Min.X, w.Min.Y, w.Min.X+width, w.Max.Y)
}

func (w *TextCol) Highlight() {
	w.highlighted = true
	w.TextStyle = ui.NewStyle(theme.Color("cursor.fg"), theme.Color("cursor.bg"), ui.ModifierBold)
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
