//go:build linux

// runc_test.go validates runC connector options, container map management, and environment parsing.
// Test Strategy: Uses t.TempDir() mock runC root directories to test filesystem resolution and in-memory container registration.
package connector

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/edsilegx/ctop/pkg/container"
	"github.com/opencontainers/runc/libcontainer"
)

func TestRuncOptsAndNewRunc(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("RUNC_ROOT", tempDir)
	t.Setenv("RUNC_SYSTEMD_CGROUP", "1")

	opts, err := NewRuncOpts()
	if err != nil {
		t.Fatalf("unexpected error from NewRuncOpts: %v", err)
	}
	if opts.root != tempDir {
		t.Fatalf("expected root '%s', got '%s'", tempDir, opts.root)
	}
	if !opts.systemdCgroups {
		t.Fatal("expected systemdCgroups to be true")
	}

	conn, err := NewRunc()
	if err != nil {
		t.Fatalf("unexpected error from NewRunc: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil Runc connector")
	}

	// Test non-existent dir error path
	t.Setenv("RUNC_ROOT", filepath.Join(tempDir, "nonexistent"))
	if _, err := NewRuncOpts(); err == nil {
		t.Fatal("expected error for nonexistent RUNC_ROOT")
	}
}

func TestRuncConnectorMethods(t *testing.T) {
	r := &Runc{
		containers:    make(map[string]*container.Container),
		libContainers: make(map[string]*libcontainer.Container),
		closed:        make(chan struct{}),
		lock:          sync.RWMutex{},
	}

	c1 := container.New("runc-1", nil, nil)
	c1.SetMeta("name", "container-1")
	r.containers["runc-1"] = c1

	// Test Get
	if found, ok := r.Get("runc-1"); !ok || found == nil {
		t.Fatal("expected to find runc-1")
	}
	if _, ok := r.Get("runc-missing"); ok {
		t.Fatal("expected runc-missing to return false")
	}

	// Test All
	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 container, got %d", len(all))
	}

	// Test MustGet existing
	if cMust := r.MustGet("runc-1"); cMust == nil {
		t.Fatal("expected non-nil container from MustGet")
	}

	// Test delByID
	r.delByID("runc-1")
	if _, ok := r.Get("runc-1"); ok {
		t.Fatal("expected runc-1 to be deleted")
	}

	// Test Wait
	go func() {
		close(r.closed)
	}()
	r.Wait()
}

func TestRuncConnectorStructures(t *testing.T) {
	tempRuncRoot := t.TempDir()
	t.Setenv("RUNC_ROOT", tempRuncRoot)
	t.Setenv("RUNC_SYSTEMD_CGROUP", "1")

	opts, err := NewRuncOpts()
	if err != nil {
		t.Fatalf("unexpected error from NewRuncOpts: %v", err)
	}
	if !opts.systemdCgroups || opts.root != tempRuncRoot {
		t.Errorf("unexpected opts: %+v", opts)
	}

	runcConn, err := NewRunc()
	if err != nil {
		t.Fatalf("unexpected error from NewRunc: %v", err)
	}
	if rc, ok := runcConn.(*Runc); ok {
		defer close(rc.closed)
		if rc.GetLibc("nonexistent") != nil {
			t.Error("expected nil for nonexistent libc")
		}
		rc.refreshAll()
		all := rc.All()
		if len(all) != 0 {
			t.Errorf("expected 0 containers in empty runc root, got %d", len(all))
		}
		rc.delByID("test-id")
	}
}
