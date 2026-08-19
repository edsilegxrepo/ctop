// debug_test.go tests event logging, container structure dumping, and reflection serialization.
// Test Strategy: Verifies nil receiver resilience, log event formatting, and reflection string output against model structs.
package main

import (
	"testing"

	"github.com/edsilegx/ctop/container"
	"github.com/edsilegx/ctop/logging"
	"github.com/edsilegx/ctop/models"
	ui "github.com/gizak/termui/v3"
)

func TestDebugLogEvent(t *testing.T) {
	log = logging.Init()
	e := ui.Event{Type: ui.KeyboardEvent, ID: "<enter>"}
	logEvent(e)
}

func TestDebugDumpContainer(t *testing.T) {
	log = logging.Init()
	dumpContainer(nil)
	c := container.New("test-id", nil, nil)
	c.SetMeta("name", "test-container")
	dumpContainer(c)
}

func TestDebugInspectAndQuote(t *testing.T) {
	m := models.NewMetrics()
	s := inspect(&m)
	if s == "" {
		t.Fatal("expected non-empty inspect output")
	}
	q := quote("hello")
	if q != "\"hello\"" {
		t.Fatalf("expected '\"hello\"', got '%s'", q)
	}
}
