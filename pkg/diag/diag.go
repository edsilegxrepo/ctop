// Package diag provides introspection, container state dumping, reflection inspection, and JSON diagnostic exporting.
package diag

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// ContainerSnapshot represents a complete serialized diagnostics snapshot of a container.
type ContainerSnapshot struct {
	ID      string            `json:"id"`
	Meta    map[string]string `json:"meta"`
	Metrics any               `json:"metrics"`
}

// DumpText formats container state into a human-readable diagnostic text dump.
func DumpText(id string, meta map[string]string, metrics any) string {
	msg := fmt.Sprintf("logging state for container: %s\n", id)
	for k, v := range meta {
		msg += fmt.Sprintf("Meta.%s = %s\n", k, v)
	}
	if metrics != nil {
		msg += Inspect(metrics)
	}
	return msg
}

// DumpJSON serializes container metadata and telemetry into pretty-printed JSON.
func DumpJSON(id string, meta map[string]string, metrics any) ([]byte, error) {
	snapshot := ContainerSnapshot{
		ID:      id,
		Meta:    meta,
		Metrics: metrics,
	}
	return json.MarshalIndent(snapshot, "", "  ")
}

// Inspect uses reflection to format struct fields and types into key-value lines.
func Inspect(i any) (s string) {
	if i == nil {
		return ""
	}
	val := reflect.ValueOf(i)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}

	elem := val.Type()
	eName := elem.String()
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		fieldVal := val.Field(i)
		s += fmt.Sprintf("%s.%s = %v (%s)\n", eName, field.Name, fieldVal, field.Type)
	}
	return s
}
