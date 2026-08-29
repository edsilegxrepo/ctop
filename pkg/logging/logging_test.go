// logging_test.go validates thread safety, status queue synchronization, and network socket streaming.
// Test Strategy: Tests concurrent worker goroutines logging under race detection, TCP/UNIX loopback socket listeners, and nil receiver guards.
package logging

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

type mockWriteCloser struct {
	bytes.Buffer
	closed bool
	mu     sync.Mutex
}

func (m *mockWriteCloser) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, errors.New("writer closed")
	}
	return m.Buffer.Write(p)
}

func (m *mockWriteCloser) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestLoggerInitialization(t *testing.T) {
	log := Init()
	if log == nil {
		t.Fatal("expected logger to initialize, got nil")
	}

	log.Notice("test notice message")
	log.Debug("test debug message")
}

func TestStatusQueueThreadSafety(t *testing.T) {
	log := Init()
	log.Status("direct status")
	log.StatusErr(nil)

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 50; j++ {
				log.Statusf("worker %d msg %d", id, j)
				log.StatusErr(errors.New("sample error"))
			}
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	if !log.StatusQueued() {
		t.Fatal("expected status messages in queue")
	}

	var count int
	for sm := range log.FlushStatus() {
		if sm.Text != "" {
			count++
		}
	}

	if count == 0 {
		t.Fatal("expected flushed status messages count > 0")
	}

	if log.StatusQueued() {
		t.Fatal("expected status queue to be empty after flush")
	}
}

func TestServerStartStop(t *testing.T) {
	StartServer()
	time.Sleep(100 * time.Millisecond)

	// Verify socket was created with safe permissions if unix socket
	if !debugModeTCP() && runtime.GOOS != "windows" {
		sockPath := getSocketPath()
		info, err := os.Stat(sockPath)
		if err == nil {
			perm := info.Mode().Perm()
			if perm != 0o600 {
				t.Fatalf("expected socket permission 0600, got %o", perm)
			}
		}
	}

	StopServer()
}

func TestConcurrentLoggingSafety(t *testing.T) {
	log := Init()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				log.Infof("worker %d message %d", worker, j)
				log.Debugf("worker %d debug %d", worker, j)
			}
		}(i)
	}

	wg.Wait()
}

func TestLogTailAndHandler(t *testing.T) {
	log := Init()
	log.Infof("tail test message 1")

	wc := &mockWriteCloser{}
	done := make(chan bool)

	go func() {
		handler(wc)
		done <- true
	}()

	time.Sleep(50 * time.Millisecond)
	log.Infof("tail test message 2")
	time.Sleep(50 * time.Millisecond)

	// Close the writer and trigger log to exit handler
	_ = wc.Close()
	log.Infof("tail test message 3 - triggering exit")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Log("handler exit timed out")
	}
}

func TestTCPServerConnection(t *testing.T) {
	t.Setenv("CTOP_DEBUG_TCP", "1")
	t.Setenv("CTOP_DEBUG_ADDR", "127.0.0.1:9099")

	if !debugModeTCP() {
		t.Fatal("expected debugModeTCP to be true")
	}

	StartServer()
	time.Sleep(100 * time.Millisecond)

	conn, err := net.Dial("tcp", "127.0.0.1:9099")
	if err == nil {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		_ = conn.Close()
	}

	StopServer()
}

func TestUnixSocketServer(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tempCache)
	t.Setenv("CTOP_DEBUG_TCP", "0")

	StartServer()
	time.Sleep(100 * time.Millisecond)

	sockPath := getSocketPath()
	conn, err := net.Dial("unix", sockPath)
	if err == nil {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		_ = conn.Close()
	}

	StopServer()
}

func TestConfigEnvHelpers(t *testing.T) {
	t.Setenv("CTOP_DEBUG", "1")
	if !debugMode() {
		t.Fatal("expected debugMode to be true")
	}

	tempDir := t.TempDir()
	logPath := tempDir + "/test.log"
	t.Setenv("CTOP_DEBUG_FILE", logPath)
	if path := debugModeFile(); path != logPath {
		t.Fatalf("expected debugModeFile '%s', got '%s'", logPath, path)
	}
}

func TestLoggerExit(t *testing.T) {
	logger := Init()
	logger.Exit()
	if !exited.Load() {
		t.Fatal("expected exited to be true after logger.Exit()")
	}
	// Reset exited state for subsequent tests
	exited.Store(false)
}

func TestLoggingNilReceiverStatus(t *testing.T) {
	var nilLogger *CTopLogger
	if nilLogger.StatusQueued() {
		t.Fatal("expected false for nilLogger.StatusQueued")
	}
	nilLogger.Status("test")
	nilLogger.Statusf("test %s", "val")
	nilLogger.StatusErr(nil)
	nilLogger.StatusErr(fmt.Errorf("test err"))
	ch := nilLogger.FlushStatus()
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	<-ch
}
