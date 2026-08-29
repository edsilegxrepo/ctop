// mock_test.go validates mock metrics generation and synthetic log stream emitters.
// Test Strategy: Verifies periodic random walk metrics channels and log tickers for offline testing.
package collector

import (
	"testing"
	"time"
)

func TestMockCollector(t *testing.T) {
	mock := NewMock(2)
	if mock == nil {
		t.Fatal("expected non-nil mock collector")
	}

	mock.Start()
	if !mock.Running() {
		t.Fatal("expected Running() to be true after Start()")
	}

	stream := mock.Stream()
	select {
	case m, ok := <-stream:
		if !ok {
			t.Fatal("expected metrics stream to yield item")
		}
		if m.MemLimit <= 0 {
			t.Errorf("expected positive MemLimit, got %d", m.MemLimit)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for mock metrics")
	}

	mock.Stop()
	time.Sleep(50 * time.Millisecond)
	if mock.Running() {
		t.Fatal("expected Running() to be false after Stop()")
	}

	if mock.Logs() == nil {
		t.Fatal("expected non-nil LogCollector from mock.Logs()")
	}
}

func TestMockLogs(t *testing.T) {
	mockLogs := &MockLogs{done: make(chan bool)}
	stream := mockLogs.Stream()

	select {
	case logLine, ok := <-stream:
		if !ok {
			t.Fatal("expected mock log stream to yield line")
		}
		if logLine.Message == "" {
			t.Errorf("expected non-empty message in mock log")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mock logs")
	}

	mockLogs.Stop()
}
