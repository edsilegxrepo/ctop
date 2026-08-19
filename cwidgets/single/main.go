package single

import (
	"image"

	"github.com/edsilegx/ctop/logging"
	"github.com/edsilegx/ctop/models"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
	tb "github.com/nsf/termbox-go"
)

var (
	log      = logging.Init()
	colWidth = [2]int{65, 0} // left,right column width
)

type Single struct {
	ui.Block
	Info  *Info
	Net   *Net
	Cpu   *Cpu
	Mem   *Mem
	IO    *IO
	Env   *Env
	Y     int
	Width int
}

func NewSingle() *Single {
	termW, termH := theme.TermDimensions()
	s := &Single{
		Block: *ui.NewBlock(),
		Info:  NewInfo(),
		Net:   NewNet(),
		Cpu:   NewCpu(),
		Mem:   NewMem(),
		IO:    NewIO(),
		Env:   NewEnv(),
		Width: termW,
	}
	s.Border = false
	s.SetRect(0, 0, termW, termH)
	return s
}

func (e *Single) Up() {
	if e.Y < 0 {
		e.Y++
		e.Align()
		if tb.IsInit {
			ui.Render(e)
		}
	}
}

func (e *Single) Down() {
	_, termH := theme.TermDimensions()
	if e.Y > (termH - e.GetHeight()) {
		e.Y--
		e.Align()
		if tb.IsInit {
			ui.Render(e)
		}
	}
}

func (e *Single) SetWidth(w int) { e.Width = w }

func (e *Single) SetMeta(m models.Meta) {
	for k, v := range m {
		if k == "[ENV-VAR]" {
			e.Env.Set(v)
		} else {
			e.Info.Set(k, v)
		}
	}
}

func (e *Single) SetMetrics(m models.Metrics) {
	e.Cpu.Update(m.CPUUtil)
	e.Net.Update(m.NetRx, m.NetTx)
	e.Mem.Update(int(m.MemUsage), int(m.MemLimit))
	e.IO.Update(m.IOBytesRead, m.IOBytesWrite)
}

// GetHeight returns total column height
func (e *Single) GetHeight() (h int) {
	h += e.Info.GetHeight()
	h += 12 // Cpu
	h += 12 // Mem
	h += 12 // Net
	h += 12 // IO
	h += e.Env.GetHeight()
	return h
}

func (e *Single) Align() {
	termW, termH := theme.TermDimensions()
	e.SetRect(0, 0, termW, termH)

	if e.GetHeight() <= termH {
		e.Y = 0
	}

	y := e.Y
	leftW := termW

	// Info
	infoH := e.Info.GetHeight()
	e.Info.SetRect(0, y, leftW, y+infoH)
	y += infoH

	// Cpu
	e.Cpu.SetRect(0, y, leftW, y+12)
	e.Cpu.Align()
	y += 12

	// Mem
	e.Mem.SetRect(0, y, leftW, y+12)
	e.Mem.Align()
	y += 12

	// Net
	e.Net.SetRect(0, y, leftW, y+12)
	e.Net.Align()
	y += 12

	// IO
	e.IO.SetRect(0, y, leftW, y+12)
	e.IO.Align()
	y += 12

	// Env
	envH := e.Env.GetHeight()
	e.Env.SetRect(0, y, leftW, y+envH)
}

func (e *Single) Draw(buf *ui.Buffer) {
	e.Block.Draw(buf)
	termW, _ := theme.TermDimensions()
	if termW < 30 {
		buf.SetString("screen too small!", theme.Style("status.danger"), image.Pt(1, 1))
		return
	}

	e.Info.Draw(buf)
	e.Cpu.Draw(buf)
	e.Mem.Draw(buf)
	e.Net.Draw(buf)
	e.IO.Draw(buf)
	e.Env.Draw(buf)
}
