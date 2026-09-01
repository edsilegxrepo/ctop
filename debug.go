// debug.go provides runtime introspection, event logging, and container state dumping.
//
// Objective:
//
//	Facilitate troubleshooting and diagnostics by capturing raw UI events and container snapshots
//	into the configured application log stream without interrupting UI loops.
//
// Core Components:
//   - logEvent: UI event interceptor and formatter.
//   - dumpContainer: Container telemetry and metadata snapshot exporter.
//   - quote: String sanitization helper for log outputs.
//
// Functionality:
//   - Formatted debug logging of incoming TermUI keyboard and window events.
//   - Text serialization of active container runtime metrics and metadata.
//
// Data Flow:
//
//	TermUI Events / Container Snapshots -> debug.go -> pkg/logging (File / Socket / STDERR).
package main

import (
	"fmt"

	"github.com/edsilegx/ctop/pkg/container"
	"github.com/edsilegx/ctop/pkg/diag"
	ui "github.com/gizak/termui/v3"
)

// logEvent serializes and writes an incoming TermUI event to the debug log.
func logEvent(e ui.Event) {
	if log == nil {
		return
	}
	s := fmt.Sprintf("Type=%v ID=%s", e.Type, quote(e.ID))
	log.Debugf("new event: %s", s)
}

// dumpContainer logs the complete metadata, metrics, and widget state of a container for debugging.
func dumpContainer(c *container.Container) {
	if c == nil || log == nil {
		return
	}
	log.Infof(diag.DumpText(c.Id, c.Meta, &c.Metrics))
}

// quote wraps a string in double quotation marks.
func quote(s string) string {
	return fmt.Sprintf("\"%s\"", s)
}
