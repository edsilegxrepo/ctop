// Package web_test provides unit, integration, and security test suites for the embedded HTTP server and SSE broadcaster.
//
// Test Strategy:
//   - REST Endpoints: Test /api/v1/health, /containers, /metrics, /schema, /diff, and file browsing.
//   - Security & Auth: Test mandatory 64-character token authentication, session cookies, CSRF/CORS, and IP rate limiting.
//   - Zero-Leak & SSRF Defense: Test loopback vs external TLS guards, unexposed port blocking, and multi-hop proxy IP extraction.
//   - Real-Time Streaming: Test SSE client subscription, broadcast fan-out, circular ring buffer, and slow-subscriber non-blocking drops.
package web

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/serviceprobe"
)

type mockContainerProvider struct {
	snapshots []ContainerSnapshot
}

func (m *mockContainerProvider) GetContainerSnapshots() []ContainerSnapshot {
	return m.snapshots
}

func TestWebServerHealth(t *testing.T) {
	s := NewServer("127.0.0.1:0", "0.9.0", nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	s.corsMiddleware(s.mux).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var status HealthStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode health JSON: %v", err)
	}
	if status.Status != "ok" || status.Version != "0.9.0" {
		t.Fatalf("unexpected health response: %+v", status)
	}
}

func TestWebServerMetricsAndContainers(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{
			{
				ID:          "cont-alpha-123",
				Name:        "web-service",
				Image:       "nginx:alpine",
				State:       "running",
				CPUUtil:     15,
				MemUsage:    104857600, // 100 MB
				MemLimit:    524288000, // 500 MB
				MemPercent:  20,
				NetRxRate:   10240,
				NetTxRate:   20480,
				IORateRead:  4096,
				IORateWrite: 8192,
				Pids:        4,
			},
			{
				ID:       "cont-beta-456",
				Name:     "db-service",
				Image:    "postgres:15",
				State:    "paused",
				CPUUtil:  0,
				MemUsage: 209715200, // 200 MB
				MemLimit: 524288000,
				Pids:     8,
			},
		},
	}

	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, nil)

	// 1. Test /api/v1/metrics
	reqMetrics := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	wMetrics := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wMetrics, reqMetrics)

	if wMetrics.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /metrics, got %d", wMetrics.Code)
	}

	var sys SystemMetrics
	if err := json.NewDecoder(wMetrics.Body).Decode(&sys); err != nil {
		t.Fatalf("failed to decode metrics JSON: %v", err)
	}
	if sys.TotalContainers != 2 || sys.RunningContainers != 1 || sys.PausedContainers != 1 {
		t.Fatalf("unexpected container counts in metrics: %+v", sys)
	}
	if sys.TotalCPUUtil != 15 || sys.TotalMemUsage != 314572800 {
		t.Fatalf("unexpected resource totals in metrics: %+v", sys)
	}

	// 2. Test /api/v1/containers
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	wList := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /containers, got %d", wList.Code)
	}

	var list []ContainerSnapshot
	if err := json.NewDecoder(wList.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode containers list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(list))
	}

	// 3. Test /api/v1/containers/{id}
	reqDetail := httptest.NewRequest(http.MethodGet, "/api/v1/containers/cont-alpha", nil)
	wDetail := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wDetail, reqDetail)

	if wDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /containers/cont-alpha, got %d", wDetail.Code)
	}
	var detail ContainerSnapshot
	if err := json.NewDecoder(wDetail.Body).Decode(&detail); err != nil {
		t.Fatalf("failed to decode container detail: %v", err)
	}
	if detail.Name != "web-service" {
		t.Fatalf("expected 'web-service', got %s", detail.Name)
	}

	// 4. Test /api/v1/containers/notfound
	reqNotFound := httptest.NewRequest(http.MethodGet, "/api/v1/containers/nonexistent", nil)
	wNotFound := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wNotFound, reqNotFound)
	if wNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent container, got %d", wNotFound.Code)
	}
}

func TestWebServerExportAndIndex(t *testing.T) {
	s := NewServer("127.0.0.1:0", "0.9.0", nil, nil)

	// Test Index HTML
	reqIndex := httptest.NewRequest(http.MethodGet, "/", nil)
	wIndex := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wIndex, reqIndex)
	if wIndex.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for index, got %d", wIndex.Code)
	}
	if !strings.Contains(wIndex.Body.String(), "<title>ctop") {
		t.Fatal("expected HTML content to contain ctop title")
	}

	// Test Export (Cluster)
	reqExport := httptest.NewRequest(http.MethodGet, "/api/v1/export", nil)
	wExport := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wExport, reqExport)
	if wExport.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for export, got %d", wExport.Code)
	}
	if !strings.Contains(wExport.Header().Get("Content-Disposition"), "ctop-telemetry-export.json") {
		t.Fatalf("unexpected content disposition: %s", wExport.Header().Get("Content-Disposition"))
	}
	if !strings.Contains(wExport.Body.String(), "  \"timestamp\":") {
		t.Fatal("expected pretty-formatted JSON with indentation")
	}

	// Test Export (Single Container)
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{
			{ID: "c1", Name: "my-app", State: "running"},
		},
	}
	sWithProv := NewServer("127.0.0.1:0", "0.9.0", mockProv, nil)
	reqSingle := httptest.NewRequest(http.MethodGet, "/api/v1/export?container=my-app", nil)
	wSingle := httptest.NewRecorder()
	sWithProv.corsMiddleware(sWithProv.mux).ServeHTTP(wSingle, reqSingle)
	if wSingle.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for single container export, got %d", wSingle.Code)
	}
	if !strings.Contains(wSingle.Header().Get("Content-Disposition"), "ctop-container-my-app.json") {
		t.Fatalf("unexpected disposition for single export: %s", wSingle.Header().Get("Content-Disposition"))
	}
}

func TestWebServerSecurityReadOnly(t *testing.T) {
	s := NewServer("127.0.0.1:0", "0.9.0", nil, nil)

	mutatingMethods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	endpoints := []string{"/", "/api/v1/metrics", "/api/v1/containers", "/api/v1/containers/123", "/api/v1/stream", "/api/v1/trigger"}

	for _, method := range mutatingMethods {
		for _, ep := range endpoints {
			req := httptest.NewRequest(method, ep, nil)
			w := httptest.NewRecorder()
			s.corsMiddleware(s.mux).ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("CRITICAL SECURITY VIOLATION: expected 405 MethodNotAllowed on %s %s, got %d", method, ep, w.Code)
			}
		}
	}
}

func TestWebBroadcaster(t *testing.T) {
	b := NewBroadcaster()
	if b.GetLatestEvent().Type != "" {
		t.Fatal("expected empty latest event initially")
	}

	b.SetMaxHistory(50)
	ch := b.Subscribe()
	if b.SubscriberCount() != 1 {
		t.Fatalf("expected 1 subscriber, got %d", b.SubscriberCount())
	}

	testEv := TelemetryEvent{
		Type:      "update",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		System: SystemMetrics{
			TotalContainers: 5,
		},
	}

	b.Broadcast(testEv)

	select {
	case received := <-ch:
		if received.System.TotalContainers != 5 {
			t.Fatalf("expected 5 total containers, got %d", received.System.TotalContainers)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for broadcast event")
	}

	latest := b.GetLatestEvent()
	if latest.System.TotalContainers != 5 {
		t.Fatalf("expected latest event to have 5 containers, got %d", latest.System.TotalContainers)
	}

	b.Unsubscribe(ch)
	if b.SubscriberCount() != 0 {
		t.Fatalf("expected 0 subscribers after unsubscribe, got %d", b.SubscriberCount())
	}
}

func TestBroadcasterMaxSubscribers(t *testing.T) {
	b := NewBroadcaster()
	b.SetMaxSubscribers(2)

	ch1 := b.Subscribe()
	if ch1 == nil {
		t.Fatal("expected ch1 to be non-nil")
	}
	ch2 := b.Subscribe()
	if ch2 == nil {
		t.Fatal("expected ch2 to be non-nil")
	}
	ch3 := b.Subscribe()
	if ch3 != nil {
		t.Fatal("expected ch3 to be nil when max subscribers reached")
	}

	b.Unsubscribe(ch1)
	ch4 := b.Subscribe()
	if ch4 == nil {
		t.Fatal("expected ch4 to succeed after unsubscribing ch1")
	}
	b.Unsubscribe(ch2)
	b.Unsubscribe(ch4)
}

func TestWebServerLifecycle(t *testing.T) {
	s := NewServer("127.0.0.1:0", "0.9.0", nil, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.Stop(ctx); err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}
}

func TestWebServerSSEStreamLive(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{
			{ID: "stream-c1", Name: "app-1", State: "running"},
		},
	}
	broadcaster := NewBroadcaster()
	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, broadcaster)

	server := httptest.NewServer(s.corsMiddleware(s.mux))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/stream", nil)
	if err != nil {
		t.Fatalf("failed to create SSE request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to SSE endpoint: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for SSE, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected Content-Type text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	reader := bufio.NewReader(resp.Body)

	// Read initial snapshot event line
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read initial SSE line: %v", err)
	}
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("expected SSE data line, got: %s", line)
	}

	var initialEv TelemetryEvent
	dataStr := strings.TrimPrefix(line, "data: ")
	if err := json.Unmarshal([]byte(dataStr), &initialEv); err != nil {
		t.Fatalf("failed to parse initial SSE JSON: %v", err)
	}
	if len(initialEv.Containers) != 1 || initialEv.Containers[0].ID != "stream-c1" {
		t.Fatalf("unexpected initial container snapshot in SSE: %+v", initialEv)
	}
}

type mockTopProvider struct {
	mockContainerProvider
}

func (m *mockTopProvider) GetContainerTop(id string) (TopResult, error) {
	return TopResult{
		Titles: []string{"PID", "USER", "%CPU", "%MEM", "COMMAND"},
		Processes: [][]string{
			{"1", "root", "0.2", "0.5", "/bin/app"},
			{"42", "root", "0.0", "0.1", "worker"},
		},
	}, nil
}

func TestWebServerTopAndInspect(t *testing.T) {
	prov := &mockTopProvider{
		mockContainerProvider: mockContainerProvider{
			snapshots: []ContainerSnapshot{
				{
					ID:      "cont-inspect-1",
					Name:    "inspect-app",
					Image:   "app:latest",
					State:   "running",
					Command: "/bin/app --verbose",
					Env:     []string{"ENV=production", "PORT=8080"},
					Mounts: []MountInfo{
						{Destination: "/data", Source: "/var/lib/data", Type: "bind", Mode: "rw"},
					},
					Networks: []NetworkInfo{
						{Name: "bridge", IPAddress: "172.17.0.2", Gateway: "172.17.0.1", PrefixLen: 16},
					},
				},
			},
		},
	}

	s := NewServer("127.0.0.1:0", "0.9.0", prov, nil)

	// Test /top endpoint
	reqTop := httptest.NewRequest(http.MethodGet, "/api/v1/containers/cont-inspect-1/top", nil)
	wTop := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wTop, reqTop)

	if wTop.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /top, got %d", wTop.Code)
	}

	var topRes TopResult
	if err := json.NewDecoder(wTop.Body).Decode(&topRes); err != nil {
		t.Fatalf("failed to decode top JSON: %v", err)
	}
	if len(topRes.Processes) != 2 || topRes.Processes[0][0] != "1" {
		t.Fatalf("unexpected top processes: %+v", topRes)
	}

	// Test inspect metadata
	reqInspect := httptest.NewRequest(http.MethodGet, "/api/v1/containers/cont-inspect-1", nil)
	wInspect := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wInspect, reqInspect)

	var snap ContainerSnapshot
	if err := json.NewDecoder(wInspect.Body).Decode(&snap); err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}
	if len(snap.Mounts) != 1 || snap.Mounts[0].Destination != "/data" {
		t.Fatalf("unexpected mounts: %+v", snap.Mounts)
	}
	if len(snap.Networks) != 1 || snap.Networks[0].IPAddress != "172.17.0.2" {
		t.Fatalf("unexpected networks: %+v", snap.Networks)
	}
	if len(snap.Env) != 2 || snap.Env[0] != "ENV=production" {
		t.Fatalf("unexpected env: %+v", snap.Env)
	}
}

func TestWebServerURLPrefix(t *testing.T) {
	s := NewServer("127.0.0.1:0", "0.9.0", nil, nil, "/probe")

	if s.URLPrefix() != "/probe" {
		t.Fatalf("expected urlPrefix '/probe', got '%s'", s.URLPrefix())
	}

	// Test index under prefix
	reqIndex := httptest.NewRequest(http.MethodGet, "/probe/", nil)
	wIndex := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wIndex, reqIndex)
	if wIndex.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /probe/, got %d", wIndex.Code)
	}
	if !strings.Contains(wIndex.Body.String(), `const BASE_PATH = "/probe";`) {
		t.Fatal("expected HTML to contain injected BASE_PATH = \"/probe\"")
	}

	// Test health endpoint under prefix
	reqHealth := httptest.NewRequest(http.MethodGet, "/probe/api/v1/health", nil)
	wHealth := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wHealth, reqHealth)
	if wHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /probe/api/v1/health, got %d", wHealth.Code)
	}

	// Test root access still works
	reqRootHealth := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	wRootHealth := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wRootHealth, reqRootHealth)
	if wRootHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for root /api/v1/health, got %d", wRootHealth.Code)
	}
}

func readSSEDataLine(reader *bufio.Reader) (string, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "data:")), nil
		}
	}
}

func TestWebServerAPIErrorsAndEdgeCases(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{
			{ID: "c-exist-1", Name: "exist-app", State: "running"},
		},
	}
	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, nil)

	// 1. Test OPTIONS request (CORS preflight)
	reqOpt := httptest.NewRequest(http.MethodOptions, "/api/v1/metrics", nil)
	wOpt := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wOpt, reqOpt)
	if wOpt.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for OPTIONS, got %d", wOpt.Code)
	}
	if wOpt.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin '*', got %s", wOpt.Header().Get("Access-Control-Allow-Origin"))
	}
	if !strings.Contains(wOpt.Header().Get("Access-Control-Allow-Methods"), "GET") {
		t.Fatalf("expected GET in Access-Control-Allow-Methods, got %s", wOpt.Header().Get("Access-Control-Allow-Methods"))
	}

	// 2. Test HEAD request
	reqHead := httptest.NewRequest(http.MethodHead, "/api/v1/health", nil)
	wHead := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wHead, reqHead)
	if wHead.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for HEAD /health, got %d", wHead.Code)
	}

	// 3. Test 404 on unknown index subpath
	reqUnknownIndex := httptest.NewRequest(http.MethodGet, "/invalid-subpath", nil)
	wUnknownIndex := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wUnknownIndex, reqUnknownIndex)
	if wUnknownIndex.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown index subpath, got %d", wUnknownIndex.Code)
	}

	// 4. Test 404 on unknown API endpoint
	reqUnknownAPI := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	wUnknownAPI := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wUnknownAPI, reqUnknownAPI)
	if wUnknownAPI.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown API route, got %d", wUnknownAPI.Code)
	}

	// 5. Test 404 for /top on non-existent container
	reqTopNotFound := httptest.NewRequest(http.MethodGet, "/api/v1/containers/missing-id/top", nil)
	wTopNotFound := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wTopNotFound, reqTopNotFound)
	if wTopNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for /top on non-existent container, got %d", wTopNotFound.Code)
	}

	// 6. Test 404 for export on non-existent container
	reqExportNotFound := httptest.NewRequest(http.MethodGet, "/api/v1/export?container=missing-id", nil)
	wExportNotFound := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wExportNotFound, reqExportNotFound)
	if wExportNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for /export with missing container, got %d", wExportNotFound.Code)
	}

	// 7. Test nil provider returns empty slice for /containers and zeroes for /metrics
	sNil := NewServer("127.0.0.1:0", "0.9.0", nil, nil)
	reqNilCont := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	wNilCont := httptest.NewRecorder()
	sNil.corsMiddleware(sNil.mux).ServeHTTP(wNilCont, reqNilCont)
	if wNilCont.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for nil provider /containers, got %d", wNilCont.Code)
	}
	var emptyList []ContainerSnapshot
	if err := json.NewDecoder(wNilCont.Body).Decode(&emptyList); err != nil {
		t.Fatalf("failed to decode empty containers list: %v", err)
	}
	if len(emptyList) != 0 {
		t.Fatalf("expected empty containers list for nil provider, got %d", len(emptyList))
	}

	// 8. Test trailing slash variants
	endpoints := []string{"/api/v1/health/", "/api/v1/metrics/", "/api/v1/export/"}
	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		w := httptest.NewRecorder()
		s.corsMiddleware(s.mux).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for trailing slash route %s, got %d", ep, w.Code)
		}
	}
}

type mockFailingTopProvider struct {
	mockContainerProvider
}

func (m *mockFailingTopProvider) GetContainerTop(id string) (TopResult, error) {
	return TopResult{}, http.ErrHandlerTimeout
}

func TestWebServerTopErrorHandling(t *testing.T) {
	prov := &mockFailingTopProvider{
		mockContainerProvider: mockContainerProvider{
			snapshots: []ContainerSnapshot{
				{ID: "c-fail", Name: "failing-cont", State: "running"},
			},
		},
	}
	s := NewServer("127.0.0.1:0", "0.9.0", prov, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c-fail/top", nil)
	w := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error when Top fails, got %d", w.Code)
	}
}

func TestWebServerBroadcasterStreamBroadcast(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{
			{ID: "stream-c2", Name: "app-2", State: "running"},
		},
	}
	broadcaster := NewBroadcaster()
	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, broadcaster)

	server := httptest.NewServer(s.corsMiddleware(s.mux))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/stream", nil)
	if err != nil {
		t.Fatalf("failed to create SSE request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to SSE stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	reader := bufio.NewReader(resp.Body)

	// Read initial event
	initialData, err := readSSEDataLine(reader)
	if err != nil {
		t.Fatalf("failed to read initial SSE event: %v", err)
	}
	if !strings.Contains(initialData, "stream-c2") {
		t.Fatalf("unexpected initial SSE data: %s", initialData)
	}

	// Broadcast an update event
	go func() {
		time.Sleep(50 * time.Millisecond)
		broadcaster.Broadcast(TelemetryEvent{
			Type:      "update",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Containers: []ContainerSnapshot{
				{ID: "stream-c2", Name: "app-2-updated", State: "running"},
			},
		})
	}()

	// Read broadcasted event
	broadcastData, err := readSSEDataLine(reader)
	if err != nil {
		t.Fatalf("failed to read broadcasted SSE data line: %v", err)
	}

	var updateEv TelemetryEvent
	if err := json.Unmarshal([]byte(broadcastData), &updateEv); err != nil {
		t.Fatalf("failed to parse broadcasted SSE JSON: %v", err)
	}
	if len(updateEv.Containers) != 1 || updateEv.Containers[0].Name != "app-2-updated" {
		t.Fatalf("unexpected updated container in broadcast: %+v", updateEv)
	}
}

func TestWebServerAuthToken(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{{ID: "auth-c1", Name: "auth-app"}},
	}
	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, nil)
	validToken, err := s.EnableAuth()
	if err != nil {
		t.Fatalf("unexpected error enabling auth: %v", err)
	}

	// 1. External client without TLS or proxy header -> 403 Forbidden
	reqExternalInsecure := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	reqExternalInsecure.RemoteAddr = "192.168.1.50:54321"
	reqExternalInsecure.Header.Set("Authorization", "Bearer "+validToken)
	wExternalInsecure := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wExternalInsecure, reqExternalInsecure)
	if wExternalInsecure.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for external unencrypted request, got %d", wExternalInsecure.Code)
	}

	// 2. External client with TLS header but missing token -> 401 Unauthorized
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	reqNoAuth.RemoteAddr = "192.168.1.50:54321"
	reqNoAuth.Header.Set("X-Forwarded-Proto", "https")
	wNoAuth := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wNoAuth, reqNoAuth)
	if wNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", wNoAuth.Code)
	}

	// 3. External client with TLS header and invalid Bearer token -> 401 Unauthorized
	reqBadAuth := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	reqBadAuth.RemoteAddr = "192.168.1.50:54321"
	reqBadAuth.Header.Set("X-Forwarded-Proto", "https")
	reqBadAuth.Header.Set("Authorization", "Bearer invalid-token")
	wBadAuth := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wBadAuth, reqBadAuth)
	if wBadAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for bad token, got %d", wBadAuth.Code)
	}

	// 4. External client with TLS header and valid Bearer header -> 200 OK
	reqGoodHeader := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	reqGoodHeader.RemoteAddr = "192.168.1.50:54321"
	reqGoodHeader.Header.Set("X-Forwarded-Proto", "https")
	reqGoodHeader.Header.Set("Authorization", "Bearer "+validToken)
	wGoodHeader := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wGoodHeader, reqGoodHeader)
	if wGoodHeader.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid Bearer token, got %d", wGoodHeader.Code)
	}

	// 5. Query parameter tokens ?token= and ?auth= MUST be rejected with 401 Unauthorized (Zero-Leak Policy)
	reqQueryToken := httptest.NewRequest(http.MethodGet, "/api/v1/containers?token="+validToken, nil)
	reqQueryToken.RemoteAddr = "192.168.1.50:54321"
	reqQueryToken.Header.Set("X-Forwarded-Proto", "https")
	wQueryToken := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wQueryToken, reqQueryToken)
	if wQueryToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for deprecated query token ?token=, got %d", wQueryToken.Code)
	}

	reqQueryAuth := httptest.NewRequest(http.MethodGet, "/api/v1/containers?auth="+validToken, nil)
	reqQueryAuth.RemoteAddr = "192.168.1.50:54321"
	reqQueryAuth.Header.Set("X-Forwarded-Proto", "https")
	wQueryAuth := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wQueryAuth, reqQueryAuth)
	if wQueryAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for deprecated query auth ?auth=, got %d", wQueryAuth.Code)
	}
}

func TestWebServerSessionCookie(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{{ID: "sess-c1", Name: "sess-app"}},
	}
	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, nil)
	validToken, err := s.EnableAuth()
	if err != nil {
		t.Fatalf("unexpected error enabling auth: %v", err)
	}

	// 1. POST /api/v1/auth/login with invalid token -> 401 Unauthorized
	badLoginBody := strings.NewReader(`{"token":"wrong-token"}`)
	reqBadLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", badLoginBody)
	reqBadLogin.RemoteAddr = "198.51.100.10:12345"
	reqBadLogin.Header.Set("X-Forwarded-Proto", "https")
	reqBadLogin.Header.Set("Content-Type", "application/json")
	wBadLogin := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wBadLogin, reqBadLogin)
	if wBadLogin.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized on bad login, got %d", wBadLogin.Code)
	}

	// 2. POST /api/v1/auth/login with valid token -> 200 OK + Set-Cookie ctop_session
	goodLoginBody := strings.NewReader(fmt.Sprintf(`{"token":%q}`, validToken))
	reqGoodLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", goodLoginBody)
	reqGoodLogin.RemoteAddr = "198.51.100.10:12345"
	reqGoodLogin.Header.Set("X-Forwarded-Proto", "https")
	reqGoodLogin.Header.Set("Content-Type", "application/json")
	wGoodLogin := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wGoodLogin, reqGoodLogin)
	if wGoodLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on valid login, got %d: %s", wGoodLogin.Code, wGoodLogin.Body.String())
	}

	cookies := wGoodLogin.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "ctop_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("expected ctop_session cookie in login response, got none")
	}
	if !sessionCookie.HttpOnly {
		t.Fatalf("expected session cookie to have HttpOnly=true")
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("expected session cookie to have SameSite=Strict")
	}

	// 3. GET /api/v1/containers with ctop_session cookie -> 200 OK
	reqWithCookie := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	reqWithCookie.RemoteAddr = "198.51.100.10:12345"
	reqWithCookie.Header.Set("X-Forwarded-Proto", "https")
	reqWithCookie.AddCookie(sessionCookie)
	wWithCookie := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wWithCookie, reqWithCookie)
	if wWithCookie.Code != http.StatusOK {
		t.Fatalf("expected 200 OK using session cookie, got %d", wWithCookie.Code)
	}

	// 4. POST /api/v1/auth/logout -> 200 OK + clears cookie
	reqLogout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	reqLogout.RemoteAddr = "198.51.100.10:12345"
	reqLogout.Header.Set("X-Forwarded-Proto", "https")
	reqLogout.AddCookie(sessionCookie)
	wLogout := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wLogout, reqLogout)
	if wLogout.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on logout, got %d", wLogout.Code)
	}

	// 5. Subsequent request with revoked cookie -> 401 Unauthorized
	reqRevoked := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	reqRevoked.RemoteAddr = "198.51.100.10:12345"
	reqRevoked.Header.Set("X-Forwarded-Proto", "https")
	reqRevoked.AddCookie(sessionCookie)
	wRevoked := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wRevoked, reqRevoked)
	if wRevoked.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized with revoked cookie, got %d", wRevoked.Code)
	}
}

func TestWebServerDirectLocalAccess(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{{ID: "loc-c1", Name: "loc-app"}},
	}
	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, nil)
	_, err := s.EnableAuth()
	if err != nil {
		t.Fatalf("unexpected error enabling auth: %v", err)
	}

	// 1. Direct local loopback request without proxy headers -> 200 OK without token
	reqDirectLocal := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	reqDirectLocal.RemoteAddr = "127.0.0.1:45678"
	wDirectLocal := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wDirectLocal, reqDirectLocal)
	if wDirectLocal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for direct local loopback request, got %d", wDirectLocal.Code)
	}

	// 2. Loopback request WITH X-Forwarded-For header -> classified as remote, requires TLS + auth -> 403 / 401
	reqProxiedLocal := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	reqProxiedLocal.RemoteAddr = "127.0.0.1:45678"
	reqProxiedLocal.Header.Set("X-Forwarded-For", "203.0.113.50")
	wProxiedLocal := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wProxiedLocal, reqProxiedLocal)
	if wProxiedLocal.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for proxied unencrypted loopback request, got %d", wProxiedLocal.Code)
	}

	// 3. Loopback request WITH X-Forwarded-For and X-Forwarded-Proto https (no token) -> 401 Unauthorized
	reqProxiedSecureNoToken := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	reqProxiedSecureNoToken.RemoteAddr = "127.0.0.1:45678"
	reqProxiedSecureNoToken.Header.Set("X-Forwarded-For", "203.0.113.50")
	reqProxiedSecureNoToken.Header.Set("X-Forwarded-Proto", "https")
	wProxiedSecureNoToken := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wProxiedSecureNoToken, reqProxiedSecureNoToken)
	if wProxiedSecureNoToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for proxied remote request without token, got %d", wProxiedSecureNoToken.Code)
	}
}

func TestWebServerSessionCapacityAndTTL(t *testing.T) {
	// Create session store with capacity 3 and 50ms TTL for testing
	store := NewSessionStore(3, 50*time.Millisecond, 50*time.Millisecond)

	s1, _ := store.CreateSession()
	s2, _ := store.CreateSession()
	s3, _ := store.CreateSession()

	if store.Count() != 3 {
		t.Fatalf("expected 3 sessions, got %d", store.Count())
	}
	if !store.ValidateSession(s1) || !store.ValidateSession(s2) || !store.ValidateSession(s3) {
		t.Fatalf("all initial sessions should be valid")
	}

	// Adding 4th session should evict oldest session (s1 was validated, s2 was validated, s3 was validated, order updated)
	s4, _ := store.CreateSession()
	if store.Count() > 3 {
		t.Fatalf("expected max 3 sessions, got %d", store.Count())
	}
	if !store.ValidateSession(s4) {
		t.Fatalf("new session s4 should be valid")
	}

	// Wait for TTL expiration
	time.Sleep(60 * time.Millisecond)
	if store.ValidateSession(s4) {
		t.Fatalf("session s4 should have expired after TTL")
	}
}

func TestWebServerLoginRateLimiting(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{{ID: "rate-c1", Name: "rate-app"}},
	}
	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, nil)
	validToken, err := s.EnableAuth()
	if err != nil {
		t.Fatalf("unexpected error enabling auth: %v", err)
	}

	attackerIP := "198.51.100.99"

	// 5 failed login attempts should be allowed (returning 401)
	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"token":"wrong"}`))
		req.RemoteAddr = attackerIP + ":1234"
		req.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		s.corsMiddleware(s.mux).ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401 Unauthorized, got %d", i, w.Code)
		}
	}

	// 6th failed attempt should trigger 429 Too Many Requests
	reqBlocked := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"token":"wrong"}`))
	reqBlocked.RemoteAddr = attackerIP + ":1234"
	reqBlocked.Header.Set("X-Forwarded-Proto", "https")
	wBlocked := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wBlocked, reqBlocked)
	if wBlocked.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 6: expected 429 Too Many Requests, got %d", wBlocked.Code)
	}
	if retry := wBlocked.Header().Get("Retry-After"); retry != "60" {
		t.Fatalf("expected Retry-After: 60 header, got %q", retry)
	}

	// Different IP should NOT be blocked
	otherIP := "198.51.100.100"
	reqOther := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(fmt.Sprintf(`{"token":%q}`, validToken)))
	reqOther.RemoteAddr = otherIP + ":1234"
	reqOther.Header.Set("X-Forwarded-Proto", "https")
	wOther := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wOther, reqOther)
	if wOther.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for separate unblocked IP, got %d", wOther.Code)
	}
}

func TestWebServerAuthStatus(t *testing.T) {
	s := NewServer("127.0.0.1:0", "0.9.0", nil, nil)
	validToken, _ := s.EnableAuth()

	// 1. Direct local -> direct_local=true, authenticated=true
	reqLocal := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	reqLocal.RemoteAddr = "127.0.0.1:1234"
	wLocal := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wLocal, reqLocal)
	var respLocal AuthStatusResponse
	_ = json.NewDecoder(wLocal.Body).Decode(&respLocal)
	if !respLocal.Authenticated || !respLocal.DirectLocal {
		t.Fatalf("expected authenticated=true and direct_local=true, got %+v", respLocal)
	}

	// 2. Remote without credentials -> authenticated=false, direct_local=false
	reqRemote := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	reqRemote.RemoteAddr = "203.0.113.1:1234"
	reqRemote.Header.Set("X-Forwarded-Proto", "https")
	wRemote := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wRemote, reqRemote)
	var respRemote AuthStatusResponse
	_ = json.NewDecoder(wRemote.Body).Decode(&respRemote)
	if respRemote.Authenticated || respRemote.DirectLocal {
		t.Fatalf("expected authenticated=false and direct_local=false, got %+v", respRemote)
	}

	// 3. Remote with Bearer header -> authenticated=true
	reqBearer := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	reqBearer.RemoteAddr = "203.0.113.1:1234"
	reqBearer.Header.Set("X-Forwarded-Proto", "https")
	reqBearer.Header.Set("Authorization", "Bearer "+validToken)
	wBearer := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wBearer, reqBearer)
	var respBearer AuthStatusResponse
	_ = json.NewDecoder(wBearer.Body).Decode(&respBearer)
	if !respBearer.Authenticated {
		t.Fatalf("expected authenticated=true with Bearer header, got %+v", respBearer)
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id2, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id1) != 64 || len(id2) != 64 {
		t.Fatalf("expected 64 hex characters for 256-bit session ID, got len(id1)=%d, len(id2)=%d", len(id1), len(id2))
	}
	if id1 == id2 {
		t.Fatalf("session IDs must be unique, got duplicate: %s", id1)
	}
}

func TestTLSVersionEnforcement(t *testing.T) {
	s := NewServer("127.0.0.1:0", "0.9.0", nil, nil)
	_ = s.Start()
	defer func() { _ = s.Stop(context.Background()) }()

	if s.httpServer == nil || s.httpServer.TLSConfig == nil {
		t.Fatalf("expected TLSConfig on server")
	}
	if s.httpServer.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected MinVersion tls.VersionTLS12, got %d", s.httpServer.TLSConfig.MinVersion)
	}
}

func TestWebServerSchema(t *testing.T) {
	s := NewServer("127.0.0.1:0", "0.9.0", nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/schema", nil)
	w := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/schema, got %d", w.Code)
	}

	var schema map[string]any
	if err := json.NewDecoder(w.Body).Decode(&schema); err != nil {
		t.Fatalf("failed to decode schema JSON: %v", err)
	}
	if schema["openapi"] != "3.0.3" {
		t.Fatalf("expected openapi 3.0.3, got %v", schema["openapi"])
	}
}

func TestBroadcasterRingBuffer(t *testing.T) {
	b := NewBroadcaster()
	b.SetMaxHistory(5)

	if h := b.GetHistory(); h != nil {
		t.Fatalf("expected nil history initially, got %v", h)
	}

	// Push 7 items to test circular wrapping of capacity 5
	for i := 1; i <= 7; i++ {
		b.Broadcast(TelemetryEvent{
			Type:      "event",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			System:    SystemMetrics{TotalContainers: i},
		})
	}

	history := b.GetHistory()
	if len(history) != 5 {
		t.Fatalf("expected history length 5, got %d", len(history))
	}
	// The oldest in history should be TotalContainers = 3, latest = 7
	if history[0].System.TotalContainers != 3 {
		t.Errorf("expected oldest event container count 3, got %d", history[0].System.TotalContainers)
	}
	if history[4].System.TotalContainers != 7 {
		t.Errorf("expected newest event container count 7, got %d", history[4].System.TotalContainers)
	}

	latest := b.GetLatestEvent()
	if latest.System.TotalContainers != 7 {
		t.Errorf("expected latest event container count 7, got %d", latest.System.TotalContainers)
	}
}

func TestWebServerPathTraversalRejection(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{{ID: "c1", Name: "app"}},
	}
	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, nil)

	// Traversal container ID in URL
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/../../etc/shadow", nil)
	w := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for path traversal, got %d", w.Code)
	}
}

func TestGenerateAuthToken(t *testing.T) {
	tok1, err := GenerateAuthToken()
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}
	if len(tok1) != 64 {
		t.Fatalf("expected 64 characters for token, got %d (%q)", len(tok1), tok1)
	}

	tok2, err := GenerateAuthToken()
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}
	if tok1 == tok2 {
		t.Fatalf("expected cryptographically unique tokens, got duplicate %q", tok1)
	}

	// Test EnableAuth
	s := NewServer("127.0.0.1:0", "0.9.0", nil, nil)
	autoTok, err := s.EnableAuth()
	if err != nil {
		t.Fatalf("unexpected error enabling auth: %v", err)
	}
	if len(autoTok) != 64 {
		t.Fatalf("expected 64-char auto generated token, got %q", autoTok)
	}
	if s.AuthToken() != autoTok {
		t.Fatalf("expected AuthToken() to match %q, got %q", autoTok, s.AuthToken())
	}
}

func TestSecureTokenFileOperations(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "sub", "test.token")
	token := "9kL2xP8vB1mN7qR4tY6wZ3aC5eG8hJ0kL2xP8vB1mN7qR4tY6wZ3aC5eG8hJ0kL2"

	writtenPath, err := WriteSecureTokenFile(targetFile, token)
	if err != nil {
		t.Fatalf("failed to write secure token file: %v", err)
	}
	if writtenPath != targetFile {
		t.Fatalf("expected written path %q, got %q", targetFile, writtenPath)
	}

	content, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read written token file: %v", err)
	}
	if strings.TrimSpace(string(content)) != token {
		t.Fatalf("expected token content %q, got %q", token, string(content))
	}

	// Test ReadSecureTokenFile
	readTok, err := ReadSecureTokenFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read secure token file: %v", err)
	}
	if readTok != token {
		t.Fatalf("expected read token %q, got %q", token, readTok)
	}

	// Test EnableAuthWithToken
	s := NewServer("127.0.0.1:0", "0.9.3", nil, nil)
	if err := s.EnableAuthWithToken("short"); err == nil {
		t.Fatalf("expected error for short token in EnableAuthWithToken")
	}
	if err := s.EnableAuthWithToken(token); err != nil {
		t.Fatalf("failed to enable auth with token: %v", err)
	}
	if s.AuthToken() != token {
		t.Fatalf("expected AuthToken() to be %q, got %q", token, s.AuthToken())
	}

	// Verify file permissions (0400) on non-Windows
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(targetFile)
		if err != nil {
			t.Fatalf("failed to stat token file: %v", err)
		}
		if fi.Mode().Perm() != 0o400 {
			t.Fatalf("expected file permission 0400, got %v", fi.Mode().Perm())
		}
	}

	// Test removal
	RemoveSecureTokenFile(targetFile)
	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Fatalf("expected token file to be deleted, stat err: %v", err)
	}

	// Test ReadSecureTokenFile on missing file
	if _, err := ReadSecureTokenFile(targetFile); err == nil {
		t.Fatalf("expected error reading non-existent token file")
	}
}

type fullMockProvider struct {
	snapshots []ContainerSnapshot
	diffs     []DiffChange
	files     []FileEntry
	fileData  string
}

func (f *fullMockProvider) GetContainerSnapshots() []ContainerSnapshot {
	return f.snapshots
}

func (f *fullMockProvider) GetContainerDiff(id string) ([]DiffChange, error) {
	return f.diffs, nil
}

func (f *fullMockProvider) ReadContainerDir(id, path string) ([]FileEntry, error) {
	return f.files, nil
}

func (f *fullMockProvider) ReadContainerFile(id, path string, maxBytes int64) (string, error) {
	return f.fileData, nil
}

func (f *fullMockProvider) SearchContainerFiles(id, basePath, pattern string, maxResults int) ([]FileEntry, error) {
	return []FileEntry{
		{Name: "traefik.yml", Path: "/etc/traefik/traefik.yml", IsDir: false, Size: 1024, Mode: "-rw-r--r--", ModTime: "2026-08-31T12:00:00Z"},
	}, nil
}

func TestWebServerDiffAndFiles(t *testing.T) {
	prov := &fullMockProvider{
		snapshots: []ContainerSnapshot{
			{
				ID:             "c123456",
				Name:           "api-gateway",
				Image:          "traefik:v3.0",
				ImageID:        "sha256:abcdef",
				ImageArch:      "linux/amd64",
				ImageSize:      "42.5 MB",
				ImageLayers:    "6 layers",
				ImageAuthor:    "Traefik Labs",
				ImageCreated:   "2026-08-30",
				ImageDockerVer: "26.1.4",
				ImageLabels:    map[string]string{"maintainer": "devops"},
				State:          "running",
			},
		},
		diffs: []DiffChange{
			{Path: "/etc/traefik/traefik.yml", Kind: "C"},
			{Path: "/var/log/access.log", Kind: "A"},
			{Path: "/tmp/old.pid", Kind: "D"},
		},
		files: []FileEntry{
			{Name: "etc", Path: "/etc", IsDir: true, Size: 4096, Mode: "drwxr-xr-x", ModTime: "2026-08-31T12:00:00Z"},
			{Name: "hosts", Path: "/etc/hosts", IsDir: false, Size: 128, Mode: "-rw-r--r--", ModTime: "2026-08-31T12:00:00Z"},
		},
		fileData: "127.0.0.1 localhost\n::1 localhost\n",
	}

	s := NewServer("127.0.0.1:0", "0.9.1", prov, nil)

	// 1. Test /api/v1/containers/c123456/diff
	reqDiff := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c123456/diff", nil)
	wDiff := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wDiff, reqDiff)
	if wDiff.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for diff, got %d: %s", wDiff.Code, wDiff.Body.String())
	}
	var diffResp []DiffChange
	if err := json.NewDecoder(wDiff.Body).Decode(&diffResp); err != nil || len(diffResp) != 3 {
		t.Fatalf("failed to decode diff response: %v, count=%d", err, len(diffResp))
	}

	// 2. Test /api/v1/containers/c123456/files?path=/
	reqFiles := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c123456/files?path=/", nil)
	wFiles := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wFiles, reqFiles)
	if wFiles.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for files, got %d: %s", wFiles.Code, wFiles.Body.String())
	}
	var filesResp []FileEntry
	if err := json.NewDecoder(wFiles.Body).Decode(&filesResp); err != nil || len(filesResp) != 2 {
		t.Fatalf("failed to decode files response: %v, count=%d", err, len(filesResp))
	}

	// 3. Test /api/v1/containers/c123456/file?path=/etc/hosts
	reqFile := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c123456/file?path=/etc/hosts", nil)
	wFile := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wFile, reqFile)
	if wFile.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for file, got %d: %s", wFile.Code, wFile.Body.String())
	}
	if !strings.Contains(wFile.Body.String(), "127.0.0.1 localhost") {
		t.Fatalf("unexpected file content: %s", wFile.Body.String())
	}

	// 4. Test /api/v1/containers/c123456/search?q=traefik
	reqSearch := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c123456/search?q=traefik", nil)
	wSearch := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wSearch, reqSearch)
	if wSearch.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for search, got %d: %s", wSearch.Code, wSearch.Body.String())
	}
	var searchResp []FileEntry
	if err := json.NewDecoder(wSearch.Body).Decode(&searchResp); err != nil || len(searchResp) != 1 {
		t.Fatalf("failed to decode search response: %v, count=%d", err, len(searchResp))
	}
	if searchResp[0].Path != "/etc/traefik/traefik.yml" {
		t.Fatalf("unexpected search match path: %s", searchResp[0].Path)
	}

	// 5. Test Image metadata in container detail
	reqDetail := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c123456", nil)
	wDetail := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wDetail, reqDetail)
	if wDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for container detail, got %d", wDetail.Code)
	}
	var detailSnap ContainerSnapshot
	if err := json.NewDecoder(wDetail.Body).Decode(&detailSnap); err != nil {
		t.Fatalf("failed to decode container detail JSON: %v", err)
	}
	if detailSnap.ImageID != "sha256:abcdef" || detailSnap.ImageArch != "linux/amd64" || detailSnap.ImageSize != "42.5 MB" {
		t.Fatalf("missing or incorrect image fields in detail snapshot: %+v", detailSnap)
	}
}

func TestWebServerProbes(t *testing.T) {
	prov := &fullMockProvider{
		snapshots: []ContainerSnapshot{
			{
				ID:    "c_net_123",
				Name:  "web-service",
				Ports: "0.0.0.0:8080->80/tcp",
				Networks: []NetworkInfo{
					{Name: "bridge", IPAddress: "172.17.0.2", Gateway: "172.17.0.1", PrefixLen: 16},
				},
				State: "running",
			},
		},
	}

	s := NewServer("127.0.0.1:0", "0.9.1", prov, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c_net_123/probes", nil)
	w := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for probes endpoint, got %d: %s", w.Code, w.Body.String())
	}

	var probes []EndpointProbe
	if err := json.NewDecoder(w.Body).Decode(&probes); err != nil {
		t.Fatalf("failed to decode probe results JSON: %v", err)
	}

	if len(probes) == 0 {
		t.Fatal("expected probe results for container with ports and networks")
	}

	// Verify probe result fields
	for _, p := range probes {
		if p.Target == "" || p.Label == "" || p.Status == "" {
			t.Fatalf("invalid probe response structure: %+v", p)
		}
	}
}

func TestLiveZeroLeakSecurityGuardE2E(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{{ID: "live-c1", Name: "live-app"}},
	}
	broadcaster := NewBroadcaster()
	s := NewServer("127.0.0.1:0", "0.9.2", mockProv, broadcaster)
	token, err := s.EnableAuth()
	if err != nil {
		t.Fatalf("failed to enable auth: %v", err)
	}

	if err := s.Start(); err != nil {
		t.Fatalf("failed to start live server: %v", err)
	}
	defer func() { _ = s.Stop(context.Background()) }()

	baseURL := "http://" + s.Addr()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("failed to create cookiejar: %v", err)
	}
	client := &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}

	// 1. Direct local access to /api/v1/containers on loopback without auth header -> 200 OK
	respLocal, err := client.Get(baseURL + "/api/v1/containers")
	if err != nil {
		t.Fatalf("direct local request failed: %v", err)
	}
	_ = respLocal.Body.Close()
	if respLocal.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for direct local loopback access, got %d", respLocal.StatusCode)
	}

	// 2. Proxied access (simulating external user behind reverse proxy with X-Forwarded-For) without HTTPS -> 403 Forbidden
	reqProxiedInsecure, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/containers", nil)
	reqProxiedInsecure.Header.Set("X-Forwarded-For", "203.0.113.195")
	respProxiedInsecure, err := client.Do(reqProxiedInsecure)
	if err != nil {
		t.Fatalf("proxied request failed: %v", err)
	}
	_ = respProxiedInsecure.Body.Close()
	if respProxiedInsecure.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unencrypted proxied request, got %d", respProxiedInsecure.StatusCode)
	}

	// 3. Proxied HTTPS access without token -> 401 Unauthorized
	reqProxiedSecureNoToken, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/containers", nil)
	reqProxiedSecureNoToken.Header.Set("X-Forwarded-For", "203.0.113.195")
	reqProxiedSecureNoToken.Header.Set("X-Forwarded-Proto", "https")
	respProxiedSecureNoToken, err := client.Do(reqProxiedSecureNoToken)
	if err != nil {
		t.Fatalf("proxied request failed: %v", err)
	}
	_ = respProxiedSecureNoToken.Body.Close()
	if respProxiedSecureNoToken.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for secure proxied request without token, got %d", respProxiedSecureNoToken.StatusCode)
	}

	// 4. Proxied HTTPS access with Bearer header -> 200 OK
	reqBearer, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/containers", nil)
	reqBearer.Header.Set("X-Forwarded-For", "203.0.113.195")
	reqBearer.Header.Set("X-Forwarded-Proto", "https")
	reqBearer.Header.Set("Authorization", "Bearer "+token)
	respBearer, err := client.Do(reqBearer)
	if err != nil {
		t.Fatalf("bearer request failed: %v", err)
	}
	_ = respBearer.Body.Close()
	if respBearer.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for valid Bearer token, got %d", respBearer.StatusCode)
	}

	// 5. Query parameter token rejection over live socket (?token=...) -> 401 Unauthorized
	reqQuery, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/containers?token="+token, nil)
	reqQuery.Header.Set("X-Forwarded-For", "203.0.113.195")
	reqQuery.Header.Set("X-Forwarded-Proto", "https")
	respQuery, err := client.Do(reqQuery)
	if err != nil {
		t.Fatalf("query token request failed: %v", err)
	}
	_ = respQuery.Body.Close()
	if respQuery.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for query parameter token over live socket, got %d", respQuery.StatusCode)
	}

	// 6. Login via POST /api/v1/auth/login and verify cookie session
	loginBody := strings.NewReader(fmt.Sprintf(`{"token":%q}`, token))
	reqLogin, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/auth/login", loginBody)
	reqLogin.Header.Set("X-Forwarded-For", "203.0.113.195")
	reqLogin.Header.Set("X-Forwarded-Proto", "https")
	reqLogin.Header.Set("Content-Type", "application/json")
	respLogin, err := client.Do(reqLogin)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	_ = respLogin.Body.Close()
	if respLogin.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on login, got %d", respLogin.StatusCode)
	}

	// 7. Session cookie now authorizes proxied HTTPS requests to /api/v1/containers
	reqWithSession, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/containers", nil)
	reqWithSession.Header.Set("X-Forwarded-For", "203.0.113.195")
	reqWithSession.Header.Set("X-Forwarded-Proto", "https")
	respWithSession, err := client.Do(reqWithSession)
	if err != nil {
		t.Fatalf("session request failed: %v", err)
	}
	_ = respWithSession.Body.Close()
	if respWithSession.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK using session cookie over live socket, got %d", respWithSession.StatusCode)
	}

	// 8. Connect to live SSE stream using session cookie
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	reqSSE, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/stream", nil)
	reqSSE.Header.Set("X-Forwarded-For", "203.0.113.195")
	reqSSE.Header.Set("X-Forwarded-Proto", "https")
	respSSE, err := client.Do(reqSSE)
	if err != nil {
		t.Fatalf("live SSE connection failed: %v", err)
	}
	defer func() { _ = respSSE.Body.Close() }()
	if respSSE.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for live SSE stream with session cookie, got %d", respSSE.StatusCode)
	}

	sseReader := bufio.NewReader(respSSE.Body)
	line, err := sseReader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read initial SSE stream data: %v", err)
	}
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("expected SSE data frame, got: %q", line)
	}

	// 9. Logout via POST /api/v1/auth/logout
	reqLogout, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/auth/logout", nil)
	reqLogout.Header.Set("X-Forwarded-For", "203.0.113.195")
	reqLogout.Header.Set("X-Forwarded-Proto", "https")
	respLogout, err := client.Do(reqLogout)
	if err != nil {
		t.Fatalf("logout request failed: %v", err)
	}
	_ = respLogout.Body.Close()
	if respLogout.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on logout, got %d", respLogout.StatusCode)
	}

	// 10. Subsequent proxied request without Bearer header is now rejected with 401 Unauthorized
	reqAfterLogout, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/containers", nil)
	reqAfterLogout.Header.Set("X-Forwarded-For", "203.0.113.195")
	reqAfterLogout.Header.Set("X-Forwarded-Proto", "https")
	respAfterLogout, err := client.Do(reqAfterLogout)
	if err != nil {
		t.Fatalf("post-logout request failed: %v", err)
	}
	_ = respAfterLogout.Body.Close()
	if respAfterLogout.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized after session logout, got %d", respAfterLogout.StatusCode)
	}
}

func TestWebServerIPv6Fallback(t *testing.T) {
	prov := &fullMockProvider{}

	// 1. Explicit IPv4 address "0.0.0.0:0" MUST bind to IPv4 0.0.0.0:
	sIPv4 := NewServer("0.0.0.0:0", "0.9.2", prov, nil)
	sIPv4.SetTLS("../../tests/tls/server.crt", "../../tests/tls/server.key")
	if err := sIPv4.Start(); err != nil {
		t.Fatalf("failed to start IPv4 server: %v", err)
	}
	defer func() { _ = sIPv4.Stop(context.Background()) }()

	if !strings.HasPrefix(sIPv4.Addr(), "0.0.0.0:") {
		t.Fatalf("expected explicit 0.0.0.0:0 to bind 0.0.0.0, got %s", sIPv4.Addr())
	}

	// 2. Dual-stack ":0" or "[::]:0" starts and binds appropriately (IPv6 if supported, otherwise IPv4)
	sDual := NewServer(":0", "0.9.2", prov, nil)
	sDual.SetTLS("../../tests/tls/server.crt", "../../tests/tls/server.key")
	if err := sDual.Start(); err != nil {
		t.Fatalf("failed to start dual-stack server: %v", err)
	}
	defer func() { _ = sDual.Stop(context.Background()) }()

	if sDual.Addr() == "" {
		t.Fatalf("expected non-empty listening address for dual-stack server")
	}
}

func TestWebServerEndpointsAndProxy(t *testing.T) {
	// Spin up a mock backend container HTTP service
	mockContainerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Test-Server", "mock-container")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "<html><body><h1>App on %s</h1></body></html>", r.URL.Path)
	}))
	defer mockContainerServer.Close()

	// Extract port
	mockPortStr := mockContainerServer.URL[strings.LastIndex(mockContainerServer.URL, ":")+1:]
	mockPort, _ := strconv.Atoi(mockPortStr)

	prov := &fullMockProvider{
		snapshots: []ContainerSnapshot{
			{
				ID:    "c_web_proxy_123",
				Name:  "web-proxy-app",
				Ports: fmt.Sprintf("0.0.0.0:%d->80/tcp", mockPort),
				Networks: []NetworkInfo{
					{Name: "bridge", IPAddress: "127.0.0.1", Gateway: "127.0.0.1", PrefixLen: 16},
				},
				Env:   []string{"PORT=8080"},
				State: "running",
			},
		},
	}

	s := NewServer("127.0.0.1:0", "0.9.2", prov, nil)

	// 1. Test /endpoints
	reqEP := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c_web_proxy_123/endpoints", nil)
	wEP := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wEP, reqEP)

	if wEP.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /endpoints, got %d", wEP.Code)
	}

	var endpoints []serviceprobe.Endpoint
	if err := json.NewDecoder(wEP.Body).Decode(&endpoints); err != nil {
		t.Fatalf("failed to decode endpoints JSON: %v", err)
	}
	if len(endpoints) == 0 {
		t.Fatalf("expected at least 1 discovered endpoint")
	}

	// 2. Test /proxy (JSON format)
	reqProxyJSON := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c_web_proxy_123/proxy?path=/healthz&format=json", nil)
	wProxyJSON := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wProxyJSON, reqProxyJSON)

	if wProxyJSON.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /proxy JSON, got %d", wProxyJSON.Code)
	}

	var probeRes serviceprobe.HTTPProbeResult
	if err := json.NewDecoder(wProxyJSON.Body).Decode(&probeRes); err != nil {
		t.Fatalf("failed to decode proxy result JSON: %v", err)
	}
	if probeRes.StatusCode != http.StatusOK || !strings.Contains(probeRes.Body, "/healthz") {
		t.Fatalf("unexpected probe result: %+v", probeRes)
	}

	// 3. Test /proxy (HTML format with CSP)
	reqProxyHTML := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c_web_proxy_123/proxy?path=/status", nil)
	wProxyHTML := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wProxyHTML, reqProxyHTML)

	if wProxyHTML.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /proxy HTML, got %d", wProxyHTML.Code)
	}
	if !strings.Contains(wProxyHTML.Body.String(), "/status") {
		t.Errorf("expected HTML body to contain '/status', got %q", wProxyHTML.Body.String())
	}

	// 4. Test SSRF protection: reject port not exposed by container
	reqUnexposed := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c_web_proxy_123/proxy?port=22&path=/", nil)
	wUnexposed := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wUnexposed, reqUnexposed)
	if wUnexposed.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for unexposed port 22, got %d", wUnexposed.Code)
	}

	// 5. Test subpath validation: reject path with @ userinfo injection
	reqInvalidPath := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c_web_proxy_123/proxy?path=@evil.com/leak", nil)
	wInvalidPath := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(wInvalidPath, reqInvalidPath)
	if wInvalidPath.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for userinfo @ subpath, got %d", wInvalidPath.Code)
	}
}

func TestWebServer_MultiHopProxyIPExtraction(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{{ID: "c_multihop_1", Name: "multihop-app"}},
	}
	s := NewServer("127.0.0.1:0", "0.9.0", mockProv, nil)
	_, _ = s.EnableAuth()

	// Multi-hop X-Forwarded-For: client, proxy1, proxy2
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"token":"wrong_token"}`))
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.195, 198.51.100.1, 127.0.0.1")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.corsMiddleware(s.mux).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}

	// Verify the rate limiter recorded against the real client IP (203.0.113.195)
	extractedIP := getClientIP(req)
	if extractedIP != "203.0.113.195" {
		t.Errorf("expected extracted client IP '203.0.113.195', got %q", extractedIP)
	}
}

func TestWebServer_SSEBroadcasterSlowSubscriberNonBlocking(t *testing.T) {
	b := NewBroadcaster()
	b.SetMaxSubscribers(32)

	// Create 5 normal subscribers and 5 slow/full subscribers
	var fastChans []chan TelemetryEvent
	for i := 0; i < 5; i++ {
		ch := b.Subscribe()
		fastChans = append(fastChans, ch)
	}

	var blockedChans []chan TelemetryEvent
	for i := 0; i < 5; i++ {
		ch := b.Subscribe()
		// Fill channel to capacity
		for capCount := 0; capCount < 64; capCount++ {
			ch <- TelemetryEvent{Timestamp: "old"}
		}
		blockedChans = append(blockedChans, ch)
	}

	// Broadcast an event - must NOT block or deadlock despite 5 fully blocked channels
	done := make(chan bool, 1)
	go func() {
		b.Broadcast(TelemetryEvent{Timestamp: "2026-09-01T21:30:00Z"})
		done <- true
	}()

	select {
	case <-done:
		// Broadcast completed non-blockingly
	case <-time.After(1 * time.Second):
		t.Fatalf("Broadcast deadlocked or blocked on full subscriber channel!")
	}

	// Cleanup
	for _, ch := range append(fastChans, blockedChans...) {
		b.Unsubscribe(ch)
	}
}

func TestWebServer_MutatingMethodsRejected_ReadOnlyOnly(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{
			{ID: "c1", Name: "app-server", State: "running"},
		},
	}
	s := NewServer("127.0.0.1:0", "0.9.3", mockProv, nil)

	mutatingMethods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	endpoints := []string{
		"/api/v1/containers",
		"/api/v1/containers/c1",
		"/api/v1/containers/c1/files",
		"/api/v1/containers/c1/file",
		"/api/v1/containers/c1/search",
		"/api/v1/containers/c1/upload",
		"/api/v1/containers/c1/delete",
		"/api/v1/containers/c1/edit",
	}

	for _, method := range mutatingMethods {
		for _, endpoint := range endpoints {
			req := httptest.NewRequest(method, endpoint, nil)
			w := httptest.NewRecorder()
			s.corsMiddleware(s.mux).ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
				t.Fatalf("expected 405 MethodNotAllowed or 404 NotFound for %s %s, got %d", method, endpoint, w.Code)
			}
		}
	}
}

func TestWebServer_FileExplorerMutations_StrictlyTUIOnly(t *testing.T) {
	mockProv := &mockContainerProvider{
		snapshots: []ContainerSnapshot{
			{ID: "c1", Name: "app-server", State: "running"},
		},
	}
	s := NewServer("127.0.0.1:0", "0.9.3", mockProv, nil)

	forbiddenEndpoints := []string{
		"/api/v1/containers/c1/upload",
		"/api/v1/containers/c1/edit",
		"/api/v1/containers/c1/delete",
	}

	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}
	for _, m := range methods {
		for _, ep := range forbiddenEndpoints {
			req := httptest.NewRequest(m, ep, nil)
			w := httptest.NewRecorder()
			s.corsMiddleware(s.mux).ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405 MethodNotAllowed for %s %s, got %d", m, ep, w.Code)
			}
			if m == http.MethodGet {
				if !strings.Contains(w.Body.String(), "strictly TUI-only") {
					t.Fatalf("expected body to indicate strictly TUI-only, got: %s", w.Body.String())
				}
			} else {
				if !strings.Contains(w.Body.String(), "read-only") {
					t.Fatalf("expected body to indicate read-only API, got: %s", w.Body.String())
				}
			}
		}
	}
}
