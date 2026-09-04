// grid.go manages UI screen rendering, layout calculations, terminal resize events, and the primary application loop.
//
// Objective:
//
//	Orchestrate the terminal visual hierarchy for both compact multi-container grid views and
//	multi-tab single-container inspection views. Dispatches keyboard events and manages display loops.
//
// Core Components:
//   - Display: Main event loop managing compact grid rendering, cursor movement, and menu dispatch.
//   - SingleView / SingleViewWithTab: Full-screen container inspection view supporting 11 distinct tabs.
//   - RedrawRows: Thread-safe rendering function synchronizing container widgets with the terminal display.
//   - ShowConnError: Resilient reconnection modal displaying daemon connectivity errors.
//
// Functionality:
//   - Multi-tab container inspection (Metrics, Logs, Mounts, Network, Process, Image, Top, Diff, Generator, Labels, Files).
//   - Dynamic terminal dimension synchronization and widget layout recalculations.
//   - Diagnostic report generation and file export orchestration.
//   - Active TCP prober triggering and background log streaming.
//
// Data Flow:
//
//	TermUI Event Queue / Tick Timer -> grid.go (Event Loop) -> Widgets Render -> Termbox Framebuffer.
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

	theme.SafeClear()
	ex := single.NewSingle()
	ex.Explorer.SetDownloadDir(config.GetDownloadDir())

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
			ex.Logs.SetWrap(config.GetSwitchVal("logWrap"))
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
				if ex.ActiveTab == single.TabWeb {
					if e.ID == "<Escape>" || e.ID == "q" || e.ID == "Q" {
						return nil
					} else if e.ID == "r" || e.ID == "R" {
						ex.Web.FetchCurrent()
						ui.Render(ex)
						continue
					} else if e.ID == "n" || e.ID == "N" {
						ex.Web.NextEndpoint()
						ui.Render(ex)
						continue
					} else if e.ID == "p" || e.ID == "P" {
						ex.Web.PrevEndpoint()
						ui.Render(ex)
						continue
					} else if e.ID == "<Tab>" {
						ex.Web.ToggleMode()
						ui.Render(ex)
						continue
					} else if e.ID == "1" {
						ex.Web.SetMode(single.ModeRendered)
						ui.Render(ex)
						continue
					} else if e.ID == "2" {
						ex.Web.SetMode(single.ModeHeaders)
						ui.Render(ex)
						continue
					} else if e.ID == "3" {
						ex.Web.SetMode(single.ModeRaw)
						ui.Render(ex)
						continue
					} else if e.ID == "k" || IsKeyMatch("up", e.ID) {
						ex.Web.ScrollUp(1)
						ui.Render(ex)
						continue
					} else if e.ID == "j" || IsKeyMatch("down", e.ID) {
						ex.Web.ScrollDown(1)
						ui.Render(ex)
						continue
					} else if e.ID == "<PageUp>" || IsKeyMatch("pgup", e.ID) {
						ex.Web.ScrollUp(10)
						ui.Render(ex)
						continue
					} else if e.ID == "<PageDown>" || IsKeyMatch("pgdown", e.ID) {
						ex.Web.ScrollDown(10)
						ui.Render(ex)
						continue
					} else if e.ID == "g" || e.ID == "G" {
						inp := widgets.NewInput()
						inp.Title = "Enter Port, Subpath, or URL (e.g. :8080/dashboard/, /ping, :8080, Enter: Apply, Esc: Cancel)"
						inp.Data = "/"
						tw, th := theme.TermDimensions()
						inp.SetRect(0, th-3, tw, th)
						ui.Render(ex, inp)
						for {
							ge := <-uiEvents
							if ge.Type == ui.KeyboardEvent {
								if ge.ID == "<Escape>" {
									ui.Clear()
									ui.Render(ex)
									break
								} else if ge.ID == "<Enter>" {
									ex.Web.SetCustomPath(inp.Data)
									ui.Clear()
									ui.Render(ex)
									break
								} else {
									inp.KeyPress(ge.ID)
									ui.Render(ex, inp)
								}
							}
						}
						continue
					}
				}

				if ex.ActiveTab == single.TabFiles {
					if ex.Explorer.IsEditing() {
						done, applied, mode, val := ex.Explorer.EditKeyPress(e.ID)
						if done {
							if applied {
								switch mode {
								case single.EditModeTargetDir:
									ex.Explorer.SetStatus(fmt.Sprintf("✔  Host download directory set to: %s", config.GetDownloadDir()), false)
								case single.EditModeFilter:
									if val != "" {
										ex.Explorer.SetStatus(fmt.Sprintf("✔  Filter applied: %q (%d items)", val, len(ex.Explorer.TotalItems())), false)
									} else {
										ex.Explorer.SetStatus("✔  Filter cleared", false)
									}
								case single.EditModeDeepSearch:
									query := strings.TrimSpace(val)
									if query != "" {
										ex.Explorer.SetStatus(fmt.Sprintf("🔍  Searching for %q across container...", query), false)
										ui.Render(ex)
										results, err := c.SearchFiles(ex.Explorer.CurrentDir, query, 100)
										if err != nil {
											ex.Explorer.SetStatus(fmt.Sprintf("❌  Search error: %v", err), true)
										} else {
											ex.Explorer.Set(ex.Explorer.CurrentDir, results)
											ex.Explorer.SetDeepSearch(query)
											ex.Explorer.SetStatus(fmt.Sprintf("✔  Found %d items matching %q", len(results), query), false)
										}
									} else {
										ex.Explorer.ClearFilterAndSearch()
										refreshExplorerDir(ex.Explorer.CurrentDir)
										ex.Explorer.SetStatus("✔  Search cleared", false)
									}
								case single.EditModeConfirmDelete:
									if err := c.DeleteFile(val); err != nil {
										ex.Explorer.SetStatus(fmt.Sprintf("❌  Delete failed: %v", err), true)
									} else {
										ex.Explorer.SetStatus(fmt.Sprintf("✔  Deleted %s", path.Base(val)), false)
										refreshExplorerDir(ex.Explorer.CurrentDir)
									}
								case single.EditModeConfirmEdit:
									modified, err := EditContainerFile(c, val)
									if err != nil {
										ex.Explorer.SetStatus(fmt.Sprintf("❌  Edit failed: %v", err), true)
									} else if modified {
										ex.Explorer.SetStatus(fmt.Sprintf("✔  Updated %s", path.Base(val)), false)
										refreshExplorerDir(ex.Explorer.CurrentDir)
									} else {
										ex.Explorer.SetStatus("ℹ  File unchanged", false)
									}
								case single.EditModeUpload:
									srcHost := strings.TrimSpace(val)
									if srcHost != "" {
										err := c.Upload(srcHost, ex.Explorer.CurrentDir)
										if err != nil {
											ex.Explorer.SetStatus(fmt.Sprintf("❌  Upload failed: %v", err), true)
										} else {
											ex.Explorer.SetStatus(fmt.Sprintf("✔  Uploaded %s -> %s", srcHost, ex.Explorer.CurrentDir), false)
											refreshExplorerDir(ex.Explorer.CurrentDir)
										}
									} else {
										ex.Explorer.SetStatus("ℹ  Upload cancelled (empty path)", false)
									}
								}
							} else {
								switch mode {
								case single.EditModeConfirmDelete:
									ex.Explorer.SetStatus("ℹ  Delete cancelled", false)
								case single.EditModeConfirmEdit:
									ex.Explorer.SetStatus("ℹ  Edit cancelled", false)
								case single.EditModeUpload:
									ex.Explorer.SetStatus("ℹ  Upload cancelled", false)
								}
							}
							ui.Clear()
						}
						ui.Render(ex)
						continue
					}

					if ex.Explorer.Previewing {
						if e.ID == "<Escape>" || e.ID == "<Enter>" || e.ID == "q" || e.ID == "Q" {
							ex.Explorer.ClearPreview()
							ui.Clear()
							ui.Render(ex)
						} else if IsKeyMatch("up", e.ID) || e.ID == "<Up>" || e.ID == "k" {
							ex.Explorer.PreviewUp()
							ui.Render(ex)
						} else if IsKeyMatch("down", e.ID) || e.ID == "<Down>" || e.ID == "j" {
							ex.Explorer.PreviewDown()
							ui.Render(ex)
						} else if IsKeyMatch("home", e.ID) || e.ID == "<Home>" || e.ID == "g" {
							ex.Explorer.PreviewHome()
							ui.Render(ex)
						} else if IsKeyMatch("end", e.ID) || e.ID == "<End>" || e.ID == "G" {
							ex.Explorer.PreviewEnd()
							ui.Render(ex)
						} else if IsKeyMatch("pgup", e.ID) || e.ID == "<PageUp>" {
							ex.Explorer.PreviewPgUp(0)
							ui.Render(ex)
						} else if IsKeyMatch("pgdown", e.ID) || e.ID == "<PageDown>" {
							ex.Explorer.PreviewPgDown(0)
							ui.Render(ex)
						}
						continue
					}

					if IsKeyMatch("up", e.ID) || e.ID == "<Up>" || e.ID == "k" {
						ex.Explorer.Up()
						ui.Render(ex)
						continue
					} else if IsKeyMatch("down", e.ID) || e.ID == "<Down>" || e.ID == "j" {
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
								ex.Explorer.ClearFilterAndSearch()
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
							ex.Explorer.ClearFilterAndSearch()
							refreshExplorerDir(parent)
						}
						continue
					} else if e.ID == "/" {
						ex.Explorer.StartEditFilter()
						ui.Render(ex)
						continue
					} else if e.ID == "f" || e.ID == "F" || e.ID == "<C-f>" {
						ex.Explorer.StartEditDeepSearch()
						ui.Render(ex)
						continue
					} else if e.ID == "c" || e.ID == "C" {
						if ex.Explorer.Filter != "" || ex.Explorer.IsDeepSearch {
							ex.Explorer.ClearFilterAndSearch()
							refreshExplorerDir(ex.Explorer.CurrentDir)
							ex.Explorer.SetStatus("✔  Filter/search cleared", false)
						} else {
							ex.Explorer.SetStatus("ℹ  No filter or search active", false)
						}
						ui.Clear()
						ui.Render(ex)
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
							targetPath := filepath.Join(config.GetDownloadDir(), destName)
							bytesDownloaded, err := c.Download(item.Path, targetPath)
							if err != nil {
								ex.Explorer.SetStatus(fmt.Sprintf("❌  Download failed: %v", err), true)
							} else {
								ex.Explorer.SetStatus(fmt.Sprintf("✔  Downloaded %s -> %s (%s)", item.Path, targetPath, cwidgets.ByteFormat64(bytesDownloaded)), false)
							}
							ui.Clear()
							ui.Render(ex)
						}
						continue
					} else if e.ID == "D" {
						ex.Explorer.StartEditTargetDir()
						ui.Render(ex)
						continue
					} else if e.ID == "u" || e.ID == "U" {
						ex.Explorer.StartEditUpload()
						ui.Render(ex)
						continue
					} else if e.ID == "e" || e.ID == "E" {
						if item, ok := ex.Explorer.Selected(); ok {
							if item.IsDir || item.Name == ".." {
								ex.Explorer.SetStatus("❌  Only files can be edited", true)
							} else {
								ex.Explorer.StartConfirmEdit(item)
							}
							ui.Clear()
							ui.Render(ex)
						}
						continue
					} else if e.ID == "x" || e.ID == "X" || e.ID == "<Delete>" {
						if item, ok := ex.Explorer.Selected(); ok {
							if item.IsDir || item.Name == ".." {
								ex.Explorer.SetStatus("❌  Only files can be deleted", true)
							} else {
								ex.Explorer.StartConfirmDelete(item)
							}
							ui.Clear()
							ui.Render(ex)
						}
						continue
					} else if e.ID == "r" || e.ID == "R" {
						ex.Explorer.ClearFilterAndSearch()
						refreshExplorerDir(ex.Explorer.CurrentDir)
						continue
					}
				}

				if ex.ActiveTab == single.TabLogs {
					if e.ID == "t" || e.ID == "T" {
						ex.Logs.ToggleTime()
						ui.Render(ex)
						continue
					} else if e.ID == "w" || e.ID == "W" {
						config.Toggle("logWrap")
						ex.Logs.SetWrap(config.GetSwitchVal("logWrap"))
						ui.Render(ex)
						continue
					} else if e.ID == "s" || e.ID == "S" {
						if savedFile, err := ex.Logs.SaveLogs(config.GetDownloadDir()); err != nil {
							ex.Logs.SetStatus(fmt.Sprintf("❌  Save err: %v", err))
						} else {
							ex.Logs.SetStatus(fmt.Sprintf("✔  Saved to %s", savedFile))
						}
						ui.Render(ex)
						continue
					} else if e.ID == "D" {
						dirInput := widgets.NewInput()
						dirInput.Title = "Set Export / Download Target Directory (Press Enter to apply, Esc to cancel)"
						dirInput.Data = config.GetDownloadDir()
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
									config.SetDownloadDir(dirInput.Data)
									ex.Logs.SetStatus(fmt.Sprintf("✔  Target dir: %s", config.GetDownloadDir()))
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
				} else if e.ID == "W" || e.ID == "w" {
					switchTab(single.TabWeb)
					ui.Clear()
					ui.Render(ex)
				} else if (e.ID == "X" || e.ID == "x") && ex.ActiveTab != single.TabFiles {
					report := diag.BuildReport(c.Id, c.Meta, &c.Metrics, c.HostID, c.GenerateRunCmd(), c.GenerateCompose())
					savedPaths, err := diag.SaveReport(report, config.GetDownloadDir(), "both")
					if err != nil {
						if ex.ActiveTab == single.TabLogs {
							ex.Logs.StatusMsg = fmt.Sprintf("❌  Export err: %v", err)
						} else {
							log.StatusErr(err)
						}
					} else {
						var basenames []string
						for _, p := range savedPaths {
							basenames = append(basenames, filepath.Base(p))
						}
						msg := fmt.Sprintf("✔  Exported: %s", strings.Join(basenames, ", "))
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

func SingleViewWeb() MenuFn {
	return SingleViewWithTab(single.TabWeb)
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
						menu = SingleViewLogs
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
					case "c", "C":
						menu = ConfigMenu
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
