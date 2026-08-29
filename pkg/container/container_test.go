// container_test.go validates container lifecycle controls, asynchronous telemetry reading, and concurrent field mutations.
// Test Strategy: Concurrent mock goroutine workers reading and writing container properties under -race detection.
package container

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/connector/collector"
	"github.com/edsilegx/ctop/pkg/models"
)

type dummyCollector struct {
	stream chan models.Metrics
}

func (d *dummyCollector) Start()                       {}
func (d *dummyCollector) Stop()                        {}
func (d *dummyCollector) Running() bool                { return true }
func (d *dummyCollector) Stream() chan models.Metrics  { return d.stream }
func (d *dummyCollector) Logs() collector.LogCollector { return nil }

type trackingManager struct {
	started   bool
	stopped   bool
	removed   bool
	paused    bool
	unpaused  bool
	restarted bool
	signal    string
	execCmd   []string
	fail      bool
}

func (m *trackingManager) Start() error {
	if m.fail {
		return errors.New("start failed")
	}
	m.started = true
	return nil
}

func (m *trackingManager) Stop() error {
	if m.fail {
		return errors.New("stop failed")
	}
	m.stopped = true
	return nil
}

func (m *trackingManager) Remove() error {
	if m.fail {
		return errors.New("remove failed")
	}
	m.removed = true
	return nil
}

func (m *trackingManager) Pause() error {
	if m.fail {
		return errors.New("pause failed")
	}
	m.paused = true
	return nil
}

func (m *trackingManager) Unpause() error {
	if m.fail {
		return errors.New("unpause failed")
	}
	m.unpaused = true
	return nil
}

func (m *trackingManager) Restart() error {
	if m.fail {
		return errors.New("restart failed")
	}
	m.restarted = true
	return nil
}

func (m *trackingManager) Exec(cmd []string) error {
	if m.fail {
		return errors.New("exec failed")
	}
	m.execCmd = cmd
	return nil
}

func (m *trackingManager) Kill(sig string) error {
	if m.fail {
		return errors.New("kill failed")
	}
	m.signal = sig
	return nil
}

func (m *trackingManager) Top(args string) (models.TopResult, error) {
	if m.fail {
		return models.TopResult{}, errors.New("top failed")
	}
	return models.TopResult{
		Titles: []string{"PID", "CMD"},
		Processes: [][]string{
			{"1", "init"},
		},
	}, nil
}

func (m *trackingManager) Changes() ([]models.Change, error) {
	if m.fail {
		return nil, errors.New("changes failed")
	}
	return []models.Change{
		{Path: "/app", Kind: 0},
	}, nil
}

func (m *trackingManager) ReadDir(path string) ([]models.FileInfo, error) {
	if m.fail {
		return nil, errors.New("readdir failed")
	}
	return []models.FileInfo{
		{Name: "app", Path: "/app", IsDir: true, Mode: "drwxr-xr-x"},
	}, nil
}

func (m *trackingManager) ReadFile(path string, maxBytes int64) (string, error) {
	if m.fail {
		return "", errors.New("readfile failed")
	}
	return "test file content", nil
}

func (m *trackingManager) Download(srcPath, dstPath string) (int64, error) {
	if m.fail {
		return 0, errors.New("download failed")
	}
	return 256, nil
}

func (m *trackingManager) Upload(srcPath, dstPath string) error {
	if m.fail {
		return errors.New("upload failed")
	}
	return nil
}

func (m *trackingManager) UpdateResources(memoryMB int64, cpus float64, restartPolicy string) error {
	if m.fail {
		return errors.New("update resources failed")
	}
	return nil
}

func TestContainerLifecycle(t *testing.T) {
	colStream := make(chan models.Metrics, 5)
	mgr := &trackingManager{}
	c := New("test1234567890123", &dummyCollector{stream: colStream}, mgr)
	c.SetMeta("name", "app")
	c.SetMeta("state", "exited")

	// Start container
	c.Start()
	if !mgr.started {
		t.Fatal("expected manager.Start() to be invoked")
	}

	// State running -> Pause
	c.SetState("running")
	c.Pause()
	if !mgr.paused {
		t.Fatal("expected manager.Pause() to be invoked")
	}

	// State paused -> Unpause
	c.SetState("paused")
	c.Unpause()
	if !mgr.unpaused {
		t.Fatal("expected manager.Unpause() to be invoked")
	}

	// State running -> Restart
	c.SetState("running")
	c.Restart()
	if !mgr.restarted {
		t.Fatal("expected manager.Restart() to be invoked")
	}

	// State running -> Stop
	c.Stop()
	if !mgr.stopped {
		t.Fatal("expected manager.Stop() to be invoked")
	}

	// Remove
	c.Remove()
	if !mgr.removed {
		t.Fatal("expected manager.Remove() to be invoked")
	}

	// Exec
	err := c.Exec([]string{"/bin/sh", "-c", "echo hello"})
	if err != nil || len(mgr.execCmd) != 3 {
		t.Fatalf("expected Exec to succeed, got %v", err)
	}

	// Signal
	if err := c.Signal("SIGHUP"); err != nil || mgr.signal != "SIGHUP" {
		t.Fatalf("expected Signal to succeed, got err=%v sig=%s", err, mgr.signal)
	}

	// Top
	if topRes, err := c.Top(); err != nil || len(topRes.Processes) == 0 {
		t.Fatalf("expected Top to succeed, got err=%v res=%+v", err, topRes)
	}

	// Changes
	if ch, err := c.Changes(); err != nil || len(ch) == 0 {
		t.Fatalf("expected Changes to succeed, got err=%v ch=%+v", err, ch)
	}

	// ReadDir, ReadFile, Download & Upload
	if entries, err := c.ReadDir("/app"); err != nil || len(entries) == 0 {
		t.Fatalf("expected ReadDir to succeed, got err=%v entries=%+v", err, entries)
	}
	if content, err := c.ReadFile("/app/config.json", 1024); err != nil || content == "" {
		t.Fatalf("expected ReadFile to succeed, got err=%v content=%s", err, content)
	}
	if n, err := c.Download("/app/config.json", "./config.json"); err != nil || n <= 0 {
		t.Fatalf("expected Download to succeed, got err=%v bytes=%d", err, n)
	}
	if err := c.Upload("./config.json", "/app"); err != nil {
		t.Fatalf("expected Upload to succeed, got err=%v", err)
	}
	if err := c.UpdateResources(512, 1.5, "unless-stopped"); err != nil {
		t.Fatalf("expected UpdateResources to succeed, got err=%v", err)
	}

	// Generators
	c.SetMeta("image", "redis:alpine")
	c.SetMeta("ports", "6379/tcp\n0.0.0.0:6379 -> 6379/tcp")
	c.SetMeta("[MOUNTS]", "/data:::/var/lib/docker/volumes/data:::volume:::rw:::local")
	c.SetMeta("[ENV-VAR]", "REDIS_PORT=6379;LOG_LEVEL=info")
	c.SetMeta("memLimit", "512 MB")
	c.SetMeta("cpuLimit", "1.00 CPUs")
	c.SetMeta("pidsLimit", "100")
	c.SetMeta("privileged", "true")
	c.SetMeta("readonlyRootfs", "true")
	c.SetMeta("restartPolicy", "always")

	runCmd := c.GenerateRunCmd()
	if runCmd == "" {
		t.Fatal("expected non-empty run command")
	}

	composeYaml := c.GenerateCompose()
	if composeYaml == "" {
		t.Fatal("expected non-empty compose YAML")
	}

	// Failure branch
	mgr.fail = true
	c.SetState("running")
	c.Stop()
	c.Start()
	c.Pause()
	c.SetState("paused")
	c.Unpause()
	c.SetState("running")
	c.Restart()
	c.Remove()
	_ = c.Exec([]string{"ls"})
	_ = c.Signal("SIGTERM")
	_, _ = c.Top()
	_, _ = c.Changes()

	// RecreateWidgets
	c.RecreateWidgets()
}

func TestContainerConcurrentAccess(t *testing.T) {
	colStream := make(chan models.Metrics, 100)
	c := New("test1234567890123", &dummyCollector{stream: colStream}, &trackingManager{})

	var wg sync.WaitGroup

	// Stream reader
	c.Read(colStream)

	// Concurrently write metrics
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			m := models.NewMetrics()
			m.CPUUtil = i
			m.MemUsage = int64(i * 1024)
			colStream <- m
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Concurrently update metadata
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.SetMeta("name", "container-name")
			_ = c.GetMeta("name")
			c.SetState("running")
			_ = c.GetMeta("state")
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
	close(colStream)
	time.Sleep(10 * time.Millisecond)
}
