// Package integration provides live end-to-end integration tests executing against real daemons.
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNGINXReverseProxyE2E executes the automated NGINX reverse proxy end-to-end test script
// against real NGINX and ctop daemon processes.
func TestNGINXReverseProxyE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping NGINX shell integration test on native Windows (run under WSL / Linux)")
	}

	// 1. Verify nginx is available on the system
	nginxPath, err := exec.LookPath("nginx")
	if err != nil {
		t.Skipf("nginx not found in PATH; skipping live reverse proxy E2E test: %v", err)
	}
	t.Logf("found nginx binary at: %s", nginxPath)

	// 2. Locate the repository root and test script
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	repoRoot := wd
	if filepath.Base(repoRoot) == "integration" {
		repoRoot = filepath.Dir(repoRoot)
	}

	scriptPath := filepath.Join(repoRoot, "tests", "nginx", "test_nginx_e2e.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("test script not found at %s: %v", scriptPath, err)
	}

	// 3. Execute the test runner script
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = repoRoot

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	t.Logf("executing NGINX E2E test runner: %s", scriptPath)
	err = cmd.Run()

	output := stdoutBuf.String()
	errOutput := stderrBuf.String()

	if err != nil {
		t.Fatalf("NGINX reverse proxy E2E test failed with error: %v\n--- STDOUT ---\n%s\n--- STDERR ---\n%s",
			err, output, errOutput)
	}

	// 4. Verify completion banner in stdout
	if !strings.Contains(output, "NGINX REVERSE PROXY E2E TESTS PASSED") {
		t.Fatalf("expected completion banner in test output, got:\n%s", output)
	}

	t.Log("NGINX Reverse Proxy E2E test completed successfully.")
}
