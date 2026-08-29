package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/connector"
	"github.com/edsilegx/ctop/pkg/container"
	"github.com/edsilegx/ctop/pkg/web"
)

func TestWebBridge(t *testing.T) {
	cSuper, err := connector.ByName("mock")
	if err != nil {
		t.Fatalf("failed to initialize mock connector: %v", err)
	}

	gc := &GridCursor{cSuper: cSuper}
	_, _ = gc.RefreshContainers()

	srv, cleanup, err := startWebServer("127.0.0.1:0", "0.9.0", "/testprefix", gc)
	if err != nil {
		t.Fatalf("failed to start web server: %v", err)
	}
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	prov := &cursorContainerProvider{cursor: gc}
	snapshots := prov.GetContainerSnapshots()
	if len(snapshots) == 0 {
		t.Fatal("expected mock containers in snapshots")
	}

	sys := web.AggregateSnapshots(snapshots)
	if sys.TotalContainers != len(snapshots) {
		t.Fatalf("expected %d total containers, got %d", len(snapshots), sys.TotalContainers)
	}

	// Test nil cursor handling
	nilProv := &cursorContainerProvider{cursor: nil}
	if nilProv.GetContainerSnapshots() != nil {
		t.Fatal("expected nil for nil cursor")
	}

	// Verify server endpoint responds
	resp, err := http.Get("http://" + srv.Addr() + "/api/v1/health")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK from web server, got %d", resp.StatusCode)
		}
	}
}

func TestWebBridgeContainerConversion(t *testing.T) {
	c := container.New("c12345678901234", nil, nil)
	c.SetMeta("name", "test-cont")
	c.SetMeta("image", "redis:alpine")
	c.SetMeta("state", "running")
	c.CPUUtil = 42
	c.MemUsage = 10485760

	gc := &GridCursor{}
	prov := &cursorContainerProvider{cursor: gc}
	_ = prov

	// Test parse helpers
	mounts := parseMounts("/var/lib/mysql:::/data/mysql:::bind:::rw:::local")
	if len(mounts) != 1 || mounts[0].Destination != "/var/lib/mysql" || mounts[0].Mode != "rw" {
		t.Fatalf("unexpected parsed mounts: %+v", mounts)
	}

	nets := parseNetworks("app-net:::172.20.0.5:::172.20.0.1:::02:42:ac:14:00:05:::16")
	if len(nets) != 1 || nets[0].IPAddress != "172.20.0.5" || nets[0].PrefixLen != 16 {
		t.Fatalf("unexpected parsed networks: %+v", nets)
	}

	labels := parseLabels("app=web;;env=prod;;API_KEY=secret123;;AWS_SECRET=abc")
	if labels["app"] != "web" || labels["env"] != "prod" {
		t.Fatalf("unexpected parsed labels: %+v", labels)
	}
	if labels["API_KEY"] != "" || labels["AWS_SECRET"] != "" {
		t.Fatalf("sensitive labels leaked in web bridge: %+v", labels)
	}

	env := parseEnv("PORT=8080;DEBUG=1;DB_PASSWORD=secret_pass;GITHUB_TOKEN=ghp_12345;DATABASE_URL=postgres://user:pass@host/db")
	if len(env) != 2 || env[0] != "PORT=8080" || env[1] != "DEBUG=1" {
		t.Fatalf("unexpected parsed env with sensitive vars: %+v", env)
	}
}

func TestWebBridgeE2E(t *testing.T) {
	cSuper, err := connector.ByName("mock")
	if err != nil {
		t.Fatalf("failed to initialize mock connector: %v", err)
	}

	gc := &GridCursor{cSuper: cSuper}
	_, _ = gc.RefreshContainers()

	srv, cleanup, err := startWebServer("127.0.0.1:0", "0.9.0", "/probe", gc)
	if err != nil {
		t.Fatalf("failed to start web server in E2E: %v", err)
	}
	defer cleanup()

	time.Sleep(150 * time.Millisecond)

	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := "http://" + srv.Addr() + "/probe"

	// 1. Health check under prefix
	respHealth, err := client.Get(baseURL + "/api/v1/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer func() { _ = respHealth.Body.Close() }()
	if respHealth.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for health, got %d", respHealth.StatusCode)
	}
	var health web.HealthStatus
	if err := json.NewDecoder(respHealth.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode health JSON: %v", err)
	}
	if health.Status != "ok" || health.Version != "0.9.0" {
		t.Fatalf("unexpected health payload: %+v", health)
	}

	// 2. Metrics endpoint
	respMetrics, err := client.Get(baseURL + "/api/v1/metrics")
	if err != nil {
		t.Fatalf("metrics query failed: %v", err)
	}
	defer func() { _ = respMetrics.Body.Close() }()
	if respMetrics.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for metrics, got %d", respMetrics.StatusCode)
	}
	var sys web.SystemMetrics
	if err := json.NewDecoder(respMetrics.Body).Decode(&sys); err != nil {
		t.Fatalf("failed to decode system metrics: %v", err)
	}
	if sys.TotalContainers == 0 {
		t.Fatal("expected mock containers in metrics")
	}

	// 3. Containers list
	respContainers, err := client.Get(baseURL + "/api/v1/containers")
	if err != nil {
		t.Fatalf("containers list failed: %v", err)
	}
	defer func() { _ = respContainers.Body.Close() }()
	if respContainers.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for containers, got %d", respContainers.StatusCode)
	}
	var list []web.ContainerSnapshot
	if err := json.NewDecoder(respContainers.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode container snapshots: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected non-empty container list")
	}
	firstID := list[0].ID

	// 4. Single container detail
	respSingle, err := client.Get(baseURL + "/api/v1/containers/" + firstID)
	if err != nil {
		t.Fatalf("single container query failed: %v", err)
	}
	defer func() { _ = respSingle.Body.Close() }()
	if respSingle.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for single container, got %d", respSingle.StatusCode)
	}

	// 5. Single container not found (404)
	respNotFound, err := client.Get(baseURL + "/api/v1/containers/nonexistent-id-999")
	if err != nil {
		t.Fatalf("not found query failed: %v", err)
	}
	defer func() { _ = respNotFound.Body.Close() }()
	if respNotFound.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing container, got %d", respNotFound.StatusCode)
	}

	// 6. In-container top endpoint
	respTop, err := client.Get(baseURL + "/api/v1/containers/" + firstID + "/top")
	if err != nil {
		t.Fatalf("top query failed: %v", err)
	}
	defer func() { _ = respTop.Body.Close() }()
	if respTop.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for top, got %d", respTop.StatusCode)
	}

	// 7. Cluster Export JSON
	respExport, err := client.Get(baseURL + "/api/v1/export")
	if err != nil {
		t.Fatalf("cluster export failed: %v", err)
	}
	defer func() { _ = respExport.Body.Close() }()
	if respExport.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for export, got %d", respExport.StatusCode)
	}

	// 8. Single container JSON export
	respSingleExport, err := client.Get(baseURL + "/api/v1/export?container=" + firstID)
	if err != nil {
		t.Fatalf("single container export failed: %v", err)
	}
	defer func() { _ = respSingleExport.Body.Close() }()
	if respSingleExport.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for single export, got %d", respSingleExport.StatusCode)
	}

	// 9. Single container export not found (404)
	respMissingExport, err := client.Get(baseURL + "/api/v1/export?container=missing-999")
	if err != nil {
		t.Fatalf("missing container export failed: %v", err)
	}
	defer func() { _ = respMissingExport.Body.Close() }()
	if respMissingExport.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing container export, got %d", respMissingExport.StatusCode)
	}

	// 10. Live SSE Stream Connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	reqSSE, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/stream", nil)
	if err != nil {
		t.Fatalf("failed to create SSE request: %v", err)
	}
	respSSE, err := client.Do(reqSSE)
	if err != nil {
		t.Fatalf("SSE connection failed: %v", err)
	}
	defer func() { _ = respSSE.Body.Close() }()
	if respSSE.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for SSE stream, got %d", respSSE.StatusCode)
	}
	sseReader := bufio.NewReader(respSSE.Body)
	initialLine, err := sseReader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read initial SSE stream line: %v", err)
	}
	if !strings.HasPrefix(initialLine, "data: ") {
		t.Fatalf("expected SSE data line, got: %s", initialLine)
	}

	// 11. Security check: Mutating request rejected across routes
	mutatingRoutes := []string{
		baseURL + "/api/v1/containers",
		baseURL + "/api/v1/containers/" + firstID,
		baseURL + "/api/v1/metrics",
	}
	for _, route := range mutatingRoutes {
		respPost, err := client.Post(route, "application/json", nil)
		if err != nil {
			t.Fatalf("post request failed on %s: %v", route, err)
		}
		_ = respPost.Body.Close()
		if respPost.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("CRITICAL SECURITY: expected 405 MethodNotAllowed on POST %s, got %d", route, respPost.StatusCode)
		}
	}
}
