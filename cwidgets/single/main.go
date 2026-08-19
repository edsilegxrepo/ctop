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

// Single manages the multi-tab container detailed inspection view.
type Single struct {
	ui.Block
	TabBar    *TabBar
	Info      *Info
	Net       *Net
	Cpu       *Cpu
	Mem       *Mem
	IO        *IO
	Env       *Env
	Mounts    *Mounts
	Network   *Network
	Process   *Process
	Labels    *Labels
	ActiveTab int
	Y         int
	Width     int
}

// NewSingle constructs a new multi-tab container inspection view.
func NewSingle() *Single {
	termW, termH := theme.TermDimensions()
	s := &Single{
		Block:     *ui.NewBlock(),
		TabBar:    NewTabBar(),
		Info:      NewInfo(),
		Net:       NewNet(),
		Cpu:       NewCpu(),
		Mem:       NewMem(),
		IO:        NewIO(),
		Env:       NewEnv(),
		Mounts:    NewMounts(),
		Network:   NewNetwork(),
		Process:   NewProcess(),
		Labels:    NewLabels(),
		ActiveTab: TabMetrics,
		Width:     termW,
	}
	s.Border = false
	s.SetRect(0, 0, termW, termH)
	return s
}

// SetTab switches active tab to the specified index.
func (e *Single) SetTab(tab int) {
	if tab >= 0 && tab < TotalTabs {
		e.ActiveTab = tab
		e.TabBar.ActiveTab = tab
		e.Y = 0
		e.Align()
		if tb.IsInit {
			ui.Render(e)
		}
	}
}

// NextTab advances to the next tab.
func (e *Single) NextTab() {
	next := (e.ActiveTab + 1) % TotalTabs
	e.SetTab(next)
}

// PrevTab switches to the previous tab.
func (e *Single) PrevTab() {
	prev := (e.ActiveTab - 1 + TotalTabs) % TotalTabs
	e.SetTab(prev)
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

// SetMeta dispatches container metadata across all inspection sub-widgets.
func (e *Single) SetMeta(m models.Meta) {
	// 1. Process & Runtime fields
	for _, k := range displayProcess {
		if v, ok := m[k]; ok {
			e.Process.Set(k, v)
		}
	}

	// 2. Specialized widgets
	if envStr, ok := m["[ENV-VAR]"]; ok {
		e.Env.Set(envStr)
	}
	if mountStr, ok := m["[MOUNTS]"]; ok {
		e.Mounts.Set(mountStr)
	}
	if labelStr, ok := m["[LABELS]"]; ok {
		e.Labels.Set(labelStr)
	}
	e.Network.Set(m["[NETWORKS]"], m["ports"], m["IPs"])

	// 3. Info header fields
	for k, v := range m {
		if k != "[ENV-VAR]" && k != "[MOUNTS]" && k != "[LABELS]" && k != "[NETWORKS]" {
			e.Info.Set(k, v)
		}
	}
}

// SetMetrics updates real-time telemetry sparklines.
func (e *Single) SetMetrics(m models.Metrics) {
	e.Cpu.Update(m.CPUUtil)
	e.Net.Update(m.NetRx, m.NetTx)
	e.Mem.Update(int(m.MemUsage), int(m.MemLimit))
	e.IO.Update(m.IOBytesRead, m.IOBytesWrite)
}

// GetHeight returns total scrollable height for the currently active tab.
func (e *Single) GetHeight() (h int) {
	h = 2 // Tab bar line + gap
	switch e.ActiveTab {
	case TabMetrics:
		h += e.Info.GetHeight()
		h += 12 // Cpu
		h += 12 // Mem
		h += 12 // Net
		h += 12 // IO
	case TabVolumes:
		h += e.Mounts.GetHeight()
	case TabNetwork:
		h += e.Network.GetHeight()
	case TabProcess:
		h += e.Process.GetHeight()
		h += e.Env.GetHeight()
	case TabLabels:
		h += e.Labels.GetHeight()
	}
	return h
}

// Align positions active widgets according to current terminal dimensions and scroll offset.
func (e *Single) Align() {
	termW, termH := theme.TermDimensions()
	e.SetRect(0, 0, termW, termH)

	if e.GetHeight() <= termH {
		e.Y = 0
	}

	// Tab Bar stays fixed at the top
	e.TabBar.SetRect(0, 0, termW, 1)

	y := e.Y + 2 // start content below tab bar
	leftW := termW

	switch e.ActiveTab {
	case TabMetrics:
		infoH := e.Info.GetHeight()
		e.Info.SetRect(0, y, leftW, y+infoH)
		y += infoH

		e.Cpu.SetRect(0, y, leftW, y+12)
		e.Cpu.Align()
		y += 12

		e.Mem.SetRect(0, y, leftW, y+12)
		e.Mem.Align()
		y += 12

		e.Net.SetRect(0, y, leftW, y+12)
		e.Net.Align()
		y += 12

		e.IO.SetRect(0, y, leftW, y+12)
		e.IO.Align()

	case TabVolumes:
		mountsH := e.Mounts.GetHeight()
		e.Mounts.SetRect(0, y, leftW, y+mountsH)

	case TabNetwork:
		netH := e.Network.GetHeight()
		e.Network.SetRect(0, y, leftW, y+netH)

	case TabProcess:
		procH := e.Process.GetHeight()
		e.Process.SetRect(0, y, leftW, y+procH)
		y += procH

		envH := e.Env.GetHeight()
		e.Env.SetRect(0, y, leftW, y+envH)

	case TabLabels:
		labelsH := e.Labels.GetHeight()
		e.Labels.SetRect(0, y, leftW, y+labelsH)
	}
}

// Draw renders the TabBar and the active tab's widgets onto the buffer.
func (e *Single) Draw(buf *ui.Buffer) {
	e.Block.Draw(buf)
	termW, _ := theme.TermDimensions()
	if termW < 30 {
		buf.SetString("screen too small!", theme.Style("status.danger"), image.Pt(1, 1))
		return
	}

	// Always draw tab bar
	e.TabBar.Draw(buf)

	// Draw active tab widgets
	switch e.ActiveTab {
	case TabMetrics:
		e.Info.Draw(buf)
		e.Cpu.Draw(buf)
		e.Mem.Draw(buf)
		e.Net.Draw(buf)
		e.IO.Draw(buf)
	case TabVolumes:
		e.Mounts.Draw(buf)
	case TabNetwork:
		e.Network.Draw(buf)
	case TabProcess:
		e.Process.Draw(buf)
		e.Env.Draw(buf)
	case TabLabels:
		e.Labels.Draw(buf)
	}
}
