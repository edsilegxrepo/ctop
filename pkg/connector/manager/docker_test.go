// docker_test.go validates container process management, multiplexed stream writers, and Docker API endpoints.
//
// Objective:
//
//	Verify Docker container actions (Start, Stop, Pause, Kill, Exec, Top, ReadDir, ReadFile, Upload, Download, UpdateResources)
//	against mock HTTP Docker daemon endpoints and stream multiplexers.
//
// Test Strategy:
//   - Tests frame writers, non-closable stream readers, and mock HTTP Docker REST handlers for Start/Stop/Pause/Unpause/Restart/Remove.
//   - Validates tar archive creation and streaming for in-container file upload and download operations.
//   - Verifies Top process output parsing and live resource update payloads.
package manager

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/edsilegx/ctop/internal/theme"
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
	if err != errWrongFrameFormat {
		t.Fatalf("expected errWrongFrameFormat error for short slice, got %v", err)
	}

	// Invalid stream type (> 2)
	invalidFrame := []byte{99, 0, 0, 0, 0, 0, 0, 4, 't', 'e', 's', 't'}
	_, err = fw.Write(invalidFrame)
	if err != errWrongFrameFormat {
		t.Fatalf("expected errWrongFrameFormat error for invalid stream type, got %v", err)
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

func TestDockerManagerExtendedMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/top"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Titles":["PID","USER","CMD"],"Processes":[["1001","root","nginx"]]}`))
		case strings.HasSuffix(r.URL.Path, "/changes"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"Path":"/etc/nginx.conf","Kind":0},{"Path":"/var/log","Kind":1}]`))
		case strings.HasSuffix(r.URL.Path, "/archive") && r.Method == "GET":
			// Produce a small valid tar stream with a file and directory
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			_ = tw.WriteHeader(&tar.Header{
				Name:     "etc/",
				Typeflag: tar.TypeDir,
				Mode:     0o755,
			})
			_ = tw.WriteHeader(&tar.Header{
				Name:     "etc/hosts",
				Typeflag: tar.TypeReg,
				Size:     12,
				Mode:     0o644,
			})
			_, _ = tw.Write([]byte("127.0.0.1 lo\n"))
			_ = tw.Close()
			w.Header().Set("Content-Type", "application/x-tar")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buf.Bytes())
		case strings.HasSuffix(r.URL.Path, "/archive") && (r.Method == "PUT" || r.Method == "POST"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/update"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/kill"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/exec"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"Id":"exec-123"}`))
		case strings.Contains(r.URL.Path, "/exec/exec-123/start"):
			w.WriteHeader(http.StatusOK)
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

	mgr := NewDocker(client, "test-ext")

	// Test Exec
	_ = mgr.Exec([]string{"echo", "hi"})

	// 1. Kill with various signal formats
	for _, sig := range []string{"SIGTERM", "9", "HUP", "SIGWINCH", "15", "INVALID_FALLBACK"} {
		if err := mgr.Kill(sig); err != nil {
			t.Errorf("failed to Kill with %s: %v", sig, err)
		}
	}

	// 2. Top
	topRes, err := mgr.Top("aux")
	if err != nil {
		t.Fatalf("failed to Top: %v", err)
	}
	if len(topRes.Titles) != 3 || len(topRes.Processes) != 1 {
		t.Errorf("unexpected top result: %+v", topRes)
	}

	// 3. Changes
	changes, err := mgr.Changes()
	if err != nil {
		t.Fatalf("failed to Changes: %v", err)
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(changes))
	}

	// 4. ReadDir
	entries, err := mgr.ReadDir("/etc")
	if err != nil {
		t.Fatalf("failed to ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Errorf("expected non-empty entries from ReadDir")
	}

	// 5. ReadFile
	content, err := mgr.ReadFile("/etc/hosts", 1024)
	if err != nil {
		t.Fatalf("failed to ReadFile: %v", err)
	}
	if !strings.Contains(content, "127.0.0.1") {
		t.Errorf("unexpected ReadFile content: %s", content)
	}

	// 6. Download
	tempDst := filepath.Join(t.TempDir(), "downloaded_hosts")
	n, err := mgr.Download("/etc/hosts", tempDst)
	if err != nil {
		t.Fatalf("failed to Download: %v", err)
	}
	if n == 0 {
		t.Errorf("expected downloaded bytes > 0")
	}

	// 7. Upload
	tempSrc := filepath.Join(t.TempDir(), "upload.txt")
	_ = os.WriteFile(tempSrc, []byte("test upload"), 0o644)
	if err := mgr.Upload(tempSrc, "/tmp"); err != nil {
		t.Fatalf("failed to Upload file: %v", err)
	}

	// Upload directory
	tempDirSrc := t.TempDir()
	_ = os.WriteFile(filepath.Join(tempDirSrc, "sub.txt"), []byte("sub"), 0o644)
	if err := mgr.Upload(tempDirSrc, "/tmp/dir"); err != nil {
		t.Fatalf("failed to Upload dir: %v", err)
	}

	// 8. UpdateResources
	if err := mgr.UpdateResources(512, 1.5, "always"); err != nil {
		t.Fatalf("failed to UpdateResources: %v", err)
	}
}

func TestMockAndRuncExtendedCoverage(t *testing.T) {
	mockMgr := NewMock()
	if err := mockMgr.Kill("9"); err != nil {
		t.Errorf("unexpected error from mock Kill: %v", err)
	}
	if topRes, err := mockMgr.Top("aux"); err != nil || len(topRes.Titles) == 0 {
		t.Errorf("unexpected mock Top: %+v, err: %v", topRes, err)
	}
	if changes, err := mockMgr.Changes(); err != nil || len(changes) == 0 {
		t.Errorf("unexpected mock Changes: %+v, err: %v", changes, err)
	}
	if entries, err := mockMgr.ReadDir("/"); err != nil || len(entries) == 0 {
		t.Errorf("unexpected mock ReadDir: %+v, err: %v", entries, err)
	}
	if entries, err := mockMgr.ReadDir("/app"); err != nil || len(entries) != 2 {
		t.Errorf("unexpected mock ReadDir /app: %+v, err: %v", entries, err)
	}
	if entries, err := mockMgr.ReadDir("/other"); err != nil || len(entries) != 0 {
		t.Errorf("unexpected mock ReadDir /other: %+v, err: %v", entries, err)
	}
	if content, err := mockMgr.ReadFile("/app/config.json", 100); err != nil || !strings.Contains(content, "port") {
		t.Errorf("unexpected mock ReadFile /app/config.json: %s, err: %v", content, err)
	}
	if content, err := mockMgr.ReadFile("/etc/hosts", 100); err != nil || content == "" {
		t.Errorf("unexpected mock ReadFile: %s, err: %v", content, err)
	}
	tempDst := filepath.Join(t.TempDir(), "mock_dl")
	if n, err := mockMgr.Download("/etc/hosts", tempDst); err != nil || n == 0 {
		t.Errorf("unexpected mock Download: %d, err: %v", n, err)
	}
	if err := mockMgr.Upload(tempDst, "/tmp"); err != nil {
		t.Errorf("unexpected mock Upload err: %v", err)
	}
	if err := mockMgr.DeleteFile("/tmp/mock_file"); err != nil {
		t.Errorf("unexpected mock DeleteFile err: %v", err)
	}
	if err := mockMgr.UpdateResources(1024, 2.0, "unless-stopped"); err != nil {
		t.Errorf("unexpected mock UpdateResources err: %v", err)
	}
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
	if err := runcMgr.Exec([]string{"sh"}); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := runcMgr.Kill("SIGTERM"); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if _, err := runcMgr.Top("aux"); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if _, err := runcMgr.Changes(); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if _, err := runcMgr.ReadDir("/"); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if _, err := runcMgr.ReadFile("/etc/hosts", 100); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if _, err := runcMgr.Download("/etc/hosts", tempDst); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := runcMgr.Upload(tempDst, "/tmp"); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
	if err := runcMgr.DeleteFile("/tmp/file"); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl for DeleteFile, got %v", err)
	}
	if err := runcMgr.UpdateResources(512, 1.0, "no"); err != ErrActionNotImpl {
		t.Errorf("expected ErrActionNotImpl, got %v", err)
	}
}

func TestDockerManagerNilClientErrors(t *testing.T) {
	dm := NewDocker(nil, "mock-id")
	if err := dm.Start(); err == nil {
		t.Error("expected error with nil client on Start")
	}
	if err := dm.Stop(); err == nil {
		t.Error("expected error with nil client on Stop")
	}
	if err := dm.Remove(); err == nil {
		t.Error("expected error with nil client on Remove")
	}
	if err := dm.Pause(); err == nil {
		t.Error("expected error with nil client on Pause")
	}
	if err := dm.Unpause(); err == nil {
		t.Error("expected error with nil client on Unpause")
	}
	if err := dm.Restart(); err == nil {
		t.Error("expected error with nil client on Restart")
	}
	if err := dm.Kill("SIGTERM"); err == nil {
		t.Error("expected error with nil client on Kill")
	}
	if _, err := dm.Top("aux"); err == nil {
		t.Error("expected error with nil client on Top")
	}
	if _, err := dm.Changes(); err == nil {
		t.Error("expected error with nil client on Changes")
	}
	if _, err := dm.ReadDir("/"); err == nil {
		t.Error("expected error with nil client on ReadDir")
	}
	if _, err := dm.ReadFile("/etc/hosts", 100); err == nil {
		t.Error("expected error with nil client on ReadFile")
	}
	tempDst := filepath.Join(t.TempDir(), "target")
	if _, err := dm.Download("/etc/hosts", tempDst); err == nil {
		t.Error("expected error with nil client on Download")
	}
	if err := dm.Upload(tempDst, "/tmp"); err == nil {
		t.Error("expected error with nil client on Upload")
	}
	if err := dm.DeleteFile("/tmp/file"); err == nil {
		t.Error("expected error with nil client on DeleteFile")
	}
	if err := dm.UpdateResources(512, 1.0, "no"); err == nil {
		t.Error("expected error with nil client on UpdateResources")
	}
	if err := dm.Exec([]string{"sh"}); err == nil {
		t.Error("expected error with nil client on Exec")
	}
}

func TestDockerManagerDownloadZipSlipProtection(t *testing.T) {
	// Mock server that returns a malicious tar stream with path traversal "../malicious.txt"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/archive") {
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			content := []byte("pwned")
			_ = tw.WriteHeader(&tar.Header{
				Name: "../../malicious.txt",
				Mode: 0o644,
				Size: int64(len(content)),
			})
			_, _ = tw.Write(content)
			_ = tw.Close()
			w.Header().Set("Content-Type", "application/x-tar")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buf.Bytes())
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := api.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	dm := NewDocker(client, "test-container")
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "safe_download.txt")

	_, err = dm.Download("/etc/passwd", targetFile)
	if err == nil {
		t.Fatal("expected error on malicious archive with path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") && !strings.Contains(err.Error(), "illegal path traversal") {
		t.Fatalf("expected security violation error message, got: %v", err)
	}

	// Verify the file was NOT created outside tempDir
	parentEscaped := filepath.Join(tempDir, "..", "malicious.txt")
	if _, statErr := os.Stat(parentEscaped); !os.IsNotExist(statErr) {
		_ = os.Remove(parentEscaped)
		t.Fatalf("CRITICAL SECURITY FAILURE: file was written outside target directory: %s", parentEscaped)
	}
}

func TestDockerManagerListFilesZipSlipProtection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/archive") {
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			legitContent := []byte("package main\n")
			_ = tw.WriteHeader(&tar.Header{
				Name: "app/server.go",
				Mode: 0o644,
				Size: int64(len(legitContent)),
			})
			_, _ = tw.Write(legitContent)

			traversalContent := []byte("secret:shadow:data\n")
			_ = tw.WriteHeader(&tar.Header{
				Name: "../../../etc/shadow",
				Mode: 0o644,
				Size: int64(len(traversalContent)),
			})
			_, _ = tw.Write(traversalContent)
			_ = tw.Close()
			w.Header().Set("Content-Type", "application/x-tar")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buf.Bytes())
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := api.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	dm := NewDocker(client, "test-container")
	entries, err := dm.ReadDir("/app")
	if err != nil {
		t.Fatalf("unexpected error reading dir: %v", err)
	}

	// Verify that the traversal entry was sanitized and ignored
	for _, entry := range entries {
		if strings.Contains(entry.Path, "shadow") || strings.Contains(entry.Path, "..") {
			t.Fatalf("expected traversal entry to be excluded, but found: %+v", entry)
		}
	}
}

func TestDockerManagerStrictAbsolutePathRejection(t *testing.T) {
	client, err := api.NewClient("http://127.0.0.1:2375")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	dm := NewDocker(client, "test-container")

	// 1. ReadDir relative path or traversal rejection
	if _, err := dm.ReadDir("relative/dir"); err == nil {
		t.Error("expected error for relative ReadDir path")
	}
	if _, err := dm.ReadDir("/var/log/../../etc"); err == nil {
		t.Error("expected error for traversal ReadDir path")
	}

	// 2. ReadFile relative path or traversal rejection
	if _, err := dm.ReadFile("relative/file.txt", 100); err == nil {
		t.Error("expected error for relative ReadFile path")
	}
	if _, err := dm.ReadFile("/etc/../../root/file", 100); err == nil {
		t.Error("expected error for traversal ReadFile path")
	}

	// 3. Download relative source container path rejection
	tempDst := filepath.Join(t.TempDir(), "target")
	if _, err := dm.Download("relative/source.txt", tempDst); err == nil {
		t.Error("expected error for relative Download source path")
	}
	if _, err := dm.Download("/etc/../secret", tempDst); err == nil {
		t.Error("expected error for traversal Download source path")
	}

	// 4. Upload relative destination container path rejection
	tempSrc := filepath.Join(t.TempDir(), "src.txt")
	_ = os.WriteFile(tempSrc, []byte("data"), 0o644)
	if err := dm.Upload(tempSrc, "relative/dst"); err == nil {
		t.Error("expected error for relative Upload destination container path")
	}
	if err := dm.Upload(tempSrc, "/app/../bin"); err == nil {
		t.Error("expected error for traversal Upload destination container path")
	}

	// 5. SearchFiles relative path or traversal rejection
	if _, err := dm.SearchFiles("relative/dir", "conf", 10); err == nil {
		t.Error("expected error for relative SearchFiles base path")
	}
	if _, err := dm.SearchFiles("/var/../../etc", "conf", 10); err == nil {
		t.Error("expected error for traversal SearchFiles base path")
	}
	if _, err := dm.SearchFiles("/", "", 10); err == nil {
		t.Error("expected error for empty search pattern")
	}

	// 6. DeleteFile path validation and security rejection
	if err := dm.DeleteFile("relative/path.txt"); err == nil {
		t.Error("expected error for relative DeleteFile path")
	}
	if err := dm.DeleteFile("/app/../etc/passwd"); err == nil {
		t.Error("expected error for traversal DeleteFile path")
	}
	if err := dm.DeleteFile(""); err == nil {
		t.Error("expected error for empty DeleteFile path")
	}
}

func TestDockerManagerExecTtyAndStream(t *testing.T) {
	var execCreated struct {
		AttachStdin  bool     `json:"AttachStdin"`
		AttachStdout bool     `json:"AttachStdout"`
		AttachStderr bool     `json:"AttachStderr"`
		Tty          bool     `json:"Tty"`
		Cmd          []string `json:"Cmd"`
		Container    string   `json:"Container"`
	}
	var execStarted struct {
		Tty bool `json:"Tty"`
	}
	var startCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/exec") && r.Method == "POST":
			_ = json.NewDecoder(r.Body).Decode(&execCreated)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"Id":"exec-tty-test-456"}`))
		case strings.Contains(r.URL.Path, "/exec/exec-tty-test-456/start") && r.Method == "POST":
			startCalled = true
			_ = json.NewDecoder(r.Body).Decode(&execStarted)
			w.WriteHeader(http.StatusOK)
			// Return raw terminal output without 8-byte multiplex headers to verify raw streaming
			_, _ = w.Write([]byte("root@container:/# "))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := api.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	dm := NewDocker(client, "test-container-id")

	testCmd := []string{"/bin/sh", "-c", "printf 'hello'"}
	if err := dm.Exec(testCmd); err != nil {
		t.Fatalf("unexpected Exec error: %v", err)
	}

	// Verify TTY is enabled on CreateExec
	if !execCreated.Tty {
		t.Error("expected Tty: true on CreateExec for interactive shell")
	}
	if !execCreated.AttachStdin || !execCreated.AttachStdout || !execCreated.AttachStderr {
		t.Errorf("expected all stdio streams attached (stdin=%v, stdout=%v, stderr=%v)",
			execCreated.AttachStdin, execCreated.AttachStdout, execCreated.AttachStderr)
	}
	if len(execCreated.Cmd) != len(testCmd) {
		t.Errorf("expected Cmd length %d, got %d", len(testCmd), len(execCreated.Cmd))
	} else {
		for i := range testCmd {
			if execCreated.Cmd[i] != testCmd[i] {
				t.Errorf("expected Cmd[%d]=%s, got %s", i, testCmd[i], execCreated.Cmd[i])
			}
		}
	}

	if !startCalled {
		t.Error("expected StartExec endpoint to be invoked")
	}
	if !execStarted.Tty {
		t.Error("expected Tty: true on StartExec for direct PTY streaming without 8-byte multiplex headers")
	}

	// Verify error when client is nil
	nilDm := NewDocker(nil, "test-container-id")
	if err := nilDm.Exec(testCmd); err == nil || !strings.Contains(err.Error(), "docker client is nil") {
		t.Errorf("expected 'docker client is nil' error, got %v", err)
	}

	// Verify error when CreateExec fails (daemon returns 500)
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"daemon failure"}`))
	}))
	defer failServer.Close()

	failClient, err := api.NewClient(failServer.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	failDm := NewDocker(failClient, "test-container-id")
	if err := failDm.Exec(testCmd); err == nil {
		t.Error("expected error when CreateExec fails with 500")
	}
}

func TestCancellableStdinUnblocksOnClose(t *testing.T) {
	// Create an OS pipe to simulate a terminal/descriptor with no active input
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	cs := newCancellableStdin(r)
	defer func() { _ = cs.Close() }()

	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 64)
		_, readErr := cs.Read(buf)
		readDone <- readErr
	}()

	// Verify Read is currently waiting
	select {
	case res := <-readDone:
		t.Fatalf("expected Read to block while waiting for input, but returned early with %v", res)
	case <-time.After(50 * time.Millisecond):
	}

	// Close cancellableStdin (simulating Docker container exit calling closer.Close())
	if err := cs.Close(); err != nil {
		t.Errorf("unexpected error on Close: %v", err)
	}

	// Read must immediately unblock with io.EOF without needing any terminal keypress
	select {
	case res := <-readDone:
		if res != io.EOF {
			t.Errorf("expected io.EOF on Close, got %v", res)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("DEADLOCK: cs.Read failed to unblock within 500ms after Close()")
	}
}

func TestDockerManagerExecResizeTTY(t *testing.T) {
	var resizeH, resizeW string
	var resizeCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/exec"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"Id":"exec-resize-100"}`))
		case strings.Contains(r.URL.Path, "/exec/exec-resize-100/start"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/exec/exec-resize-100/resize"):
			resizeCalled = true
			resizeH = r.URL.Query().Get("h")
			resizeW = r.URL.Query().Get("w")
			w.WriteHeader(http.StatusOK)
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

	mgr := NewDocker(client, "test-resize")
	termW, termH := theme.TermDimensions()

	if err := mgr.Exec([]string{"/bin/sh"}); err != nil {
		t.Fatalf("unexpected Exec error: %v", err)
	}

	if !resizeCalled {
		t.Errorf("expected ResizeExecTTY to be called during Exec, but was not")
	}
	expectedH := strconv.Itoa(termH)
	expectedW := strconv.Itoa(termW)
	if resizeH != expectedH || resizeW != expectedW {
		t.Errorf("expected dimensions h=%s, w=%s; got h=%s, w=%s", expectedH, expectedW, resizeH, resizeW)
	}
}

func TestFlushTerminalInput(t *testing.T) {
	// 1. Calling on invalid fd must safely return without panicking
	flushTerminalInput(^uintptr(0))

	// 2. Calling on pipe with unread data must drain/flush without blocking
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	if _, err := w.Write([]byte("stray bytes\n")); err != nil {
		t.Fatalf("failed to write to pipe: %v", err)
	}

	flushTerminalInput(r.Fd())
}

func TestCancellableStdinNormalReadAndIdempotentClose(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	cs := newCancellableStdin(r)

	// 1. Normal read
	go func() {
		_, _ = w.Write([]byte("terminal payload\n"))
	}()

	buf := make([]byte, 32)
	n, err := cs.Read(buf)
	if err != nil {
		t.Fatalf("expected successful read, got error: %v", err)
	}
	if string(buf[:n]) != "terminal payload\n" {
		t.Errorf("expected 'terminal payload\\n', got %q", string(buf[:n]))
	}

	// 2. Idempotent close: multiple calls must succeed without panicking
	for i := 0; i < 3; i++ {
		if err := cs.Close(); err != nil {
			t.Errorf("Close iteration %d returned error: %v", i, err)
		}
	}

	// 3. Subsequent reads after Close must return immediately with io.EOF
	n2, err2 := cs.Read(buf)
	if n2 != 0 || err2 != io.EOF {
		t.Errorf("expected 0, io.EOF after close, got n=%d, err=%v", n2, err2)
	}
}
