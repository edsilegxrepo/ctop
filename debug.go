package main

import (
	"fmt"
	"reflect"

	"github.com/bcicen/ctop/container"
	ui "github.com/gizak/termui/v3"
)

func logEvent(e ui.Event) {
	s := fmt.Sprintf("Type=%v ID=%s", e.Type, quote(e.ID))
	log.Debugf("new event: %s", s)
}

// log container, metrics, and widget state
func dumpContainer(c *container.Container) {
	if c == nil {
		return
	}
	msg := fmt.Sprintf("logging state for container: %s\n", c.Id)
	for k, v := range c.Meta {
		msg += fmt.Sprintf("Meta.%s = %s\n", k, v)
	}
	msg += inspect(&c.Metrics)
	log.Infof(msg)
}

func inspect(i interface{}) (s string) {
	val := reflect.ValueOf(i)
	elem := val.Type().Elem()

	eName := elem.String()
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		fieldVal := reflect.Indirect(val).FieldByName(field.Name)
		s += fmt.Sprintf("%s.%s = ", eName, field.Name)
		s += fmt.Sprintf("%v (%s)\n", fieldVal, field.Type)
	}
	return s
}

func quote(s string) string {
	return fmt.Sprintf("\"%s\"", s)
}
