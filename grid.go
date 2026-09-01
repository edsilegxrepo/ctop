// grid.go manages UI screen rendering, layout calculations, terminal resize events, and the primary application loop.
package main

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/edsilegx/ctop/internal/cwidgets"
	"github.com/edsilegx/ctop/internal/cwidgets/single"
	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/internal/widgets"
	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/diag"
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

var redrawLock sync.Mutex

// RedrawRows recalculates layout heights, repopulates compact grid rows, and executes TermUI render.
func RedrawRows(clr bool) {
	redrawLock.Lock()
	defer redrawLock.Unlock()

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
		header.Align()
		y += header.Height() + 1
	}

	termW, termH := theme.TermDimensions()
	cGrid.SetWidth(termW)
	cGrid.SetY(y)
	cGrid.SetRect(0, y, termW, termH)

	for _, c := range cursor.Filtered() {
		cGrid.AddRows(c.Widgets)
	}

	if clr {
		theme.SafeClear()
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

	refreshExplorerDir := func(p string) {
		ents, err := c.ReadDir(p)
		if err != nil {
			log.Errorf("failed to read dir %s: %s", p, err)
		}
		ex.Explorer.ClearPreview()
		ex.SetExplorer(p, ents)
		ex.Explorer.CursorPos = 0
		ui.Clear()
		ui.Render(ex)
	}

	var lastProbeTime time.Time
	var logQuit chan bool
	var logStreamActive bool

	startLogStream := func() {
		if logStreamActive {
			return
		}
		lc := c.Logs()
		if lc == nil {
			return
		}
		stream := lc.Stream()
		logQuit = make(chan bool, 1)
		logStreamActive = true
		go func() {
			defer func() {
				lc.Stop()
				logStreamActive = false
			}()
			for {
				select {
				case l, ok := <-stream:
					if !ok {
						return
					}
					ex.Logs.Add(l)
				case <-logQuit:
					return
				}
			}
		}()
	}

	stopLogStream := func() {
		if logStreamActive && logQuit != nil {
			select {
			case logQuit <- true:
			default:
			}
			logStreamActive = false
		}
	}
	defer stopLogStream()

	switchTab := func(tab int) {
		switch tab {
		case single.TabLogs:
			startLogStream()
		case single.TabNetwork:
			lastProbeTime = time.Now()
			ex.RunNetworkProbes()
		case single.TabTop:
			if topRes, err := c.Top(); err == nil {
				ex.SetTop(topRes)
			}
		case single.TabDiff:
			ex.SetTab(tab)
			ex.Align()
			ui.Render(ex)
			go func() {
				if changes, err := c.Changes(); err == nil {
					ex.SetDiff(changes)
					ex.Align()
					ui.Render(ex)
				}
			}()
			return
		case single.TabGenerator:
			ex.SetGenerator(c.GenerateRunCmd(), c.GenerateCompose())
		case single.TabFiles:
			dir := ex.Explorer.CurrentDir
			if dir == "" {
				dir = "/"
			}
			if entries, err := c.ReadDir(dir); err == nil {
				ex.SetExplorer(dir, entries)
			}
		}
		ex.SetTab(tab)
	}

	c.SetUpdater(ex)
	defer c.SetUpdater(c.Widgets)
	defer ex.StopNetworkProbes()

	switchTab(initialTab)

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
				if ex.ActiveTab == single.TabFiles {
					if ex.Explorer.Previewing {
						if e.ID == "<Escape>" || e.ID == "<Enter>" || e.ID == "q" || e.ID == "Q" {
							ex.Explorer.ClearPreview()
							ui.Clear()
							ui.Render(ex)
						}
						continue
					}

					if IsKeyMatch("up", e.ID) || e.ID == "k" {
						ex.Explorer.Up()
						ui.Render(ex)
						continue
					} else if IsKeyMatch("down", e.ID) || e.ID == "j" {
						ex.Explorer.Down()
						ui.Render(ex)
						continue
					} else if IsKeyMatch("pgup", e.ID) || e.ID == "<PageUp>" || e.ID == "<C-u>" {
						ex.Explorer.PgUp(15)
						ui.Render(ex)
						continue
					} else if IsKeyMatch("pgdown", e.ID) || e.ID == "<PageDown>" || e.ID == "<C-d>" {
						ex.Explorer.PgDown(15)
						ui.Render(ex)
						continue
					} else if IsKeyMatch("home", e.ID) || e.ID == "<Home>" || e.ID == "g" {
						ex.Explorer.Home()
						ui.Render(ex)
						continue
					} else if IsKeyMatch("end", e.ID) || e.ID == "<End>" || e.ID == "G" {
						ex.Explorer.End()
						ui.Render(ex)
						continue
					} else if e.ID == "<Enter>" {
						if item, ok := ex.Explorer.Selected(); ok {
							if item.IsDir {
								refreshExplorerDir(item.Path)
							} else {
								content, err := c.ReadFile(item.Path, 128*1024)
								if err != nil {
									content = fmt.Sprintf("Error reading file: %v", err)
								}
								ex.Explorer.SetPreview(content)
								ui.Clear()
								ui.Render(ex)
							}
						}
						continue
					} else if e.ID == "<Backspace>" {
						cur := ex.Explorer.CurrentDir
						if cur != "/" && cur != "" {
							parent := path.Dir(cur)
							if parent == "" {
								parent = "/"
							}
							refreshExplorerDir(parent)
						}
						continue
					} else if e.ID == "/" || e.ID == "f" || e.ID == "F" {
						input := widgets.NewInput()
						input.Title = "Filter Files (*, ? wildcard supported, Enter: Apply, Esc: Clear)"
						input.Data = ex.Explorer.Filter
						w, h := theme.TermDimensions()
						input.SetRect(0, h-3, w, h)
						ui.Render(ex, input)

						for {
							fe := <-uiEvents
							if fe.Type == ui.KeyboardEvent {
								if fe.ID == "<Escape>" {
									ex.Explorer.SetFilter("")
									ui.Clear()
									ui.Render(ex)
									break
								} else if fe.ID == "<Enter>" {
									ex.Explorer.SetFilter(input.Data)
									ui.Clear()
									ui.Render(ex)
									break
								} else {
									input.KeyPress(fe.ID)
									ex.Explorer.SetFilter(input.Data)
									ui.Render(ex, input)
								}
							}
						}
						continue
					} else if e.ID == "v" || e.ID == "<Space>" {
						if item, ok := ex.Explorer.Selected(); ok && !item.IsDir {
							content, err := c.ReadFile(item.Path, 128*1024)
							if err != nil {
								content = fmt.Sprintf("Error reading file: %v", err)
							}
							ex.Explorer.SetPreview(content)
							ui.Clear()
							ui.Render(ex)
						}
						continue
					} else if e.ID == "d" {
						if item, ok := ex.Explorer.Selected(); ok {
							destName := item.Name
							if destName == ".." {
								destName = path.Base(ex.Explorer.CurrentDir)
								if destName == "/" || destName == "." || destName == "" {
									destName = "root"
								}
							}
							activeDlDir := config.GetVal("downloadDir")
							if activeDlDir == "" {
								activeDlDir = "."
							}
							targetPath := filepath.Join(activeDlDir, destName)
							bytesDownloaded, err := c.Download(item.Path, targetPath)
							if err != nil {
								ex.Explorer.SetStatus(fmt.Sprintf("❌ Download failed: %v", err), true)
							} else {
								ex.Explorer.SetStatus(fmt.Sprintf("✔ Downloaded %s -> %s (%s)", item.Path, targetPath, cwidgets.ByteFormat64(bytesDownloaded)), false)
							}
							ui.Clear()
							ui.Render(ex)
						}
						continue
					} else if e.ID == "D" {
						inp := widgets.NewInput()
						inp.Title = "Set Host Download Target Directory (Press Enter to apply, Esc to cancel)"
						curDl := config.GetVal("downloadDir")
						if curDl == "" {
							curDl = "."
						}
						inp.Data = curDl
						ui.Clear()
						ui.Render(inp)
						for {
							ie := <-uiEvents
							if ie.Type == ui.ResizeEvent {
								theme.SyncTerm()
								inp.Align()
								ui.Clear()
								ui.Render(inp)
								continue
							}
							if ie.Type == ui.KeyboardEvent {
								if ie.ID == "<Escape>" {
									break
								} else if ie.ID == "<Enter>" {
									newDir := strings.TrimSpace(inp.Data)
									if newDir == "" {
										newDir = "."
									}
									config.Update("downloadDir", newDir)
									ex.Explorer.SetDownloadDir(newDir)
									ex.Explorer.SetStatus(fmt.Sprintf("✔ Host download directory set to: %s", newDir), false)
									break
								} else {
									inp.KeyPress(ie.ID)
									ui.Render(inp)
								}
							}
						}
						ui.Clear()
						ui.Render(ex)
						continue
					} else if e.ID == "u" || e.ID == "U" {
						inp := widgets.NewInput()
						inp.Title = fmt.Sprintf("Upload Host File/Dir to %s (Enter path, Esc to cancel)", ex.Explorer.CurrentDir)
						inp.Data = ""
						ui.Clear()
						ui.Render(inp)
						for {
							ie := <-uiEvents
							if ie.Type == ui.ResizeEvent {
								theme.SyncTerm()
								inp.Align()
								ui.Clear()
								ui.Render(inp)
								continue
							}
							if ie.Type == ui.KeyboardEvent {
								if ie.ID == "<Escape>" {
									break
								} else if ie.ID == "<Enter>" {
									srcHost := strings.TrimSpace(inp.Data)
									if srcHost != "" {
										err := c.Upload(srcHost, ex.Explorer.CurrentDir)
										if err != nil {
											ex.Explorer.SetStatus(fmt.Sprintf("❌ Upload failed: %v", err), true)
										} else {
											ex.Explorer.SetStatus(fmt.Sprintf("✔ Uploaded %s -> %s", srcHost, ex.Explorer.CurrentDir), false)
											refreshExplorerDir(ex.Explorer.CurrentDir)
										}
									}
									break
								} else {
									inp.KeyPress(ie.ID)
									ui.Render(inp)
								}
							}
						}
						ui.Clear()
						ui.Render(ex)
						continue
					} else if e.ID == "r" || e.ID == "R" {
						refreshExplorerDir(ex.Explorer.CurrentDir)
						continue
					}
				}

				if ex.ActiveTab == single.TabLogs {
					if e.ID == "t" || e.ID == "T" {
						ex.Logs.ToggleTime()
						ui.Render(ex)
						continue
					} else if e.ID == "s" || e.ID == "S" {
						dlDir := config.GetVal("downloadDir")
						if dlDir == "" {
							dlDir = "."
						}
						if savedFile, err := ex.Logs.SaveLogs(dlDir); err != nil {
							ex.Logs.StatusMsg = fmt.Sprintf("❌ Save err: %v", err)
						} else {
							ex.Logs.StatusMsg = fmt.Sprintf("✔ Saved to %s", filepath.Base(savedFile))
						}
						ui.Render(ex)
						continue
					} else if e.ID == "D" {
						dirInput := widgets.NewInput()
						dirInput.Title = "Set Export / Download Target Directory (Press Enter to apply, Esc to cancel)"
						curDl := config.GetVal("downloadDir")
						if curDl == "" {
							curDl = "."
						}
						dirInput.Data = curDl
						tw, th := theme.TermDimensions()
						dirInput.SetRect(0, th-3, tw, th)
						ui.Render(ex, dirInput)
						for {
							de := <-uiEvents
							if de.Type == ui.KeyboardEvent {
								if de.ID == "<Escape>" {
									ui.Clear()
									ui.Render(ex)
									break
								} else if de.ID == "<Enter>" {
									newDir := strings.TrimSpace(dirInput.Data)
									if newDir == "" {
										newDir = "."
									}
									config.Update("downloadDir", newDir)
									ex.Logs.StatusMsg = fmt.Sprintf("✔ Target dir: %s", newDir)
									ui.Clear()
									ui.Render(ex)
									break
								} else {
									dirInput.KeyPress(de.ID)
									ui.Render(ex, dirInput)
								}
							}
						}
						continue
					} else if e.ID == "/" || e.ID == "f" {
						filterInput := widgets.NewInput()
						filterInput.Title = "Filter Logs (Press Enter to apply, Esc to clear)"
						filterInput.Data = ex.Logs.Filter
						tw, th := theme.TermDimensions()
						filterInput.SetRect(0, th-3, tw, th)
						ui.Render(ex, filterInput)
						for {
							fe := <-uiEvents
							if fe.Type == ui.KeyboardEvent {
								if fe.ID == "<Escape>" {
									ex.Logs.SetFilter("")
									ui.Clear()
									ui.Render(ex)
									break
								} else if fe.ID == "<Enter>" {
									ex.Logs.SetFilter(filterInput.Data)
									ui.Clear()
									ui.Render(ex)
									break
								} else {
									filterInput.KeyPress(fe.ID)
									ui.Render(ex, filterInput)
								}
							}
						}
						continue
					} else if e.ID == "<Home>" || e.ID == "g" {
						ex.Logs.ScrollTop()
						ui.Render(ex)
						continue
					} else if e.ID == "<End>" || e.ID == "G" {
						ex.Logs.ScrollBottom()
						ui.Render(ex)
						continue
					}
				}

				if ex.ActiveTab == single.TabNetwork && (e.ID == "p" || e.ID == "P") {
					lastProbeTime = time.Now()
					ex.RunNetworkProbes()
					continue
				}

				if IsKeyMatch("up", e.ID) {
					ex.Up()
					ui.Render(ex)
				} else if IsKeyMatch("down", e.ID) {
					ex.Down()
					ui.Render(ex)
				} else if IsKeyMatch("pgup", e.ID) {
					ex.PgUp()
					ui.Render(ex)
				} else if IsKeyMatch("pgdown", e.ID) {
					ex.PgDown()
					ui.Render(ex)
				} else if IsKeyMatch("home", e.ID) {
					ex.Home()
					ui.Render(ex)
				} else if IsKeyMatch("end", e.ID) {
					ex.End()
					ui.Render(ex)
				} else if e.ID == "<Tab>" || e.ID == "<Right>" {
					switchTab((ex.ActiveTab + 1) % single.TotalTabs)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "<BackTab>" || e.ID == "<Left>" || e.ID == "h" {
					switchTab((ex.ActiveTab - 1 + single.TotalTabs) % single.TotalTabs)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "1" || e.ID == "o" {
					switchTab(single.TabMetrics)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "2" || e.ID == "l" || e.ID == "L" {
					switchTab(single.TabLogs)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "3" || e.ID == "v" {
					switchTab(single.TabVolumes)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "4" || e.ID == "n" {
					switchTab(single.TabNetwork)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "5" || e.ID == "E" {
					switchTab(single.TabProcess)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "6" || e.ID == "i" || e.ID == "I" {
					switchTab(single.TabImage)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "7" || e.ID == "P" {
					switchTab(single.TabTop)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "8" || e.ID == "D" {
					switchTab(single.TabDiff)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "9" || (e.ID == "G" && ex.ActiveTab != single.TabLogs) {
					switchTab(single.TabGenerator)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "0" {
					switchTab(single.TabLabels)
					ui.Clear()
					ui.Render(ex)
				} else if e.ID == "F" {
					switchTab(single.TabFiles)
					ui.Clear()
					ui.Render(ex)
				} else if (e.ID == "X" || e.ID == "x") && ex.ActiveTab != single.TabFiles {
					report := diag.BuildReport(c.Id, c.Meta, &c.Metrics, c.HostID, c.GenerateRunCmd(), c.GenerateCompose())
					exportDir := config.GetVal("downloadDir")
					if exportDir == "" {
						exportDir = "."
					}
					savedPaths, err := diag.SaveReport(report, exportDir, "both")
					if err != nil {
						if ex.ActiveTab == single.TabLogs {
							ex.Logs.StatusMsg = fmt.Sprintf("❌ Export err: %v", err)
						} else {
							log.StatusErr(err)
						}
					} else {
						var basenames []string
						for _, p := range savedPaths {
							basenames = append(basenames, filepath.Base(p))
						}
						msg := fmt.Sprintf("✔ Exported: %s", strings.Join(basenames, ", "))
						if ex.ActiveTab == single.TabLogs {
							ex.Logs.StatusMsg = msg
						} else {
							log.Statusf("%s", msg)
						}
					}
					ui.Render(ex)
					continue
				} else if (e.ID == "u" || e.ID == "U") && ex.ActiveTab != single.TabFiles && ex.ActiveTab != single.TabLogs {
					ex.ToggleSecretMask()
					ui.Render(ex)
				} else if e.ID == "q" || e.ID == "Q" || e.ID == "<Escape>" {
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
			if ex.ActiveTab == single.TabNetwork {
				interval := 5 * time.Second
				if cfgInterval := config.GetVal("probeInterval"); cfgInterval != "" {
					if d, err := time.ParseDuration(cfgInterval); err == nil && d > 0 {
						interval = d
					}
				}
				if time.Since(lastProbeTime) >= interval {
					lastProbeTime = time.Now()
					ex.RunNetworkProbes()
				}
			}
			ui.Render(ex)
		}
	}
}

func SingleView() MenuFn {
	return SingleViewWithTab(single.TabMetrics)
}

func SingleViewLogs() MenuFn {
	return SingleViewWithTab(single.TabLogs)
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

func SingleViewImage() MenuFn {
	return SingleViewWithTab(single.TabImage)
}

func SingleViewTop() MenuFn {
	return SingleViewWithTab(single.TabTop)
}

func SingleViewDiff() MenuFn {
	return SingleViewWithTab(single.TabDiff)
}

func SingleViewGenerator() MenuFn {
	return SingleViewWithTab(single.TabGenerator)
}

func SingleViewLabels() MenuFn {
	return SingleViewWithTab(single.TabLabels)
}

func SingleViewFiles() MenuFn {
	return SingleViewWithTab(single.TabFiles)
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
					case "v":
						menu = SingleViewVolumes
						goto RunMenu
					case "n":
						menu = SingleViewNetwork
						goto RunMenu
					case "i", "I":
						menu = SingleViewImage
						goto RunMenu
					case "U":
						menu = ResourceMenu
						goto RunMenu
					case "F":
						menu = FileExplorerMenu
						goto RunMenu
					case "e":
						menu = ExecShell
						goto RunMenu
					case "w":
						OpenInBrowser()
					case "X", "x":
						if c := cursor.Selected(); c != nil {
							menu = ExportReportMenu(c)
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
					case "g", "G":
						config.Toggle("groupByCompose")
						_ = RefreshDisplay()
					case "H":
						config.Toggle("enableHeader")
						RedrawRows(true)
					case "m":
						config.Toggle("rateMode")
						if config.GetSwitchVal("rateMode") {
							log.Status("metrics mode: real-time rate (/s)")
						} else {
							log.Status("metrics mode: cumulative total")
						}
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
