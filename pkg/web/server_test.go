package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
