// Package audit_test provides unit and concurrency test suites for NDJSON audit logging.
//
// Test Strategy:
//   - Schema Compliance: Test that logged NDJSON records contain all mandatory fields and valid JSON.
//   - Daily Rotation: Test that mock date transitions cleanly create new dated files and flush previous files.
//   - Concurrency & Thread Safety: Test parallel logging across 30+ goroutines with zero data loss or corrupted lines.
//   - Global Singleton Safety: Test that package-level helper functions handle nil and initialized states safely.
package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuditLoggerBasicNDJSON(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.ndjson")

	logger, err := NewLogger(logPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Write access event
	err = logger.Log(Event{
		Level:      LevelInfo,
		Category:   CategoryAccess,
		Action:     "http_request",
		ClientIP:   "192.168.1.100",
		Method:     "GET",
		Path:       "/api/v1/containers",
		Status:     200,
		DurationMS: 1.45,
		Auth: &AuthInfo{
			Type:          "bearer",
			Authenticated: true,
			TokenPrefix:   "9469...",
		},
		Details: map[string]interface{}{
			"user_agent": "curl/7.76.1",
			"tls":        "TLSv1.3",
		},
	})
	if err != nil {
		t.Fatalf("failed to log event: %v", err)
	}

	// Write auth event
	err = logger.Log(Event{
		Level:    LevelWarn,
		Category: CategoryAuth,
		Action:   "login_failure",
		ClientIP: "10.0.0.5",
		Details: map[string]interface{}{
			"reason": "invalid_token",
		},
	})
	if err != nil {
		t.Fatalf("failed to log auth event: %v", err)
	}

	_ = logger.Close()

	activeFile := logger.ActivePath()
	if activeFile == "" {
		t.Fatalf("expected non-empty active file path")
	}

	data, err := os.ReadFile(activeFile)
	if err != nil {
		t.Fatalf("failed to read written audit file %s: %v", activeFile, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d:\n%s", len(lines), string(data))
	}

	for i, line := range lines {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatalf("line %d is not valid JSON: %v\nContent: %s", i, err, line)
		}
		if raw["timestamp"] == nil || raw["category"] == nil || raw["action"] == nil {
			t.Fatalf("line %d missing mandatory audit schema keys: %+v", i, raw)
		}
	}
}

func TestAuditLoggerDailyRotation(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.ndjson")

	logger, err := NewLogger(logPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Day 1: 2026-09-01
	day1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	logger.nowFunc = func() time.Time { return day1 }

	if err := logger.Log(Event{Category: CategoryApp, Action: "day1_event"}); err != nil {
		t.Fatalf("failed to log day 1 event: %v", err)
	}

	pathDay1 := logger.ActivePath()
	if !strings.Contains(pathDay1, "2026-09-01") {
		t.Fatalf("expected day 1 path to contain 2026-09-01, got %s", pathDay1)
	}

	// Day 2: 2026-09-02 (Midnight transition)
	day2 := time.Date(2026, 9, 2, 0, 0, 1, 0, time.UTC)
	logger.nowFunc = func() time.Time { return day2 }

	if err := logger.Log(Event{Category: CategoryApp, Action: "day2_event"}); err != nil {
		t.Fatalf("failed to log day 2 event: %v", err)
	}

	pathDay2 := logger.ActivePath()
	if !strings.Contains(pathDay2, "2026-09-02") {
		t.Fatalf("expected day 2 path to contain 2026-09-02, got %s", pathDay2)
	}

	if pathDay1 == pathDay2 {
		t.Fatalf("expected daily rotation to create different file paths, got same: %s", pathDay1)
	}

	_ = logger.Close()

	// Verify both files exist and contain their respective events
	data1, err := os.ReadFile(pathDay1)
	if err != nil || !strings.Contains(string(data1), "day1_event") {
		t.Fatalf("day 1 file contents corrupted: %v / %s", err, string(data1))
	}

	data2, err := os.ReadFile(pathDay2)
	if err != nil || !strings.Contains(string(data2), "day2_event") {
		t.Fatalf("day 2 file contents corrupted: %v / %s", err, string(data2))
	}
}

func TestAuditLoggerConcurrentSafety(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit_concurrent.log")

	logger, err := NewLogger(logPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	const goroutines = 20
	const eventsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				_ = logger.Log(Event{
					Category: CategoryAccess,
					Action:   "concurrent_test",
					Details: map[string]interface{}{
						"goroutine": id,
						"seq":       i,
					},
				})
			}
		}(g)
	}

	wg.Wait()
	_ = logger.Close()

	// Verify all records written without corrupt lines
	file, err := os.Open(logger.ActivePath())
	if err != nil {
		t.Fatalf("failed to open audit file: %v", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatalf("unmarshal error on concurrent line: %v (line: %s)", err, line)
		}
		count++
	}

	expected := goroutines * eventsPerGoroutine
	if count != expected {
		t.Fatalf("expected %d total audit records, counted %d", expected, count)
	}
}

func TestGlobalAuditHelpers(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "global_audit.ndjson")

	_, err := Init(logPath)
	if err != nil {
		t.Fatalf("failed to initialize global audit: %v", err)
	}
	defer Close()

	LogAccess("127.0.0.1", "GET", "/api/v1/containers", 200, 500*time.Microsecond, &AuthInfo{Type: "loopback", Authenticated: true}, nil)
	LogAuth("login_success", LevelInfo, "192.168.1.5", &AuthInfo{Type: "session", Authenticated: true}, nil)
	LogContainer("container_start", "cid_123", "my-web", map[string]interface{}{"image": "nginx:alpine"})
	LogApp("startup", LevelInfo, map[string]interface{}{"version": "0.9.2"})

	Close()

	// Verify helpers when global logger is nil (no-op safety)
	LogAccess("127.0.0.1", "GET", "/test", 200, 0, nil, nil)
	LogAuth("test", LevelInfo, "127.0.0.1", nil, nil)
	LogContainer("test", "c1", "name", nil)
	LogApp("test", LevelInfo, nil)
}

func TestAuditLoggerConcurrentRotationStress(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "stress_audit.ndjson")

	logger, err := NewLogger(logPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	const goroutines = 30
	const eventsPerGoroutine = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for e := 0; e < eventsPerGoroutine; e++ {
				_ = logger.Log(Event{
					Level:    LevelInfo,
					Category: CategoryAccess,
					Action:   "stress_event",
					ClientIP: "10.0.0.1",
					Details: map[string]interface{}{
						"g_id": id,
						"e_id": e,
					},
				})
			}
		}(g)
	}

	wg.Wait()
	_ = logger.Close()

	// Verify the written file contains exactly goroutines * eventsPerGoroutine valid NDJSON lines
	file, err := os.Open(logger.ActivePath())
	if err != nil {
		t.Fatalf("failed to open stress audit file: %v", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatalf("corrupted JSON on line %d: %v\nContent: %s", count+1, err, line)
		}
		count++
	}

	expected := goroutines * eventsPerGoroutine
	if count != expected {
		t.Fatalf("expected %d total audit records, got %d", expected, count)
	}
}
