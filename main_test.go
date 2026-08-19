// main_test.go validates CLI argument parsing, help output, theme initialization, and shutdown lifecycle.
// Test Strategy: Fast headless execution using simulated stdout buffers and safe non-panicking recovery checks.
package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/edsilegx/ctop/config"
	"github.com/edsilegx/ctop/container"
	"github.com/edsilegx/ctop/logging"
	ui "github.com/gizak/termui/v3"
)

func TestInitTheme(t *testing.T) {
	initTheme()
	InvertColorMap()
	if ui.Theme.Block.Title.Fg == 0 {
		t.Log("theme initialized")
	}
}

func TestPrintHelp(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printHelp()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	if !bytes.Contains(buf.Bytes(), []byte("ctop - interactive container viewer")) {
		t.Fatalf("expected help message, got %s", buf.String())
	}
}

func TestValidSort(t *testing.T) {
	validSort("name")
	validSort("cpu")
	validSort("mem")
}

func TestPanicExit(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("recovered: %v", r)
		}
	}()
	// Non-panicking path
	panicExit()
}

func TestMenusWithMockEvents(t *testing.T) {
	config.Init()
	mockEvents := make(chan ui.Event, 10)
	uiEvents = mockEvents

	// Test HelpMenu
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "q"}
	fn := HelpMenu()
	if fn != nil {
		t.Fatalf("expected HelpMenu to return nil after 'q'")
	}

	// Test SortMenu
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	fn = SortMenu()
	if fn != nil {
		t.Fatalf("expected SortMenu to return nil after Enter")
	}

	// Test FilterMenu
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "a"}
	mockEvents <- ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"}
	fn = FilterMenu()
	if fn != nil {
		t.Fatalf("expected FilterMenu to return nil after Enter")
	}
}

func TestRedrawRowsSafe(t *testing.T) {
	config.Init()
	// RedrawRows should be safely callable when UI is not fully initialized
	RedrawRows(false)
	RedrawRows(true)
}

func TestShutdown(t *testing.T) {
	log = logging.Init()
	Shutdown()
	log = nil
}

func TestDefaultAllContainersVisible(t *testing.T) {
	config.Init()
	// Ensure default state is true
	if !config.GetSwitchVal("allContainers") {
		t.Fatalf("expected allContainers to be true by default")
	}

	createC := func(id, name, state string) *container.Container {
		c := container.New(id, &mockCursorCollector{}, &mockCursorManager{})
		c.SetMeta("name", name)
		c.SetState(state)
		return c
	}

	// Verify that containers of all states are visible when allContainers is true
	cRunning := createC("1", "run", "running")
	cPaused := createC("2", "pause", "paused")
	cExited := createC("3", "exit", "exited")
	cCreated := createC("4", "create", "created")

	list := container.Containers{cRunning, cPaused, cExited, cCreated}
	list.Filter()

	for _, c := range list {
		if !c.Display {
			t.Errorf("expected container %s (state: %s) to be displayed by default, got Display=false",
				c.GetMeta("name"), c.GetMeta("state"))
		}
	}
}
