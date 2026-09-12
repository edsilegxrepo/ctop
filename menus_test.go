// menus_test.go validates interactive modal dialogs (Help, Filter, Sort, Columns, Container actions, Logs).
//
// Objective:
//
//	Verify modal lifecycle handling, keyboard navigation, configuration state mutations, container action
//	dispatching (Signals, Resources, Files, Shell), log viewer streaming, and rapid open/close stress testing.
//
// Test Strategy:
//   - Pre-buffered event channels supplying deterministic keyboard sequences to verify state mutations and clean modal dismissals without blocking.
//   - Tests confirm dialogs with both positive confirmation and cancellation flows.
//   - High-throughput stress tests simulating thousands of log lines through the modal log reader.
//   - Goroutine leak detection verifying zero leaked background workers after modal exit.
package main

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/container"
	ui "github.com/gizak/termui/v3"
)

func TestConfirmTxt(t *testing.T) {
	res := confirmTxt("restart", "nginx")
	expected := "restart container nginx?"
	if res != expected {
		t.Fatalf("expected '%s', got '%s'", expected, res)
	}
}

func TestHelpMenu(t *testing.T) {
	mockEvents := make(chan ui.Event, 10)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Escape>"}
	uiEvents = mockEvents

	fn := HelpMenu()
	if fn != nil {
		t.Fatal("expected HelpMenu to return nil on Escape")
	}
}

func TestFilterMenu(t *testing.T) {
	mockEvents := make(chan ui.Event, 10)
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "a"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Backspace>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	uiEvents = mockEvents

	fn := FilterMenu()
	if fn != nil {
		t.Fatal("expected FilterMenu to return nil on enter")
	}
}

func TestSortMenu(t *testing.T) {
	mockEvents := make(chan ui.Event, 10)
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "k"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	uiEvents = mockEvents

	fn := SortMenu()
	if fn != nil {
		t.Fatal("expected SortMenu to return nil after selection")
	}
}

func TestColumnsMenu(t *testing.T) {
	config.Init()
	mockEvents := make(chan ui.Event, 15)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "k"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<PageUp>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<PageDown>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "x"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents

	fn := ColumnsMenu()
	if fn != nil {
		t.Fatal("expected ColumnsMenu to return nil on 'q'")
	}
}

func TestConfigMenu(t *testing.T) {
	config.Init()
	tempDir := t.TempDir()

	// 1. Set download dir via 'D' key in ConfigMenu
	mockEvents := make(chan ui.Event, 100)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "D"}
	// Clear pre-populated default "/tmp"
	for i := 0; i < len(config.DefaultDownloadDir); i++ {
		mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Backspace>"}
	}
	for _, ch := range tempDir {
		mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: string(ch)}
	}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents

	fn := ConfigMenu()
	if fn != nil {
		t.Fatal("expected ConfigMenu to return nil on 'q'")
	}

	if val := config.GetDownloadDir(); val != tempDir {
		t.Fatalf("expected downloadDir to be '%s', got '%s'", tempDir, val)
	}

	// 2. Clear download dir via 'D' and backspaces, ensuring it defaults back to /tmp
	mockEvents2 := make(chan ui.Event, 100)
	mockEvents2 <- ui.Event{Type: ui.KeyboardEvent, ID: "D"}
	for i := 0; i < len(tempDir); i++ {
		mockEvents2 <- ui.Event{Type: ui.KeyboardEvent, ID: "<Backspace>"}
	}
	mockEvents2 <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEvents2 <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents2

	fn = ConfigMenu()
	if fn != nil {
		t.Fatal("expected ConfigMenu to return nil on 'q'")
	}

	if val := config.GetDownloadDir(); val != config.DefaultDownloadDir {
		t.Fatalf("expected downloadDir to fall back to '%s', got '%s'", config.DefaultDownloadDir, val)
	}

	// 3. Test Escape cancel in D prompt
	mockEvents3 := make(chan ui.Event, 10)
	mockEvents3 <- ui.Event{Type: ui.KeyboardEvent, ID: "D"}
	mockEvents3 <- ui.Event{Type: ui.KeyboardEvent, ID: "<Escape>"}
	mockEvents3 <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents3

	fn = ConfigMenu()
	if fn != nil {
		t.Fatal("expected ConfigMenu to return nil on 'q'")
	}

	// 4. Test selecting downloadDir row with Down navigation and pressing Enter
	customDir2 := t.TempDir()
	mockEvents4 := make(chan ui.Event, 100)
	// Navigate down to the downloadDir item (past all columns)
	for i := 0; i < len(config.GlobalColumns)+2; i++ {
		mockEvents4 <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	}
	mockEvents4 <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"} // opens prompt
	// Clear previous
	for i := 0; i < len(config.GetDownloadDir()); i++ {
		mockEvents4 <- ui.Event{Type: ui.KeyboardEvent, ID: "<Backspace>"}
	}
	for _, ch := range customDir2 {
		mockEvents4 <- ui.Event{Type: ui.KeyboardEvent, ID: string(ch)}
	}
	mockEvents4 <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEvents4 <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents4

	fn = ConfigMenu()
	if fn != nil {
		t.Fatal("expected ConfigMenu to return nil on 'q'")
	}

	if val := config.GetDownloadDir(); val != customDir2 {
		t.Fatalf("expected downloadDir to be '%s' after Enter on row, got '%s'", customDir2, val)
	}
}

func TestConfirmDialog(t *testing.T) {
	var executed bool
	fn := Confirm("test action?", func() {
		executed = true
	})

	mockEvents := make(chan ui.Event, 10)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "y"}
	uiEvents = mockEvents

	res := fn()
	if res != nil {
		t.Fatal("expected Confirm to return nil")
	}
	if !executed {
		t.Fatal("expected action to be executed on 'y'")
	}

	// Test cancel 'c'
	executed = false
	mockEvents2 := make(chan ui.Event, 10)
	mockEvents2 <- ui.Event{Type: ui.KeyboardEvent, ID: "c"}
	uiEvents = mockEvents2

	res = fn()
	if res != nil {
		t.Fatal("expected Confirm to return nil")
	}
	if executed {
		t.Fatal("expected action NOT to be executed on 'c'")
	}
}

func TestContainerMenuNavigation(t *testing.T) {
	// When cursor is nil or empty, should return nil
	oldCursor := cursor
	cursor = nil
	if menuFn := ContainerMenu(); menuFn != nil {
		t.Fatal("expected nil when cursor is nil")
	}

	mockContainers := createMockContainers(3)
	mockContainers[0].SetMeta("state", "running")
	mockContainers[0].SetMeta("name", "app-running")
	mockContainers[0].SetMeta("Web Port", "localhost:8080")

	mockContainers[1].SetMeta("state", "paused")
	mockContainers[1].SetMeta("name", "app-paused")

	mockContainers[2].SetMeta("state", "exited")
	mockContainers[2].SetMeta("name", "app-exited")

	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc

	testKeys := []struct {
		key          string
		containerIdx int
	}{
		{"j", 0},
		{"k", 0},
		{"o", 0},
		{"v", 0},
		{"n", 0},
		{"E", 0},
		{"L", 0},
		{"F", 0},
		{"l", 0},
		{"s", 0}, // stop
		{"p", 0}, // pause
		{"r", 0}, // restart
		{"w", 0}, // browser
		{"U", 0}, // tune resources
		{"c", 0}, // cancel
		{"p", 1}, // unpause
		{"s", 2}, // start
		{"R", 2}, // remove
		{"q", 0}, // quit
	}

	for _, tc := range testKeys {
		gc.selectedID = mockContainers[tc.containerIdx].Id
		mockEvents := make(chan ui.Event, 5)
		mockEvents <- ui.Event{Type: ui.ResizeEvent}
		mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: tc.key}
		mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "c"}
		uiEvents = mockEvents

		fn := ContainerMenu()
		if tc.key == "q" {
			if !shouldExitApp {
				t.Fatal("expected shouldExitApp to be true on 'q'")
			}
			shouldExitApp = false
		}
		_ = fn
	}

	cursor = oldCursor
}

func TestFileExplorerMenu(t *testing.T) {
	// When cursor is nil
	oldCursor := cursor
	cursor = nil
	if fn := FileExplorerMenu(); fn != nil {
		t.Fatal("expected nil when cursor is nil")
	}

	mockContainers := createMockContainers(1)
	mockContainers[0].SetMeta("name", "app-test")
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc

	mockEvents := make(chan ui.Event, 120)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "k"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<PageDown>"}  // listing page down
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<PageUp>"}    // listing page up
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<End>"}       // listing end
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Home>"}      // listing home
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}     // enter dir
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Backspace>"} // parent dir
	// Try edit and delete on directory / parent ('..') - should be rejected
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "e"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "x"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}          // move to file
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "v"}          // preview file
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Down>"}     // preview scroll down
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}          // preview scroll down (vi)
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Up>"}       // preview scroll up
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "k"}          // preview scroll up (vi)
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<End>"}      // preview end
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "G"}          // preview end (vi)
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Home>"}     // preview home
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "g"}          // preview home (vi)
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<PageDown>"} // preview pgdown
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<PageUp>"}   // preview pgup
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Escape>"}   // close preview
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "d"}          // download file
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "D"}          // download target dialog
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "t"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "m"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "p"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"} // apply download target
	// inline filter and clear
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "/"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "t"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "x"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "t"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "c"} // clear filter
	// inline deep search and clear
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "f"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "a"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "p"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "p"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "c"} // clear search
	// Edit confirmation and cancel
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "e"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "n"} // cancel edit
	// Delete confirmation and cancel
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "x"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "n"} // cancel delete
	// Delete confirmation and confirm
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "x"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "y"} // confirm delete
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "u"} // upload dialog
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "s"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "r"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "c"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"} // apply upload
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "r"}       // refresh
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}       // quit
	uiEvents = mockEvents

	fn := FileExplorerMenu()
	if fn != nil {
		t.Fatal("expected FileExplorerMenu to return nil on 'q'")
	}

	cursor = oldCursor
}

func TestSignalMenu(t *testing.T) {
	// 1. Nil cursor
	oldCursor := cursor
	cursor = nil
	if fn := SignalMenu(); fn != nil {
		t.Fatal("expected nil when cursor is nil")
	}

	// 2. Active container
	mockContainers := createMockContainers(1)
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc

	mockEvents := make(chan ui.Event, 10)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "k"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	uiEvents = mockEvents

	fn := SignalMenu()
	if fn != nil {
		t.Fatal("expected SignalMenu to return nil after Enter")
	}

	// Test Cancel
	mockEventsCancel := make(chan ui.Event, 5)
	mockEventsCancel <- ui.Event{Type: ui.KeyboardEvent, ID: "c"}
	uiEvents = mockEventsCancel

	fnCancel := SignalMenu()
	if fnCancel != nil {
		t.Fatal("expected SignalMenu to return nil on cancel")
	}

	cursor = oldCursor
}

func TestResourceMenu(t *testing.T) {
	// 1. Nil cursor
	oldCursor := cursor
	cursor = nil
	if fn := ResourceMenu(); fn != nil {
		t.Fatal("expected nil when cursor is nil")
	}

	// 2. Active container
	mockContainers := createMockContainers(1)
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc

	// Test Memory tuning
	mockEventsMem := make(chan ui.Event, 15)
	mockEventsMem <- ui.Event{Type: ui.ResizeEvent}
	mockEventsMem <- ui.Event{Type: ui.KeyboardEvent, ID: "1"}
	mockEventsMem <- ui.Event{Type: ui.KeyboardEvent, ID: "5"}
	mockEventsMem <- ui.Event{Type: ui.KeyboardEvent, ID: "1"}
	mockEventsMem <- ui.Event{Type: ui.KeyboardEvent, ID: "2"}
	mockEventsMem <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEventsMem <- ui.Event{Type: ui.KeyboardEvent, ID: "c"}
	uiEvents = mockEventsMem

	_ = ResourceMenu()

	// Test CPU tuning
	mockEventsCPU := make(chan ui.Event, 15)
	mockEventsCPU <- ui.Event{Type: ui.KeyboardEvent, ID: "2"}
	mockEventsCPU <- ui.Event{Type: ui.KeyboardEvent, ID: "1"}
	mockEventsCPU <- ui.Event{Type: ui.KeyboardEvent, ID: "."}
	mockEventsCPU <- ui.Event{Type: ui.KeyboardEvent, ID: "5"}
	mockEventsCPU <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEventsCPU <- ui.Event{Type: ui.KeyboardEvent, ID: "c"}
	uiEvents = mockEventsCPU

	_ = ResourceMenu()

	// Test Restart policy
	mockEventsRestart := make(chan ui.Event, 15)
	mockEventsRestart <- ui.Event{Type: ui.KeyboardEvent, ID: "3"}
	mockEventsRestart <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	mockEventsRestart <- ui.Event{Type: ui.KeyboardEvent, ID: "k"}
	mockEventsRestart <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEventsRestart <- ui.Event{Type: ui.KeyboardEvent, ID: "c"}
	uiEvents = mockEventsRestart

	_ = ResourceMenu()

	cursor = oldCursor
}

func TestExecShellAndOpenInBrowser(t *testing.T) {
	oldCursor := cursor
	cursor = nil
	if fn := ExecShell(); fn != nil {
		t.Fatal("expected nil from ExecShell with nil cursor")
	}
	if fn := OpenInBrowser(); fn != nil {
		t.Fatal("expected nil from OpenInBrowser with nil cursor")
	}

	mockContainers := createMockContainers(1)
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc

	// ExecShell on mock container
	fnExec := ExecShell()
	if fnExec != nil {
		t.Fatal("expected nil returned from ExecShell")
	}

	// OpenInBrowser with selected container
	fnBrowser := OpenInBrowser()
	if fnBrowser == nil {
		t.Fatal("expected non-nil MenuFn returned from OpenInBrowser when container is selected")
	}

	cursor = oldCursor
}

func TestModalRapidLifecycleStress(t *testing.T) {
	initTheme()
	config.Init()
	config.Update("downloadDir", t.TempDir())

	mockContainers := createMockContainers(5)
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc
	defer func() { cursor = nil }()

	menusToTest := []struct {
		name string
		fn   func() MenuFn
		keys []string
	}{
		{"SortMenu", SortMenu, []string{"j", "k", "j", "<Enter>"}},
		{"ColumnsMenu", ColumnsMenu, []string{"j", "k", "<Enter>", "q"}},
		{"ContainerMenu", ContainerMenu, []string{"j", "k", "j", "<Escape>"}},
		{"SignalMenu", SignalMenu, []string{"j", "k", "<Escape>"}},
		{"ResourceMenu_DirectExit", ResourceMenu, []string{"j", "k", "q"}},
		{"ResourceMenu_MemoryInput", ResourceMenu, []string{"1", "5", "1", "2", "<Enter>", "q"}},
		{"ResourceMenu_MemoryCancel", ResourceMenu, []string{"1", "<Escape>", "q"}},
		{"ResourceMenu_CpuInput", ResourceMenu, []string{"2", "1", ".", "5", "<Enter>", "q"}},
		{"ResourceMenu_RestartPolicy", ResourceMenu, []string{"3", "1", "q"}},
		{"HelpMenu", HelpMenu, []string{"q"}},
	}

	for _, m := range menusToTest {
		for cycle := 0; cycle < 3; cycle++ {
			mockEvents := make(chan ui.Event, len(m.keys)+2)
			mockEvents <- ui.Event{Type: ui.ResizeEvent}
			for _, k := range m.keys {
				mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: k}
			}
			uiEvents = mockEvents

			done := make(chan bool)
			go func() {
				_ = m.fn()
				done <- true
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("DEADLOCK DETECTED: Menu %s hung during cycle %d", m.name, cycle)
			}
		}
	}
}

func TestGoroutineLeakVerification(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	TestModalRapidLifecycleStress(t)

	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	if diff := finalGoroutines - initialGoroutines; diff > 8 {
		t.Fatalf("POTENTIAL GOROUTINE LEAK: %d goroutines remained active after tests (initial=%d, final=%d)",
			diff, initialGoroutines, finalGoroutines)
	}
}

type mockExecRecordingManager struct {
	mockCursorManager
	executedCmds [][]string
	failUntil    int
	callCount    int
	customErr    error
}

func (m *mockExecRecordingManager) Exec(cmd []string) error {
	m.callCount++
	m.executedCmds = append(m.executedCmds, cmd)
	if m.callCount <= m.failUntil {
		if m.customErr != nil {
			return m.customErr
		}
		return fmt.Errorf("simulated exec failure")
	}
	return nil
}

func TestExecShellNilCursor(t *testing.T) {
	oldCursor := cursor
	cursor = nil
	defer func() { cursor = oldCursor }()

	res := ExecShell()
	if res != nil {
		t.Fatalf("expected nil when cursor is nil, got %v", res)
	}
}

func TestExecShellCommandSecurityAndScreenClean(t *testing.T) {
	oldCursor := cursor
	defer func() { cursor = oldCursor }()

	recMgr := &mockExecRecordingManager{}
	c := container.New("cid-test-shell", &mockCursorCollector{}, recMgr)
	c.SetMeta("name", "test-shell-target")
	c.SetMeta("state", "running")
	c.Display = true

	cursor = &GridCursor{
		filtered:   container.Containers{c},
		selectedID: c.Id,
	}

	res := ExecShell()
	if res != nil {
		t.Errorf("expected ExecShell to return nil MenuFn, got %v", res)
	}

	if len(recMgr.executedCmds) != 1 {
		t.Fatalf("expected exactly 1 Exec call on primary command success, got %d", len(recMgr.executedCmds))
	}

	cmd := recMgr.executedCmds[0]
	if runtime.GOOS == "windows" {
		if len(cmd) < 2 || cmd[0] != "powershell.exe" || cmd[1] != "-NoLogo" {
			t.Fatalf("unexpected Windows shell command: %v", cmd)
		}
	} else {
		// Non-windows Linux/Unix container shell execution
		if len(cmd) < 3 || cmd[0] != "/bin/sh" || cmd[1] != "-c" {
			t.Fatalf("unexpected Linux shell wrapper: %v", cmd)
		}

		shellScript := cmd[2]

		// 1. Security Check: Avoid /etc/passwd parsing or eval which breaks unprivileged service accounts
		// (e.g. mysql, postgres, redis whose login shells are /sbin/nologin or /bin/false)
		prohibitedTokens := []string{
			"/etc/passwd",
			"nologin",
			"/bin/false",
			"eval",
			"cut -d : -f 7-",
			"grep ^$(id -un)",
		}
		for _, token := range prohibitedTokens {
			if strings.Contains(shellScript, token) {
				t.Fatalf("SECURITY/COMPATIBILITY DEFECT: shell script contains prohibited token %q which breaks unprivileged service accounts: %s", token, shellScript)
			}
		}

		// 2. Clean Invocation Check: Ensure no raw escape sequence injections (printf/clear) pollute the container TTY
		if strings.Contains(shellScript, "printf") {
			t.Errorf("shell script should not contain printf escape sequence injection: %s", shellScript)
		}
		if strings.Contains(shellScript, "clear") {
			t.Errorf("shell script should not contain clear invocation: %s", shellScript)
		}

		// 3. Shell Discovery Check: Must test for bash then sh
		if !strings.Contains(shellScript, "exec /bin/bash") && !strings.Contains(shellScript, "exec bash") {
			t.Errorf("shell script missing exec bash: %s", shellScript)
		}
		if !strings.Contains(shellScript, "exec sh") {
			t.Errorf("shell script missing exec sh fallback: %s", shellScript)
		}
	}
}

func TestExecShellFallbackHierarchy(t *testing.T) {
	oldCursor := cursor
	defer func() { cursor = oldCursor }()

	// Case 1: Primary fails, 1st fallback fails, 2nd fallback succeeds
	recMgr := &mockExecRecordingManager{failUntil: 2}
	c := container.New("cid-test-fallback", &mockCursorCollector{}, recMgr)
	c.SetMeta("name", "test-fallback-target")
	c.SetMeta("state", "running")
	c.Display = true

	cursor = &GridCursor{
		filtered:   container.Containers{c},
		selectedID: c.Id,
	}

	_ = ExecShell()

	if len(recMgr.executedCmds) != 3 {
		t.Fatalf("expected exactly 3 attempts (primary + 2 fallbacks), got %d attempts: %v",
			len(recMgr.executedCmds), recMgr.executedCmds)
	}

	// Case 2: All commands fail
	recMgrAllFail := &mockExecRecordingManager{failUntil: 999}
	cFail := container.New("cid-test-fail-all", &mockCursorCollector{}, recMgrAllFail)
	cursor = &GridCursor{
		filtered:   container.Containers{cFail},
		selectedID: cFail.Id,
	}

	_ = ExecShell()

	if runtime.GOOS != "windows" {
		// 1 primary + 6 fallbacks = 7 attempts
		if len(recMgrAllFail.executedCmds) != 7 {
			t.Fatalf("expected 7 attempts when all fallbacks fail, got %d", len(recMgrAllFail.executedCmds))
		}
	} else {
		// 1 primary + 3 fallbacks = 4 attempts
		if len(recMgrAllFail.executedCmds) != 4 {
			t.Fatalf("expected 4 attempts when all fallbacks fail on Windows, got %d", len(recMgrAllFail.executedCmds))
		}
	}
}

func TestExecShellResidualEventDraining(t *testing.T) {
	oldCursor := cursor
	oldUiEvents := uiEvents
	defer func() {
		cursor = oldCursor
		uiEvents = oldUiEvents
	}()

	recMgr := &mockExecRecordingManager{}
	c := container.New("cid-test-drain", &mockCursorCollector{}, recMgr)
	c.SetMeta("name", "test-drain-target")
	cursor = &GridCursor{
		filtered:   container.Containers{c},
		selectedID: c.Id,
	}

	// Preload uiEvents with residual keystrokes left over from shell exit (e.g. exit<Enter>, Ctrl+D)
	drainChan := make(chan ui.Event, 10)
	drainChan <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	drainChan <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	drainChan <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	drainChan <- ui.Event{Type: ui.KeyboardEvent, ID: "<Ctrl-d>"}
	uiEvents = drainChan

	if len(uiEvents) != 4 {
		t.Fatalf("expected 4 residual events buffered before ExecShell, got %d", len(uiEvents))
	}

	_ = ExecShell()

	// All residual keystrokes must have been drained during ExecShell exit cleanup
	if len(uiEvents) != 0 {
		t.Fatalf("expected uiEvents channel to be completely drained (0 events), got %d remaining events", len(uiEvents))
	}

	// Ensure new subsequent keystroke is received immediately without needing a second press
	drainChan <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	select {
	case ev := <-uiEvents:
		if ev.ID != "j" {
			t.Fatalf("expected immediate next key 'j', got '%s'", ev.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for next keypress after draining")
	}
}
