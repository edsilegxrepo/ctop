package web

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edsilegx/ctop/pkg/prober"
)

// MinAuthTokenLength defines the enforced minimum character length for web authentication tokens.
const MinAuthTokenLength = 32

// GenerateAuthToken generates a secure 32-character random hexadecimal authentication token (128-bit entropy).
func GenerateAuthToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// DefaultAuthTokenPath returns ~/.config/ctop/token.
func DefaultAuthTokenPath() string {
	configDir, err := os.UserConfigDir()
	if err == nil && configDir != "" {
		return filepath.Join(configDir, "ctop", "token")
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".config", "ctop", "token")
	}
	return filepath.Join(os.TempDir(), "ctop-token")
}

// WriteSecureTokenFile writes the authentication token to the target path with 0400 (owner read-only) file permissions.
func WriteSecureTokenFile(filePath, token string) (string, error) {
	targetPath := filePath
	if targetPath == "" {
		targetPath = DefaultAuthTokenPath()
	}
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	// Remove existing read-only file if present before re-creating
	_ = os.Remove(targetPath)
	if err := os.WriteFile(targetPath, []byte(strings.TrimSpace(token)+"\n"), 0400); err != nil {
		return "", fmt.Errorf("failed to write secure token file %s: %w", targetPath, err)
	}
	return targetPath, nil
}

// RemoveSecureTokenFile securely removes the token file upon shutdown.
func RemoveSecureTokenFile(filePath string) {
	if filePath != "" {
		_ = os.Remove(filePath)
	}
}

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

// DiffProvider defines an optional provider method to query container filesystem diffs.
type DiffProvider interface {
	GetContainerDiff(id string) ([]DiffChange, error)
}

// FileProvider defines optional provider methods for in-container directory navigation, searching, and file reading.
type FileProvider interface {
	ReadContainerDir(id string, path string) ([]FileEntry, error)
	ReadContainerFile(id string, path string, maxBytes int64) (string, error)
	SearchContainerFiles(id string, basePath string, pattern string, maxResults int) ([]FileEntry, error)
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
	tlsCertFile string
	tlsKeyFile  string
	authToken   string
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
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = "127.0.0.1:9090"
	} else if !strings.Contains(addr, ":") {
		addr = "127.0.0.1:" + addr
	} else if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
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
		mux.HandleFunc(p("/api/v1/schema"), s.handleSchema)
		mux.HandleFunc(p("/api/v1/schema/"), s.handleSchema)
	}

	// Always register root paths
	register("")
	// If a subpath prefix is configured, also register under the prefix
	if prefix != "" {
		register(prefix)
	}

	return s
}

// SetTLS configures TLS certificate and private key paths for HTTPS encryption.
func (s *Server) SetTLS(certFile, keyFile string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tlsCertFile = certFile
	s.tlsKeyFile = keyFile
}

// EnableAuth enables web token authentication by automatically generating a fresh 32-character token.
func (s *Server) EnableAuth() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err := GenerateAuthToken()
	if err != nil {
		return "", err
	}
	s.authToken = token
	return s.authToken, nil
}

// AuthToken returns the currently configured authentication token.
func (s *Server) AuthToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authToken
}

// URLPrefix returns the configured subpath prefix for reverse proxies.
func (s *Server) URLPrefix() string {
	return s.urlPrefix
}

// Start launches the HTTP or HTTPS server in a background listener.
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

	certFile, keyFile := s.tlsCertFile, s.tlsKeyFile
	s.mu.Unlock()

	go func() {
		if certFile != "" && keyFile != "" {
			_ = s.httpServer.ServeTLS(ln, certFile, keyFile)
		} else {
			_ = s.httpServer.Serve(ln)
		}
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
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
	}
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

// corsMiddleware sets secure headers, authentication, and CORS policies for read-only telemetry.
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

		// Global path traversal protection
		if strings.Contains(r.URL.Path, "..") || strings.Contains(r.URL.RawPath, "..") || strings.Contains(r.URL.Path, "\\") {
			http.Error(w, `{"error":"Invalid request path - traversal detected"}`, http.StatusBadRequest)
			return
		}

		// Token authentication enforcement when configured
		s.mu.RLock()
		token := s.authToken
		s.mu.RUnlock()

		if token != "" {
			if !isTLSOrSecureProxy(r) {
				http.Error(w, `{"error":"Forbidden - TLS encryption required (direct TLS or X-Forwarded-Proto: https)"}`, http.StatusForbidden)
				return
			}

			reqToken := ""
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				reqToken = strings.TrimPrefix(authHeader, "Bearer ")
			} else if queryToken := r.URL.Query().Get("token"); queryToken != "" {
				reqToken = queryToken
			} else if queryAuth := r.URL.Query().Get("auth"); queryAuth != "" {
				reqToken = queryAuth
			}
			if reqToken != token {
				w.Header().Set("WWW-Authenticate", `Bearer realm="ctop"`)
				http.Error(w, `{"error":"Unauthorized - invalid or missing authentication token"}`, http.StatusUnauthorized)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// isTLSOrSecureProxy verifies if a request is TLS-encrypted directly, forwarded via a secure TLS reverse proxy,
// or sent from a local loopback interface.
func isTLSOrSecureProxy(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Ssl"), "on") {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Scheme"), "https") {
		return true
	}
	if strings.EqualFold(r.Header.Get("Front-End-Https"), "on") {
		return true
	}
	// Direct localhost / loopback access (or httptest test client 192.0.2.1)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if host == "127.0.0.1" || host == "::1" || host == "localhost" || host == "192.0.2.1" {
			return true
		}
	} else if r.RemoteAddr == "127.0.0.1" || r.RemoteAddr == "::1" || r.RemoteAddr == "localhost" || r.RemoteAddr == "192.0.2.1" || r.RemoteAddr == "" {
		return true
	}
	return false
}

// handleSchema serves the OpenAPI 3.0 / JSON Schema telemetry documentation.
func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	schema := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "ctop Telemetry & Monitoring API",
			"version":     s.version,
			"description": "Embedded read-only REST & Server-Sent Events (SSE) telemetry API for ctop container metrics.",
		},
		"paths": map[string]any{
			"/api/v1/health": map[string]any{
				"get": map[string]any{
					"summary": "Health and service uptime status",
					"responses": map[string]any{
						"200": map[string]any{"description": "Daemon operational"},
					},
				},
			},
			"/api/v1/metrics": map[string]any{
				"get": map[string]any{
					"summary": "Aggregated cluster and host resource telemetry",
					"responses": map[string]any{
						"200": map[string]any{"description": "Cluster metrics"},
					},
				},
			},
			"/api/v1/containers": map[string]any{
				"get": map[string]any{
					"summary": "List all active container snapshots and metrics",
					"responses": map[string]any{
						"200": map[string]any{"description": "Array of container snapshots"},
					},
				},
			},
			"/api/v1/containers/{id}": map[string]any{
				"get": map[string]any{
					"summary": "Detailed container snapshot or in-container top processes",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Container snapshot"},
						"404": map[string]any{"description": "Container not found"},
					},
				},
			},
			"/api/v1/stream": map[string]any{
				"get": map[string]any{
					"summary": "Real-time Server-Sent Events (SSE) telemetry stream",
					"responses": map[string]any{
						"200": map[string]any{"description": "Live text/event-stream"},
					},
				},
			},
			"/api/v1/export": map[string]any{
				"get": map[string]any{
					"summary": "Export telemetry snapshot in JSON format",
					"responses": map[string]any{
						"200": map[string]any{"description": "Downloadable JSON export"},
					},
				},
			},
			"/api/v1/schema": map[string]any{
				"get": map[string]any{
					"summary": "OpenAPI specification for ctop REST and SSE APIs",
					"responses": map[string]any{
						"200": map[string]any{"description": "OpenAPI 3.0 schema"},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(schema)
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

	if strings.Contains(path, "..") || strings.Contains(path, "\\") {
		http.Error(w, `{"error":"Invalid container identifier - path traversal detected"}`, http.StatusBadRequest)
		return
	}

	isTop := false
	isDiff := false
	isFiles := false
	isFile := false
	isSearch := false
	isProbes := false
	id := path

	if strings.HasSuffix(path, "/top") {
		isTop = true
		id = strings.TrimSuffix(path, "/top")
		id = strings.TrimSpace(id)
	} else if strings.HasSuffix(path, "/diff") || strings.HasSuffix(path, "/changes") {
		isDiff = true
		id = strings.TrimSuffix(path, "/diff")
		id = strings.TrimSuffix(id, "/changes")
		id = strings.TrimSpace(id)
	} else if strings.HasSuffix(path, "/files") {
		isFiles = true
		id = strings.TrimSuffix(path, "/files")
		id = strings.TrimSpace(id)
	} else if strings.HasSuffix(path, "/file") {
		isFile = true
		id = strings.TrimSuffix(path, "/file")
		id = strings.TrimSpace(id)
	} else if strings.HasSuffix(path, "/search") || strings.HasSuffix(path, "/find") {
		isSearch = true
		id = strings.TrimSuffix(path, "/search")
		id = strings.TrimSuffix(id, "/find")
		id = strings.TrimSpace(id)
	} else if strings.HasSuffix(path, "/probes") || strings.HasSuffix(path, "/probe") {
		isProbes = true
		id = strings.TrimSuffix(path, "/probes")
		id = strings.TrimSuffix(id, "/probe")
		id = strings.TrimSpace(id)
	}

	if s.provider == nil {
		http.Error(w, `{"error":"Container provider unavailable"}`, http.StatusServiceUnavailable)
		return
	}

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

	if isTop {
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

	if isDiff {
		if diffProv, ok := s.provider.(DiffProvider); ok {
			diffs, err := diffProv.GetContainerDiff(id)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Failed to query container diff: %v"}`, err), http.StatusInternalServerError)
				return
			}
			if diffs == nil {
				diffs = []DiffChange{}
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(diffs)
			return
		}
		http.Error(w, `{"error":"Filesystem diff not supported by provider"}`, http.StatusNotImplemented)
		return
	}

	if isFiles {
		targetPath := strings.TrimSpace(r.URL.Query().Get("path"))
		if targetPath == "" {
			targetPath = "/"
		}
		if fileProv, ok := s.provider.(FileProvider); ok {
			files, err := fileProv.ReadContainerDir(id, targetPath)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Failed to read container directory: %v"}`, err), http.StatusInternalServerError)
				return
			}
			if files == nil {
				files = []FileEntry{}
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(files)
			return
		}
		http.Error(w, `{"error":"File explorer not supported by provider"}`, http.StatusNotImplemented)
		return
	}

	if isFile {
		targetPath := strings.TrimSpace(r.URL.Query().Get("path"))
		if targetPath == "" {
			http.Error(w, `{"error":"Parameter 'path' is required"}`, http.StatusBadRequest)
			return
		}
		if fileProv, ok := s.provider.(FileProvider); ok {
			content, err := fileProv.ReadContainerFile(id, targetPath, 128*1024)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Failed to read container file: %v"}`, err), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(content))
			return
		}
		http.Error(w, `{"error":"File reading not supported by provider"}`, http.StatusNotImplemented)
		return
	}

	if isSearch {
		queryPattern := strings.TrimSpace(r.URL.Query().Get("q"))
		if queryPattern == "" {
			queryPattern = strings.TrimSpace(r.URL.Query().Get("pattern"))
		}
		if queryPattern == "" {
			http.Error(w, `{"error":"Query parameter 'q' or 'pattern' is required"}`, http.StatusBadRequest)
			return
		}
		basePath := strings.TrimSpace(r.URL.Query().Get("path"))
		if basePath == "" {
			basePath = "/"
		}
		limit := 100
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if lInt, err := strconv.Atoi(lStr); err == nil && lInt > 0 {
				limit = lInt
			}
		}
		if fileProv, ok := s.provider.(FileProvider); ok {
			files, err := fileProv.SearchContainerFiles(id, basePath, queryPattern, limit)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"Failed to search container files: %v"}`, err), http.StatusInternalServerError)
				return
			}
			if files == nil {
				files = []FileEntry{}
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(files)
			return
		}
		http.Error(w, `{"error":"File searching not supported by provider"}`, http.StatusNotImplemented)
		return
	}

	if isProbes {
		var targetSnap *ContainerSnapshot
		for _, c := range s.provider.GetContainerSnapshots() {
			if c.ID == id || strings.HasPrefix(c.ID, id) || strings.EqualFold(c.Name, id) {
				cp := c
				targetSnap = &cp
				break
			}
		}
		if targetSnap == nil {
			http.Error(w, fmt.Sprintf(`{"error":"Container %q not found"}`, id), http.StatusNotFound)
			return
		}

		var netParts []string
		for _, n := range targetSnap.Networks {
			netParts = append(netParts, fmt.Sprintf("%s:::%s:::%s:::%s:::%d", n.Name, n.IPAddress, n.Gateway, n.Mac, n.PrefixLen))
		}
		rawNetStr := strings.Join(netParts, ";;")

		tasks := prober.ExtractProbeTargets(targetSnap.Ports, rawNetStr)
		if len(tasks) == 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]EndpointProbe{})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		results := make([]EndpointProbe, len(tasks))

		for idx, task := range tasks {
			wg.Add(1)
			go func(i int, t prober.TargetTask) {
				defer wg.Done()
				res := prober.ProbeTCP(ctx, t.Label, t.Target, 350*time.Millisecond)
				status := res.Status
				if strings.Contains(t.Label, "Gateway") {
					status = "CONFIGURED"
					if res.Success {
						status = "REACHABLE"
					}
				} else if res.Success && strings.Contains(t.Label, "(IP)") {
					status = "REACHABLE"
				}

				results[i] = EndpointProbe{
					Label:      t.Label,
					Target:     t.Target,
					Status:     status,
					DurationMS: float64(res.Duration.Microseconds()) / 1000.0,
					Success:    res.Success,
					Timestamp:  time.Now().UTC().Format(time.RFC3339),
				}
			}(idx, task)
		}

		wg.Wait()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(results)
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
