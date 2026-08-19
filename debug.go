// debug.go provides runtime introspection, event logging, and container state dumping.
package main

import (
	"fmt"
	"reflect"

	"github.com/edsilegx/ctop/container"
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
	msg := fmt.Sprintf("logging state for container: %s\n", c.Id)
	for k, v := range c.Meta {
		msg += fmt.Sprintf("Meta.%s = %s\n", k, v)
	}
	msg += inspect(&c.Metrics)
	log.Infof(msg)
}

// inspect uses reflection to dynamically serialize all struct fields and values into a string.
func inspect(i interface{}) (s string) {
	val := reflect.ValueOf(i)
	elem := val.Type().Elem()

	eName := elem.String()
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		fieldVal := reflect.Indirect(val).Field(i)
		s += fmt.Sprintf("%s.%s = ", eName, field.Name)
		s += fmt.Sprintf("%v (%s)\n", fieldVal, field.Type)
	}
	return s
}

// quote wraps a string in double quotation marks.
func quote(s string) string {
	return fmt.Sprintf("\"%s\"", s)
}
