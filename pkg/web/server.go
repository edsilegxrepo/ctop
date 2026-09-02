// Package web implements the read-only embedded HTTP dashboard, REST APIs, and Server-Sent Events (SSE) broadcaster.
//
// Objective:
//
//	Provide a modern, responsive web monitoring dashboard streaming real-time container metrics,
//	logs, top processes, compose generators, diagnostics, and in-container file exploration over HTTP/SSE.
//
// Core Components:
//   - Server: HTTP multiplexer serving embedded assets and REST endpoints under an optional prefix.
//   - Broadcaster: High-capacity Server-Sent Events hub fanning out telemetry to connected browsers.
//   - Middleware: Bearer token authentication, Security Headers (nosniff, clickjacking guards), Read-Only guards.
//
// Data Flow:
//
//	Container Snapshots -> Broadcaster -> SSE Stream -> Browser Client (Vanilla JS Dashboard).
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edsilegx/ctop/pkg/audit"
	"github.com/edsilegx/ctop/pkg/prober"
	"github.com/edsilegx/ctop/pkg/serviceprobe"
)

// MinAuthTokenLength defines the enforced minimum character length for web authentication tokens (mandatory 64 chars).
const MinAuthTokenLength = 64

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateAuthToken generates a secure 64-character random alphanumeric authentication token (~381-bit entropy).
func GenerateAuthToken() (string, error) {
	bytes := make([]byte, 64)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	result := make([]byte, 64)
	for i, b := range bytes {
		result[i] = base62Chars[b%byte(len(base62Chars))]
	}
	return string(result), nil
}

// GenerateSessionID generates a secure 64-character random hexadecimal session ID (256-bit entropy).
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// secureCompare compares two strings in constant time using crypto/subtle to mitigate timing attacks.
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Session represents an active authenticated browser session.
type Session struct {
	ID        string
	CreatedAt time.Time
	LastSeen  time.Time
}

// SessionStore provides thread-safe, bounded in-memory session management with LRU eviction and TTL.
type SessionStore struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	order       []string // ordered slice of session IDs for LRU eviction
	maxSessions int
	absoluteTTL time.Duration
	idleTTL     time.Duration
}

// NewSessionStore constructs a new session store with specified limits.
func NewSessionStore(maxCapacity int, absoluteTTL, idleTTL time.Duration) *SessionStore {
	if maxCapacity <= 0 {
		maxCapacity = 100
	}
	if absoluteTTL <= 0 {
		absoluteTTL = 24 * time.Hour
	}
	if idleTTL <= 0 {
		idleTTL = 2 * time.Hour
	}
	return &SessionStore{
		sessions:    make(map[string]*Session),
		order:       make([]string, 0, maxCapacity),
		maxSessions: maxCapacity,
		absoluteTTL: absoluteTTL,
		idleTTL:     idleTTL,
	}
}

// CreateSession generates a new 256-bit session ID, stores it, and returns the ID.
func (s *SessionStore) CreateSession() (string, error) {
	id, err := GenerateSessionID()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.cleanupExpiredLocked(now)

	// If at capacity, evict oldest session
	if len(s.sessions) >= s.maxSessions && len(s.order) > 0 {
		oldestID := s.order[0]
		s.order = s.order[1:]
		delete(s.sessions, oldestID)
	}

	sess := &Session{
		ID:        id,
		CreatedAt: now,
		LastSeen:  now,
	}
	s.sessions[id] = sess
	s.order = append(s.order, id)
	return id, nil
}

// ValidateSession checks if a session ID is valid, unexpired, and refreshes its LastSeen timestamp.
func (s *SessionStore) ValidateSession(id string) bool {
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, exists := s.sessions[id]
	if !exists {
		return false
	}

	now := time.Now()
	if now.Sub(sess.CreatedAt) > s.absoluteTTL || now.Sub(sess.LastSeen) > s.idleTTL {
		// Session expired
		s.removeLocked(id)
		return false
	}

	sess.LastSeen = now
	// Move to end of order (most recently used)
	s.touchOrderLocked(id)
	return true
}

// RevokeSession explicitly destroys a session.
func (s *SessionStore) RevokeSession(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(id)
}

// Count returns the number of active sessions in memory.
func (s *SessionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

func (s *SessionStore) removeLocked(id string) {
	delete(s.sessions, id)
	for i, sid := range s.order {
		if sid == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

func (s *SessionStore) touchOrderLocked(id string) {
	for i, sid := range s.order {
		if sid == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			s.order = append(s.order, id)
			break
		}
	}
}

func (s *SessionStore) cleanupExpiredLocked(now time.Time) {
	for id, sess := range s.sessions {
		if now.Sub(sess.CreatedAt) > s.absoluteTTL || now.Sub(sess.LastSeen) > s.idleTTL {
			s.removeLocked(id)
		}
	}
}

// LoginRateLimiter enforces a sliding-window failed login attempt threshold per IP.
type LoginRateLimiter struct {
	mu           sync.Mutex
	failures     map[string][]time.Time
	maxAttempts  int
	windowPeriod time.Duration
}

// NewLoginRateLimiter creates a rate limiter allowing up to maxAttempts failures per windowPeriod.
func NewLoginRateLimiter(maxAttempts int, windowPeriod time.Duration) *LoginRateLimiter {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if windowPeriod <= 0 {
		windowPeriod = 1 * time.Minute
	}
	return &LoginRateLimiter{
		failures:     make(map[string][]time.Time),
		maxAttempts:  maxAttempts,
		windowPeriod: windowPeriod,
	}
}

// Allow returns true if the IP has not exceeded the failure threshold.
func (rl *LoginRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.windowPeriod)

	attempts, exists := rl.failures[ip]
	if !exists {
		return true
	}

	// Filter out old attempts outside the window
	var valid []time.Time
	for _, t := range attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	rl.failures[ip] = valid

	return len(valid) < rl.maxAttempts
}

// RecordFailure records a failed login attempt for the given IP.
func (rl *LoginRateLimiter) RecordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.failures[ip] = append(rl.failures[ip], time.Now())
}

// RecordSuccess clears failure history for the given IP on successful login.
func (rl *LoginRateLimiter) RecordSuccess(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.failures, ip)
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
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	// Remove existing read-only file if present before re-creating
	_ = os.Remove(targetPath)
	if err := os.WriteFile(targetPath, []byte(strings.TrimSpace(token)+"\n"), 0o400); err != nil {
		return "", fmt.Errorf("failed to write secure token file %s: %w", targetPath, err)
	}
	return targetPath, nil
}

// ReadSecureTokenFile reads and validates the authentication token from filePath (or DefaultAuthTokenPath).
func ReadSecureTokenFile(filePath string) (string, error) {
	targetPath := filePath
	if targetPath == "" {
		targetPath = DefaultAuthTokenPath()
	}
	cleanPath := filepath.Clean(targetPath)
	// #nosec G304 -- cleanPath is cleaned and resolved to standard ~/.config/ctop/token or explicit configuration
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if len(token) < MinAuthTokenLength {
		return "", fmt.Errorf("token in %s is too short (got %d chars, minimum %d required)", cleanPath, len(token), MinAuthTokenLength)
	}
	return token, nil
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
	ReadContainerDir(id, path string) ([]FileEntry, error)
	ReadContainerFile(id, path string, maxBytes int64) (string, error)
	SearchContainerFiles(id, basePath, pattern string, maxResults int) ([]FileEntry, error)
}

// Server provides the embedded real-time HTTP server, REST telemetry APIs, and SSE stream.
type Server struct {
	addr         string
	urlPrefix    string
	provider     ContainerProvider
	broadcaster  *Broadcaster
	httpServer   *http.Server
	mux          *http.ServeMux
	startTime    time.Time
	version      string
	tlsCertFile  string
	tlsKeyFile   string
	authToken    string
	sessionStore *SessionStore
	rateLimiter  *LoginRateLimiter
	mu           sync.RWMutex
	running      bool
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
func NewServer(addr, version string, provider ContainerProvider, broadcaster *Broadcaster, urlPrefix ...string) *Server {
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
		addr:         addr,
		urlPrefix:    prefix,
		version:      version,
		provider:     provider,
		broadcaster:  broadcaster,
		sessionStore: NewSessionStore(100, 24*time.Hour, 2*time.Hour),
		rateLimiter:  NewLoginRateLimiter(5, 1*time.Minute),
		startTime:    time.Now(),
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
		mux.HandleFunc(p("/api/v1/auth/login"), s.handleAuthLogin)
		mux.HandleFunc(p("/api/v1/auth/login/"), s.handleAuthLogin)
		mux.HandleFunc(p("/api/v1/auth/logout"), s.handleAuthLogout)
		mux.HandleFunc(p("/api/v1/auth/logout/"), s.handleAuthLogout)
		mux.HandleFunc(p("/api/v1/auth/status"), s.handleAuthStatus)
		mux.HandleFunc(p("/api/v1/auth/status/"), s.handleAuthStatus)
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

// EnableAuth enables web token authentication by automatically generating a fresh 64-character token.
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

// EnableAuthWithToken configures a pre-existing authentication token (e.g. for persistent tokens).
func (s *Server) EnableAuthWithToken(token string) error {
	token = strings.TrimSpace(token)
	if len(token) < MinAuthTokenLength {
		return fmt.Errorf("authentication token must be at least %d characters, got %d", MinAuthTokenLength, len(token))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authToken = token
	return nil
}

// AuthToken returns the currently configured authentication token.
func (s *Server) AuthToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authToken
}

// SessionStore returns the active in-memory session store.
func (s *Server) SessionStore() *SessionStore {
	return s.sessionStore
}

// RateLimiter returns the active login rate limiter.
func (s *Server) RateLimiter() *LoginRateLimiter {
	return s.rateLimiter
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

	// Mandatory Security Invariant:
	// If authentication is enabled without TLS certificates, enforce loopback (127.0.0.1) binding.
	// No plain HTTP server with auth tokens may listen on external / non-loopback interfaces.
	if s.authToken != "" && (s.tlsCertFile == "" || s.tlsKeyFile == "") {
		host, port, err := net.SplitHostPort(s.addr)
		if err == nil {
			if host != "127.0.0.1" && host != "localhost" && host != "::1" {
				s.addr = net.JoinHostPort("127.0.0.1", port)
			}
		} else if !strings.Contains(s.addr, ":") {
			s.addr = "127.0.0.1:" + s.addr
		}
	}

	// Determine network protocol (tcp4 vs dual-stack tcp):
	network := "tcp"
	listenAddr := s.addr

	host, port, splitErr := net.SplitHostPort(listenAddr)
	if splitErr == nil {
		if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
			// Explicit IPv4 host (e.g. 0.0.0.0, 127.0.0.1, 192.168.x.x) MUST use tcp4
			network = "tcp4"
		} else if host == "" || host == "::" || host == "::1" {
			// Dual-stack wildcard or IPv6 localhost
			if !prober.SupportsIPv6() {
				// IPv6 not supported on this host: sanitize to IPv4
				if host == "::1" {
					listenAddr = net.JoinHostPort("127.0.0.1", port)
				} else {
					listenAddr = net.JoinHostPort("0.0.0.0", port)
				}
				network = "tcp4"
			}
		}
	} else if strings.HasPrefix(listenAddr, ":") {
		// ":port" format: uses dual-stack if IPv6 supported, otherwise IPv4
		if !prober.SupportsIPv6() {
			listenAddr = "0.0.0.0" + listenAddr
			network = "tcp4"
		}
	}

	ln, err := net.Listen(network, listenAddr)
	if err != nil && network != "tcp4" {
		// Fallback to tcp4 if dual-stack tcp failed due to IPv6 protocol restrictions
		var fallbackAddr string
		if splitErr == nil {
			if host == "::1" {
				fallbackAddr = net.JoinHostPort("127.0.0.1", port)
			} else {
				fallbackAddr = net.JoinHostPort("0.0.0.0", port)
			}
		} else if strings.HasPrefix(listenAddr, ":") {
			fallbackAddr = "0.0.0.0" + listenAddr
		} else {
			fallbackAddr = listenAddr
		}
		ln4, err4 := net.Listen("tcp4", fallbackAddr)
		if err4 == nil {
			ln = ln4
			err = nil
		}
	}
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}
	s.addr = ln.Addr().String()

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}

	s.httpServer = &http.Server{
		Handler:           s.corsMiddleware(s.mux),
		TLSConfig:         tlsConfig,
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

type responseCaptureWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *responseCaptureWriter) WriteHeader(code int) {
	if !w.written {
		w.statusCode = code
		w.written = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseCaptureWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.statusCode = http.StatusOK
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *responseCaptureWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func tlsVersionString(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLSv1.3"
	case tls.VersionTLS12:
		return "TLSv1.2"
	case tls.VersionTLS11:
		return "TLSv1.1"
	case tls.VersionTLS10:
		return "TLSv1.0"
	default:
		return fmt.Sprintf("0x%04x", v)
	}
}

// corsMiddleware sets secure headers, authentication, and CORS policies for read-only telemetry.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseCaptureWriter{ResponseWriter: w, statusCode: http.StatusOK}
		clientIP := getClientIP(r)
		authInfo := &audit.AuthInfo{Type: "none", Authenticated: false}

		defer func() {
			duration := time.Since(start)
			details := map[string]interface{}{
				"user_agent": r.UserAgent(),
			}
			if r.TLS != nil {
				details["tls"] = tlsVersionString(r.TLS.Version)
			}
			if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
				details["forwarded_proto"] = proto
			}
			audit.LogAccess(clientIP, r.Method, r.URL.Path, rw.statusCode, duration, authInfo, details)
		}()

		rw.Header().Set("Access-Control-Allow-Origin", "*")
		rw.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, OPTIONS")
		rw.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		rw.Header().Set("X-Frame-Options", "SAMEORIGIN")

		if r.Method == http.MethodOptions {
			rw.WriteHeader(http.StatusNoContent)
			return
		}

		// Global path traversal protection
		if strings.Contains(r.URL.Path, "..") || strings.Contains(r.URL.RawPath, "..") || strings.Contains(r.URL.Path, "\\") {
			http.Error(rw, `{"error":"Invalid request path - traversal detected"}`, http.StatusBadRequest)
			return
		}

		reqPath := r.URL.Path
		if s.urlPrefix != "" {
			reqPath = strings.TrimPrefix(reqPath, s.urlPrefix)
		}
		isAuthAction := reqPath == "/api/v1/auth/login" || reqPath == "/api/v1/auth/login/" ||
			reqPath == "/api/v1/auth/logout" || reqPath == "/api/v1/auth/logout/"
		isAuthStatus := reqPath == "/api/v1/auth/status" || reqPath == "/api/v1/auth/status/"

		// Strictly enforce READ-ONLY access across all routes except auth actions
		if !isAuthAction && r.Method != http.MethodGet && r.Method != http.MethodHead {
			rw.Header().Set("Allow", "GET, HEAD, OPTIONS")
			http.Error(rw, `{"error":"Method Not Allowed - ctop web API is strictly read-only"}`, http.StatusMethodNotAllowed)
			return
		}

		// Token authentication enforcement when configured
		s.mu.RLock()
		token := s.authToken
		s.mu.RUnlock()

		if token != "" {
			// Direct local loopback access (without proxy forwarding headers) bypasses authentication for local developer UI
			if isDirectLocalAccess(r) {
				authInfo.Type = "loopback"
				authInfo.Authenticated = true
				next.ServeHTTP(rw, r)
				return
			}

			// Remote or proxied requests MUST be TLS encrypted (direct TLS or reverse proxy with HTTPS)
			if !isTLSOrSecureProxy(r) {
				http.Error(rw, `{"error":"Forbidden - TLS encryption required (direct TLS or X-Forwarded-Proto: https)"}`, http.StatusForbidden)
				return
			}

			// Allow unauthenticated access to auth endpoints (login, logout, status) and root dashboard SPA (to render unlock modal)
			if isAuthAction || isAuthStatus {
				next.ServeHTTP(rw, r)
				return
			}

			// 1. Check HTTP Authorization Header: Bearer <token>
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				reqToken := strings.TrimPrefix(authHeader, "Bearer ")
				if secureCompare(reqToken, token) {
					authInfo.Type = "bearer"
					authInfo.Authenticated = true
					if len(token) >= 4 {
						authInfo.TokenPrefix = token[:4] + "..."
					}
					next.ServeHTTP(rw, r)
					return
				}
				rw.Header().Set("WWW-Authenticate", `Bearer realm="ctop"`)
				http.Error(rw, `{"error":"Unauthorized - invalid or missing authentication token"}`, http.StatusUnauthorized)
				return
			}

			// 2. Check ctop_session Cookie
			if cookie, err := r.Cookie("ctop_session"); err == nil && cookie != nil {
				if s.sessionStore != nil && s.sessionStore.ValidateSession(cookie.Value) {
					authInfo.Type = "session"
					authInfo.Authenticated = true
					if len(cookie.Value) >= 8 {
						authInfo.SessionID = cookie.Value[:8]
					}
					next.ServeHTTP(rw, r)
					return
				}
			}

			// 3. Reject URL Query Parameter tokens explicitly (Zero-Leak Policy)
			if r.URL.Query().Get("token") != "" || r.URL.Query().Get("auth") != "" {
				rw.Header().Set("WWW-Authenticate", `Bearer realm="ctop"`)
				http.Error(rw, `{"error":"Unauthorized - URL query parameter tokens are discontinued. Use 'Authorization: Bearer <token>' header or Web Dashboard session login."}`, http.StatusUnauthorized)
				return
			}

			// 4. For Root Index (dashboard SPA), serve the page so it can render the unlock modal
			if r.URL.Path == "/" || r.URL.Path == s.urlPrefix || r.URL.Path == s.urlPrefix+"/" {
				next.ServeHTTP(rw, r)
				return
			}

			// All other endpoints require authentication
			rw.Header().Set("WWW-Authenticate", `Bearer realm="ctop"`)
			http.Error(rw, `{"error":"Unauthorized - invalid or missing authentication token"}`, http.StatusUnauthorized)
			return
		} else {
			authInfo.Authenticated = true
		}

		next.ServeHTTP(rw, r)
	})
}

// isLoopbackAddr returns true if the remote address string represents a local loopback IP interface.
func isLoopbackAddr(remoteAddr string) bool {
	if remoteAddr == "" {
		return true
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// isDirectLocalAccess verifies if a request originates directly from a local loopback interface
// without passing through any reverse proxy forwarding headers.
func isDirectLocalAccess(r *http.Request) bool {
	if r == nil {
		return false
	}
	hasProxyHeaders := r.Header.Get("X-Forwarded-For") != "" ||
		r.Header.Get("X-Real-IP") != "" ||
		r.Header.Get("X-Forwarded-Proto") != "" ||
		r.Header.Get("X-Forwarded-Host") != "" ||
		r.Header.Get("X-Forwarded-Scheme") != "" ||
		r.Header.Get("Front-End-Https") != ""
	if hasProxyHeaders {
		return false
	}
	return isLoopbackAddr(r.RemoteAddr)
}

// isTLSOrSecureProxy verifies if a request is TLS-encrypted directly, forwarded via a secure TLS reverse proxy,
// or sent from a local loopback interface.
func isTLSOrSecureProxy(r *http.Request) bool {
	if r == nil {
		return false
	}
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
	return isDirectLocalAccess(r)
}

// getClientIP extracts the remote client IP for rate limiting from proxy headers or RemoteAddr.
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// LoginRequest represents the JSON payload for /api/v1/auth/login.
type LoginRequest struct {
	Token string `json:"token"`
}

// LoginResponse represents the status response from authentication actions.
type LoginResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// AuthStatusResponse represents the result of querying /api/v1/auth/status.
type AuthStatusResponse struct {
	Authenticated bool `json:"authenticated"`
	AuthEnabled   bool `json:"auth_enabled"`
	DirectLocal   bool `json:"direct_local"`
}

// handleAuthLogin authenticates a token payload, enforces rate limiting, and issues an HttpOnly session cookie.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, `{"error":"Method Not Allowed - use POST"}`, http.StatusMethodNotAllowed)
		return
	}

	clientIP := getClientIP(r)
	if s.rateLimiter != nil && !s.rateLimiter.Allow(clientIP) {
		audit.LogAuth("rate_limited", audit.LevelWarn, clientIP, &audit.AuthInfo{Type: "none", Authenticated: false}, map[string]interface{}{"retry_after_sec": 60})
		w.Header().Set("Retry-After", "60")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Too many failed login attempts. Please retry after 60 seconds.",
		})
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		audit.LogAuth("login_failure", audit.LevelWarn, clientIP, &audit.AuthInfo{Type: "none", Authenticated: false}, map[string]interface{}{"reason": "invalid_payload"})
		http.Error(w, `{"error":"Invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	token := s.authToken
	s.mu.RUnlock()

	if token != "" && !secureCompare(req.Token, token) {
		if s.rateLimiter != nil {
			s.rateLimiter.RecordFailure(clientIP)
		}
		audit.LogAuth("login_failure", audit.LevelWarn, clientIP, &audit.AuthInfo{Type: "none", Authenticated: false}, map[string]interface{}{"reason": "invalid_token"})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid authentication token",
		})
		return
	}

	if s.rateLimiter != nil {
		s.rateLimiter.RecordSuccess(clientIP)
	}

	sessID := ""
	if s.sessionStore != nil {
		var err error
		sessID, err = s.sessionStore.CreateSession()
		if err != nil {
			http.Error(w, `{"error":"Internal session generation error"}`, http.StatusInternalServerError)
			return
		}
	}

	sessionPrefix := ""
	if len(sessID) >= 8 {
		sessionPrefix = sessID[:8]
	}
	audit.LogAuth("login_success", audit.LevelInfo, clientIP, &audit.AuthInfo{Type: "session", Authenticated: true, SessionID: sessionPrefix}, nil)

	isSecure := (r.TLS != nil) || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	// #nosec G124 -- Cookie enforces HttpOnly and SameSite=Strict; Secure attribute is dynamic based on incoming TLS / reverse-proxy scheme
	cookie := &http.Cookie{
		Name:     "ctop_session",
		Value:    sessID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   isSecure,
		MaxAge:   86400, // 24 hours
	}
	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(LoginResponse{
		Status: "authenticated",
	})
}

// handleAuthLogout terminates the active session and clears the ctop_session cookie.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, `{"error":"Method Not Allowed - use POST"}`, http.StatusMethodNotAllowed)
		return
	}

	clientIP := getClientIP(r)
	if cookie, err := r.Cookie("ctop_session"); err == nil && cookie != nil {
		if s.sessionStore != nil {
			s.sessionStore.RevokeSession(cookie.Value)
		}
	}
	audit.LogAuth("logout", audit.LevelInfo, clientIP, &audit.AuthInfo{Type: "session", Authenticated: false}, nil)

	isSecure := (r.TLS != nil) || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	// #nosec G124 -- Cookie enforces HttpOnly and SameSite=Strict; Secure attribute is dynamic based on incoming TLS / reverse-proxy scheme
	http.SetCookie(w, &http.Cookie{
		Name:     "ctop_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   isSecure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(LoginResponse{
		Status: "logged_out",
	})
}

// handleAuthStatus returns the current authentication and locality status.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	token := s.authToken
	s.mu.RUnlock()

	directLocal := isDirectLocalAccess(r)
	if token == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(AuthStatusResponse{
			Authenticated: true,
			AuthEnabled:   false,
			DirectLocal:   directLocal,
		})
		return
	}

	if directLocal {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(AuthStatusResponse{
			Authenticated: true,
			AuthEnabled:   true,
			DirectLocal:   true,
		})
		return
	}

	// Check Bearer header
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		reqToken := strings.TrimPrefix(authHeader, "Bearer ")
		if secureCompare(reqToken, token) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(AuthStatusResponse{
				Authenticated: true,
				AuthEnabled:   true,
				DirectLocal:   false,
			})
			return
		}
	}

	// Check session cookie
	if cookie, err := r.Cookie("ctop_session"); err == nil && cookie != nil {
		if s.sessionStore != nil && s.sessionStore.ValidateSession(cookie.Value) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(AuthStatusResponse{
				Authenticated: true,
				AuthEnabled:   true,
				DirectLocal:   false,
			})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(AuthStatusResponse{
		Authenticated: false,
		AuthEnabled:   true,
		DirectLocal:   false,
	})
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
	isProxy := false
	isEndpoints := false
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
	} else if strings.HasSuffix(path, "/proxy") || strings.HasSuffix(path, "/preview") {
		isProxy = true
		id = strings.TrimSuffix(path, "/proxy")
		id = strings.TrimSuffix(id, "/preview")
		id = strings.TrimSpace(id)
	} else if strings.HasSuffix(path, "/endpoints") || strings.HasSuffix(path, "/web-endpoints") {
		isEndpoints = true
		id = strings.TrimSuffix(path, "/endpoints")
		id = strings.TrimSuffix(id, "/web-endpoints")
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
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusOK)
			// #nosec G705 -- Content-Type is strictly text/plain with nosniff header, preventing browser HTML/XSS interpretation
			_, _ = w.Write([]byte(content)) // nosemgrep
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

	if isEndpoints {
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

		endpoints := serviceprobe.DiscoverEndpoints(targetSnap.Ports, rawNetStr, targetSnap.Env)
		if endpoints == nil {
			endpoints = []serviceprobe.Endpoint{}
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(endpoints)
		return
	}

	if isProxy {
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

		endpoints := serviceprobe.DiscoverEndpoints(targetSnap.Ports, rawNetStr, targetSnap.Env)

		targetPort := 80
		if pStr := r.URL.Query().Get("port"); pStr != "" {
			if pInt, err := strconv.Atoi(pStr); err == nil && pInt > 0 {
				targetPort = pInt
			}
		} else if len(endpoints) > 0 {
			targetPort = endpoints[0].Port
		}

		subpath := r.URL.Query().Get("path")
		if subpath == "" {
			subpath = "/"
		}
		if !strings.HasPrefix(subpath, "/") {
			subpath = "/" + subpath
		}
		for strings.HasPrefix(subpath, "//") {
			subpath = strings.TrimPrefix(subpath, "/")
		}
		if !strings.HasPrefix(subpath, "/") {
			subpath = "/" + subpath
		}
		if strings.Contains(subpath, "@") || strings.Contains(subpath, "\\") {
			http.Error(w, `{"error":"Invalid characters in subpath parameter"}`, http.StatusBadRequest)
			return
		}

		targetHost := "127.0.0.1"
		proto := "http"

		// Match host and protocol from discovered endpoints
		var matchedEP *serviceprobe.Endpoint
		for i := range endpoints {
			if endpoints[i].Port == targetPort || endpoints[i].HostPort == targetPort {
				matchedEP = &endpoints[i]
				break
			}
		}

		if matchedEP != nil {
			targetHost = matchedEP.HostIP
			targetPort = matchedEP.HostPort
			proto = matchedEP.Protocol
		} else if len(endpoints) > 0 {
			http.Error(w, fmt.Sprintf(`{"error":"Port %d is not exposed or mapped by container %q"}`, targetPort, id), http.StatusBadRequest)
			return
		}

		targetURL := fmt.Sprintf("%s://%s:%d%s", proto, targetHost, targetPort, subpath)

		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()

		probeRes := serviceprobe.ProbeHTTP(ctx, targetURL, 3*time.Second)

		format := strings.ToLower(r.URL.Query().Get("format"))
		if format == "json" || strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(probeRes)
			return
		}

		// HTML / Raw proxy mode
		if probeRes.Error != "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline';")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusBadGateway)
			// #nosec G705 -- Content is escaped using html.EscapeString and served with restrictive CSP
			_, _ = fmt.Fprintf(w, "<html><body style='font-family:sans-serif;padding:2rem;background:#18181b;color:#f43f5e;'><h3>Service Preview Unavailable</h3><p>%s</p><p>Target: <code>%s</code></p></body></html>", html.EscapeString(probeRes.Error), html.EscapeString(targetURL))
			return
		}

		ct := probeRes.ContentType
		if ct == "" {
			ct = "text/html; charset=utf-8"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-forms; default-src 'self' 'unsafe-inline' data:;")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(probeRes.StatusCode)
		// #nosec G705 -- Raw body is served inside isolated sandbox iframe with restrictive Content-Security-Policy
		_, _ = w.Write([]byte(probeRes.Body))
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

	clientIP := getClientIP(r)
	audit.Log(audit.Event{
		Level:    audit.LevelInfo,
		Category: audit.CategoryAccess,
		Action:   "sse_connect",
		ClientIP: clientIP,
		Path:     r.URL.Path,
		Details:  map[string]interface{}{"subscribers": s.broadcaster.SubscriberCount() + 1},
	})
	defer func() {
		audit.Log(audit.Event{
			Level:    audit.LevelInfo,
			Category: audit.CategoryAccess,
			Action:   "sse_disconnect",
			ClientIP: clientIP,
			Path:     r.URL.Path,
		})
	}()

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
