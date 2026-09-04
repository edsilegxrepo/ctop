// single_test.go validates detailed single-container inspection widgets (CPU/Mem sparklines, environment tables, network/IO diffs).
//
// Objective:
//
//	Verify detailed inspection widget models, history ring buffer calculations, table row formatting,
//	and sparkline coordinate conversions.
//
// Test Strategy:
//   - Tests history ring buffers, multiline table generators, and metric delta calculations.
//   - Verifies CPU utilization, memory limit/usage sparkline series, and network rate converters.
//   - Validates secret masking across environment variable tables and label trees.
package single

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/models"
	ui "github.com/gizak/termui/v3"
)

func TestMkInfoRows(t *testing.T) {
	rows := mkInfoRows("ports", "80/tcp\n443/tcp")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for multiline ports, got %d", len(rows))
	}
	if rows[0][0] != "ports" || rows[0][1] != "80/tcp" {
		t.Errorf("unexpected first row: %v", rows[0])
	}
	if rows[1][0] != "" || rows[1][1] != "443/tcp" {
		t.Errorf("unexpected second row: %v", rows[1])
	}
}

func TestToFloat64Slice(t *testing.T) {
	input := []int{10, -5, 20, 0}
	res := toFloat64Slice(input)
	if len(res) != 4 {
		t.Fatalf("expected length 4, got %d", len(res))
	}
	if res[0] != 10.0 || res[1] != 0.0 || res[2] != 20.0 || res[3] != 0.0 {
		t.Errorf("unexpected result: %v", res)
	}
}

func TestNetUpdate(t *testing.T) {
	net := NewNet()
	net.Update(1000, 2000)
	net.Update(3000, 5000)

	if len(net.rxHist.Data) != 60 {
		t.Errorf("expected 60 points in rxHist, got %d", len(net.rxHist.Data))
	}
	if net.rxHist.Val != 2000 {
		t.Errorf("expected rx rate 2000, got %d", net.rxHist.Val)
	}
	if net.txHist.Val != 3000 {
		t.Errorf("expected tx rate 3000, got %d", net.txHist.Val)
	}
	if net.rxTitle != "RX [1.95kib/s]" {
		t.Errorf("unexpected rxTitle: %s", net.rxTitle)
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	net.SetRect(0, 0, 80, 12)
	net.Draw(buf)
}

func TestIOUpdate(t *testing.T) {
	io := NewIO()
	io.Update(500, 1000)
	io.Update(1500, 3000)

	if len(io.readHist.Data) != 60 {
		t.Errorf("expected 60 points in readHist, got %d", len(io.readHist.Data))
	}
	if io.readHist.Val != 1000 {
		t.Errorf("expected read rate 1000, got %d", io.readHist.Val)
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	io.SetRect(0, 0, 80, 12)
	io.Draw(buf)
}

func TestCpuAndMemWidgets(t *testing.T) {
	cpu := NewCpu()
	cpu.Update(85)
	if len(cpu.hist.Data) == 0 || cpu.hist.Data[len(cpu.hist.Data)-1] != 85.0 {
		t.Errorf("expected CPU data ending in 85.0")
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	cpu.SetRect(0, 0, 80, 12)
	cpu.Draw(buf)

	mem := NewMem()
	mem.Update(512*1024*1024, 1024*1024*1024)
	if mem.val != 512*1024*1024 || mem.limit != 1024*1024*1024 {
		t.Errorf("expected mem val and limit to be updated")
	}
	mem.SetRect(0, 0, 80, 12)
	mem.Draw(buf)
}

func TestEnvAndInfoWidgets(t *testing.T) {
	env := NewEnv()
	env.Set("FOO=BAR;BAZ=QUX")
	if env.GetHeight() != 4 { // 2 lines + 2 borders
		t.Errorf("expected env height 4, got %d", env.GetHeight())
	}
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	env.SetRect(0, 0, 80, 10)
	env.Draw(buf)

	info := NewInfo()
	info.Set("name", "test-server")
	info.Set("state", "running")
	if info.GetHeight() <= 0 {
		t.Errorf("expected positive info height, got %d", info.GetHeight())
	}
	info.SetRect(0, 0, 80, 10)
	info.Draw(buf)
}

func TestSingleView(t *testing.T) {
	s := NewSingle()
	if s == nil {
		t.Fatal("expected non-nil Single")
	}

	meta := models.NewMeta("name", "nginx-ingress")
	meta["[ENV-VAR]"] = "PORT=80"
	s.SetMeta(meta)

	metrics := models.NewMetrics()
	metrics.CPUUtil = 45
	metrics.NetRx = 2048
	metrics.NetTx = 4096
	metrics.MemUsage = 256 * 1024 * 1024
	metrics.MemLimit = 512 * 1024 * 1024
	metrics.IOBytesRead = 1024
	metrics.IOBytesWrite = 2048
	s.SetMetrics(metrics)

	s.SetWidth(120)
	s.Up()
	s.Down()

	if height := s.GetHeight(); height <= 0 {
		t.Fatalf("expected positive height, got %d", height)
	}

	s.Align()
	buf := ui.NewBuffer(image.Rect(0, 0, 120, 80))
	s.Draw(buf)
}

func TestLogsWidget(t *testing.T) {
	logsWidget := NewLogs()
	logsWidget.SetContainerName("my-app")
	logsWidget.Add(models.Log{Timestamp: time.Now(), Message: "starting service..."})
	logsWidget.Add(models.Log{Timestamp: time.Now(), Message: "service ready"})

	buf := ui.NewBuffer(image.Rect(0, 0, 80, 20))
	logsWidget.SetRect(0, 0, 80, 20)
	logsWidget.Draw(buf)

	logsWidget.ToggleTime()
	logsWidget.SetFilter("service")
	logsWidget.Up()
	logsWidget.Down()
	logsWidget.PgUp()
	logsWidget.PgDown()
	logsWidget.ScrollTop()
	logsWidget.ScrollBottom()
	logsWidget.Draw(buf)

	tmpDir := t.TempDir()
	savedPath, err := logsWidget.SaveLogs(tmpDir)
	if err != nil || savedPath == "" {
		t.Fatalf("expected successful logs export, got err: %v", err)
	}

	// Test SaveLogs with empty destDir falling back to config.GetDownloadDir()
	config.Init()
	customLogDir := t.TempDir()
	config.SetDownloadDir(customLogDir)
	savedPathDefault, err := logsWidget.SaveLogs("")
	if err != nil || !strings.HasPrefix(savedPathDefault, customLogDir) {
		t.Fatalf("expected SaveLogs('') to save into %s, got %s (err: %v)", customLogDir, savedPathDefault, err)
	}

	// Test ephemeral status message replacing the action bar, then auto-clearing
	logsWidget.SetStatus("✔ Saved to /tmp/ctop_logs_test.log")
	logsWidget.Draw(buf)
	if !strings.Contains(logsWidget.Title, "✔ Saved to /tmp/ctop_logs_test.log") {
		t.Fatalf("expected logsWidget.Title to contain status message, got '%s'", logsWidget.Title)
	}
	if strings.Contains(logsWidget.Title, "D: target") {
		t.Fatalf("expected status message to replace action bar in title, got '%s'", logsWidget.Title)
	}

	// Fast-forward past 2.5s expiration
	logsWidget.StatusTime = time.Now().Add(-3 * time.Second)
	logsWidget.Draw(buf)
	if strings.Contains(logsWidget.Title, "✔ Saved to") {
		t.Fatalf("expected status message to disappear after timeout, got '%s'", logsWidget.Title)
	}
	if !strings.Contains(logsWidget.Title, "D: target") {
		t.Fatalf("expected title to revert to action bar with 'D: target', got '%s'", logsWidget.Title)
	}
}

func TestMountsWidget(t *testing.T) {
	m := NewMounts()
	if m.GetHeight() != 5 {
		t.Fatalf("expected empty mounts height 5, got %d", m.GetHeight())
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	m.SetRect(0, 0, 100, 10)
	m.Draw(buf)

	// Set valid mounts
	m.Set("/var/lib/data:::/opt/volumes/data:::volume:::rw:::;;/etc/app/config.yaml:::/etc/app/config.yaml:::bind:::ro:::")
	if len(m.Rows) != 2 {
		t.Fatalf("expected 2 mount rows, got %d", len(m.Rows))
	}
	if m.Rows[0].Destination != "/var/lib/data" || m.Rows[0].Type != "volume" || m.Rows[0].Mode != "rw" {
		t.Errorf("unexpected first mount row: %+v", m.Rows[0])
	}
	if m.Rows[1].Destination != "/etc/app/config.yaml" || m.Rows[1].Type != "bind" || m.Rows[1].Mode != "ro" {
		t.Errorf("unexpected second mount row: %+v", m.Rows[1])
	}

	m.SetRect(0, 0, 100, 15)
	m.Draw(buf)
}

func TestNetworkWidget(t *testing.T) {
	nw := NewNetwork()
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))

	// Empty draw
	nw.SetRect(0, 0, 100, 10)
	nw.Draw(buf)

	// Set valid network and ports
	nw.Set("bridge:::172.17.0.2:::172.17.0.1:::02:42:ac:11:00:02:::16", "0.0.0.0:8080 -> 80/tcp", "bridge:172.17.0.2")
	if len(nw.Networks) != 1 {
		t.Fatalf("expected 1 network interface, got %d", len(nw.Networks))
	}
	if nw.Networks[0].Name != "bridge" || nw.Networks[0].IP != "172.17.0.2" {
		t.Errorf("unexpected network interface: %+v", nw.Networks[0])
	}
	if nw.GetHeight() <= 0 {
		t.Errorf("expected positive height, got %d", nw.GetHeight())
	}

	nw.SetRect(0, 0, 100, 15)
	nw.Draw(buf)

	// Test probes
	nw.RunProbes()
	time.Sleep(50 * time.Millisecond)
	nw.Draw(buf)
}

func TestProcessWidget(t *testing.T) {
	proc := NewProcess()
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))

	// Empty draw
	proc.SetRect(0, 0, 100, 10)
	proc.Draw(buf)

	proc.Set("cmd", "postgres -c shared_buffers=256MB")
	proc.Set("entrypoint", "/docker-entrypoint.sh")
	proc.Set("workdir", "/app")
	proc.Set("user", "1000:1000")
	proc.Set("restartPolicy", "unless-stopped")
	proc.Set("exitCode", "0")
	proc.Set("memLimit", "1024 MB")
	proc.Set("cpuLimit", "2.00 CPUs")
	proc.Set("pidsLimit", "256")
	proc.Set("privileged", "false")

	if len(proc.Rows) == 0 {
		t.Fatalf("expected populated process rows, got 0")
	}
	if proc.GetHeight() <= 0 {
		t.Errorf("expected positive process height, got %d", proc.GetHeight())
	}

	proc.SetRect(0, 0, 100, 20)
	proc.Draw(buf)
}

func TestLabelsWidget(t *testing.T) {
	lbl := NewLabels()
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))

	// Empty draw
	lbl.SetRect(0, 0, 100, 10)
	lbl.Draw(buf)

	lbl.Set("com.docker.compose.project=myapp;;com.docker.compose.service=db;;version=1.0.0;;maintainer=ops")
	if len(lbl.ComposeRows) != 2 {
		t.Fatalf("expected 2 compose rows, got %d", len(lbl.ComposeRows))
	}
	if len(lbl.GeneralRows) != 2 {
		t.Fatalf("expected 2 general label rows, got %d", len(lbl.GeneralRows))
	}
	if lbl.GetHeight() <= 0 {
		t.Errorf("expected positive labels height, got %d", lbl.GetHeight())
	}

	lbl.SetRect(0, 0, 100, 15)
	lbl.Draw(buf)
}

func TestTopWidget(t *testing.T) {
	top := NewTop()
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))

	// Empty draw
	top.SetRect(0, 0, 100, 10)
	top.Draw(buf)
	if top.GetHeight() != 5 {
		t.Fatalf("expected empty top height 5, got %d", top.GetHeight())
	}

	top.Set(models.TopResult{
		Titles: []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
		Processes: [][]string{
			{"root", "1234", "1", "0.0", "12:00", "?", "00:00:01", "/app/server"},
		},
	})
	if len(top.Result.Processes) != 1 {
		t.Fatalf("expected 1 process, got %d", len(top.Result.Processes))
	}
	top.SetRect(0, 0, 100, 15)
	top.Draw(buf)
}

func TestDiffWidget(t *testing.T) {
	diff := NewDiff()
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))

	// Empty draw
	diff.SetRect(0, 0, 100, 10)
	diff.Draw(buf)
	if diff.GetHeight() != 5 {
		t.Fatalf("expected empty diff height 5, got %d", diff.GetHeight())
	}

	diff.Set([]models.Change{
		{Path: "/app/config.json", Kind: 0},
		{Path: "/tmp/app.log", Kind: 1},
		{Path: "/var/cache/old.tmp", Kind: 2},
	})
	if len(diff.Changes) != 3 {
		t.Fatalf("expected 3 changes, got %d", len(diff.Changes))
	}
	diff.SetRect(0, 0, 100, 15)
	diff.Draw(buf)
}

func TestGeneratorWidget(t *testing.T) {
	gen := NewGenerator()
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))

	// Empty draw
	gen.SetRect(0, 0, 100, 10)
	gen.Draw(buf)

	gen.Set("docker run -d --name test redis:alpine", "version: '3.8'\nservices:\n  test:\n    image: redis:alpine")
	if gen.RunCmd == "" || gen.Compose == "" {
		t.Fatalf("expected non-empty run/compose strings")
	}
	gen.SetRect(0, 0, 100, 15)
	gen.Draw(buf)
}

func TestEnvSecretMasking(t *testing.T) {
	env := NewEnv()
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))

	env.Set("DB_PASSWORD=supersecret;API_KEY=12345;LOG_LEVEL=debug")
	env.SetRect(0, 0, 100, 10)
	env.Draw(buf)

	// Verify password masked
	var foundMasked bool
	for _, r := range env.Rows {
		if r[0] == "DB_PASSWORD" && r[1] == "•••••••••••• [masked]" {
			foundMasked = true
		}
	}
	if !foundMasked {
		t.Fatalf("expected DB_PASSWORD to be masked by default")
	}

	// Toggle unmask
	env.ToggleMask()
	foundMasked = false
	for _, r := range env.Rows {
		if r[0] == "DB_PASSWORD" && r[1] == "supersecret" {
			foundMasked = true
		}
	}
	if !foundMasked {
		t.Fatalf("expected DB_PASSWORD to be unmasked after ToggleMask")
	}
}

func TestExplorerWidget(t *testing.T) {
	exp := NewExplorer()
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))

	// Empty draw
	exp.SetRect(0, 0, 100, 10)
	exp.Draw(buf)
	if exp.GetHeight() != 6 {
		t.Fatalf("expected empty explorer height 6, got %d", exp.GetHeight())
	}

	exp.Set("/app", []models.FileInfo{
		{Name: "config.json", Path: "/app/config.json", IsDir: false, Size: 128, Mode: "-rw-r--r--", ModTime: "2026-08-18 12:00:00"},
		{Name: "src", Path: "/app/src", IsDir: true, Mode: "drwxr-xr-x", ModTime: "2026-08-18 12:00:00"},
	})

	if len(exp.TotalItems()) != 3 { // ".." + config.json + src
		t.Fatalf("expected 3 total items, got %d", len(exp.TotalItems()))
	}

	// Host download directory defaults and fallbacks
	if exp.HostDownloadDir != config.GetDownloadDir() {
		t.Fatalf("expected HostDownloadDir to match config.GetDownloadDir(), got %s", exp.HostDownloadDir)
	}
	exp.SetDownloadDir("")
	if exp.HostDownloadDir != config.GetDownloadDir() {
		t.Fatalf("expected SetDownloadDir('') to fall back to config.GetDownloadDir(), got %s", exp.HostDownloadDir)
	}
	exp.SetDownloadDir("/tmp/custom-test")
	if exp.HostDownloadDir != "/tmp/custom-test" {
		t.Fatalf("expected SetDownloadDir to update to /tmp/custom-test, got %s", exp.HostDownloadDir)
	}

	// Navigation
	exp.Down()
	exp.Up()
	item, ok := exp.Selected()
	if !ok || item.Name != ".." {
		t.Fatalf("expected first item to be '..', got %+v", item)
	}

	exp.Down()
	item, ok = exp.Selected()
	if !ok || item.Name != "config.json" {
		t.Fatalf("expected second item to be 'config.json', got %+v", item)
	}

	// Preview mode
	exp.SetPreview("{\"name\": \"ctop\"}")
	if !exp.Previewing {
		t.Fatalf("expected Previewing to be true")
	}
	if exp.PreviewPath != "/app/config.json" {
		t.Fatalf("expected PreviewPath to be '/app/config.json', got '%s'", exp.PreviewPath)
	}
	exp.Draw(buf)

	// Multi-line preview scrolling tests
	var multiLines []string
	for i := 1; i <= 50; i++ {
		multiLines = append(multiLines, fmt.Sprintf("line %d of content", i))
	}
	exp.SetPreview(strings.Join(multiLines, "\n"))
	exp.SetRect(0, 0, 80, 15)
	exp.Draw(buf)
	if exp.PreviewOffset != 0 {
		t.Fatalf("expected initial PreviewOffset 0, got %d", exp.PreviewOffset)
	}

	// Up when already at 0 should stay at 0
	exp.PreviewUp()
	if exp.PreviewOffset != 0 {
		t.Fatalf("expected PreviewOffset to remain 0, got %d", exp.PreviewOffset)
	}

	exp.PreviewDown()
	if exp.PreviewOffset != 1 {
		t.Fatalf("expected PreviewOffset 1 after PreviewDown, got %d", exp.PreviewOffset)
	}

	exp.PreviewUp()
	if exp.PreviewOffset != 0 {
		t.Fatalf("expected PreviewOffset 0 after PreviewUp, got %d", exp.PreviewOffset)
	}

	exp.PreviewEnd()
	if exp.PreviewOffset == 0 {
		t.Fatalf("expected PreviewOffset > 0 after PreviewEnd")
	}
	maxOff := exp.PreviewOffset
	// Down when already at end should not exceed max
	exp.PreviewDown()
	if exp.PreviewOffset != maxOff {
		t.Fatalf("expected PreviewOffset not to exceed maxOff %d, got %d", maxOff, exp.PreviewOffset)
	}

	exp.PreviewHome()
	if exp.PreviewOffset != 0 {
		t.Fatalf("expected PreviewOffset 0 after PreviewHome, got %d", exp.PreviewOffset)
	}

	exp.PreviewPgDown(5)
	if exp.PreviewOffset != 5 {
		t.Fatalf("expected PreviewOffset 5 after PreviewPgDown, got %d", exp.PreviewOffset)
	}

	exp.PreviewPgUp(5)
	if exp.PreviewOffset != 0 {
		t.Fatalf("expected PreviewOffset 0 after PreviewPgUp, got %d", exp.PreviewOffset)
	}

	// Test default page step (step <= 0)
	exp.PreviewPgDown(0)
	if exp.PreviewOffset <= 0 {
		t.Fatalf("expected PreviewPgDown(0) to advance offset, got %d", exp.PreviewOffset)
	}
	exp.PreviewPgUp(0)
	if exp.PreviewOffset != 0 {
		t.Fatalf("expected PreviewPgUp(0) to return to 0, got %d", exp.PreviewOffset)
	}

	exp.Draw(buf)

	exp.ClearPreview()
	if exp.Previewing {
		t.Fatalf("expected Previewing to be false")
	}
	if exp.PreviewOffset != 0 || exp.PreviewPath != "" {
		t.Fatalf("expected ClearPreview to reset offset and path")
	}
	exp.Draw(buf)

	// Test Large List Navigation (PageUp, PageDown, Home, End)
	var largeList []models.FileInfo
	for i := 0; i < 100; i++ {
		largeList = append(largeList, models.FileInfo{
			Name:  fmt.Sprintf("file_%03d.txt", i),
			Path:  fmt.Sprintf("/app/file_%03d.txt", i),
			IsDir: false,
			Size:  int64(i * 10),
		})
	}
	exp.Set("/app", largeList)
	exp.Home()
	if exp.CursorPos != 0 {
		t.Fatalf("expected cursor at 0, got %d", exp.CursorPos)
	}

	// End
	exp.End()
	if exp.CursorPos != 100 { // 1 parent ("..") + 100 files = 101 items, last idx 100
		t.Fatalf("expected cursor at 100 after End, got %d", exp.CursorPos)
	}

	// PgUp
	exp.PgUp(15)
	if exp.CursorPos != 85 {
		t.Fatalf("expected cursor at 85 after PgUp(15), got %d", exp.CursorPos)
	}

	// PgDown
	exp.PgDown(10)
	if exp.CursorPos != 95 {
		t.Fatalf("expected cursor at 95 after PgDown(10), got %d", exp.CursorPos)
	}

	// Home
	exp.Home()
	if exp.CursorPos != 0 {
		t.Fatalf("expected cursor at 0 after Home, got %d", exp.CursorPos)
	}

	// Test inline host target directory editing
	exp.StartEditTargetDir()
	if !exp.EditingTargetDir || exp.EditDirBuffer != exp.HostDownloadDir {
		t.Fatalf("expected EditingTargetDir to be true and buffer to match HostDownloadDir")
	}

	// Draw while editing
	exp.Draw(buf)

	// Cancel with Escape
	done, applied := exp.EditDirKeyPress("<Escape>")
	if !done || applied || exp.EditingTargetDir {
		t.Fatalf("expected Escape to cancel editing without applying")
	}

	// Start again and edit path
	exp.StartEditTargetDir()
	exp.EditDirKeyPress("<C-u>")
	if exp.EditDirBuffer != "" {
		t.Fatalf("expected C-u to clear EditDirBuffer, got %s", exp.EditDirBuffer)
	}

	for _, ch := range "/var/test_dl" {
		exp.EditDirKeyPress(string(ch))
	}
	if exp.EditDirBuffer != "/var/test_dl" {
		t.Fatalf("expected buffer to be '/var/test_dl', got %s", exp.EditDirBuffer)
	}

	// Test backspace and space
	exp.EditDirKeyPress("<Backspace>")
	if exp.EditDirBuffer != "/var/test_d" {
		t.Fatalf("expected buffer to be '/var/test_d', got %s", exp.EditDirBuffer)
	}
	exp.EditDirKeyPress("l")
	exp.EditDirKeyPress("<Space>")
	exp.EditDirKeyPress("<Backspace>")

	// Draw while editing with modified path
	exp.Draw(buf)

	// Apply with Enter
	done, applied = exp.EditDirKeyPress("<Enter>")
	if !done || !applied || exp.EditingTargetDir {
		t.Fatalf("expected Enter to complete and apply editing")
	}
	if exp.HostDownloadDir != "/var/test_dl" {
		t.Fatalf("expected HostDownloadDir to be '/var/test_dl', got %s", exp.HostDownloadDir)
	}
	if config.GetDownloadDir() != "/var/test_dl" {
		t.Fatalf("expected config downloadDir to be '/var/test_dl', got %s", config.GetDownloadDir())
	}
}

func TestExplorerInlineFilterAndSearch(t *testing.T) {
	exp := NewExplorer()
	exp.Set("/app", []models.FileInfo{
		{Name: "config.json", Path: "/app/config.json", IsDir: false, Size: 128},
		{Name: "server.go", Path: "/app/server.go", IsDir: false, Size: 1024},
		{Name: "server_test.go", Path: "/app/server_test.go", IsDir: false, Size: 512},
		{Name: "data", Path: "/app/data", IsDir: true},
	})
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	exp.SetRect(0, 0, 100, 15)

	// 1. Start inline filter
	exp.StartEditFilter()
	if !exp.IsEditing() || exp.EditMode != EditModeFilter {
		t.Fatalf("expected EditModeFilter and IsEditing()=true")
	}
	exp.Draw(buf)

	// Type filter query "server*"
	for _, ch := range "server*" {
		done, applied, _, _ := exp.EditKeyPress(string(ch))
		if done || applied {
			t.Fatalf("keystroke should not complete edit")
		}
	}
	if exp.InputBuffer != "server*" {
		t.Fatalf("expected InputBuffer to be 'server*', got %s", exp.InputBuffer)
	}

	// Test backspace and character re-entry
	exp.EditKeyPress("<Backspace>")
	if exp.InputBuffer != "server" {
		t.Fatalf("expected InputBuffer to be 'server', got %s", exp.InputBuffer)
	}
	exp.EditKeyPress("*")

	// Render while editing
	exp.Draw(buf)

	// Apply filter with Enter
	done, applied, mode, val := exp.EditKeyPress("<Enter>")
	if !done || !applied || mode != EditModeFilter || val != "server*" {
		t.Fatalf("expected applied filter 'server*', got done=%v applied=%v mode=%v val=%s", done, applied, mode, val)
	}
	if exp.IsEditing() {
		t.Fatalf("expected editing to be finished")
	}
	if exp.Filter != "server*" {
		t.Fatalf("expected exp.Filter to be 'server*', got %s", exp.Filter)
	}
	// ".." + server.go + server_test.go = 3 items
	if len(exp.TotalItems()) != 3 {
		t.Fatalf("expected 3 filtered items, got %d", len(exp.TotalItems()))
	}

	// Draw with active filter badge
	exp.Draw(buf)

	// Clear filter using ClearFilterAndSearch
	exp.ClearFilterAndSearch()
	if exp.Filter != "" {
		t.Fatalf("expected filter to be cleared")
	}
	if len(exp.TotalItems()) != 5 { // ".." + 4 entries = 5
		t.Fatalf("expected 5 total items after clear, got %d", len(exp.TotalItems()))
	}

	// 2. Start inline deep search
	exp.StartEditDeepSearch()
	if !exp.IsEditing() || exp.EditMode != EditModeDeepSearch {
		t.Fatalf("expected EditModeDeepSearch and IsEditing()=true")
	}
	exp.Draw(buf)

	// Type search query and cancel with Escape
	for _, ch := range "query" {
		exp.EditKeyPress(string(ch))
	}
	done, applied, _, _ = exp.EditKeyPress("<Escape>")
	if !done || applied || exp.IsEditing() {
		t.Fatalf("expected Escape to cancel search edit")
	}

	// Start deep search again, type and confirm
	exp.StartEditDeepSearch()
	for _, ch := range "main.go" {
		exp.EditKeyPress(string(ch))
	}
	done, applied, mode, val = exp.EditKeyPress("<Enter>")
	if !done || !applied || mode != EditModeDeepSearch || val != "main.go" {
		t.Fatalf("expected deep search confirmed with 'main.go'")
	}

	// Simulate deep search results applied
	exp.Set("/app", []models.FileInfo{
		{Name: "main.go", Path: "/app/cmd/main.go", IsDir: false},
	})
	exp.SetDeepSearch("main.go")
	if !exp.IsDeepSearch || exp.DeepSearchTerm != "main.go" {
		t.Fatalf("expected deep search state active")
	}
	exp.Draw(buf)

	// Clear deep search
	exp.ClearFilterAndSearch()
	if exp.IsDeepSearch || exp.DeepSearchTerm != "" {
		t.Fatalf("expected deep search state cleared")
	}
}

func TestExplorerInlineUpload(t *testing.T) {
	exp := NewExplorer()
	exp.Set("/app", []models.FileInfo{
		{Name: "server.go", Path: "/app/server.go", IsDir: false},
	})
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	exp.SetRect(0, 0, 100, 15)

	// 1. Start inline upload
	exp.StartEditUpload()
	if !exp.IsEditing() || exp.EditMode != EditModeUpload {
		t.Fatalf("expected EditModeUpload and IsEditing()=true")
	}

	for _, ch := range "/tmp/local.tar" {
		exp.EditKeyPress(string(ch))
	}
	exp.Draw(buf)

	// Check buffer contains Upload prompt
	var drawnLines []string
	for y := buf.Min.Y; y < buf.Max.Y; y++ {
		var lineRunes []rune
		for x := buf.Min.X; x < buf.Max.X; x++ {
			cell := buf.GetCell(image.Pt(x, y))
			if cell.Rune != 0 {
				lineRunes = append(lineRunes, cell.Rune)
			}
		}
		if len(lineRunes) > 0 {
			drawnLines = append(drawnLines, strings.TrimSpace(string(lineRunes)))
		}
	}
	joined := strings.Join(drawnLines, "\n")
	if !strings.Contains(joined, "Upload:") {
		t.Fatalf("expected inline upload prompt in header, got:\n%s", joined)
	}

	// 2. Cancel with Escape
	done, applied, mode, _ := exp.EditKeyPress("<Escape>")
	if !done || applied || exp.IsEditing() || mode != EditModeUpload {
		t.Fatalf("expected Escape to cancel upload mode")
	}

	// 3. Start again and confirm with Enter
	exp.StartEditUpload()
	for _, ch := range "/etc/hosts" {
		exp.EditKeyPress(string(ch))
	}
	done, applied, mode, val := exp.EditKeyPress("<Enter>")
	if !done || !applied || mode != EditModeUpload || val != "/etc/hosts" {
		t.Fatalf("expected Enter to apply upload with '/etc/hosts', got %v, %v, %v, %s", done, applied, mode, val)
	}
}

func TestExplorerInlineDeleteAndEditConfirm(t *testing.T) {
	exp := NewExplorer()
	fileItem := models.FileInfo{Name: "app.log", Path: "/var/log/app.log", IsDir: false}
	exp.Set("/var/log", []models.FileInfo{fileItem})
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	exp.SetRect(0, 0, 100, 15)

	// 1. Delete confirmation - Cancel with 'n'
	exp.StartConfirmDelete(fileItem)
	if !exp.IsEditing() || exp.EditMode != EditModeConfirmDelete {
		t.Fatalf("expected EditModeConfirmDelete and IsEditing()=true")
	}
	exp.Draw(buf)

	// Verify prompt is rendered above Directory and not duplicated on Directory line
	var drawnLines []string
	for y := buf.Min.Y; y < buf.Max.Y; y++ {
		var lineRunes []rune
		for x := buf.Min.X; x < buf.Max.X; x++ {
			cell := buf.GetCell(image.Pt(x, y))
			if cell.Rune != 0 {
				lineRunes = append(lineRunes, cell.Rune)
			}
		}
		if len(lineRunes) > 0 {
			drawnLines = append(drawnLines, strings.TrimSpace(string(lineRunes)))
		}
	}
	joinedBuf := strings.Join(drawnLines, "\n")
	if !strings.Contains(joinedBuf, "Delete app.log? [y: confirm, n/Esc: cancel]") {
		t.Fatalf("expected delete confirmation prompt above directory, buf:\n%s", joinedBuf)
	}
	// Verify directory line is clean
	for _, l := range drawnLines {
		if strings.Contains(l, "Directory:") && strings.Contains(l, "Delete app.log?") {
			t.Fatalf("directory line should not contain duplicate delete prompt, got: %s", l)
		}
	}

	done, applied, mode, _ := exp.EditKeyPress("n")
	if !done || applied || exp.IsEditing() || mode != EditModeConfirmDelete {
		t.Fatalf("expected 'n' to cancel delete confirmation")
	}

	// 2. Delete confirmation - Cancel with Escape
	exp.StartConfirmDelete(fileItem)
	done, applied, _, _ = exp.EditKeyPress("<Escape>")
	if !done || applied || exp.IsEditing() {
		t.Fatalf("expected Escape to cancel delete confirmation")
	}

	// 3. Delete confirmation - Confirm with 'y'
	exp.StartConfirmDelete(fileItem)
	done, applied, mode, val := exp.EditKeyPress("y")
	if !done || !applied || mode != EditModeConfirmDelete || val != fileItem.Path {
		t.Fatalf("expected 'y' to confirm delete of %s, got applied=%v val=%s", fileItem.Path, applied, val)
	}

	// 4. Edit confirmation - Cancel with 'n'
	exp.StartConfirmEdit(fileItem)
	if !exp.IsEditing() || exp.EditMode != EditModeConfirmEdit {
		t.Fatalf("expected EditModeConfirmEdit and IsEditing()=true")
	}
	exp.Draw(buf)

	done, applied, mode, _ = exp.EditKeyPress("n")
	if !done || applied || exp.IsEditing() || mode != EditModeConfirmEdit {
		t.Fatalf("expected 'n' to cancel edit confirmation")
	}

	// 5. Edit confirmation - Confirm with 'y'
	exp.StartConfirmEdit(fileItem)
	done, applied, mode, val = exp.EditKeyPress("y")
	if !done || !applied || mode != EditModeConfirmEdit || val != fileItem.Path {
		t.Fatalf("expected 'y' to confirm edit of %s, got applied=%v val=%s", fileItem.Path, applied, val)
	}
}

func TestSingleTabNavigation(t *testing.T) {
	s := NewSingle()
	meta := models.NewMeta("name", "postgres-db", "state", "running")
	meta["[ENV-VAR]"] = "POSTGRES_DB=appdb;POSTGRES_USER=admin;POSTGRES_PASSWORD=secret"
	meta["[MOUNTS]"] = "/var/lib/data:::/opt/volumes/data:::volume:::rw:::"
	meta["[LABELS]"] = "com.docker.compose.project=webapp;;service=db"
	meta["[NETWORKS]"] = "bridge:::172.17.0.2:::172.17.0.1:::02:42:ac:11:00:02:::16"
	meta["cmd"] = "postgres"
	meta["workdir"] = "/var/lib/postgresql"
	s.SetMeta(meta)

	s.SetTop(models.TopResult{
		Titles:    []string{"PID", "CMD"},
		Processes: [][]string{{"1", "postgres"}},
	})
	s.SetDiff([]models.Change{{Path: "/var/lib/postgresql/data", Kind: 0}})
	s.SetGenerator("docker run -d postgres", "version: '3.8'")
	s.SetExplorer("/", []models.FileInfo{{Name: "var", Path: "/var", IsDir: true}})

	buf := ui.NewBuffer(image.Rect(0, 0, 120, 40))

	// Iterate through all 11 tabs
	for tab := 0; tab < TotalTabs; tab++ {
		s.SetTab(tab)
		if s.ActiveTab != tab {
			t.Fatalf("expected ActiveTab %d, got %d", tab, s.ActiveTab)
		}
		s.Align()
		s.Draw(buf)
	}

	// Test NextTab and PrevTab cycling
	s.SetTab(TabWeb)
	s.NextTab()
	if s.ActiveTab != TabMetrics {
		t.Errorf("expected NextTab from TabWeb to cycle to TabMetrics, got %d", s.ActiveTab)
	}

	s.PrevTab()
	if s.ActiveTab != TabWeb {
		t.Errorf("expected PrevTab from TabMetrics to cycle to TabWeb, got %d", s.ActiveTab)
	}

	// Test ToggleSecretMask
	s.ToggleSecretMask()
}

func TestImageWidget(t *testing.T) {
	im := NewImage()
	if im == nil {
		t.Fatal("expected non-nil Image widget")
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 120, 40))
	im.SetRect(0, 0, 120, 30)
	im.Draw(buf)

	meta := map[string]string{
		"imageId":            "sha256:7f3b8901c2d3e4f5a6b7c8d9e0f1a2b3",
		"imageRepoTags":      "authelia/authelia:4.38.19, authelia/authelia:latest",
		"imageRepoDigests":   "authelia/authelia@sha256:abcd1234",
		"imageArch":          "linux/amd64",
		"imageAuthor":        "Authelia Team",
		"imageCreated":       "Mon Jan 02 15:04:05 2026",
		"imageDockerVersion": "24.0.7",
		"imageSize":          "124.50 MB (130548120 bytes)",
		"imageLayers":        "8 layers",
		"imageEntrypoint":    "/app/authelia",
		"imageCmd":           "--config /config/configuration.yml",
		"imageWorkdir":       "/app",
		"imageUser":          "1000:1000",
		"imageExposedPorts":  "9091/tcp",
		"imageVolumes":       "/config",
		"imageEnv":           "PATH=/usr/local/sbin:/usr/local/bin;;AUTHELIA_PORT=9091",
		"imageLabels":        "org.opencontainers.image.title=Authelia;;org.opencontainers.image.version=4.38.19",
	}

	im.Set(meta)
	if im.GetHeight() <= 0 {
		t.Fatalf("expected positive height, got %d", im.GetHeight())
	}
	im.Draw(buf)
}

func TestLogsWidgetWrapAndTruncate(t *testing.T) {
	config.Init()
	l := NewLogs()
	if l == nil {
		t.Fatal("expected non-nil Logs widget")
	}

	// Default wrap should be false
	if l.IsWrap() {
		t.Errorf("expected default Wrap to be false, got true")
	}

	// Test ToggleWrap
	val := l.ToggleWrap()
	if !val || !l.IsWrap() {
		t.Errorf("expected Wrap to be true after toggle")
	}

	// Test SetWrap
	l.SetWrap(false)
	if l.IsWrap() {
		t.Errorf("expected Wrap to be false after SetWrap(false)")
	}

	// Add long log entry
	longMsg := strings.Repeat("ABCDEFGHIJ", 20) // 200 chars
	l.Add(models.Log{
		Timestamp: time.Now(),
		Message:   longMsg,
	})

	// Render truncated (Wrap = false)
	bufTruncated := ui.NewBuffer(image.Rect(0, 0, 80, 20))
	l.SetRect(0, 0, 80, 20)
	l.Draw(bufTruncated)

	// Render wrapped (Wrap = true)
	l.SetWrap(true)
	bufWrapped := ui.NewBuffer(image.Rect(0, 0, 80, 20))
	l.SetRect(0, 0, 80, 20)
	l.Draw(bufWrapped)
}

func TestEnsureIconSpacing(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ℹ Delete cancelled", "ℹ  Delete cancelled"},
		{"ℹDelete cancelled", "ℹ  Delete cancelled"},
		{"ℹ  Delete cancelled", "ℹ  Delete cancelled"},
		{"🗑 Delete foo? [y: confirm, n/Esc: cancel]", "🗑  Delete foo? [y: confirm, n/Esc: cancel]"},
		{"✏ Edit bar? [y: confirm, n/Esc: cancel]", "✏  Edit bar? [y: confirm, n/Esc: cancel]"},
		{"✔ Filter applied", "✔  Filter applied"},
		{"❌ Search error", "❌  Search error"},
		{"🔍 Searching...", "🔍  Searching..."},
		{"📁 Directory: /app   ⬇ Host Target: /tmp", "📁  Directory: /app   ⬇  Host Target: /tmp"},
		{"no icons here", "no icons here"},
	}

	for _, tc := range tests {
		got := ensureIconSpacing(tc.input)
		if got != tc.expected {
			t.Errorf("ensureIconSpacing(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}
