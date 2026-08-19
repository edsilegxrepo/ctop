// grid_test.go tests screen layout construction, compact row rendering, modal errors, and interactive Display loops.
// Test Strategy: Fast in-memory synthetic event feeds to simulate keyboard/resize events deterministically without real terminals.
package main

import (
	"fmt"
	"testing"

	"github.com/edsilegx/ctop/config"
	"github.com/edsilegx/ctop/cwidgets/compact"
	"github.com/edsilegx/ctop/widgets"
	ui "github.com/gizak/termui/v3"
)

func TestRedrawRowsFull(t *testing.T) {
	config.Init()
	header = widgets.NewCTopHeader()
	status = widgets.NewStatusLine()
	cGrid = compact.NewCompactGrid()

	mockContainers := createMockContainers(3)
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc

	RedrawRows(true)
	RedrawRows(false)

	// Clean up globals
	header = nil
	status = nil
	cGrid = nil
	cursor = nil
}

func TestSingleViewNavigation(t *testing.T) {
	mockContainers := createMockContainers(2)
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc

	mockEvents := make(chan ui.Event, 10)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "k"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents

	fn := SingleView()
	if fn != nil {
		t.Fatal("expected SingleView to return nil on 'q'")
	}

	cursor = nil
}

func TestRefreshDisplayWithCursor(t *testing.T) {
	gc := &GridCursor{}
	cursor = gc
	if err := RefreshDisplay(); err != nil {
		t.Fatalf("unexpected error from RefreshDisplay: %v", err)
	}
	cursor = nil
}

func TestDisplayLoop(t *testing.T) {
	config.Init()
	header = widgets.NewCTopHeader()
	status = widgets.NewStatusLine()
	cGrid = compact.NewCompactGrid()

	mockContainers := createMockContainers(3)
	gc := &GridCursor{
		filtered:   mockContainers,
		selectedID: mockContainers[0].Id,
	}
	cursor = gc

	// Test normal navigation and keys ending with 'q' (exit)
	mockEvents := make(chan ui.Event, 20)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "j"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "k"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "a"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "r"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "H"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "D"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "S"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents

	exit := Display()
	if !exit {
		t.Fatal("expected Display to return true on 'q' (exit)")
	}

	// Test menu transitions through Display
	menuTriggers := []struct {
		openKey string
		exitKey string
	}{
		{"?", "<Escape>"},
		{"f", "<Escape>"},
		{"s", "<Enter>"},
		{"c", "q"},
		{"o", "q"},
		{"l", "q"},
	}

	for _, mt := range menuTriggers {
		mEvents := make(chan ui.Event, 10)
		mEvents <- ui.Event{Type: ui.KeyboardEvent, ID: mt.openKey}
		mEvents <- ui.Event{Type: ui.KeyboardEvent, ID: mt.exitKey}
		uiEvents = mEvents
		_ = Display()
	}

	header = nil
	status = nil
	cGrid = nil
	cursor = nil
}

func TestShowConnError(t *testing.T) {
	errView = widgets.NewErrorView()
	mockEvents := make(chan ui.Event, 10)
	mockEvents <- ui.Event{Type: ui.ResizeEvent}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	uiEvents = mockEvents

	exit := ShowConnError(fmt.Errorf("test connection error"))
	if !exit {
		t.Fatal("expected ShowConnError to return true on 'q'")
	}
	errView = nil
}
