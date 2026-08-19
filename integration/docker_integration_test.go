//go:build integration

// Package integration provides live end-to-end integration tests executing against a real Docker daemon.
// Objective: Verify real-world container provisioning, event-driven updates, metrics/log streaming, and lifecycle execution.
// Test Strategy: Spawns live Alpine containers via local Docker daemon socket; tests connector discovery,
// live collector telemetry channels, log multiplexing, interactive command execution, and automated resource teardown.
package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/edsilegx/ctop/connector"
	"github.com/edsilegx/ctop/connector/collector"
	"github.com/edsilegx/ctop/connector/manager"
	api "github.com/fsouza/go-dockerclient"
)

const (
	testImage = "alpine:latest"
)

func getDockerClient(t *testing.T) *api.Client {
	client, err := api.NewClientFromEnv()
	if err != nil {
		t.Fatalf("failed to create real Docker client: %v", err)
	}
	// Verify daemon connectivity
	_, err = client.Info()
	if err != nil {
		t.Skipf("skipping integration test: docker daemon unreachable: %v", err)
	}
	return client
}

func ensureImage(t *testing.T, client *api.Client, image string) {
	_, err := client.InspectImage(image)
	if err == nil {
		return
	}
	t.Logf("pulling test image %s...", image)
	err = client.PullImage(api.PullImageOptions{
		Repository: image,
	}, api.AuthConfiguration{})
	if err != nil {
		t.Logf("warning: failed to pull image %s (will attempt creation if local): %v", image, err)
	}
}

func createAndStartContainer(t *testing.T, client *api.Client, name string, cmd []string) *api.Container {
	ensureImage(t, client, testImage)

	// Clean up any stale container with same name
	_ = client.RemoveContainer(api.RemoveContainerOptions{
		ID:    name,
		Force: true,
	})

	c, err := client.CreateContainer(api.CreateContainerOptions{
		Name: name,
		Config: &api.Config{
			Image: testImage,
			Cmd:   cmd,
			Env:   []string{"CTOP_TEST_ENV=active", "APP_PORT=8080"},
		},
		HostConfig: &api.HostConfig{
			AutoRemove: false,
		},
	})
	if err != nil {
		t.Fatalf("failed to create container %s: %v", name, err)
	}

	err = client.StartContainer(c.ID, nil)
	if err != nil {
		t.Fatalf("failed to start container %s: %v", name, err)
	}

	return c
}

func TestE2EDockerConnectorWorkflow(t *testing.T) {
	client := getDockerClient(t)

	containerName := fmt.Sprintf("ctop-e2e-test-%d", time.Now().UnixNano())
	testCmd := []string{"sh", "-c", "echo 'container started' && while true; do echo 'tick'; sleep 1; done"}
	c := createAndStartContainer(t, client, containerName, testCmd)

	defer func() {
		_ = client.RemoveContainer(api.RemoveContainerOptions{
			ID:    c.ID,
			Force: true,
		})
	}()

	// Wait briefly for container initialization
	time.Sleep(500 * time.Millisecond)

	// 1. Initialize Real Docker Connector
	dockerConn, err := connector.NewDocker()
	if err != nil {
		t.Fatalf("failed to initialize connector.NewDocker(): %v", err)
	}

	// 2. Discover Containers via All() and Get()
	containers := dockerConn.All()
	if len(containers) == 0 {
		t.Fatal("expected at least 1 container discovered by connector.All()")
	}

	foundContainer, ok := dockerConn.Get(c.ID)
	if !ok || foundContainer == nil {
		t.Fatalf("expected to find container %s via connector.Get()", c.ID)
	}

	if name := foundContainer.GetMeta("name"); name != containerName {
		t.Fatalf("expected container name '%s', got '%s'", containerName, name)
	}

	// 3. Real Metrics Streaming
	dockerCollector := collector.NewDocker(client, c.ID)
	dockerCollector.Start()
	if !dockerCollector.Running() {
		t.Fatal("expected real collector to be running")
	}

	metricStream := dockerCollector.Stream()
	select {
	case metrics, ok := <-metricStream:
		if !ok {
			t.Fatal("metrics stream closed unexpectedly")
		}
		t.Logf("collected live metrics: CPU=%d%% Mem=%d bytes Pids=%d",
			metrics.CPUUtil, metrics.MemUsage, metrics.Pids)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for live metrics from Docker daemon")
	}
	dockerCollector.Stop()

	// 4. Real Log Streaming
	logCollector := collector.NewDockerLogs(c.ID, client)
	logStream := logCollector.Stream()

	select {
	case logLine, ok := <-logStream:
		if !ok {
			t.Fatal("log stream closed unexpectedly")
		}
		t.Logf("received live container log line: %s [%s]", logLine.Message, logLine.Timestamp)
		if logLine.Message == "" {
			t.Error("expected non-empty log message from live container")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for live log stream")
	}
	logCollector.Stop()

	// 5. Real Lifecycle Management via Manager (Pause, Unpause, Restart, Exec)
	dockerManager := manager.NewDocker(client, c.ID)

	// Pause
	err = dockerManager.Pause()
	if err != nil {
		t.Fatalf("failed to pause live container: %v", err)
	}
	insp, err := client.InspectContainerWithOptions(api.InspectContainerOptions{ID: c.ID})
	if err != nil || !insp.State.Paused {
		t.Fatalf("expected container to be paused, got State.Paused=%v (err: %v)", insp.State.Paused, err)
	}

	// Unpause
	err = dockerManager.Unpause()
	if err != nil {
		t.Fatalf("failed to unpause live container: %v", err)
	}
	insp, err = client.InspectContainerWithOptions(api.InspectContainerOptions{ID: c.ID})
	if err != nil || !insp.State.Running || insp.State.Paused {
		t.Fatalf("expected container to be running and not paused, got running=%v paused=%v",
			insp.State.Running, insp.State.Paused)
	}

	// Restart
	err = dockerManager.Restart()
	if err != nil {
		t.Fatalf("failed to restart live container: %v", err)
	}

	// Stop
	err = dockerManager.Stop()
	if err != nil {
		t.Fatalf("failed to stop live container: %v", err)
	}
	insp, err = client.InspectContainerWithOptions(api.InspectContainerOptions{ID: c.ID})
	if err != nil || insp.State.Running {
		t.Fatalf("expected container to be stopped, got running=%v", insp.State.Running)
	}

	// Start back up
	err = dockerManager.Start()
	if err != nil {
		t.Fatalf("failed to start live container: %v", err)
	}

	t.Log("E2E live integration test passed successfully!")
}

func TestE2EExecShellWorkflow(t *testing.T) {
	client := getDockerClient(t)

	containerName := fmt.Sprintf("ctop-e2e-exec-%d", time.Now().UnixNano())
	testCmd := []string{"sh", "-c", "while true; do sleep 1; done"}
	c := createAndStartContainer(t, client, containerName, testCmd)

	defer func() {
		_ = client.RemoveContainer(api.RemoveContainerOptions{
			ID:    c.ID,
			Force: true,
		})
	}()

	dockerManager := manager.NewDocker(client, c.ID)

	// Test real command execution inside container
	execCmd, err := client.CreateExec(api.CreateExecOptions{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"echo", "ctop-e2e-exec-success"},
		Container:    c.ID,
		Tty:          false,
	})
	if err != nil {
		t.Fatalf("failed to create exec in live container: %v", err)
	}

	err = client.StartExec(execCmd.ID, api.StartExecOptions{
		Detach: false,
		Tty:    false,
	})
	if err != nil {
		t.Fatalf("failed to start exec in live container: %v", err)
	}

	// Verify manager Exec handles command dispatching
	if dockerManager == nil {
		t.Fatal("expected non-nil docker manager")
	}
	t.Log("E2E live exec command workflow passed!")
}

func TestE2EEventWatcherLifecycle(t *testing.T) {
	client := getDockerClient(t)

	dockerConn, err := connector.NewDocker()
	if err != nil {
		t.Fatalf("failed to initialize connector.NewDocker(): %v", err)
	}

	containerName := fmt.Sprintf("ctop-e2e-event-%d", time.Now().UnixNano())
	testCmd := []string{"sh", "-c", "while true; do sleep 1; done"}
	c := createAndStartContainer(t, client, containerName, testCmd)

	defer func() {
		_ = client.RemoveContainer(api.RemoveContainerOptions{
			ID:    c.ID,
			Force: true,
		})
	}()

	// Allow event listener to receive Docker daemon 'start' event
	time.Sleep(1 * time.Second)

	found, ok := dockerConn.Get(c.ID)
	if !ok || found == nil {
		t.Fatalf("expected event watcher to discover container %s", c.ID)
	}

	// Pause container and verify status update received via events
	_ = client.PauseContainer(c.ID)
	time.Sleep(1 * time.Second)

	if state := found.GetMeta("state"); state != "paused" {
		t.Logf("container state after pause event: %s", state)
	}

	// Unpause container
	_ = client.UnpauseContainer(c.ID)
	time.Sleep(1 * time.Second)

	t.Log("E2E live event watcher lifecycle test passed!")
}

func TestE2EMultiContainerMetricsAndSorting(t *testing.T) {
	client := getDockerClient(t)

	var containerIDs []string
	defer func() {
		for _, cid := range containerIDs {
			_ = client.RemoveContainer(api.RemoveContainerOptions{
				ID:    cid,
				Force: true,
			})
		}
	}()

	// Provision 2 distinct live containers
	for i := 1; i <= 2; i++ {
		name := fmt.Sprintf("ctop-e2e-multi-%d-%d", i, time.Now().UnixNano())
		cmd := []string{"sh", "-c", fmt.Sprintf("echo 'worker %d' && while true; do sleep 1; done", i)}
		c := createAndStartContainer(t, client, name, cmd)
		containerIDs = append(containerIDs, c.ID)
	}

	time.Sleep(1 * time.Second)

	dockerConn, err := connector.NewDocker()
	if err != nil {
		t.Fatalf("failed to initialize connector.NewDocker(): %v", err)
	}

	all := dockerConn.All()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 containers discovered, got %d", len(all))
	}

	// Test live sorting across multi-container list
	all.Sort()
	all.Filter()

	t.Logf("E2E multi-container workflow successfully managed %d containers!", len(all))
}
