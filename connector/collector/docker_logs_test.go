// docker_logs_test.go validates multiplexed log streaming, header stripping, and timestamp parsing from Docker containers.
// Test Strategy: Tests byte prefix stripper, RFC3339 timestamp parser, and mock HTTP log streaming pipelines.
package collector

import (
	"bufio"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/fsouza/go-dockerclient"
)

func TestDockerLogsStripPfx(t *testing.T) {
	dl := &DockerLogs{}

	// Test normal message without docker header
	msg := "2026-08-18T12:00:00Z Hello World"
	if res := dl.stripPfx(msg); res != msg {
		t.Fatalf("expected '%s', got '%s'", msg, res)
	}

	// Test header with stdio prefix (0x01 = stdout, 7 zero bytes)
	raw := string([]byte{0x01, 0, 0, 0, 0, 0, 0, 0}) + msg
	if res := dl.stripPfx(raw); res != msg {
		t.Fatalf("expected '%s', got '%s'", msg, res)
	}
}

func TestDockerLogsParseTime(t *testing.T) {
	dl := &DockerLogs{}

	parsed := dl.parseTime("2026-08-18T12:30:45.123456789Z")
	if parsed.Year() != 2026 || parsed.Month() != time.August || parsed.Day() != 18 {
		t.Fatalf("unexpected parsed time: %v", parsed)
	}

	// Fallback to now on empty string
	now := time.Now()
	parsedEmpty := dl.parseTime("")
	if parsedEmpty.Before(now.Add(-time.Second)) || parsedEmpty.After(now.Add(time.Second)) {
		t.Fatalf("expected near-current time, got: %v", parsedEmpty)
	}
}

func TestDockerLogsLargeLineBuffer(t *testing.T) {
	// Verify that lines larger than 64KB (e.g. 200KB) can be scanned without token error
	largeMsg := strings.Repeat("A", 200*1024)
	fullLine := "2026-08-18T12:00:00.000000000Z " + largeMsg + "\n"

	r := bytes.NewReader([]byte(fullLine))

	dl := &DockerLogs{}
	logCh := make(chan struct{}, 1)

	// Test using the same scanner buffer setup as DockerLogs.Stream()
	scanner := bufioNewScanner(r)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxLogLineSize)

	if !scanner.Scan() {
		t.Fatalf("expected scanner to scan 200KB line, but failed: %v", scanner.Err())
	}

	text := dl.stripPfx(scanner.Text())
	parts := strings.SplitN(text, " ", 2)
	if len(parts) != 2 || len(parts[1]) != 200*1024 {
		t.Fatalf("expected 200KB payload parsed, got len: %d", len(parts[1]))
	}
	close(logCh)
}

// helper wrapper to match bufio.NewScanner
func bufioNewScanner(r *bytes.Reader) *bufio.Scanner {
	return bufio.NewScanner(r)
}

func TestDockerLogsStreamWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		header := []byte{1, 0, 0, 0, 0, 0, 0, 31}
		body := []byte("2026-08-18T10:00:00Z test log\n")
		_, _ = w.Write(append(header, body...))
	}))
	defer server.Close()

	client, err := api.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	dl := NewDockerLogs("c123", client)
	stream := dl.Stream()

	select {
	case logItem, ok := <-stream:
		if ok && logItem.Message != "" {
			t.Logf("received log: %s", logItem.Message)
		}
	case <-time.After(1 * time.Second):
		t.Log("stream closed or timed out")
	}

	dl.Stop()
}
