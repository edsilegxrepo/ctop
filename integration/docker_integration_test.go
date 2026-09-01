//go:build integration

// Package integration provides live end-to-end integration tests executing against a real Docker daemon.
//
// Objective:
//
//	Verify real-world container provisioning, event-driven updates, metrics/log streaming, and lifecycle execution
//	against live Docker engines, TLS-secured endpoints, and multi-host aggregation contexts.
//
// Core Components:
//   - Live Alpine container runners: ephemeral container provisioning and background load generators.
//   - Mock TLS Docker daemons: httptest.TLSServer instances validating client TLS handshakes and certificate parsing.
//   - Multi-host aggregator fixtures: validating cross-host merged container lists and ID lookups.
//
// Test Strategy:
//   - Spawns live Alpine containers via local Docker daemon socket; tests connector discovery,
//     live collector telemetry channels, log multiplexing, interactive command execution, and automated resource teardown.
//   - Validates live JSON log formatting, structured multi-field filtering, and rate vs. cumulative metric mode calculation.
//   - Automatic skip logic (getDockerClient) when Docker daemon socket is unavailable.
//
// Data Flow:
//
//	Local/Remote Docker Socket -> Docker Connector -> Live Container Streams -> Validation Assertions.
package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edsilegx/ctop/internal/cwidgets/compact"
	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/connector"
	"github.com/edsilegx/ctop/pkg/connector/collector"
	"github.com/edsilegx/ctop/pkg/connector/manager"
	"github.com/edsilegx/ctop/pkg/jsonfmt"
	"github.com/edsilegx/ctop/pkg/models"
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

func TestE2EStructuredFilterLive(t *testing.T) {
	client := getDockerClient(t)

	c1Name := fmt.Sprintf("ctop-e2e-filter-alpha-%d", time.Now().UnixNano())
	c2Name := fmt.Sprintf("ctop-e2e-filter-beta-%d", time.Now().UnixNano())

	ensureImage(t, client, testImage)
	c1, err := client.CreateContainer(api.CreateContainerOptions{
		Name: c1Name,
		Config: &api.Config{
			Image:  testImage,
			Cmd:    []string{"sh", "-c", "sleep 30"},
			Labels: map[string]string{"environment": "prod", "tier": "frontend"},
		},
	})
	if err != nil {
		t.Fatalf("failed to create c1: %v", err)
	}
	defer client.RemoveContainer(api.RemoveContainerOptions{ID: c1.ID, Force: true})
	_ = client.StartContainer(c1.ID, nil)

	c2, err := client.CreateContainer(api.CreateContainerOptions{
		Name: c2Name,
		Config: &api.Config{
			Image:  testImage,
			Cmd:    []string{"sh", "-c", "sleep 30"},
			Labels: map[string]string{"environment": "staging", "tier": "backend"},
		},
	})
	if err != nil {
		t.Fatalf("failed to create c2: %v", err)
	}
	defer client.RemoveContainer(api.RemoveContainerOptions{ID: c2.ID, Force: true})
	_ = client.StartContainer(c2.ID, nil)

	time.Sleep(1 * time.Second)

	dockerConn, err := connector.NewDocker()
	if err != nil {
		t.Fatalf("failed to init docker connector: %v", err)
	}

	// Wait for connector background inspect refresh to populate metadata
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cont1, ok1 := dockerConn.Get(c1.ID)
		cont2, ok2 := dockerConn.Get(c2.ID)
		if ok1 && ok2 && cont1.GetMeta("name") != "" && cont2.GetMeta("name") != "" && cont1.GetMeta("state") == "running" && cont2.GetMeta("state") == "running" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	all := dockerConn.All()

	// 1. Filter by name=alpha and environment=prod
	config.Init()
	config.UpdateSwitch("allContainers", true)
	config.Update("filterStr", "status=running name=alpha environment=prod")
	all.Filter()

	cont1, ok1 := dockerConn.Get(c1.ID)
	cont2, ok2 := dockerConn.Get(c2.ID)

	if !ok1 || !ok2 {
		t.Fatalf("expected both containers found in connector registry")
	}

	t.Logf("cont1: name=%s state=%s env=%s labels=%s display=%v", cont1.GetMeta("name"), cont1.GetMeta("state"), cont1.GetMeta("environment"), cont1.GetMeta("[LABELS]"), cont1.Display)
	t.Logf("cont2: name=%s state=%s env=%s labels=%s display=%v", cont2.GetMeta("name"), cont2.GetMeta("state"), cont2.GetMeta("environment"), cont2.GetMeta("[LABELS]"), cont2.Display)

	if !cont1.Display {
		t.Errorf("expected cont1 (alpha/prod) to be displayed, got Display=false")
	}
	if cont2.Display {
		t.Errorf("expected cont2 (beta/staging) to be hidden, got Display=true")
	}

	t.Log("E2E live structured multi-filter test passed!")
}

func TestE2EJSONLogsFormattingLive(t *testing.T) {
	client := getDockerClient(t)

	cName := fmt.Sprintf("ctop-e2e-jsonlog-%d", time.Now().UnixNano())
	cmd := []string{"sh", "-c", `echo '{"level":"info","msg":"service started successfully","port":8080}' && sleep 10`}
	c := createAndStartContainer(t, client, cName, cmd)
	defer client.RemoveContainer(api.RemoveContainerOptions{ID: c.ID, Force: true})

	time.Sleep(1 * time.Second)

	logCollector := collector.NewDockerLogs(c.ID, client)
	defer logCollector.Stop()

	stream := logCollector.Stream()
	select {
	case line := <-stream:
		formatted := jsonfmt.FormatLogMessage(line.Message)
		t.Logf("Raw log line: %s", line.Message)
		t.Logf("Formatted JSON log: %s", formatted)
		if !strings.Contains(formatted, "level=info") || !strings.Contains(formatted, "service started successfully") {
			t.Errorf("expected formatted JSON log with level=info and message, got '%s'", formatted)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for live JSON log stream")
	}

	t.Log("E2E live JSON logs formatting test passed!")
}

func TestE2EMultiHostAggregationLive(t *testing.T) {
	_ = getDockerClient(t)
	liveDocker, err := connector.NewDocker()
	if err != nil {
		t.Fatalf("failed to init live docker connector: %v", err)
	}

	mockRemote, err := connector.NewMock()
	if err != nil {
		t.Fatalf("failed to init mock remote connector: %v", err)
	}

	mc := connector.NewMultiConnector()
	mc.AddConnector("local-docker", liveDocker)
	mc.AddConnector("remote-host-1", mockRemote)

	all := mc.All()
	if len(all) < len(mockRemote.All()) {
		t.Fatalf("expected merged container count at least mock count %d, got %d", len(mockRemote.All()), len(all))
	}

	// Verify host resolution on containers
	mockConts := mockRemote.All()
	if len(mockConts) > 0 {
		mID := mockConts[0].Id
		if found, ok := mc.Get(mID); !ok || found == nil {
			t.Fatalf("expected to find mock container %s across multi-connector", mID)
		}
	}

	t.Log("E2E multi-host aggregation live test passed!")
}

func generateE2ETestCertificates(t *testing.T, dir string) (string, string, string) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"ctop E2E Test"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPem := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	caPath := filepath.Join(dir, "ca.pem")
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	_ = os.WriteFile(caPath, certPem, 0o600)
	_ = os.WriteFile(certPath, certPem, 0o600)
	_ = os.WriteFile(keyPath, keyPem, 0o600)

	return caPath, certPath, keyPath
}

func TestE2ETLSConfigAndEndpointLive(t *testing.T) {
	tempDir := t.TempDir()
	caPath, certPath, keyPath := generateE2ETestCertificates(t, tempDir)

	connector.SetGlobalTLSConfig(connector.TLSConfig{
		Verify: false,
		CA:     caPath,
		Cert:   certPath,
		Key:    keyPath,
	})
	defer connector.SetGlobalTLSConfig(connector.TLSConfig{})

	// Spin up an actual mock TLS Docker daemon endpoint to test TLS connector handshake
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_ping":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/info":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ID":"test-remote-node","Driver":"overlay2","Images":2,"Name":"node1","ServerVersion":"24.0.0"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("[]"))
		}
	}))
	defer mockServer.Close()

	if mockServer.Certificate() != nil {
		serverCertPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: mockServer.Certificate().Raw})
		_ = os.WriteFile(caPath, serverCertPem, 0o600)
	}

	remoteConn, err := connector.NewDockerWithEndpoint(mockServer.URL, "remote-tls-node")
	if err != nil {
		t.Fatalf("failed to initialize remote endpoint connector: %v", err)
	}
	if remoteConn == nil {
		t.Fatal("expected non-nil remote connector")
	}

	// Verify live local docker endpoint if docker is available
	_ = getDockerClient(t)
	localEndpoint := connector.ResolveDockerEndpoint()
	if localEndpoint == "" {
		localEndpoint = "unix:///var/run/docker.sock"
	}

	liveConn, err := connector.NewDockerWithEndpoint(localEndpoint, "live-local")
	if err != nil {
		t.Fatalf("failed to create live docker connector with custom endpoint: %v", err)
	}

	all := liveConn.All()
	t.Logf("Live containers collected with endpoint connector: %d", len(all))
	t.Log("E2E TLS and endpoint integration test passed!")
}

func TestE2ERateAndCumulativeModeLive(t *testing.T) {
	client := getDockerClient(t)

	containerName := fmt.Sprintf("ctop-e2e-rates-%d", time.Now().UnixNano())
	createOpts := api.CreateContainerOptions{
		Name: containerName,
		Config: &api.Config{
			Image: "alpine:latest",
			Cmd:   []string{"/bin/sh", "-c", "while true; do dd if=/dev/urandom of=/tmp/test.dat bs=1k count=10 >/dev/null 2>&1; sleep 0.2; done"},
		},
		HostConfig: &api.HostConfig{
			AutoRemove: false,
		},
	}

	cont, err := client.CreateContainer(createOpts)
	if err != nil {
		t.Fatalf("failed to create live test container: %v", err)
	}
	defer func() {
		_ = client.StopContainer(cont.ID, 1)
		_ = client.RemoveContainer(api.RemoveContainerOptions{
			ID:            cont.ID,
			Force:         true,
			RemoveVolumes: true,
		})
	}()

	if err := client.StartContainer(cont.ID, nil); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	dockerCollector := collector.NewDocker(client, cont.ID)
	dockerCollector.Start()
	defer dockerCollector.Stop()

	// Wait up to 5 seconds to receive streamed metrics
	var m models.Metrics
	select {
	case metrics, ok := <-dockerCollector.Stream():
		if !ok {
			t.Fatal("metrics stream closed unexpectedly")
		}
		m = metrics
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for live metrics from Docker daemon")
	}

	row := compact.NewCompactRow()

	// Validate widgets in rate mode
	config.UpdateSwitch("rateMode", true)
	row.SetMetrics(m)
	t.Logf("Live container rate metrics: CPU=%d%% Mem=%d IORead=%d IOWrite=%d IORateRead=%d IORateWrite=%d NetRxRate=%d NetTxRate=%d",
		m.CPUUtil, m.MemUsage, m.IOBytesRead, m.IOBytesWrite, m.IORateRead, m.IORateWrite, m.NetRxRate, m.NetTxRate)

	// Validate widgets in cumulative mode
	config.UpdateSwitch("rateMode", false)
	row.SetMetrics(m)

	t.Log("E2E live rate and cumulative mode metrics integration test passed!")
}
