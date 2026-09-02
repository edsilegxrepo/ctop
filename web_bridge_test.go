// web_bridge_test.go validates the web server bridge adapters, SSE live broadcasting, REST API endpoints, and live container conversion.
//
// Objective:
//
//	Verify end-to-end functionality of the web dashboard integration, including container telemetry snapshotting,
//	real-time SSE streaming, HTTP endpoint responses, file previewing, top processes, and error handling.
//
// Test Strategy:
//   - Unit tests against mock connector fixtures verifying provider conversion logic.
//   - Integration tests spinning up actual HTTP listeners on dynamic ports (`127.0.0.1:0`).
//   - Live SSE stream consumption verifying event formatting and event subscriber lifecycle.
//   - URL prefix routing and authentication token validation.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/audit"
	"github.com/edsilegx/ctop/pkg/connector"
	"github.com/edsilegx/ctop/pkg/container"
	"github.com/edsilegx/ctop/pkg/web"
)

func TestWebBridge(t *testing.T) {
	cSuper, err := connector.ByName("mock")
	if err != nil {
		t.Fatalf("failed to initialize mock connector: %v", err)
	}

	srv, cleanup, err := startWebServer("127.0.0.1:0", "0.9.0", "/testprefix", cSuper)
	if err != nil {
		t.Fatalf("failed to start web server: %v", err)
	}
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	prov := &superContainerProvider{cSuper: cSuper}
	snapshots := prov.GetContainerSnapshots()
	if len(snapshots) == 0 {
		t.Fatal("expected mock containers in snapshots")
	}

	sys := web.AggregateSnapshots(snapshots)
	if sys.TotalContainers != len(snapshots) {
		t.Fatalf("expected %d total containers, got %d", len(snapshots), sys.TotalContainers)
	}

	// Test nil cSuper handling
	nilProv := &superContainerProvider{cSuper: nil}
	if nilProv.GetContainerSnapshots() != nil {
		t.Fatal("expected nil for nil cSuper")
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

	prov := &superContainerProvider{cSuper: nil}
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

	srv, cleanup, err := startWebServer("127.0.0.1:0", "0.9.0", "/probe", cSuper)
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

func TestWebBridgeWithOptionsAndAuth(t *testing.T) {
	cSuper, err := connector.ByName("mock")
	if err != nil {
		t.Fatalf("failed to initialize mock connector: %v", err)
	}

	srv, cleanup, err := startWebServer("127.0.0.1:0", "0.9.2", "/authprefix", cSuper, WebOptions{
		URLPrefix: "/authprefix",
		AuthToken: true,
	})
	if err != nil {
		t.Fatalf("failed to start authenticated web bridge: %v", err)
	}

	tokenPath := web.DefaultAuthTokenPath()
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		cleanup()
		t.Fatalf("expected token file at %s: %v", tokenPath, err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if len(token) < 32 {
		cleanup()
		t.Fatalf("expected token length >= 32, got %d: %s", len(token), token)
	}

	// Make authenticated request over live bridge
	client := &http.Client{Timeout: 3 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "http://"+srv.Addr()+"/authprefix/api/v1/containers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		cleanup()
		t.Fatalf("authenticated request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cleanup()
		t.Fatalf("expected 200 OK for valid bearer token, got %d", resp.StatusCode)
	}

	// Cleanup and verify token removal
	cleanup()
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("expected token file to be deleted on shutdown, but it still exists at %s", tokenPath)
	}
}

func TestWebBridgePersistentToken(t *testing.T) {
	cSuper, err := connector.ByName("mock")
	if err != nil {
		t.Fatalf("failed to initialize mock connector: %v", err)
	}

	tokenPath := web.DefaultAuthTokenPath()
	// Pre-cleanup
	web.RemoveSecureTokenFile(tokenPath)
	defer web.RemoveSecureTokenFile(tokenPath)

	// Run 1: Autogenerate persistent token
	srv1, cleanup1, err := startWebServer("127.0.0.1:0", "0.9.3", "", cSuper, WebOptions{
		AuthToken:       true,
		PersistentToken: true,
	})
	if err != nil {
		t.Fatalf("failed to start server 1 with persistent token: %v", err)
	}
	tok1 := srv1.AuthToken()
	if len(tok1) != 64 {
		cleanup1()
		t.Fatalf("expected 64-char token, got %d chars: %s", len(tok1), tok1)
	}

	// Verify token file was written
	tokFile1, err := web.ReadSecureTokenFile(tokenPath)
	if err != nil {
		cleanup1()
		t.Fatalf("failed to read token file: %v", err)
	}
	if tokFile1 != tok1 {
		cleanup1()
		t.Fatalf("expected token file to contain %q, got %q", tok1, tokFile1)
	}

	// Shutdown Run 1: Token MUST persist on disk
	cleanup1()
	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("expected persistent token file to remain on disk after shutdown: %v", err)
	}

	// Run 2: Start new server with persistent token - MUST reuse existing token
	srv2, cleanup2, err := startWebServer("127.0.0.1:0", "0.9.3", "", cSuper, WebOptions{
		AuthToken:       true,
		PersistentToken: true,
	})
	if err != nil {
		t.Fatalf("failed to start server 2 with persistent token: %v", err)
	}
	defer cleanup2()

	tok2 := srv2.AuthToken()
	if tok2 != tok1 {
		t.Fatalf("expected server 2 to reuse persistent token %q, but got newly generated %q", tok1, tok2)
	}

	// Verify authenticated API request works with original token
	client := &http.Client{Timeout: 3 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "http://"+srv2.Addr()+"/api/v1/containers", nil)
	req.Header.Set("Authorization", "Bearer "+tok1)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("authenticated request to server 2 failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for persistent token, got %d", resp.StatusCode)
	}
}

func TestWebBridgeTLSBinding(t *testing.T) {
	cSuper, err := connector.ByName("mock")
	if err != nil {
		t.Fatalf("failed to initialize mock connector: %v", err)
	}

	// 1. Plain HTTP without host binds to 127.0.0.1
	srvHTTP, cleanupHTTP, err := startWebServer(":0", "0.9.2", "", cSuper)
	if err != nil {
		t.Fatalf("failed to start plain HTTP web server: %v", err)
	}
	defer cleanupHTTP()

	if !strings.HasPrefix(srvHTTP.Addr(), "127.0.0.1:") {
		t.Fatalf("expected plain HTTP server to bind 127.0.0.1, got %s", srvHTTP.Addr())
	}

	// 2. TLS server with ":0" binds to dual-stack or IPv4 fallback
	srvTLS, cleanupTLS, err := startWebServer(":0", "0.9.2", "", cSuper, WebOptions{
		TLSCert: "tests/tls/server.crt",
		TLSKey:  "tests/tls/server.key",
	})
	if err != nil {
		t.Fatalf("failed to start TLS web server: %v", err)
	}
	defer cleanupTLS()

	if srvTLS.Addr() == "" {
		t.Fatalf("expected non-empty listening address for TLS server, got %s", srvTLS.Addr())
	}

	// 3. Plain HTTP with explicit 0.0.0.0 and web-auth-token is FORCED to 127.0.0.1
	srvForced, cleanupForced, err := startWebServer("0.0.0.0:0", "0.9.2", "", cSuper, WebOptions{
		AuthToken: true,
	})
	if err != nil {
		t.Fatalf("failed to start web server with auth token: %v", err)
	}
	defer cleanupForced()

	if !strings.HasPrefix(srvForced.Addr(), "127.0.0.1:") {
		t.Fatalf("CRITICAL SECURITY: expected auth-token server without TLS to be forced to 127.0.0.1, got %s", srvForced.Addr())
	}

	// 4. TLS server with explicit 0.0.0.0:0 MUST bind strictly to IPv4 0.0.0.0:
	srvIPv4, cleanupIPv4, err := startWebServer("0.0.0.0:0", "0.9.2", "", cSuper, WebOptions{
		TLSCert: "tests/tls/server.crt",
		TLSKey:  "tests/tls/server.key",
	})
	if err != nil {
		t.Fatalf("failed to start TLS web server on 0.0.0.0:0: %v", err)
	}
	defer cleanupIPv4()

	if !strings.HasPrefix(srvIPv4.Addr(), "0.0.0.0:") {
		t.Fatalf("expected explicit 0.0.0.0:0 to bind strictly to IPv4 0.0.0.0:, got %s", srvIPv4.Addr())
	}
}

func TestWebBridgeAuditLog(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.ndjson")

	cSuper, err := connector.ByName("mock")
	if err != nil {
		t.Fatalf("failed to initialize mock connector: %v", err)
	}

	srv, cleanup, err := startWebServer(":0", "0.9.2", "/probe", cSuper, WebOptions{
		AuditLog: logPath,
	})
	if err != nil {
		t.Fatalf("failed to start web server with audit log: %v", err)
	}
	defer cleanup()

	// Perform HTTP requests
	resp, err := http.Get(fmt.Sprintf("http://%s/probe/api/v1/health", srv.Addr()))
	if err != nil {
		t.Fatalf("failed to execute GET request: %v", err)
	}
	_ = resp.Body.Close()

	// Flush and close audit logger
	audit.Close()

	activePath := audit.Get().ActivePath()
	if activePath == "" {
		// Calculate expected date path
		activePath = filepath.Join(tempDir, fmt.Sprintf("audit-%s.ndjson", time.Now().Format("2006-01-02")))
	}

	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("failed to read audit log file %s: %v", activePath, err)
	}

	content := string(data)
	if !strings.Contains(content, "/probe/api/v1/health") {
		t.Fatalf("expected audit log to record /probe/api/v1/health request, got:\n%s", content)
	}
}
