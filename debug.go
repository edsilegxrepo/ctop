// debug.go provides runtime introspection, event logging, and container state dumping.
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
