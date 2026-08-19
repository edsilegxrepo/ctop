// menus_test.go validates interactive modal dialogs (Help, Filter, Sort, Columns, Container actions, Logs).
// Test Strategy: Pre-buffered event channels supplying deterministic keyboard sequences to verify state mutations and clean modal dismissals without blocking.
package main

import (
	"testing"
	"time"

	"github.com/edsilegx/ctop/config"
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
	mockEvents := make(chan ui.Event, 10)
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Space>"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "h"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "l"}
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
		{"l", 0},
		{"s", 0}, // stop
		{"p", 0}, // pause
		{"r", 0}, // restart
		{"w", 0}, // browser
		{"c", 0}, // cancel
		{"p", 1}, // unpause
		{"s", 2}, // start
		{"R", 2}, // remove
		{"q", 0}, // quit
	}

	for _, tc := range testKeys {
		gc.selectedID = mockContainers[tc.containerIdx].Id
		mockEvents := make(chan ui.Event, 10)
		mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: tc.key}
		mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "c"}
		uiEvents = mockEvents
		_ = ContainerMenu()
	}

	cursor = oldCursor
}

func TestLogMenuAndReader(t *testing.T) {
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

	// Test LogMenu keyboard events
	mockEvents := make(chan ui.Event, 10)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "t"}
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
