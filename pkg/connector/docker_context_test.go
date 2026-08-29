package connector

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestDockerContextResolution(t *testing.T) {
	// Test DOCKER_HOST takes precedence
	t.Setenv("DOCKER_HOST", "tcp://1.2.3.4:2375")

	if ep := ResolveDockerEndpoint(); ep != "tcp://1.2.3.4:2375" {
		t.Fatalf("expected tcp://1.2.3.4:2375, got %s", ep)
	}

	_ = os.Unsetenv("DOCKER_HOST")

	// Create temp home dir with mock context
	tmpHome := t.TempDir()
	t.Setenv("USERPROFILE", tmpHome)
	t.Setenv("HOME", tmpHome)

	ctxName := "colima"
	hash := sha256.Sum256([]byte(ctxName))
	dirName := hex.EncodeToString(hash[:])

	metaDir := filepath.Join(tmpHome, ".docker", "contexts", "meta", dirName)
	if err := os.MkdirAll(metaDir, 0o750); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}

	metaContent := `{"Name":"colima","Endpoints":{"docker":{"Host":"unix:///tmp/colima.sock"}}}`
	if err := os.WriteFile(filepath.Join(metaDir, "meta.json"), []byte(metaContent), 0o600); err != nil {
		t.Fatalf("failed to write meta.json: %v", err)
	}

	t.Setenv("DOCKER_CONTEXT", ctxName)
	if ep := ResolveDockerEndpoint(); ep != "unix:///tmp/colima.sock" {
		t.Fatalf("expected unix:///tmp/colima.sock, got %s", ep)
	}
}
