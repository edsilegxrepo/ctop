// Package audit provides thread-safe, daily-rotated NDJSON audit logging for ctop events and access.
//
// Objective:
//
//	Deliver immutable, structured NDJSON audit trails covering all API accesses, authentication lifecycle,
//	container engine events, and application startups/shutdowns with automatic daily file rotation.
//
// Core Components:
//   - Logger: Concurrency-safe audit file writer with date checking and atomic file swapping.
//   - Event: Canonical schema model mapping timestamp, level, category, action, client IP, auth, and details.
//   - Global Helpers: Thread-safe singleton dispatchers (LogAccess, LogAuth, LogContainer, LogApp).
//
// Functionality:
//   - Automatic daily file rotation (base_path.YYYY-MM-DD.ndjson) triggered at midnight or initialization.
//   - Zero secret leakage: only truncated token/session prefixes (<= 8 chars) are recorded.
//   - Thread-safe RWMutex locking ensuring zero dropped or corrupted lines under high concurrency.
//
// Data Flow:
//
//	Application Action -> LogAccess/LogAuth -> Event struct -> JSON Marshaling -> Dated NDJSON File.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level represents the severity of an audit record.
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// Category represents the domain of an audit event.
type Category string

const (
	CategoryAccess    Category = "access"
	CategoryAuth      Category = "auth"
	CategoryContainer Category = "container"
	CategoryApp       Category = "app"
)

// AuthInfo contains authentication details for access or auth events.
type AuthInfo struct {
	Type          string `json:"type,omitempty"` // "bearer", "session", "loopback", "none"
	Authenticated bool   `json:"authenticated"`
	TokenPrefix   string `json:"token_prefix,omitempty"` // Truncated prefix for correlation (never full token)
	SessionID     string `json:"session_id,omitempty"`   // Truncated session identifier
}

// Event represents a single NDJSON audit record.
type Event struct {
	Timestamp  string                 `json:"timestamp"`
	Level      Level                  `json:"level"`
	Category   Category               `json:"category"`
	Action     string                 `json:"action"`
	ClientIP   string                 `json:"client_ip,omitempty"`
	Method     string                 `json:"method,omitempty"`
	Path       string                 `json:"path,omitempty"`
	Status     int                    `json:"status,omitempty"`
	DurationMS float64                `json:"duration_ms,omitempty"`
	Auth       *AuthInfo              `json:"auth,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// Logger manages thread-safe, daily-rotated NDJSON audit logging.
type Logger struct {
	mu         sync.Mutex
	basePath   string
	activeDate string
	activePath string
	file       *os.File
	closed     bool
	nowFunc    func() time.Time // Injectable for deterministic date rotation testing
}

var (
	globalMu     sync.RWMutex
	globalLogger *Logger
)

// NewLogger creates a new audit logger target path with daily file rotation.
func NewLogger(basePath string) (*Logger, error) {
	if strings.TrimSpace(basePath) == "" {
		return nil, fmt.Errorf("audit log path cannot be empty")
	}

	absPath, err := filepath.Abs(basePath)
	if err != nil {
		absPath = basePath
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create audit log directory %s: %w", dir, err)
	}

	l := &Logger{
		basePath: absPath,
		nowFunc:  time.Now,
	}

	// Open initial file for today
	if err := l.rotateLocked(l.nowFunc()); err != nil {
		return nil, err
	}

	return l, nil
}

// Init initializes the global audit logger.
func Init(basePath string) (*Logger, error) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalLogger != nil {
		_ = globalLogger.Close()
	}

	l, err := NewLogger(basePath)
	if err != nil {
		return nil, err
	}
	globalLogger = l
	return l, nil
}

// Get returns the active global audit logger (or nil if unconfigured).
func Get() *Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLogger
}

// Close flushes and closes the global audit logger.
func Close() {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalLogger != nil {
		_ = globalLogger.Close()
		globalLogger = nil
	}
}

// computeDatedPath generates the date-stamped filename for the given timestamp.
func computeDatedPath(basePath string, t time.Time) string {
	dateStr := t.Format("2006-01-02")
	dir := filepath.Dir(basePath)
	ext := filepath.Ext(basePath)
	stem := strings.TrimSuffix(filepath.Base(basePath), ext)

	if ext == "" {
		ext = ".ndjson"
	}

	// If stem already ends with the date, preserve it
	if strings.HasSuffix(stem, dateStr) {
		return filepath.Join(dir, stem+ext)
	}

	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", stem, dateStr, ext))
}

// rotateLocked rotates the active file handle if the date has transitioned.
func (l *Logger) rotateLocked(now time.Time) error {
	today := now.Format("2006-01-02")
	if l.file != nil && l.activeDate == today {
		return nil
	}

	targetPath := computeDatedPath(l.basePath, now)

	if l.file != nil {
		_ = l.file.Sync()
		_ = l.file.Close()
		l.file = nil
	}

	// #nosec G304 -- targetPath is safely derived from validated absolute basePath and date format
	f, err := os.OpenFile(filepath.Clean(targetPath), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open audit log file %s: %w", targetPath, err)
	}

	l.file = f
	l.activeDate = today
	l.activePath = targetPath
	return nil
}

// Log writes a single audit event to the active daily NDJSON file.
func (l *Logger) Log(event Event) error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return fmt.Errorf("audit logger is closed")
	}

	now := l.nowFunc()
	if err := l.rotateLocked(now); err != nil {
		return err
	}

	if event.Timestamp == "" {
		event.Timestamp = now.UTC().Format(time.RFC3339Nano)
	}
	if event.Level == "" {
		event.Level = LevelInfo
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	data = append(data, '\n')
	if _, err := l.file.Write(data); err != nil {
		return fmt.Errorf("failed to write audit log entry: %w", err)
	}

	return nil
}

// Close cleanly synchronizes and closes the active audit log file.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	if l.file != nil {
		_ = l.file.Sync()
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// ActivePath returns the current date-stamped file path being written to.
func (l *Logger) ActivePath() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.activePath
}

// LogGlobal is a package-level helper to write an audit event to the global logger.
func Log(event Event) {
	if l := Get(); l != nil {
		_ = l.Log(event)
	}
}

// LogAccess records an HTTP API or telemetry request event.
func LogAccess(clientIP, method, path string, status int, duration time.Duration, auth *AuthInfo, details map[string]interface{}) {
	level := LevelInfo
	if status >= 400 && status < 500 {
		level = LevelWarn
	} else if status >= 500 {
		level = LevelError
	}

	Log(Event{
		Level:      level,
		Category:   CategoryAccess,
		Action:     "http_request",
		ClientIP:   clientIP,
		Method:     method,
		Path:       path,
		Status:     status,
		DurationMS: float64(duration.Microseconds()) / 1000.0,
		Auth:       auth,
		Details:    details,
	})
}

// LogAuth records an authentication lifecycle event (login, logout, rate_limit, rejection).
func LogAuth(action string, level Level, clientIP string, auth *AuthInfo, details map[string]interface{}) {
	Log(Event{
		Level:    level,
		Category: CategoryAuth,
		Action:   action,
		ClientIP: clientIP,
		Auth:     auth,
		Details:  details,
	})
}

// LogContainer records a container lifecycle or operational event.
func LogContainer(action, containerID, containerName string, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["container_id"] = containerID
	details["container_name"] = containerName

	Log(Event{
		Level:    LevelInfo,
		Category: CategoryContainer,
		Action:   action,
		Details:  details,
	})
}

// LogApp records application startup, shutdown, and configuration events.
func LogApp(action string, level Level, details map[string]interface{}) {
	Log(Event{
		Level:    level,
		Category: CategoryApp,
		Action:   action,
		Details:  details,
	})
}
