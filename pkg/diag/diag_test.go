package diag

import (
	"strings"
	"testing"
)

type sampleMetrics struct {
	CPU int
	Mem int64
}

func TestDumpText(t *testing.T) {
	meta := map[string]string{
		"name":  "test-container",
		"image": "alpine:latest",
	}
	m := sampleMetrics{CPU: 45, Mem: 1048576}

	out := DumpText("c123456", meta, &m)
	if !strings.Contains(out, "logging state for container: c123456") {
		t.Errorf("expected header with container ID, got: %s", out)
	}
	if !strings.Contains(out, "Meta.name = test-container") {
		t.Errorf("expected Meta.name, got: %s", out)
	}
	if !strings.Contains(out, "sampleMetrics.CPU = 45") {
		t.Errorf("expected sampleMetrics.CPU, got: %s", out)
	}
}

func TestDumpJSON(t *testing.T) {
	meta := map[string]string{
		"name": "test-json",
	}
	m := sampleMetrics{CPU: 10, Mem: 2048}

	data, err := DumpJSON("c999", meta, m)
	if err != nil {
		t.Fatalf("failed to dump JSON: %v", err)
	}
	str := string(data)
	if !strings.Contains(str, `"id": "c999"`) {
		t.Errorf("expected id in JSON, got: %s", str)
	}
	if !strings.Contains(str, `"name": "test-json"`) {
		t.Errorf("expected name in JSON, got: %s", str)
	}
}

func TestInspectNil(t *testing.T) {
	if res := Inspect(nil); res != "" {
		t.Errorf("expected empty string for nil inspect, got: %s", res)
	}
}
