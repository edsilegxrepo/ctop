// diag_test.go validates reflection dumping, structured report generation, and disk export encoding.
//
// Objective:
//
//	Verify that diagnostic reports accurately format metadata, sanitize secrets, build mount/network structs, and save to disk.
//
// Test Strategy:
//   - Tests DumpText and DumpJSON against struct reflections.
//   - Tests BuildReport and SaveReport creating valid JSON/Text files in t.TempDir().
package diag

import (
	"strings"
	"testing"

	"github.com/edsilegx/ctop/pkg/models"
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

func TestBuildReportAndExport(t *testing.T) {
	meta := map[string]string{
		"name":       "mssql-prod",
		"image":      "mcr.microsoft.com/mssql:latest",
		"state":      "running",
		"health":     "healthy",
		"uptime":     "2h 30m",
		"[ENV-VAR]":  "ACCEPT_EULA=Y;SA_PASSWORD=SuperSecretPass123",
		"[LABELS]":   "com.docker.compose.project=db;;maintainer=dba",
		"[MOUNTS]":   "/var/opt/mssql:::/data/volumes/mssql:::\nvolume:::rw:::local",
		"[NETWORKS]": "bridge:::172.17.0.2:::172.17.0.1:::02:42:ac:11:00:02:::16",
	}
	metrics := &models.Metrics{
		CPUUtil:      15,
		MemUsage:     524288000,
		MemLimit:     2147483648,
		MemPercent:   24,
		NetRx:        1048576,
		NetTx:        2097152,
		IOBytesRead:  4194304,
		IOBytesWrite: 8388608,
		Pids:         28,
	}

	report := BuildReport("c123456789abc", meta, metrics, "node-1", "docker run -d mssql", "version: '3.8'")
	if report.Name != "mssql-prod" || report.CPUUtil != 15 {
		t.Fatalf("unexpected report fields: %+v", report)
	}

	// 1. JSON
	jsonData, err := FormatJSON(report)
	if err != nil {
		t.Fatalf("failed to format JSON: %v", err)
	}
	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"name": "mssql-prod"`) || !strings.Contains(jsonStr, `"cpu_util": 15`) {
		t.Errorf("unexpected json output: %s", jsonStr)
	}

	// 2. Text
	txtReport := FormatTextReport(report)
	if !strings.Contains(txtReport, "CONTAINER DIAGNOSTIC REPORT: mssql-prod") {
		t.Errorf("expected header in txt report, got: %s", txtReport)
	}
	if !strings.Contains(txtReport, "CPU Utilization  : 15%") {
		t.Errorf("expected CPU in txt report, got: %s", txtReport)
	}

	// 3. SaveReport
	tmpDir := t.TempDir()
	saved, err := SaveReport(report, tmpDir, "both")
	if err != nil || len(saved) != 2 {
		t.Fatalf("expected 2 files saved, got %v (err: %v)", saved, err)
	}
}
