package web

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed dashboard.html
var dashboardHTML []byte

// ContainerProvider defines the supplier interface to query active container snapshots from ctop's engine.
type ContainerProvider interface {
	GetContainerSnapshots() []ContainerSnapshot
}

// TopResult represents running process details in a container.
type TopResult struct {
	Titles    []string   `json:"titles"`
	Processes [][]string `json:"processes"`
}

// TopProvider defines an optional provider method to query in-container top processes.
type TopProvider interface {
	GetContainerTop(id string) (TopResult, error)
}

// Server provides the embedded real-time HTTP server, REST telemetry APIs, and SSE stream.
type Server struct {
	addr        string
	urlPrefix   string
	provider    ContainerProvider
	broadcaster *Broadcaster
	httpServer  *http.Server
	mux         *http.ServeMux
	startTime   time.Time
	version     string
	mu          sync.RWMutex
	running     bool
}

// cleanURLPrefix normalizes URL prefixes to standard /subpath format without trailing slashes.
func cleanURLPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimRight(prefix, "/")
}

// NewServer constructs a new web dashboard and telemetry server with optional subpath prefix.
func NewServer(addr string, version string, provider ContainerProvider, broadcaster *Broadcaster, urlPrefix ...string) *Server {
	if addr == "" {
		addr = "127.0.0.1:9090"
	}
	if broadcaster == nil {
		broadcaster = NewBroadcaster()
	}

	var prefix string
	if len(urlPrefix) > 0 {
		prefix = cleanURLPrefix(urlPrefix[0])
	}

	s := &Server{
		addr:        addr,
		urlPrefix:   prefix,
		version:     version,
		provider:    provider,
		broadcaster: broadcaster,
		startTime:   time.Now(),
	}

	mux := http.NewServeMux()
	s.mux = mux

	register := func(base string) {
		p := func(path string) string {
			if base == "" {
				return path
			}
			return base + path
		}

		mux.HandleFunc(p("/"), s.handleIndex)
		mux.HandleFunc(p("/api/v1/health"), s.handleHealth)
		mux.HandleFunc(p("/api/v1/health/"), s.handleHealth)
		mux.HandleFunc(p("/api/v1/metrics"), s.handleMetrics)
		mux.HandleFunc(p("/api/v1/metrics/"), s.handleMetrics)
		mux.HandleFunc(p("/api/v1/containers"), s.handleContainers)
		mux.HandleFunc(p("/api/v1/containers/"), s.handleContainerDetail)
		mux.HandleFunc(p("/api/v1/stream"), s.handleStream)
		mux.HandleFunc(p("/api/v1/stream/"), s.handleStream)
		mux.HandleFunc(p("/api/v1/export"), s.handleExport)
		mux.HandleFunc(p("/api/v1/export/"), s.handleExport)
	}

	// Always register root paths
	register("")
	// If a subpath prefix is configured, also register under the prefix
	if prefix != "" {
		register(prefix)
	}

	return s
}

// URLPrefix returns the configured subpath prefix for reverse proxies.
func (s *Server) URLPrefix() string {
	return s.urlPrefix
}

// Start launches the HTTP server in a background listener.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}
	s.addr = ln.Addr().String()

	s.httpServer = &http.Server{
		Handler:           s.corsMiddleware(s.mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0, // Infinite for SSE streaming
		IdleTimeout:       60 * time.Second,
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		_ = s.httpServer.Serve(ln)
	}()

	return nil
}

// Stop gracefully shuts down the embedded web server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.httpServer == nil {
		return nil
	}

	s.running = false
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the listening address of the server.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.addr
}

// Broadcaster returns the associated SSE broadcaster.
func (s *Server) Broadcaster() *Broadcaster {
	return s.broadcaster
}

// corsMiddleware sets secure headers and CORS policies for read-only telemetry.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Strictly enforce READ-ONLY access across all routes
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			http.Error(w, `{"error":"Method Not Allowed - ctop web API is strictly read-only"}`, http.StatusMethodNotAllowed)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleIndex serves the embedded HTML5 dashboard.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != s.urlPrefix && r.URL.Path != s.urlPrefix+"/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)

	if s.urlPrefix != "" {
		html := strings.Replace(string(dashboardHTML), `const BASE_PATH = "";`, fmt.Sprintf(`const BASE_PATH = %q;`, s.urlPrefix), 1)
		_, _ = w.Write([]byte(html)) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- static embedded HTML SPA
		return
	}
	_, _ = w.Write(dashboardHTML) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- static embedded HTML SPA
}

// handleHealth returns liveness/readiness information.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := HealthStatus{
		Status:    "ok",
		Version:   s.version,
		Uptime:    time.Since(s.startTime).Truncate(time.Second).String(),
		Timestamp: time.Now().UTC(),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}

// handleMetrics returns aggregated cluster-wide telemetry metrics.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	sys := s.aggregateMetrics()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(sys)
}

// handleContainers returns the list of current container telemetry snapshots.
func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	var snapshots []ContainerSnapshot
	if s.provider != nil {
		snapshots = s.provider.GetContainerSnapshots()
	}
	if snapshots == nil {
		snapshots = []ContainerSnapshot{}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(snapshots)
}

// handleContainerDetail returns details or top processes for a specific container by ID or Name.
func (s *Server) handleContainerDetail(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if s.urlPrefix != "" {
		path = strings.TrimPrefix(path, s.urlPrefix)
	}
	path = strings.TrimPrefix(path, "/api/v1/containers/")
	path = strings.TrimSpace(path)
	if path == "" {
		s.handleContainers(w, r)
		return
	}

	isTop := false
	id := path
	if strings.HasSuffix(path, "/top") {
		isTop = true
		id = strings.TrimSuffix(path, "/top")
		id = strings.TrimSpace(id)
	}

	if s.provider == nil {
		http.Error(w, `{"error":"Container provider unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	if isTop {
		// First verify container exists in active snapshots
		var found bool
		for _, c := range s.provider.GetContainerSnapshots() {
			if c.ID == id || strings.HasPrefix(c.ID, id) || strings.EqualFold(c.Name, id) {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, fmt.Sprintf(`{"error":"Container %q not found"}`, id), http.StatusNotFound)
			return
		}

		if topProv, ok := s.provider.(TopProvider); ok {
			top, err := topProv.GetContainerTop(id)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Failed to query container top: %v"}`, err), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(top)
			return
		}
		http.Error(w, `{"error":"Top not supported by provider"}`, http.StatusNotImplemented)
		return
	}

	snapshots := s.provider.GetContainerSnapshots()
	for _, c := range snapshots {
		if c.ID == id || strings.HasPrefix(c.ID, id) || strings.EqualFold(c.Name, id) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(c)
			return
		}
	}

	http.Error(w, fmt.Sprintf(`{"error":"Container %q not found"}`, id), http.StatusNotFound)
}

// handleExport outputs a pretty-formatted JSON snapshot of all telemetry or single-container details.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	containerID := strings.TrimSpace(r.URL.Query().Get("container"))
	if containerID != "" {
		if s.provider != nil {
			snapshots := s.provider.GetContainerSnapshots()
			for _, c := range snapshots {
				if c.ID == containerID || strings.HasPrefix(c.ID, containerID) || strings.EqualFold(c.Name, containerID) {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					filename := fmt.Sprintf("ctop-container-%s.json", c.Name)
					if c.Name == "" {
						filename = fmt.Sprintf("ctop-container-%s.json", c.ID)
					}
					w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
					w.WriteHeader(http.StatusOK)
					enc := json.NewEncoder(w)
					enc.SetIndent("", "  ")
					_ = enc.Encode(c)
					return
				}
			}
		}
		http.Error(w, fmt.Sprintf(`{"error":"Container %q not found"}`, containerID), http.StatusNotFound)
		return
	}

	sys := s.aggregateMetrics()
	var snapshots []ContainerSnapshot
	if s.provider != nil {
		snapshots = s.provider.GetContainerSnapshots()
	}
	if snapshots == nil {
		snapshots = []ContainerSnapshot{}
	}

	exportData := struct {
		Timestamp  time.Time           `json:"timestamp"`
		System     SystemMetrics       `json:"system"`
		Containers []ContainerSnapshot `json:"containers"`
	}{
		Timestamp:  time.Now().UTC(),
		System:     sys,
		Containers: snapshots,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"ctop-telemetry-export.json\"")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(exportData)
}

// handleStream handles Server-Sent Events (SSE) client connections.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := s.broadcaster.Subscribe()
	if ch == nil {
		http.Error(w, `{"error":"Too many concurrent streaming subscribers"}`, http.StatusServiceUnavailable)
		return
	}
	defer s.broadcaster.Unsubscribe(ch)

	// Send initial snapshot immediately
	initialEv := s.createSnapshotEvent("initial")
	if data, err := json.Marshal(initialEv); err == nil {
		_, _ = w.Write([]byte("data: " + string(data) + "\n\n")) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter, go.lang.security.audit.xss.no-fprintf-to-responsewriter.no-fprintf-to-responsewriter -- SSE data frame formatted as text/event-stream
		flusher.Flush()
	}

	// Keepalive ticker to keep connection alive through proxies
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = w.Write([]byte(": keepalive\n\n")) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- SSE keepalive comment
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if data, err := json.Marshal(ev); err == nil {
				_, _ = w.Write([]byte("data: " + string(data) + "\n\n")) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter, go.lang.security.audit.xss.no-fprintf-to-responsewriter.no-fprintf-to-responsewriter -- SSE data frame formatted as text/event-stream
				flusher.Flush()
			}
		}
	}
}

// createSnapshotEvent builds a complete TelemetryEvent from the current provider state.
func (s *Server) createSnapshotEvent(evType string) TelemetryEvent {
	sys := s.aggregateMetrics()
	var snapshots []ContainerSnapshot
	if s.provider != nil {
		snapshots = s.provider.GetContainerSnapshots()
	}
	return TelemetryEvent{
		Type:       evType,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		System:     sys,
		Containers: snapshots,
	}
}

// aggregateMetrics calculates cluster or host totals from container snapshots.
func (s *Server) aggregateMetrics() SystemMetrics {
	var snapshots []ContainerSnapshot
	if s.provider != nil {
		snapshots = s.provider.GetContainerSnapshots()
	}
	sys := AggregateSnapshots(snapshots)
	sys.UptimeSeconds = int64(time.Since(s.startTime).Seconds())
	return sys
}
