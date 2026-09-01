// Package single implements the full-screen, multi-tab container inspection view.
//
// main.go orchestrates sub-widget lifecycles, tab navigation, layout alignment, and telemetry dispatch.
//
// Objective:
//
//	Provide an in-depth, multi-dimensional terminal inspector displaying real-time graphs, logs, mounts,
//	networking, process trees, images, filesystem diffs, generator templates, and in-container file explorers.
//
// Core Components:
//   - Single: Parent TermUI widget managing 11 child inspection sub-widgets and the TabBar header.
//   - Sub-widgets: Cpu, Mem, IO, Net, Env, Mounts, Network, Image, Logs, Process, Top, Diff, Generator, Labels, Explorer.
//
// Functionality:
//   - Implements cwidgets.WidgetUpdater to receive streaming container telemetry.
//   - Tab switching, keyboard navigation, vertical scrolling, and secret masking.
//
// Data Flow:
//
//	Container Telemetry -> Single.SetMetrics() / SetMeta() -> Sub-Widgets -> TermUI Draw Buffer.
package single

import (
	"image"
	"sync"

	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/models"
	ui "github.com/gizak/termui/v3"
)

var colWidth = [2]int{65, 0} // left,right column width

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
	Image     *Image
	Logs      *Logs
	Process   *Process
	Top       *Top
	Diff      *Diff
	Generator *Generator
	Labels    *Labels
	Explorer  *Explorer
	ActiveTab int
	Y         int
	Width     int
	mu        sync.Mutex
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
		Image:     NewImage(),
		Logs:      NewLogs(),
		Process:   NewProcess(),
		Top:       NewTop(),
		Diff:      NewDiff(),
		Generator: NewGenerator(),
		Labels:    NewLabels(),
		Explorer:  NewExplorer(),
		ActiveTab: TabMetrics,
		Width:     termW,
	}
	s.Border = false
	s.SetRect(0, 0, termW, termH)
	return s
}

// SetTab switches active tab to the specified index.
func (e *Single) SetTab(tab int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if tab >= 0 && tab < TotalTabs {
		e.ActiveTab = tab
		e.TabBar.ActiveTab = tab
		e.Y = 0
		e.Image.Offset = 0
		e.Generator.Offset = 0
		e.alignUnsafe()
	}
}

// NextTab advances to the next tab.
func (e *Single) NextTab() {
	e.mu.Lock()
	next := (e.ActiveTab + 1) % TotalTabs
	e.mu.Unlock()
	e.SetTab(next)
}

// PrevTab switches to the previous tab.
func (e *Single) PrevTab() {
	e.mu.Lock()
	prev := (e.ActiveTab - 1 + TotalTabs) % TotalTabs
	e.mu.Unlock()
	e.SetTab(prev)
}

func (e *Single) Up() {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.ActiveTab {
	case TabFiles:
		e.Explorer.Up()
		return
	case TabLogs:
		e.Logs.Up()
		return
	case TabImage:
		e.Image.Up()
		return
	case TabGenerator:
		e.Generator.Up()
		return
	}
	if e.Y < 0 {
		e.Y += 2
		if e.Y > 0 {
			e.Y = 0
		}
		e.alignUnsafe()
	}
}

func (e *Single) Down() {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.ActiveTab {
	case TabFiles:
		e.Explorer.Down()
		return
	case TabLogs:
		e.Logs.Down()
		return
	case TabImage:
		e.Image.Down()
		return
	case TabGenerator:
		e.Generator.Down()
		return
	}
	_, termH := theme.TermDimensions()
	limit := termH - e.getHeightUnsafe()
	if e.Y > limit {
		e.Y -= 2
		if e.Y < limit {
			e.Y = limit
		}
		e.alignUnsafe()
	}
}

func (e *Single) PgUp() {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.ActiveTab {
	case TabFiles:
		e.Explorer.PgUp(15)
		return
	case TabLogs:
		e.Logs.PgUp()
		return
	case TabImage:
		e.Image.PgUp()
		return
	case TabGenerator:
		e.Generator.PgUp()
		return
	}
	_, termH := theme.TermDimensions()
	e.Y += termH / 2
	if e.Y > 0 {
		e.Y = 0
	}
	e.alignUnsafe()
}

func (e *Single) PgDown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.ActiveTab {
	case TabFiles:
		e.Explorer.PgDown(15)
		return
	case TabLogs:
		e.Logs.PgDown()
		return
	case TabImage:
		e.Image.PgDown()
		return
	case TabGenerator:
		e.Generator.PgDown()
		return
	}
	_, termH := theme.TermDimensions()
	limit := termH - e.getHeightUnsafe()
	e.Y -= termH / 2
	if e.Y < limit {
		e.Y = limit
	}
	e.alignUnsafe()
}

func (e *Single) Home() {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.ActiveTab {
	case TabFiles:
		e.Explorer.Home()
		return
	case TabLogs:
		e.Logs.ScrollTop()
		return
	}
	e.Y = 0
	e.alignUnsafe()
}

func (e *Single) End() {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.ActiveTab {
	case TabFiles:
		e.Explorer.End()
		return
	case TabLogs:
		e.Logs.ScrollBottom()
		return
	}
	_, termH := theme.TermDimensions()
	limit := termH - e.getHeightUnsafe()
	e.Y = limit
	e.alignUnsafe()
}

func (e *Single) SetWidth(w int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Width = w
}

func (e *Single) ToggleSecretMask() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Env.ToggleMask()
	e.alignUnsafe()
}

func (e *Single) SetTop(res models.TopResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Top.Set(res)
	e.alignUnsafe()
}

func (e *Single) SetDiff(changes []models.Change) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Diff.Set(changes)
	e.alignUnsafe()
}

func (e *Single) SetGenerator(runCmd, compose string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Generator.Set(runCmd, compose)
	e.alignUnsafe()
}

func (e *Single) SetExplorer(dirPath string, entries []models.FileInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Explorer.Set(dirPath, entries)
	e.alignUnsafe()
}

func (e *Single) RunNetworkProbes() {
	e.Network.RunProbes()
}

func (e *Single) StopNetworkProbes() {
	e.Network.StopProbes()
}

// SetMeta dispatches container metadata across all inspection sub-widgets.
func (e *Single) SetMeta(m models.Meta) {
	e.mu.Lock()
	defer e.mu.Unlock()
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
	e.Image.Set(m)
	if name, ok := m["name"]; ok {
		e.Logs.SetContainerName(name)
	}

	// 3. Info header fields
	for k, v := range m {
		if k != "[ENV-VAR]" && k != "[MOUNTS]" && k != "[LABELS]" && k != "[NETWORKS]" {
			e.Info.Set(k, v)
		}
	}
}

// SetMetrics updates real-time telemetry sparklines.
func (e *Single) SetMetrics(m models.Metrics) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Cpu.Update(m.CPUUtil)
	e.Net.Update(m.NetRx, m.NetTx)
	e.Mem.Update(int(m.MemUsage), int(m.MemLimit))
	e.IO.Update(m.IOBytesRead, m.IOBytesWrite)
}

// GetHeight returns total scrollable height for the currently active tab.
func (e *Single) GetHeight() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.getHeightUnsafe()
}

func (e *Single) getHeightUnsafe() (h int) {
	h = 2 // Tab bar line + gap
	switch e.ActiveTab {
	case TabMetrics:
		h += e.Info.GetHeight()
		h += 12 // Cpu
		h += 12 // Mem
		h += 12 // Net
		h += 12 // IO
	case TabLogs:
		h += e.Logs.GetHeight()
	case TabVolumes:
		h += e.Mounts.GetHeight()
	case TabNetwork:
		h += e.Network.GetHeight()
	case TabProcess:
		h += e.Process.GetHeight()
		h += e.Env.GetHeight()
	case TabImage:
		h += e.Image.GetHeight()
	case TabTop:
		h += e.Top.GetHeight()
	case TabDiff:
		h += e.Diff.GetHeight()
	case TabGenerator:
		h += e.Generator.GetHeight()
	case TabLabels:
		h += e.Labels.GetHeight()
	case TabFiles:
		h += e.Explorer.GetHeight()
	}
	return h
}

// Align positions active widgets according to current terminal dimensions and scroll offset.
func (e *Single) Align() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.alignUnsafe()
}

func (e *Single) alignUnsafe() {
	termW, termH := theme.TermDimensions()
	e.SetRect(0, 0, termW, termH)

	if e.getHeightUnsafe() <= termH {
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

	case TabLogs:
		_, termH := theme.TermDimensions()
		expH := termH - y
		if expH < 6 {
			expH = 6
		}
		e.Logs.SetRect(0, y, leftW, y+expH)

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

	case TabImage:
		_, termH := theme.TermDimensions()
		expH := termH - y
		if expH < 6 {
			expH = 6
		}
		e.Image.SetRect(0, y, leftW, y+expH)

	case TabTop:
		topH := e.Top.GetHeight()
		e.Top.SetRect(0, y, leftW, y+topH)

	case TabDiff:
		diffH := e.Diff.GetHeight()
		e.Diff.SetRect(0, y, leftW, y+diffH)

	case TabGenerator:
		_, termH := theme.TermDimensions()
		expH := termH - y
		if expH < 6 {
			expH = 6
		}
		e.Generator.SetRect(0, y, leftW, y+expH)

	case TabLabels:
		labelsH := e.Labels.GetHeight()
		e.Labels.SetRect(0, y, leftW, y+labelsH)

	case TabFiles:
		_, termH := theme.TermDimensions()
		expH := termH - y
		if expH < 6 {
			expH = 6
		}
		e.Explorer.SetRect(0, y, leftW, y+expH)
	}
}

// Draw renders the TabBar and the active tab's widgets onto the buffer.
func (e *Single) Draw(buf *ui.Buffer) {
	e.mu.Lock()
	defer e.mu.Unlock()
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
	case TabLogs:
		e.Logs.Draw(buf)
	case TabVolumes:
		e.Mounts.Draw(buf)
	case TabNetwork:
		e.Network.Draw(buf)
	case TabProcess:
		e.Process.Draw(buf)
		e.Env.Draw(buf)
	case TabImage:
		e.Image.Draw(buf)
	case TabTop:
		e.Top.Draw(buf)
	case TabDiff:
		e.Diff.Draw(buf)
	case TabGenerator:
		e.Generator.Draw(buf)
	case TabLabels:
		e.Labels.Draw(buf)
	case TabFiles:
		e.Explorer.Draw(buf)
	}
}
