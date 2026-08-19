// container_test.go validates container lifecycle controls, asynchronous telemetry reading, and concurrent field mutations.
// Test Strategy: Concurrent mock goroutine workers reading and writing container properties under -race detection.
package container

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/edsilegx/ctop/connector/collector"
	"github.com/edsilegx/ctop/models"
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
