// docker_test.go validates container process management, multiplexed stream writers, and Docker API endpoints.
// Test Strategy: Tests frame writers, non-closable stream readers, and mock HTTP Docker REST handlers for Start/Stop/Pause/Unpause/Restart/Remove.
package manager

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/fsouza/go-dockerclient"
)

func TestNoClosableReader(t *testing.T) {
	input := "hello world"
	reader := &noClosableReader{Reader: strings.NewReader(input)}

	buf := make([]byte, len(input))
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if n != len(input) || string(buf) != input {
		t.Fatalf("expected '%s', got '%s'", input, string(buf))
	}
}

func TestFrameWriterStdout(t *testing.T) {
	var stdout, stderr, stdin bytes.Buffer
	fw := &frameWriter{
		stdout: &stdout,
		stderr: &stderr,
		stdin:  &stdin,
	}

	// Frame header: [1 (STDOUT), 0, 0, 0, 0, 0, 0, 5] + "hello"
	payload := []byte("hello")
	frame := append([]byte{STDOUT, 0, 0, 0, 0, 0, 0, 5}, payload...)

	n, err := fw.Write(frame)
	if err != nil {
		t.Fatalf("unexpected error writing stdout frame: %v", err)
	}
	if n != len(frame) {
		t.Fatalf("expected written len %d, got %d", len(frame), n)
	}
	if stdout.String() != "hello" {
		t.Fatalf("expected stdout 'hello', got '%s'", stdout.String())
	}
	if stderr.Len() != 0 || stdin.Len() != 0 {
		t.Fatal("expected stderr and stdin to remain empty")
	}
}

func TestFrameWriterStderr(t *testing.T) {
	var stdout, stderr, stdin bytes.Buffer
	fw := &frameWriter{
		stdout: &stdout,
		stderr: &stderr,
		stdin:  &stdin,
	}

	payload := []byte("error message")
	frame := append([]byte{STDERR, 0, 0, 0, 0, 0, 0, byte(len(payload))}, payload...)

	n, err := fw.Write(frame)
	if err != nil {
		t.Fatalf("unexpected error writing stderr frame: %v", err)
	}
	if n != len(frame) {
		t.Fatalf("expected written len %d, got %d", len(frame), n)
	}
	if stderr.String() != "error message" {
		t.Fatalf("expected stderr 'error message', got '%s'", stderr.String())
	}
}

func TestFrameWriterStdin(t *testing.T) {
	var stdout, stderr, stdin bytes.Buffer
	fw := &frameWriter{
		stdout: &stdout,
		stderr: &stderr,
		stdin:  &stdin,
	}

	payload := []byte("input echo")
	frame := append([]byte{STDIN, 0, 0, 0, 0, 0, 0, byte(len(payload))}, payload...)

	n, err := fw.Write(frame)
	if err != nil {
		t.Fatalf("unexpected error writing stdin frame: %v", err)
	}
	if n != len(frame) {
		t.Fatalf("expected written len %d, got %d", len(frame), n)
	}
	if stdin.String() != "input echo" {
		t.Fatalf("expected stdin 'input echo', got '%s'", stdin.String())
	}
}

func TestFrameWriterEmptyAndInvalid(t *testing.T) {
	var stdout, stderr, stdin bytes.Buffer
	fw := &frameWriter{
		stdout: &stdout,
		stderr: &stderr,
		stdin:  &stdin,
	}

	// Empty payload
	n, err := fw.Write([]byte{})
	if err != nil || n != 0 {
		t.Fatalf("expected 0 bytes and no error for empty slice, got n=%d err=%v", n, err)
	}

	// Short invalid header (< 8 bytes)
	_, err = fw.Write([]byte{1, 2, 3})
	if err != wrongFrameFormat {
		t.Fatalf("expected wrongFrameFormat error for short slice, got %v", err)
	}

	// Invalid stream type (> 2)
	invalidFrame := []byte{99, 0, 0, 0, 0, 0, 0, 4, 't', 'e', 's', 't'}
	_, err = fw.Write(invalidFrame)
	if err != wrongFrameFormat {
		t.Fatalf("expected wrongFrameFormat error for invalid stream type, got %v", err)
	}
}

func TestMockAndRuncManagers(t *testing.T) {
	mockMgr := NewMock()
	if err := mockMgr.Start(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := mockMgr.Stop(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := mockMgr.Remove(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := mockMgr.Pause(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := mockMgr.Unpause(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := mockMgr.Restart(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := mockMgr.Exec([]string{"ls"}); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}

	runcMgr := NewRunc()
	if err := runcMgr.Start(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := runcMgr.Stop(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := runcMgr.Remove(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := runcMgr.Pause(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := runcMgr.Unpause(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := runcMgr.Restart(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := runcMgr.Exec([]string{"ls"}); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
}

func TestDockerManagerLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/containers/test-c/json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Id":"test-c","HostConfig":{}}`))
		case strings.HasPrefix(r.URL.Path, "/containers/test-c/"):
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/containers/test-c":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	client, err := api.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	mgr := NewDocker(client, "test-c")

	if err := mgr.Start(); err != nil {
		t.Fatalf("failed to Start: %v", err)
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("failed to Stop: %v", err)
	}
	if err := mgr.Pause(); err != nil {
		t.Fatalf("failed to Pause: %v", err)
	}
	if err := mgr.Unpause(); err != nil {
		t.Fatalf("failed to Unpause: %v", err)
	}
	if err := mgr.Restart(); err != nil {
		t.Fatalf("failed to Restart: %v", err)
	}
	if err := mgr.Remove(); err != nil {
		t.Fatalf("failed to Remove: %v", err)
	}
}
