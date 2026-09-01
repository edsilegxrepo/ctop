// menus_test.go validates interactive modal dialogs (Help, Filter, Sort, Columns, Container actions, Logs).
// Test Strategy: Pre-buffered event channels supplying deterministic keyboard sequences to verify state mutations and clean modal dismissals without blocking.
package main

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/config"
	ui "github.com/gizak/termui/v3"
)

func TestConfirmTxt(t *testing.T) {
	res := confirmTxt("restart", "nginx")
	expected := "restart container nginx?"
	if res != expected {
		t.Fatalf("expected '%s', got '%s'", expected, res)
	}
}

func TestToggleLog(t *testing.T) {
	fixedTime := time.Date(2026, 8, 18, 12, 34, 56, 0, time.UTC)
	tl := &toggleLog{
		timestamp: fixedTime,
		message:   "server started",
	}

	// Toggle false -> message only
	if msg := tl.Toggle(false); msg != "server started" {
		t.Fatalf("expected 'server started', got '%s'", msg)
	}

	// Toggle true -> timestamp + message
	expectedWithTs := fixedTime.Local().Format("2006-01-02 15:04:05.000") + "  server started"
	if msg := tl.Toggle(true); msg != expectedWithTs {
		t.Fatalf("expected '%s', got '%s'", expectedWithTs, msg)
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

func TestLogMenuAndReader(t *testing.T) {
	tempDir := t.TempDir()
	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer func() { _ = os.Chdir(origWd) }()

	// LogMenu when cursor is nil
	oldCursor := cursor
	cursor = nil
	if fn := LogMenu(); fn != nil {
		t.Fatal("expected nil when cursor is nil")
	}

	mockContainers := createMockContainers(1)
	mockContainers[0].SetMeta("name", "nginx-test")
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc

	// Test logReader with nil collector
	logsCh, quitCh := logReader(mockContainers[0])
	if logsCh != nil {
		// channel should be closed
		<-logsCh
	}
	if quitCh != nil {
		select {
		case quitCh <- true:
		default:
		}
	}

	// Configure isolated temp directory for log exports
	config.Update("downloadDir", t.TempDir())

	// Test LogMenu keyboard events including D (set dir) and s (export logs)
	mockEvents := make(chan ui.Event, 20)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "t"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "D"}        // open directory prompt
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Escape>"} // cancel
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "D"}        // open directory prompt again
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "t"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "m"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "p"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"} // apply
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "s"}       // export logs
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "/"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "e"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "r"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents

	fn := LogMenu()
	if fn != nil {
		t.Fatal("expected LogMenu to return nil on 'q'")
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

	mockEvents := make(chan ui.Event, 30)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "k"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}     // enter dir
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Backspace>"} // parent dir
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}           // move to file
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "v"}           // preview file
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Escape>"}    // close preview
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "d"}           // download file
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "D"}           // download target dialog
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "t"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "m"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "p"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"} // apply download target
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "u"}       // upload dialog
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

	// OpenInBrowser without Web Port
	fnBrowserNoPort := OpenInBrowser()
	if fnBrowserNoPort != nil {
		t.Fatal("expected nil when no Web Port set")
	}

	// OpenInBrowser with Web Port
	var openedURL string
	origOpen := openBrowserURL
	openBrowserURL = func(u string) error {
		openedURL = u
		return nil
	}
	defer func() { openBrowserURL = origOpen }()

	mockContainers[0].SetMeta("Web Port", "localhost:8080")
	fnBrowserWithPort := OpenInBrowser()
	if fnBrowserWithPort != nil {
		t.Fatal("expected nil returned from OpenInBrowser")
	}
	if openedURL != "http://localhost:8080/" {
		t.Fatalf("expected openedURL 'http://localhost:8080/', got '%s'", openedURL)
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

func TestLogMenuHighThroughputStress(t *testing.T) {
	initTheme()
	config.Init()
	config.Update("downloadDir", t.TempDir())

	mockContainers := createMockContainers(1)
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc
	defer func() { cursor = nil }()

	mockEvents := make(chan ui.Event, 20)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "t"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "/"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "e"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "r"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "r"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "s"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents

	done := make(chan bool)
	go func() {
		fn := LogMenu()
		if fn != nil {
			t.Errorf("expected LogMenu to return nil on 'q'")
		}
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("DEADLOCK DETECTED: LogMenu hung under streaming log load")
	}
}

func TestGoroutineLeakVerification(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	TestModalRapidLifecycleStress(t)
	TestLogMenuHighThroughputStress(t)

	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	if diff := finalGoroutines - initialGoroutines; diff > 8 {
		t.Fatalf("POTENTIAL GOROUTINE LEAK: %d goroutines remained active after tests (initial=%d, final=%d)",
			diff, initialGoroutines, finalGoroutines)
	}
}
