// menus.go provides modal dialogs and interactive menus for Help, Filters, Sorting, Columns, Logs, Container Actions, and Shell execution.
//
// Objective:
//
//	Provide interactive modal overlays for runtime configuration, container control operations
//	(start, stop, pause, kill, remove, restart, resource hot-tuning), in-container shell spawning,
//	log tailing with search/export, and column/theme customizations.
//
// Core Components:
//   - MenuFn: Recursive closure signature returning the next modal window or nil upon dismissal.
//   - HelpMenu / FilterMenu / SortMenu / ColumnsMenu: Configuration & layout modal dialogs.
//   - ContainerMenu / SignalMenu / ResourceMenu / FileExplorerMenu: Lifecycle & container administration modals.
//   - ExecShell: Interactive pseudoterminal launcher dropping user into in-container bash/sh.
//
// Functionality:
//   - Modal navigation (j/k, Up/Down, Enter, Esc).
//   - Container lifecycle invocation via container.Manager interfaces.
//   - Live resource hot-tuning (Memory limits, CPU quotas, Restart policies).
//   - Diagnostic report generation and web browser port launcher.
//
// Data Flow:
//
//	User Keystroke -> Menu Event Loop -> Container Manager / Config Engine -> Terminal Modal Redraw.
package main

import (
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/edsilegx/ctop/internal/cwidgets"
	"github.com/edsilegx/ctop/internal/cwidgets/single"
	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/internal/widgets"
	"github.com/edsilegx/ctop/internal/widgets/menu"
	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/container"
	"github.com/edsilegx/ctop/pkg/diag"
	ui "github.com/gizak/termui/v3"
	tb "github.com/nsf/termbox-go"
)

// MenuFn executes a menu window, returning the next menu or nil
type MenuFn func() MenuFn

var shouldExitApp bool

var helpDialog = []menu.Item{
	{Val: "── CONTAINER ACTIONS ──", Label: "── CONTAINER ACTIONS ──"},
	{Val: "<enter> - open container action menu", Label: "<enter> - open container action menu"},
	{Val: "[e]     - exec shell inside container", Label: "[e]     - exec shell inside container"},
	{Val: "[U]     - live resource hot-tuning (CPU, Memory, Restart)", Label: "[U]     - live resource hot-tuning (CPU, Memory, Restart)"},
	{Val: "[w]     - open published web port in browser", Label: "[w]     - open published web port in browser"},

	{Val: "── INSPECTION & TABS ──", Label: "── INSPECTION & TABS ──"},
	{Val: "[o]     - open container inspector (1-9 tabs)", Label: "[o]     - open container inspector (1-9 tabs)"},
	{Val: "[l]     - open container logs tab (Tab 2)", Label: "[l]     - open container logs tab (Tab 2)"},
	{Val: "[v]     - open volumes & mounts tab (Tab 3)", Label: "[v]     - open volumes & mounts tab (Tab 3)"},
	{Val: "[n]     - open networking & ports tab (Tab 4)", Label: "[n]     - open networking & ports tab (Tab 4)"},
	{Val: "[i]     - open image details tab (Tab 6)", Label: "[i]     - open image details tab (Tab 6)"},
	{Val: "[F]     - in-container file explorer (Tab F)", Label: "[F]     - in-container file explorer (Tab F)"},
	{Val: "[X]     - export container diagnostic report (JSON/Text)", Label: "[X]     - export container diagnostic report (JSON/Text)"},

	{Val: "── FILTERING & SORTING ──", Label: "── FILTERING & SORTING ──"},
	{Val: "[f]     - filter displayed containers (name, ID, labels)", Label: "[f]     - filter displayed containers (name, ID, labels)"},
	{Val: "[a]     - toggle display of all containers vs active only", Label: "[a]     - toggle display of all containers vs active only"},
	{Val: "[s]     - select container sort field (state, name, cpu...)", Label: "[s]     - select container sort field (state, name, cpu...)"},
	{Val: "[r]     - reverse container sort order", Label: "[r]     - reverse container sort order"},
	{Val: "[g]     - toggle Compose stack grouping", Label: "[g]     - toggle Compose stack grouping"},

	{Val: "── VIEW & CONFIGURATION ──", Label: "── VIEW & CONFIGURATION ──"},
	{Val: "[c]     - configure column layout & default download dir", Label: "[c]     - configure column layout & default download dir"},
	{Val: "[m]     - toggle rate (/s) vs. cumulative total metrics", Label: "[m]     - toggle rate (/s) vs. cumulative total metrics"},
	{Val: "[H]     - toggle top ctop header bar", Label: "[H]     - toggle top ctop header bar"},
	{Val: "[S]     - save current configuration to file", Label: "[S]     - save current configuration to file"},
	{Val: "[h]     - open this help dialog", Label: "[h]     - open this help dialog"},
	{Val: "[q]     - exit ctop", Label: "[q]     - exit ctop"},
}

func HelpMenu() MenuFn {
	ui.Clear()
	m := menu.NewMenu()
	m.Title = "Help"
	m.AddItems(helpDialog...)
	ui.Render(m)

	for {
		e := <-uiEvents
		switch e.Type {
		case ui.ResizeEvent:
			ui.Clear()
			ui.Render(m)
		case ui.KeyboardEvent:
			return nil
		}
	}
}

func FilterMenu() MenuFn {
	i := widgets.NewInput()
	i.Title = "Filter"
	i.Data = config.GetVal("filterStr")
	ui.Render(i)

	for {
		e := <-uiEvents
		switch e.Type {
		case ui.ResizeEvent:
			ui.Clear()
			ui.Render(i)
		case ui.KeyboardEvent:
			switch e.ID {
			case "<Escape>":
				config.Update("filterStr", "")
				_ = RefreshDisplay()
				return nil
			case "<Enter>":
				config.Update("filterStr", i.Data)
				_ = RefreshDisplay()
				return nil
			default:
				i.KeyPress(e.ID)
				config.Update("filterStr", i.Data)
				_ = RefreshDisplay()
				ui.Render(i)
			}
		}
	}
}

func SortMenu() MenuFn {
	ui.Clear()
	m := menu.NewMenu()
	m.Selectable = true
	m.SortItems = true
	m.Title = "Sort Field"

	for _, field := range container.SortFields() {
		m.AddItems(menu.Item{Val: field, Label: ""})
	}

	// set cursor position to current sort field
	m.SetCursor(config.GetVal("sortField"))
	ui.Render(m)

	for {
		e := <-uiEvents
		switch e.Type {
		case ui.ResizeEvent:
			ui.Clear()
			ui.Render(m)
		case ui.KeyboardEvent:
			if IsKeyMatch("up", e.ID) {
				m.Up()
				ui.Render(m)
			} else if IsKeyMatch("down", e.ID) {
				m.Down()
				ui.Render(m)
			} else if IsKeyMatch("exit", e.ID) || e.ID == "c" {
				return nil
			} else if e.ID == "<Enter>" {
				config.Update("sortField", m.SelectedValue())
				return nil
			}
		}
	}
}

// ConfigMenu opens the runtime configuration menu (columns layout and default download directory).
func ConfigMenu() MenuFn {
	return ColumnsMenu()
}

func ColumnsMenu() MenuFn {
	const (
		enabledStr  = "[X]"
		disabledStr = "[ ]"
		padding     = 2
	)

	ui.Clear()
	m := menu.NewMenu()
	m.Selectable = true
	m.SortItems = false
	m.Title = "Configuration & Columns"
	m.SubText = "Re-order: <Page Up> / <Page Down> | [D] Download Dir"

	rebuild := func() {
		var maxLen int
		for _, col := range config.GlobalColumns {
			if len(col.Label) > maxLen {
				maxLen = len(col.Label)
			}
		}
		maxLen += padding

		m.ClearItems()
		for _, col := range config.GlobalColumns {
			txt := col.Label + strings.Repeat(" ", maxLen-len(col.Label))
			if col.Enabled {
				txt += enabledStr
			} else {
				txt += disabledStr
			}
			m.AddItems(menu.Item{Val: col.Name, Label: txt})
		}

		m.AddItems(menu.NewSeparator())
		m.AddItems(menu.Item{
			Val:   "downloadDir",
			Label: fmt.Sprintf("[D] Download Directory: %s", config.GetDownloadDir()),
		})

		ui.Clear()
		ui.Render(m)
	}

	setDownloadDirPrompt := func() {
		inp := widgets.NewInput()
		inp.Title = "Set Default Host Download Directory (Press Enter to apply, Esc to cancel)"
		inp.Data = config.GetDownloadDir()
		tw, th := theme.TermDimensions()
		inp.SetRect(0, th-3, tw, th)
		ui.Render(inp)
		for {
			ie := <-uiEvents
			if ie.Type == ui.KeyboardEvent {
				if ie.ID == "<Escape>" {
					break
				} else if ie.ID == "<Enter>" {
					config.SetDownloadDir(inp.Data)
					break
				} else {
					inp.KeyPress(ie.ID)
					ui.Render(inp)
				}
			}
		}
		rebuild()
	}

	upFn := func() {
		val := m.SelectedValue()
		if val != "" && val != "downloadDir" {
			config.ColumnLeft(val)
		}
		m.Up()
		rebuild()
	}

	downFn := func() {
		val := m.SelectedValue()
		if val != "" && val != "downloadDir" {
			config.ColumnRight(val)
		}
		m.Down()
		rebuild()
	}

	toggleFn := func() {
		val := m.SelectedValue()
		if val == "downloadDir" {
			setDownloadDirPrompt()
			return
		}
		if val != "" {
			config.ColumnToggle(val)
			rebuild()
		}
	}

	rebuild()

	for {
		e := <-uiEvents
		switch e.Type {
		case ui.ResizeEvent:
			ui.Clear()
			ui.Render(m)
		case ui.KeyboardEvent:
			if IsKeyMatch("up", e.ID) {
				m.Up()
				ui.Render(m)
			} else if IsKeyMatch("down", e.ID) {
				m.Down()
				ui.Render(m)
			} else if IsKeyMatch("pgup", e.ID) {
				upFn()
			} else if IsKeyMatch("pgdown", e.ID) {
				downFn()
			} else if IsKeyMatch("exit", e.ID) || e.ID == "c" {
				if cursor != nil && cursor.cSuper != nil {
					cSource, err := cursor.cSuper.Get()
					if err == nil {
						for _, c := range cSource.All() {
							c.RecreateWidgets()
						}
					}
				}
				return nil
			} else if e.ID == "d" || e.ID == "D" {
				setDownloadDirPrompt()
			} else if e.ID == "<Enter>" || e.ID == "x" || e.ID == "<Space>" {
				toggleFn()
			}
		}
	}
}

func ContainerMenu() MenuFn {
	if cursor == nil {
		return nil
	}
	c := cursor.Selected()
	if c == nil {
		return nil
	}

	m := menu.NewMenu()
	m.Selectable = true
	m.Title = "Menu"

	items := []menu.Item{
		// Group 1: Viewers & Inspectors
		{Val: "single", Label: "[o] overview & metrics"},
		{Val: "single_volumes", Label: "[v] volumes & mounts"},
		{Val: "single_network", Label: "[n] networking & ports"},
		{Val: "single_image", Label: "[i] image details"},
		{Val: "single_process", Label: "[E] process & env"},
		{Val: "single_top", Label: "[P] in-container top"},
		{Val: "single_diff", Label: "[D] filesystem diff"},
		{Val: "single_generator", Label: "[G] generate run/compose"},
		{Val: "single_labels", Label: "[L] labels & compose"},
		{Val: "single_files", Label: "[F] in-container file explorer"},
		{Val: "logs", Label: "[l] log view"},
		{Val: "export_report", Label: "[X] export report (JSON/Text)"},
		menu.NewSeparator(),
	}

	readOnly := config.GetSwitchVal("readOnly")

	if !readOnly {
		if c.Meta["state"] == "running" {
			// Group 2: Lifecycle controls
			items = append(items, menu.Item{Val: "stop", Label: "[s] stop"})
			items = append(items, menu.Item{Val: "pause", Label: "[p] pause"})
			items = append(items, menu.Item{Val: "restart", Label: "[r] restart"})
			items = append(items, menu.Item{Val: "signal", Label: "[k] send signal..."})
			items = append(items, menu.NewSeparator())

			// Group 3: Tools
			items = append(items, menu.Item{Val: "exec", Label: "[e] exec shell"})
			items = append(items, menu.Item{Val: "tune_resources", Label: "[U] tune resources (cpu/mem/restart)"})
			if c.Meta["Web Port"] != "" {
				items = append(items, menu.Item{Val: "browser", Label: "[w] open in browser"})
			}
			items = append(items, menu.NewSeparator())
		}
		if c.Meta["state"] == "exited" || c.Meta["state"] == "created" {
			items = append(items, menu.Item{Val: "start", Label: "[s] start"})
			items = append(items, menu.Item{Val: "remove", Label: "[R] remove"})
			items = append(items, menu.NewSeparator())
		}
		if c.Meta["state"] == "paused" {
			items = append(items, menu.Item{Val: "unpause", Label: "[p] unpause"})
			items = append(items, menu.NewSeparator())
		}
	}

	// Group 4: Actions
	items = append(items, menu.Item{Val: "cancel", Label: "[c] cancel"})
	items = append(items, menu.Item{Val: "quit", Label: "[q] quit"})

	m.AddItems(items...)
	ui.Render(m)

	var selected string

	for {
		e := <-uiEvents
		switch e.Type {
		case ui.ResizeEvent:
			ui.Clear()
			ui.Render(m)
		case ui.KeyboardEvent:
			if IsKeyMatch("up", e.ID) {
				m.Up()
				ui.Render(m)
			} else if IsKeyMatch("down", e.ID) {
				m.Down()
				ui.Render(m)
			} else if e.ID == "<Enter>" {
				selected = m.SelectedValue()
				goto Handled
			} else {
				switch e.ID {
				case "o":
					selected = "single"
					goto Handled
				case "v":
					selected = "single_volumes"
					goto Handled
				case "n":
					selected = "single_network"
					goto Handled
				case "i", "I":
					selected = "single_image"
					goto Handled
				case "E":
					selected = "single_process"
					goto Handled
				case "P":
					selected = "single_top"
					goto Handled
				case "D":
					selected = "single_diff"
					goto Handled
				case "G":
					selected = "single_generator"
					goto Handled
				case "L":
					selected = "single_labels"
					goto Handled
				case "F":
					selected = "single_files"
					goto Handled
				case "l":
					selected = "logs"
					goto Handled
				case "X", "x":
					selected = "export_report"
					goto Handled
				case "k":
					if !readOnly && c.Meta["state"] == "running" {
						selected = "signal"
						goto Handled
					}
				case "U":
					if !readOnly && c.Meta["state"] == "running" {
						selected = "tune_resources"
						goto Handled
					}
				case "s":
					if !readOnly {
						if c.Meta["state"] == "running" {
							selected = "stop"
						} else {
							selected = "start"
						}
						goto Handled
					}
				case "p":
					if !readOnly {
						if c.Meta["state"] == "paused" {
							selected = "unpause"
						} else {
							selected = "pause"
						}
						goto Handled
					}
				case "e":
					if !readOnly && c.Meta["state"] == "running" {
						selected = "exec"
						goto Handled
					}
				case "r":
					if !readOnly && c.Meta["state"] == "running" {
						selected = "restart"
						goto Handled
					}
				case "w":
					if !readOnly && c.Meta["Web Port"] != "" {
						selected = "browser"
						goto Handled
					}
				case "R":
					if !readOnly {
						selected = "remove"
						goto Handled
					}
				case "c", "<Escape>":
					return nil
				case "q":
					selected = "quit"
					goto Handled
				default:
					return nil
				}
			}
		}
	}

Handled:
	var nextMenu MenuFn
	switch selected {
	case "single", "single_metrics":
		nextMenu = SingleView
	case "single_volumes":
		nextMenu = SingleViewVolumes
	case "single_network":
		nextMenu = SingleViewNetwork
	case "single_image":
		nextMenu = SingleViewImage
	case "single_process":
		nextMenu = SingleViewProcess
	case "single_top":
		nextMenu = SingleViewTop
	case "single_diff":
		nextMenu = SingleViewDiff
	case "single_generator":
		nextMenu = SingleViewGenerator
	case "single_labels":
		nextMenu = SingleViewLabels
	case "single_files":
		nextMenu = FileExplorerMenu
	case "logs":
		nextMenu = SingleViewLogs
	case "export_report":
		nextMenu = ExportReportMenu(c)
	case "signal":
		nextMenu = SignalMenu
	case "tune_resources":
		nextMenu = ResourceMenu
	case "exec":
		nextMenu = ExecShell
	case "browser":
		nextMenu = OpenInBrowser
	case "start":
		nextMenu = Confirm(confirmTxt("start", c.GetMeta("name")), c.Start)
	case "stop":
		nextMenu = Confirm(confirmTxt("stop", c.GetMeta("name")), c.Stop)
	case "remove":
		nextMenu = Confirm(confirmTxt("remove", c.GetMeta("name")), c.Remove)
	case "pause":
		nextMenu = Confirm(confirmTxt("pause", c.GetMeta("name")), c.Pause)
	case "unpause":
		nextMenu = Confirm(confirmTxt("unpause", c.GetMeta("name")), c.Unpause)
	case "restart":
		nextMenu = Confirm(confirmTxt("restart", c.GetMeta("name")), c.Restart)
	case "quit":
		shouldExitApp = true
		return nil
	}

	return nextMenu
}

func ResourceMenu() MenuFn {
	c := cursor.Selected()
	if c == nil {
		return nil
	}

	ui.Clear()
	m := menu.NewMenu()
	m.Selectable = true
	m.Title = fmt.Sprintf("RESOURCE HOT-TUNING: %s", c.GetMeta("name"))

	items := []menu.Item{
		{Val: "mem", Label: "[1] Set Memory Limit (MB)"},
		{Val: "cpu", Label: "[2] Set CPU Quota (e.g. 1.5)"},
		{Val: "restart", Label: "[3] Set Restart Policy"},
		menu.NewSeparator(),
		{Val: "cancel", Label: "[c] cancel"},
	}
	m.AddItems(items...)
	ui.Render(m)

	for {
		e := <-uiEvents
		switch e.Type {
		case ui.ResizeEvent:
			ui.Clear()
			ui.Render(m)
		case ui.KeyboardEvent:
			if IsKeyMatch("up", e.ID) {
				m.Up()
				ui.Render(m)
			} else if IsKeyMatch("down", e.ID) {
				m.Down()
				ui.Render(m)
			} else if IsKeyMatch("exit", e.ID) || e.ID == "c" {
				return nil
			} else if e.ID == "<Enter>" || e.ID == "1" || e.ID == "2" || e.ID == "3" {
				val := m.SelectedValue()
				switch e.ID {
				case "1":
					val = "mem"
				case "2":
					val = "cpu"
				case "3":
					val = "restart"
				}

				if val == "mem" {
					inp := widgets.NewInput()
					inp.Title = "Enter Memory Limit in MB (e.g. 512, 1024, 0 to clear, Esc to cancel)"
					ui.Clear()
					ui.Render(inp)
					for {
						ie := <-uiEvents
						if ie.Type == ui.KeyboardEvent {
							if ie.ID == "<Escape>" {
								break
							} else if ie.ID == "<Enter>" {
								mbStr := strings.TrimSpace(inp.Data)
								if mb, err := strconv.ParseInt(mbStr, 10, 64); err == nil {
									if err := c.UpdateResources(mb, 0, ""); err != nil {
										log.StatusErr(err)
									} else {
										log.Statusf("updated memory limit to %d MB", mb)
									}
								}
								break
							} else {
								inp.KeyPress(ie.ID)
								ui.Render(inp)
							}
						}
					}
				} else if val == "cpu" {
					inp := widgets.NewInput()
					inp.Title = "Enter CPU Allocation (e.g. 1.0, 2.5, 0.5, Esc to cancel)"
					ui.Clear()
					ui.Render(inp)
					for {
						ie := <-uiEvents
						if ie.Type == ui.KeyboardEvent {
							if ie.ID == "<Escape>" {
								break
							} else if ie.ID == "<Enter>" {
								cpuStr := strings.TrimSpace(inp.Data)
								if cpu, err := strconv.ParseFloat(cpuStr, 64); err == nil {
									if err := c.UpdateResources(0, cpu, ""); err != nil {
										log.StatusErr(err)
									} else {
										log.Statusf("updated CPU allocation to %.2f CPUs", cpu)
									}
								}
								break
							} else {
								inp.KeyPress(ie.ID)
								ui.Render(inp)
							}
						}
					}
				} else if val == "restart" {
					rm := menu.NewMenu()
					rm.Selectable = true
					rm.Title = "Select Restart Policy"
					rm.AddItems(
						menu.Item{Val: "always", Label: "[1] always"},
						menu.Item{Val: "unless-stopped", Label: "[2] unless-stopped"},
						menu.Item{Val: "on-failure", Label: "[3] on-failure"},
						menu.Item{Val: "no", Label: "[4] no"},
						menu.NewSeparator(),
						menu.Item{Val: "cancel", Label: "[c] cancel"},
					)
					ui.Clear()
					ui.Render(rm)
					for {
						re := <-uiEvents
						if re.Type == ui.KeyboardEvent {
							if IsKeyMatch("up", re.ID) {
								rm.Up()
								ui.Render(rm)
							} else if IsKeyMatch("down", re.ID) {
								rm.Down()
								ui.Render(rm)
							} else if IsKeyMatch("exit", re.ID) || re.ID == "c" {
								break
							} else if re.ID == "<Enter>" || re.ID == "1" || re.ID == "2" || re.ID == "3" || re.ID == "4" {
								p := rm.SelectedValue()
								switch re.ID {
								case "1":
									p = "always"
								case "2":
									p = "unless-stopped"
								case "3":
									p = "on-failure"
								case "4":
									p = "no"
								}
								if p != "" && p != "cancel" {
									if err := c.UpdateResources(0, 0, p); err != nil {
										log.StatusErr(err)
									} else {
										log.Statusf("updated restart policy to %s", p)
									}
								}
								break
							}
						}
					}
				}
				return nil
			}
		}
	}
}

func ExportReportMenu(c *container.Container) MenuFn {
	return func() MenuFn {
		if c == nil {
			return nil
		}
		ui.Clear()

		exportDir := config.GetDownloadDir()

		buildMenu := func() *menu.Menu {
			m := menu.NewMenu()
			m.Selectable = true
			m.Title = fmt.Sprintf("Export Diagnostic Report: %s", c.GetMeta("name"))
			items := []menu.Item{
				{Val: "json", Label: "[1] JSON Diagnostic Snapshot (*.json)"},
				{Val: "txt", Label: "[2] Formatted Text Report (*.txt)"},
				{Val: "both", Label: "[3] Full Diagnostic Bundle (*.json + *.txt)"},
				{Val: "dir", Label: fmt.Sprintf("[D] Target Directory: %s", exportDir)},
				menu.NewSeparator(),
				{Val: "cancel", Label: "[c] cancel / back"},
			}
			m.AddItems(items...)
			return m
		}

		m := buildMenu()
		ui.Render(m)

		changeDirPrompt := func() {
			inp := widgets.NewInput()
			inp.Title = "Enter Destination Export Directory (Press Enter to apply, Esc to cancel)"
			inp.Data = exportDir
			tw, th := theme.TermDimensions()
			inp.SetRect(0, th-3, tw, th)
			ui.Render(inp)
			for {
				ie := <-uiEvents
				if ie.Type == ui.KeyboardEvent {
					if ie.ID == "<Escape>" {
						break
					} else if ie.ID == "<Enter>" {
						config.SetDownloadDir(inp.Data)
						exportDir = config.GetDownloadDir()
						break
					} else {
						inp.KeyPress(ie.ID)
						ui.Render(inp)
					}
				}
			}
			m = buildMenu()
			m.SetCursor("dir")
			ui.Clear()
			ui.Render(m)
		}

		for {
			e := <-uiEvents
			switch e.Type {
			case ui.ResizeEvent:
				ui.Clear()
				ui.Render(m)
			case ui.KeyboardEvent:
				if IsKeyMatch("up", e.ID) {
					m.Up()
					ui.Render(m)
				} else if IsKeyMatch("down", e.ID) {
					m.Down()
					ui.Render(m)
				} else if IsKeyMatch("exit", e.ID) || e.ID == "c" || e.ID == "q" {
					return nil
				} else if e.ID == "D" || e.ID == "d" {
					changeDirPrompt()
				} else if e.ID == "<Enter>" || e.ID == "<Space>" || e.ID == "1" || e.ID == "2" || e.ID == "3" {
					val := m.SelectedValue()
					switch e.ID {
					case "1":
						val = "json"
					case "2":
						val = "txt"
					case "3":
						val = "both"
					}

					if val == "cancel" {
						return nil
					}
					if val == "dir" {
						changeDirPrompt()
						continue
					}

					report := diag.BuildReport(c.Id, c.Meta, &c.Metrics, c.HostID, c.GenerateRunCmd(), c.GenerateCompose())
					savedPaths, err := diag.SaveReport(report, exportDir, val)
					if err != nil {
						log.StatusErr(err)
						return nil
					}

					var basenames []string
					for _, p := range savedPaths {
						basenames = append(basenames, filepath.Base(p))
					}
					log.Statusf("✔  Exported report: %s (in %s)", strings.Join(basenames, ", "), exportDir)
					return nil
				}
			}
		}
	}
}

func FileExplorerMenu() MenuFn {
	c := cursor.Selected()
	if c == nil {
		return nil
	}

	ui.Clear()
	exp := single.NewExplorer()
	currentPath := "/"
	entries, _ := c.ReadDir(currentPath)
	exp.Set(currentPath, entries)

	dlDir := config.GetDownloadDir()
	exp.SetDownloadDir(dlDir)

	tw, th := theme.TermDimensions()
	exp.SetWidth(tw)
	exp.SetRect(0, 0, tw, th)
	ui.Render(exp)

	refreshDir := func(p string) {
		currentPath = p
		ents, err := c.ReadDir(currentPath)
		if err != nil {
			log.Errorf("failed to read dir %s: %s", currentPath, err)
		}
		exp.ClearPreview()
		exp.Set(currentPath, ents)
		exp.CursorPos = 0
		ui.Clear()
		ui.Render(exp)
	}

	for {
		e := <-uiEvents
		switch e.Type {
		case ui.ResizeEvent:
			theme.SyncTerm()
			tw, th := theme.TermDimensions()
			exp.SetWidth(tw)
			exp.SetRect(0, 0, tw, th)
			ui.Clear()
			ui.Render(exp)
		case ui.KeyboardEvent:
			if exp.IsEditing() {
				done, applied, mode, val := exp.EditKeyPress(e.ID)
				if done {
					if applied {
						switch mode {
						case single.EditModeTargetDir:
							exp.SetStatus(fmt.Sprintf("✔  Host download directory set to: %s", config.GetDownloadDir()), false)
						case single.EditModeFilter:
							if val != "" {
								exp.SetStatus(fmt.Sprintf("✔  Filter applied: %q (%d items)", val, len(exp.TotalItems())), false)
							} else {
								exp.SetStatus("✔  Filter cleared", false)
							}
						case single.EditModeDeepSearch:
							query := strings.TrimSpace(val)
							if query != "" {
								exp.SetStatus(fmt.Sprintf("🔍  Searching for %q across container...", query), false)
								ui.Render(exp)
								results, err := c.SearchFiles(currentPath, query, 100)
								if err != nil {
									exp.SetStatus(fmt.Sprintf("❌  Search error: %v", err), true)
								} else {
									exp.Set(currentPath, results)
									exp.SetDeepSearch(query)
									exp.SetStatus(fmt.Sprintf("✔  Found %d items matching %q", len(results), query), false)
								}
							} else {
								exp.ClearFilterAndSearch()
								refreshDir(currentPath)
								exp.SetStatus("✔  Search cleared", false)
							}
						case single.EditModeConfirmDelete:
							if err := c.DeleteFile(val); err != nil {
								exp.SetStatus(fmt.Sprintf("❌  Delete failed: %v", err), true)
							} else {
								exp.SetStatus(fmt.Sprintf("✔  Deleted %s", path.Base(val)), false)
								refreshDir(currentPath)
							}
						case single.EditModeConfirmEdit:
							modified, err := EditContainerFile(c, val)
							if err != nil {
								exp.SetStatus(fmt.Sprintf("❌  Edit failed: %v", err), true)
							} else if modified {
								exp.SetStatus(fmt.Sprintf("✔  Updated %s", path.Base(val)), false)
								refreshDir(currentPath)
							} else {
								exp.SetStatus("ℹ  File unchanged", false)
							}
						case single.EditModeUpload:
							srcHost := strings.TrimSpace(val)
							if srcHost != "" {
								err := c.Upload(srcHost, currentPath)
								if err != nil {
									exp.SetStatus(fmt.Sprintf("❌  Upload failed: %v", err), true)
								} else {
									exp.SetStatus(fmt.Sprintf("✔  Uploaded %s -> %s", srcHost, currentPath), false)
									refreshDir(currentPath)
								}
							} else {
								exp.SetStatus("ℹ  Upload cancelled (empty path)", false)
							}
						}
					} else {
						switch mode {
						case single.EditModeConfirmDelete:
							exp.SetStatus("ℹ  Delete cancelled", false)
						case single.EditModeConfirmEdit:
							exp.SetStatus("ℹ  Edit cancelled", false)
						case single.EditModeUpload:
							exp.SetStatus("ℹ  Upload cancelled", false)
						}
					}
					ui.Clear()
				}
				ui.Render(exp)
				continue
			}

			if exp.Previewing {
				if e.ID == "<Escape>" || e.ID == "<Enter>" || e.ID == "q" || e.ID == "Q" {
					exp.ClearPreview()
					ui.Clear()
					ui.Render(exp)
				} else if IsKeyMatch("up", e.ID) || e.ID == "<Up>" || e.ID == "k" {
					exp.PreviewUp()
					ui.Render(exp)
				} else if IsKeyMatch("down", e.ID) || e.ID == "<Down>" || e.ID == "j" {
					exp.PreviewDown()
					ui.Render(exp)
				} else if IsKeyMatch("home", e.ID) || e.ID == "<Home>" || e.ID == "g" {
					exp.PreviewHome()
					ui.Render(exp)
				} else if IsKeyMatch("end", e.ID) || e.ID == "<End>" || e.ID == "G" {
					exp.PreviewEnd()
					ui.Render(exp)
				} else if IsKeyMatch("pgup", e.ID) || e.ID == "<PageUp>" {
					exp.PreviewPgUp(0)
					ui.Render(exp)
				} else if IsKeyMatch("pgdown", e.ID) || e.ID == "<PageDown>" {
					exp.PreviewPgDown(0)
					ui.Render(exp)
				}
				continue
			}

			if IsKeyMatch("up", e.ID) || e.ID == "<Up>" || e.ID == "k" {
				exp.Up()
				ui.Render(exp)
			} else if IsKeyMatch("down", e.ID) || e.ID == "<Down>" || e.ID == "j" {
				exp.Down()
				ui.Render(exp)
			} else if IsKeyMatch("pgup", e.ID) || e.ID == "<PageUp>" || e.ID == "<C-u>" {
				exp.PgUp(15)
				ui.Render(exp)
			} else if IsKeyMatch("pgdown", e.ID) || e.ID == "<PageDown>" || e.ID == "<C-d>" {
				exp.PgDown(15)
				ui.Render(exp)
			} else if IsKeyMatch("home", e.ID) || e.ID == "<Home>" || e.ID == "g" {
				exp.Home()
				ui.Render(exp)
			} else if IsKeyMatch("end", e.ID) || e.ID == "<End>" || e.ID == "G" {
				exp.End()
				ui.Render(exp)
			} else if e.ID == "<Enter>" || e.ID == "<Right>" {
				if item, ok := exp.Selected(); ok {
					if item.IsDir {
						exp.ClearFilterAndSearch()
						refreshDir(item.Path)
					} else {
						content, err := c.ReadFile(item.Path, 128*1024)
						if err != nil {
							content = fmt.Sprintf("Error reading file: %v", err)
						}
						exp.SetPreview(content)
						ui.Clear()
						ui.Render(exp)
					}
				}
			} else if e.ID == "<Backspace>" || e.ID == "<Left>" {
				if currentPath != "/" && currentPath != "" {
					parent := path.Dir(currentPath)
					if parent == "" {
						parent = "/"
					}
					exp.ClearFilterAndSearch()
					refreshDir(parent)
				}
			} else if e.ID == "v" || e.ID == "<Space>" {
				if item, ok := exp.Selected(); ok && !item.IsDir {
					content, err := c.ReadFile(item.Path, 128*1024)
					if err != nil {
						content = fmt.Sprintf("Error reading file: %v", err)
					}
					exp.SetPreview(content)
					ui.Clear()
					ui.Render(exp)
				}
			} else if e.ID == "d" {
				if item, ok := exp.Selected(); ok {
					destName := item.Name
					if destName == ".." {
						destName = path.Base(currentPath)
						if destName == "/" || destName == "." || destName == "" {
							destName = "root"
						}
					}
					targetPath := filepath.Join(config.GetDownloadDir(), destName)
					bytesDownloaded, err := c.Download(item.Path, targetPath)
					if err != nil {
						exp.SetStatus(fmt.Sprintf("❌  Download failed: %v", err), true)
					} else {
						exp.SetStatus(fmt.Sprintf("✔  Downloaded %s -> %s (%s)", item.Path, targetPath, cwidgets.ByteFormat64(bytesDownloaded)), false)
					}
					ui.Clear()
					ui.Render(exp)
				}
			} else if e.ID == "D" {
				exp.StartEditTargetDir()
				ui.Render(exp)
			} else if e.ID == "u" || e.ID == "U" {
				exp.StartEditUpload()
				ui.Render(exp)
			} else if e.ID == "r" || e.ID == "R" {
				exp.ClearFilterAndSearch()
				refreshDir(currentPath)
			} else if e.ID == "c" || e.ID == "C" {
				if exp.Filter != "" || exp.IsDeepSearch {
					exp.ClearFilterAndSearch()
					refreshDir(currentPath)
					exp.SetStatus("✔  Filter/search cleared", false)
				} else {
					exp.SetStatus("ℹ  No filter or search active", false)
				}
				ui.Clear()
				ui.Render(exp)
			} else if e.ID == "/" {
				exp.StartEditFilter()
				ui.Render(exp)
			} else if e.ID == "f" || e.ID == "F" || e.ID == "<C-f>" {
				exp.StartEditDeepSearch()
				ui.Render(exp)
			} else if e.ID == "e" || e.ID == "E" {
				if item, ok := exp.Selected(); ok {
					if item.IsDir || item.Name == ".." {
						exp.SetStatus("❌  Only files can be edited", true)
					} else {
						exp.StartConfirmEdit(item)
					}
					ui.Clear()
					ui.Render(exp)
				}
			} else if e.ID == "x" || e.ID == "X" || e.ID == "<Delete>" {
				if item, ok := exp.Selected(); ok {
					if item.IsDir || item.Name == ".." {
						exp.SetStatus("❌  Only files can be deleted", true)
					} else {
						exp.StartConfirmDelete(item)
					}
					ui.Clear()
					ui.Render(exp)
				}
			} else if e.ID == "q" || e.ID == "Q" || e.ID == "<Escape>" {
				return nil
			}
		}
	}
}

func SignalMenu() MenuFn {
	c := cursor.Selected()
	if c == nil {
		return nil
	}

	ui.Clear()
	m := menu.NewMenu()
	m.Selectable = true
	m.Title = fmt.Sprintf("Send Signal [%s]", c.GetMeta("name"))

	signals := []struct {
		name string
		desc string
	}{
		{"SIGHUP", "Reload configuration (1)"},
		{"SIGINT", "Terminal interrupt / Ctrl+C (2)"},
		{"SIGQUIT", "Quit & dump core / thread dump (3)"},
		{"SIGKILL", "Forced kill / uncatchable (9)"},
		{"SIGUSR1", "User-defined signal 1 (10)"},
		{"SIGUSR2", "User-defined signal 2 (12)"},
		{"SIGTERM", "Graceful termination (15)"},
		{"SIGSTOP", "Pause container process (19)"},
		{"SIGWINCH", "Window size change (28)"},
	}

	for _, sig := range signals {
		m.AddItems(menu.Item{
			Val:   sig.name,
			Label: fmt.Sprintf("%-10s - %s", sig.name, sig.desc),
		})
	}
	m.AddItems(menu.NewSeparator())
	m.AddItems(menu.Item{Val: "cancel", Label: "[c] cancel"})

	ui.Render(m)

	for {
		e := <-uiEvents
		switch e.Type {
		case ui.ResizeEvent:
			ui.Clear()
			ui.Render(m)
		case ui.KeyboardEvent:
			if IsKeyMatch("up", e.ID) {
				m.Up()
				ui.Render(m)
			} else if IsKeyMatch("down", e.ID) {
				m.Down()
				ui.Render(m)
			} else if IsKeyMatch("exit", e.ID) || e.ID == "c" {
				return nil
			} else if e.ID == "<Enter>" {
				val := m.SelectedValue()
				if val != "" && val != "cancel" {
					if err := c.Signal(val); err != nil {
						log.Errorf("failed to send signal %s: %s", val, err)
						log.StatusErr(err)
					}
				}
				return nil
			}
		}
	}
}

func ExecShell() MenuFn {
	c := cursor.Selected()

	if c == nil {
		return nil
	}

	var cmd []string
	if runtime.GOOS == "windows" {
		cmd = []string{"powershell.exe", "-NoLogo"}
	} else {
		cmd = []string{"/bin/sh", "-c", "printf '\\e[0m\\e[?25h' && clear && eval `grep ^$(id -un): /etc/passwd | cut -d : -f 7-`"}
	}

	if err := c.Exec(cmd); err != nil {
		// Fallback to basic shell if advanced command failed
		var fbCmd []string
		if runtime.GOOS == "windows" {
			fbCmd = []string{"cmd.exe"}
		} else {
			fbCmd = []string{"/bin/sh"}
		}
		if fbErr := c.Exec(fbCmd); fbErr != nil {
			log.StatusErr(fbErr)
		}
	}

	if tb.IsInit {
		tb.HideCursor()
		_ = tb.Sync()
	}
	RedrawRows(true)
	return nil
}

func OpenInBrowser() MenuFn {
	c := cursor.Selected()
	if c == nil {
		return nil
	}
	return func() MenuFn {
		return SingleViewWithTab(single.TabWeb)
	}
}

// Confirm creates a confirmation dialog with a given description string and func to perform if confirmed
func Confirm(txt string, fn func()) MenuFn {
	return func() MenuFn {
		m := menu.NewMenu()
		m.Selectable = true
		m.Title = "Confirm"
		m.SubText = txt

		items := []menu.Item{
			{Val: "cancel", Label: "[c]ancel"},
			{Val: "yes", Label: "[y]es"},
		}

		m.AddItems(items...)
		ui.Render(m)

		for {
			e := <-uiEvents
			switch e.Type {
			case ui.ResizeEvent:
				ui.Clear()
				ui.Render(m)
			case ui.KeyboardEvent:
				if IsKeyMatch("up", e.ID) {
					m.Up()
					ui.Render(m)
				} else if IsKeyMatch("down", e.ID) {
					m.Down()
					ui.Render(m)
				} else if IsKeyMatch("exit", e.ID) || e.ID == "c" {
					return nil
				} else if e.ID == "y" {
					fn()
					return nil
				} else if e.ID == "<Enter>" {
					if m.SelectedValue() == "yes" {
						fn()
					}
					return nil
				}
			}
		}
	}
}

func confirmTxt(a, n string) string { return fmt.Sprintf("%s container %s?", a, n) }
