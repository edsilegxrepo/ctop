package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/bcicen/ctop/config"
	"github.com/bcicen/ctop/container"
	"github.com/bcicen/ctop/theme"
	"github.com/bcicen/ctop/widgets"
	"github.com/bcicen/ctop/widgets/menu"
	ui "github.com/gizak/termui/v3"
	tb "github.com/nsf/termbox-go"
	"github.com/pkg/browser"
)

// MenuFn executes a menu window, returning the next menu or nil
type MenuFn func() MenuFn

var shouldExitApp bool

var helpDialog = []menu.Item{
	{Val: "<enter> - open container menu", Label: ""},
	{Val: "", Label: ""},
	{Val: "[a] - toggle display of all containers", Label: ""},
	{Val: "[f] - filter displayed containers", Label: ""},
	{Val: "[h] - open this help dialog", Label: ""},
	{Val: "[H] - toggle ctop header", Label: ""},
	{Val: "[s] - select container sort field", Label: ""},
	{Val: "[r] - reverse container sort order", Label: ""},
	{Val: "[o] - open single view", Label: ""},
	{Val: "[l] - view container logs ([t] to toggle timestamp when open)", Label: ""},
	{Val: "[e] - exec shell", Label: ""},
	{Val: "[w] - open browser (first port is http)", Label: ""},
	{Val: "[c] - configure columns", Label: ""},
	{Val: "[S] - save current configuration to file", Label: ""},
	{Val: "[q] - exit ctop", Label: ""},
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
			} else if IsKeyMatch("down", e.ID) {
				m.Down()
			} else if IsKeyMatch("exit", e.ID) {
				return nil
			} else if e.ID == "<Enter>" {
				config.Update("sortField", m.SelectedValue())
				return nil
			}
		}
	}
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
	m.Title = "Columns"
	m.SubText = "Re-order: <Page Up> / <Page Down>"

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
	}

	upFn := func() {
		config.ColumnLeft(m.SelectedValue())
		m.Up()
		rebuild()
	}

	downFn := func() {
		config.ColumnRight(m.SelectedValue())
		m.Down()
		rebuild()
	}

	toggleFn := func() {
		config.ColumnToggle(m.SelectedValue())
		rebuild()
	}

	rebuild()
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
			} else if IsKeyMatch("down", e.ID) {
				m.Down()
			} else if IsKeyMatch("pgup", e.ID) {
				upFn()
			} else if IsKeyMatch("pgdown", e.ID) {
				downFn()
			} else if IsKeyMatch("exit", e.ID) {
				cSource, err := cursor.cSuper.Get()
				if err == nil {
					for _, c := range cSource.All() {
						c.RecreateWidgets()
					}
				}
				return nil
			} else if e.ID == "<Enter>" || e.ID == "x" {
				toggleFn()
			}
		}
	}
}

func ContainerMenu() MenuFn {
	c := cursor.Selected()
	if c == nil {
		return nil
	}

	m := menu.NewMenu()
	m.Selectable = true
	m.Title = "Menu"

	items := []menu.Item{
		// Group 1: Viewers
		{Val: "single", Label: "[o] single view"},
		{Val: "logs", Label: "[l] log view"},
		menu.NewSeparator(),
	}

	if c.Meta["state"] == "running" {
		// Group 2: Lifecycle controls
		items = append(items, menu.Item{Val: "stop", Label: "[s] stop"})
		items = append(items, menu.Item{Val: "pause", Label: "[p] pause"})
		items = append(items, menu.Item{Val: "restart", Label: "[r] restart"})
		if runtime.GOOS != "windows" || c.Meta["Web Port"] != "" {
			items = append(items, menu.NewSeparator())
		}
		// Group 3: Tools
		if runtime.GOOS != "windows" {
			items = append(items, menu.Item{Val: "exec", Label: "[e] exec shell"})
		}
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
			} else if IsKeyMatch("down", e.ID) {
				m.Down()
			} else if e.ID == "<Enter>" {
				selected = m.SelectedValue()
				goto Handled
			} else {
				switch e.ID {
				case "o":
					selected = "single"
					goto Handled
				case "l":
					selected = "logs"
					goto Handled
				case "s":
					if c.Meta["state"] == "running" {
						selected = "stop"
					} else {
						selected = "start"
					}
					goto Handled
				case "p":
					if c.Meta["state"] == "paused" {
						selected = "unpause"
					} else {
						selected = "pause"
					}
					goto Handled
				case "e":
					if c.Meta["state"] == "running" {
						selected = "exec"
						goto Handled
					}
				case "r":
					if c.Meta["state"] == "running" {
						selected = "restart"
						goto Handled
					}
				case "w":
					if c.Meta["Web Port"] != "" {
						selected = "browser"
						goto Handled
					}
				case "R":
					selected = "remove"
					goto Handled
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
	case "single":
		nextMenu = SingleView
	case "logs":
		nextMenu = LogMenu
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

func LogMenu() MenuFn {
	c := cursor.Selected()
	if c == nil {
		return nil
	}

	ui.Clear()
	logs, quit := logReader(c)
	m := widgets.NewTextView(logs)

	updateTitle := func() {
		filterInfo := ""
		if m.Filter() != "" {
			filterInfo = fmt.Sprintf(" [filter: %s]", m.Filter())
		}
		m.Title = fmt.Sprintf("Logs [%s]%s (t: time, /: filter, q: close)", c.GetMeta("name"), filterInfo)
	}
	updateTitle()

	input := widgets.NewInput()
	input.Title = "Filter Logs"
	filtering := false

	renderAll := func() {
		if filtering {
			ui.Render(m, input)
		} else {
			ui.Render(m)
		}
	}
	renderAll()

	// Inactivity timer to resume background refresh after typing pause
	inactivityTimer := time.NewTimer(0)
	if !inactivityTimer.Stop() {
		select {
		case <-inactivityTimer.C:
		default:
		}
	}

	resetInactivity := func() {
		m.Pause()
		inactivityTimer.Reset(1200 * time.Millisecond)
	}

	for {
		select {
		case e := <-uiEvents:
			switch e.Type {
			case ui.ResizeEvent:
				m.Resize()
				if filtering {
					w, h := theme.TermDimensions()
					input.SetRect(0, h-3, w, h)
				}
				renderAll()
			case ui.KeyboardEvent:
				if filtering {
					resetInactivity()
					switch e.ID {
					case "<Escape>":
						filtering = false
						inactivityTimer.Stop()
						m.SetFilter("")
						updateTitle()
						m.Resume()
						ui.Clear()
						renderAll()
					case "<Enter>":
						filtering = false
						inactivityTimer.Stop()
						m.SetFilter(input.Data)
						updateTitle()
						m.Resume()
						ui.Clear()
						renderAll()
					default:
						input.KeyPress(e.ID)
						m.SetFilter(input.Data)
						updateTitle()
						renderAll()
					}
				} else {
					switch e.ID {
					case "t", "T":
						m.Toggle()
					case "/", "f", "F":
						filtering = true
						resetInactivity()
						input.Data = m.Filter()
						w, h := theme.TermDimensions()
						input.SetRect(0, h-3, w, h)
						m.RecomputeTextOut()
						renderAll()
					case "q", "Q", "<Escape>", "<C-c>":
						quit <- true
						inactivityTimer.Stop()
						return nil
					default:
						quit <- true
						inactivityTimer.Stop()
						return nil
					}
				}
			}
		case <-inactivityTimer.C:
			if filtering {
				m.Resume()
				renderAll()
			}
		}
	}
}

func ExecShell() MenuFn {
	c := cursor.Selected()

	if c == nil {
		return nil
	}

	if err := c.Exec([]string{"/bin/sh", "-c", "printf '\\e[0m\\e[?25h' && clear && eval `grep ^$(id -un): /etc/passwd | cut -d : -f 7-`"}); err != nil {
		log.StatusErr(err)
	}

	tb.HideCursor()
	_ = tb.Sync()
	RedrawRows(true)
	return nil
}

func OpenInBrowser() MenuFn {
	c := cursor.Selected()
	if c == nil {
		return nil
	}

	webPort := c.Meta.Get("Web Port")
	if webPort == "" {
		return nil
	}
	link := "http://" + webPort + "/"
	if err := browser.OpenURL(link); err != nil {
		log.Errorf("failed to open browser: %s", err)
	}
	return nil
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
				} else if IsKeyMatch("down", e.ID) {
					m.Down()
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

type toggleLog struct {
	timestamp time.Time
	message   string
}

func (t *toggleLog) Toggle(on bool) string {
	if on {
		return fmt.Sprintf("%s  %s", t.timestamp.Local().Format("2006-01-02 15:04:05.000"), t.message)
	}
	return t.message
}

func logReader(container *container.Container) (logs chan widgets.ToggleText, quit chan bool) {
	logCollector := container.Logs()
	if logCollector == nil {
		logs = make(chan widgets.ToggleText)
		quit = make(chan bool)
		close(logs)
		return
	}
	stream := logCollector.Stream()
	logs = make(chan widgets.ToggleText)
	quit = make(chan bool)

	go func() {
		for {
			select {
			case log := <-stream:
				logs <- &toggleLog{timestamp: log.Timestamp, message: log.Message}
			case <-quit:
				logCollector.Stop()
				close(logs)
				return
			}
		}
	}()
	return
}

func confirmTxt(a, n string) string { return fmt.Sprintf("%s container %s?", a, n) }
