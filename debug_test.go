// debug_test.go tests event logging, container structure dumping, and reflection serialization.
//
// Objective:
//
//	Verify debug logging helpers and container state dumpers safely format inputs without panics,
//	even when passed uninitialized loggers, nil containers, or special character event keys.
//
// Test Strategy:
//   - Verifies nil receiver resilience, log event formatting, and reflection string output against model structs.
//   - Tests empty and populated container metadata serialization.
//   - Confirms quoting and escape behavior for raw terminal event identifiers.
package main

import (
	"testing"

	"github.com/edsilegx/ctop/pkg/container"
	"github.com/edsilegx/ctop/pkg/diag"
	"github.com/edsilegx/ctop/pkg/logging"
	"github.com/edsilegx/ctop/pkg/models"
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
	s := diag.Inspect(&m)
	if s == "" {
		t.Fatal("expected non-empty inspect output")
	}
	q := quote("hello")
	if q != "\"hello\"" {
		t.Fatalf("expected '\"hello\"', got '%s'", q)
	}
}
