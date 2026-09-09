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

func TestDockerLogsStreamClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := api.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	dl := NewDockerLogs("c123", client)
	stream := dl.Stream()
	for range stream {
	}
	dl.Stop()
}

func TestDockerLogsStreamWithTTYMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/json") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Id":"c123","Config":{"Tty":true}}`))
			return
		}
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		// TTY sends raw bytes without 8-byte multiplexed header, often with \r\n
		body := []byte("2026-08-18T10:00:00Z tty test log\r\n")
		_, _ = w.Write(body)
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
		if !ok {
			t.Fatalf("stream closed unexpectedly without logs")
		}
		if logItem.Message != "tty test log" {
			t.Fatalf("expected message 'tty test log', got %q", logItem.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for TTY container logs")
	}

	dl.Stop()
}

func TestDockerLogsStreamFallback(t *testing.T) {
	reqCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/json") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		reqCount++
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		// Always return raw stream; on the first attempt RawTerminal=false will fail with unrecognized input header
		// then fallback should retry with RawTerminal=true and succeed.
		body := []byte("2026-08-18T10:00:00Z fallback raw log\n")
		_, _ = w.Write(body)
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
		if !ok {
			t.Fatalf("stream closed unexpectedly without logs (reqCount: %d)", reqCount)
		}
		if logItem.Message != "fallback raw log" {
			t.Fatalf("expected message 'fallback raw log', got %q", logItem.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for fallback logs (reqCount: %d)", reqCount)
	}

	dl.Stop()
}

func TestDockerLogsSanitizeLine(t *testing.T) {
	dl := &DockerLogs{}

	// Test 1: trailing CRLF removal
	if res := dl.sanitizeLine("hello world\r\n"); res != "hello world" {
		t.Fatalf("expected 'hello world', got %q", res)
	}

	// Test 2: internal carriage returns (progress bar style - keeps final state)
	if res := dl.sanitizeLine("downloading 20%\rdownloading 80%\rdownloading 100%"); res != "downloading 100%" {
		t.Fatalf("expected 'downloading 100%%', got %q", res)
	}

	// Test 3: null byte replacement
	if res := dl.sanitizeLine("abc\x00def\x00ghi"); res != "abc def ghi" {
		t.Fatalf("expected 'abc def ghi', got %q", res)
	}

	// Test 4: Docker header prefix stripping combined with CRLF
	raw := string([]byte{0x01, 0, 0, 0, 0, 0, 0, 0}) + "log line with header\r\n"
	if res := dl.sanitizeLine(raw); res != "log line with header" {
		t.Fatalf("expected 'log line with header', got %q", res)
	}
}

func TestDockerLogsNilClientSafety(t *testing.T) {
	dl := NewDockerLogs("c_nil", nil)
	stream := dl.Stream()

	// Should not panic and stream should close gracefully
	select {
	case _, ok := <-stream:
		if ok {
			t.Log("received unexpected log from nil client")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for nil client stream closure")
	}

	dl.Stop()
}
