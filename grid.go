// grid.go manages UI screen rendering, layout calculations, terminal resize events, and the primary application loop.
package main

import (
	"time"

	"github.com/edsilegx/ctop/config"
	"github.com/edsilegx/ctop/cwidgets/single"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

// ShowConnError renders a full-screen modal error view and attempts automatic reconnection every second.
func ShowConnError(err error) (exit bool) {
	ui.Clear()
	setErr := func(err error) {
		errView.Append(err.Error())
		errView.Append("attempting to reconnect...")
		ui.Render(errView)
	}

	errView.Resize()
	setErr(err)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case e := <-uiEvents:
			switch e.Type {
			case ui.KeyboardEvent:
				if IsKeyMatch("exit", e.ID) {
					return true
				}
			case ui.ResizeEvent:
				errView.Resize()
				ui.Clear()
				ui.Render(errView)
			}
		case <-ticker.C:
			_, refreshErr := cursor.RefreshContainers()
			if refreshErr == nil {
				return false
			}
			setErr(refreshErr)
		}
	}
}

// RedrawRows recalculates layout heights, repopulates compact grid rows, and executes TermUI render.
func RedrawRows(clr bool) {
	if cGrid == nil || cursor == nil {
		return
	}
	// reinit body rows
	cGrid.Clear()

	// build layout
	y := 0
	if config.GetSwitchVal("enableHeader") && header != nil {
		header.SetCount(cursor.Len())
		header.SetFilter(config.GetVal("filterStr"))
		y += header.Height() + 1
	}

	termW, termH := theme.TermDimensions()
	cGrid.SetWidth(termW)
	cGrid.SetY(y)
	cGrid.SetRect(0, y, termW, termH)

	for _, c := range cursor.filtered {
		cGrid.AddRows(c.Widgets)
	}

	if clr {
		ui.Clear()
		if log != nil {
			log.Debugf("screen cleared")
		}
	}

	cGrid.Align()

	var drawables []ui.Drawable
	if config.GetSwitchVal("enableHeader") {
		drawables = append(drawables, header)
	}
	drawables = append(drawables, cGrid)
	if status.Message != "" {
		status.Align()
		drawables = append(drawables, status)
	}

	ui.Render(drawables...)
}

func SingleViewWithTab(initialTab int) MenuFn {
	c := cursor.Selected()
	if c == nil {
		return nil
	}

	ui.Clear()
	ex := single.NewSingle()
	ex.SetTab(initialTab)
	c.SetUpdater(ex)
	defer c.SetUpdater(c.Widgets)

	termW, _ := theme.TermDimensions()
	ex.SetWidth(termW)
	ex.Align()
	ui.Render(ex)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case e := <-uiEvents:
			switch e.Type {
			case ui.KeyboardEvent:
				if IsKeyMatch("up", e.ID) {
					ex.Up()
				} else if IsKeyMatch("down", e.ID) {
					ex.Down()
				} else if e.ID == "<Tab>" || e.ID == "<Right>" || e.ID == "l" {
					ex.NextTab()
				} else if e.ID == "<BackTab>" || e.ID == "<Left>" || e.ID == "h" {
					ex.PrevTab()
				} else if e.ID == "1" || e.ID == "o" {
					ex.SetTab(single.TabMetrics)
				} else if e.ID == "2" || e.ID == "v" {
					ex.SetTab(single.TabVolumes)
				} else if e.ID == "3" || e.ID == "n" {
					ex.SetTab(single.TabNetwork)
				} else if e.ID == "4" || e.ID == "E" || e.ID == "e" || e.ID == "p" {
					ex.SetTab(single.TabProcess)
				} else if e.ID == "5" || e.ID == "L" {
					ex.SetTab(single.TabLabels)
				} else if e.ID == "q" || e.ID == "Q" || e.ID == "<Escape>" {
					return nil
				} else {
					return nil
				}
			case ui.ResizeEvent:
				theme.SyncTerm()
				tw, _ := theme.TermDimensions()
				ex.SetWidth(tw)
				ex.Align()
				ui.Clear()
				ui.Render(ex)
			}
		case <-ticker.C:
			ui.Render(ex)
		}
	}
}

func SingleView() MenuFn {
	return SingleViewWithTab(single.TabMetrics)
}

func SingleViewVolumes() MenuFn {
	return SingleViewWithTab(single.TabVolumes)
}

func SingleViewNetwork() MenuFn {
	return SingleViewWithTab(single.TabNetwork)
}

func SingleViewProcess() MenuFn {
	return SingleViewWithTab(single.TabProcess)
}

func SingleViewLabels() MenuFn {
	return SingleViewWithTab(single.TabLabels)
}

func RefreshDisplay() error {
	if cursor == nil {
		return nil
	}
	// skip display refresh during scroll
	if !cursor.isScrolling {
		needsClear, err := cursor.RefreshContainers()
		if err != nil {
			return err
		}
		RedrawRows(needsClear)
	}
	return nil
}

func Display() bool {
	shouldExitApp = false
	var menu MenuFn
	var connErr error

	termW, _ := theme.TermDimensions()
	cGrid.SetWidth(termW)

	// initial draw
	header.Align()
	status.Align()
	if _, err := cursor.RefreshContainers(); err != nil {
		log.Errorf("failed to refresh containers: %s", err)
	}
	RedrawRows(true)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case e := <-uiEvents:
			logEvent(e)
			switch e.Type {
			case ui.ResizeEvent:
				theme.SyncTerm()
				header.Align()
				status.Align()
				cursor.ScrollPage()
				tw, _ := theme.TermDimensions()
				cGrid.SetWidth(tw)
				RedrawRows(true)
			case ui.KeyboardEvent:
				if IsKeyMatch("up", e.ID) {
					cursor.Up()
				} else if IsKeyMatch("down", e.ID) {
					cursor.Down()
				} else if IsKeyMatch("pgup", e.ID) {
					cursor.PgUp()
				} else if IsKeyMatch("pgdown", e.ID) {
					cursor.PgDown()
				} else if IsKeyMatch("exit", e.ID) {
					return true
				} else if IsKeyMatch("help", e.ID) {
					menu = HelpMenu
					goto RunMenu
				} else {
					switch e.ID {
					case "<Enter>":
						menu = ContainerMenu
						goto RunMenu
					case "<Left>", "l":
						menu = LogMenu
						goto RunMenu
					case "<Right>", "o":
						menu = SingleView
						goto RunMenu
					case "e":
						menu = ExecShell
						goto RunMenu
					case "w":
						menu = OpenInBrowser()
						if menu != nil {
							goto RunMenu
						}
					case "a":
						config.Toggle("allContainers")
						connErr = RefreshDisplay()
						if connErr != nil {
							goto HandleErr
						}
					case "D":
						dumpContainer(cursor.Selected())
					case "f":
						menu = FilterMenu
						goto RunMenu
					case "H":
						config.Toggle("enableHeader")
						RedrawRows(true)
					case "r":
						config.Toggle("sortReversed")
						_ = RefreshDisplay()
					case "s":
						menu = SortMenu
						goto RunMenu
					case "c":
						menu = ColumnsMenu
						goto RunMenu
					case "S":
						path, err := config.Write()
						if err == nil {
							log.Statusf("wrote config to %s", path)
						} else {
							log.StatusErr(err)
						}
						_ = RefreshDisplay()
					}
				}
			}
		case <-ticker.C:
			if log.StatusQueued() {
				for sm := range log.FlushStatus() {
					if sm.IsError {
						status.ShowErr(sm.Text)
					} else {
						status.Show(sm.Text)
					}
				}
			}
			connErr = RefreshDisplay()
			if connErr != nil {
				goto HandleErr
			}
		}
	}

HandleErr:
	if connErr != nil {
		return ShowConnError(connErr)
	}

RunMenu:
	if menu != nil {
		for menu != nil {
			menu = menu()
			if shouldExitApp {
				return true
			}
		}
		return shouldExitApp
	}

	return false
}
