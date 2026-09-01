// Package logging provides thread-safe structured logging, ring buffer memory backends, status queues, and network stream listeners.
//
// Objective:
//
//	Deliver formatted diagnostic logs, in-memory circular event history, UI notification status queues, and optional remote TCP/Unix socket streaming.
//
// Core Components:
//   - CTopLogger: Wrapper around go-logging providing level filtering, formatted outputs, and status queue dispatches.
//   - safeMemoryBackend: Thread-safe circular memory buffer for log inspection.
//   - loggingServer: Streaming TCP or Unix domain socket server broadcasting live log events.
//
// Data Flow:
//
//	Application Events -> Logger -> Safe Memory Backend / File / Unix Socket / TCP Listener -> Status Line.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/op/go-logging"
)

const (
	size = 1024
)

var (
	Log    *CTopLogger
	exited atomic.Bool
	level  = logging.INFO // default level
	format = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
)

type safeMemoryBackend struct {
	sync.Mutex
	*logging.MemoryBackend
}

func (b *safeMemoryBackend) Log(level logging.Level, calldepth int, rec *logging.Record) error {
	b.Lock()
	defer b.Unlock()
	return b.MemoryBackend.Log(level, calldepth, rec)
}

type statusMsg struct {
	Text    string
	IsError bool
}

type CTopLogger struct {
	*logging.Logger
	backend *safeMemoryBackend
	logFile *os.File
	sLog    []statusMsg
	sLock   sync.Mutex
}

func (c *CTopLogger) FlushStatus() chan statusMsg {
	ch := make(chan statusMsg)
	if c == nil {
		close(ch)
		return ch
	}
	c.sLock.Lock()
	msgs := make([]statusMsg, len(c.sLog))
	copy(msgs, c.sLog)
	c.sLog = c.sLog[:0]
	c.sLock.Unlock()

	go func() {
		for _, sm := range msgs {
			ch <- sm
		}
		close(ch)
	}()
	return ch
}

func (c *CTopLogger) StatusQueued() bool {
	if c == nil {
		return false
	}
	c.sLock.Lock()
	defer c.sLock.Unlock()
	return len(c.sLog) > 0
}

func (c *CTopLogger) Status(s string) {
	if c == nil {
		return
	}
	c.addStatus(statusMsg{s, false})
}

func (c *CTopLogger) StatusErr(err error) {
	if c == nil || err == nil {
		return
	}
	c.addStatus(statusMsg{err.Error(), true})
}

func (c *CTopLogger) addStatus(sm statusMsg) {
	if c == nil {
		return
	}
	c.sLock.Lock()
	defer c.sLock.Unlock()
	c.sLog = append(c.sLog, sm)
}

func (c *CTopLogger) Statusf(s string, a ...interface{}) {
	if c == nil {
		return
	}
	c.Status(fmt.Sprintf(s, a...))
}

var initOnce sync.Once

func Init() *CTopLogger {
	initOnce.Do(func() {
		logging.SetFormatter(format) // setup default formatter

		Log = &CTopLogger{
			Logger:  logging.MustGetLogger("ctop"),
			backend: &safeMemoryBackend{MemoryBackend: logging.NewMemoryBackend(size)},
			logFile: nil,
			sLog:    []statusMsg{},
			sLock:   sync.Mutex{},
		}

		debugMode := debugMode()
		if debugMode {
			level = logging.DEBUG
		}
		backendLvl := logging.AddModuleLevel(Log.backend)
		backendLvl.SetLevel(level, "")

		logFilePath := filepath.Clean(debugModeFile())
		if logFilePath == "." || logFilePath == "" {
			logging.SetBackend(backendLvl)
		} else {
			// #nosec G304 - user-specified debug log file path from environment variable
			logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				logging.SetBackend(backendLvl)
				Log.Errorf("Unable to create log file: %s", err.Error())
			} else {
				backendFile := logging.NewLogBackend(logFile, "", 0)
				backendFileLvl := logging.AddModuleLevel(backendFile)
				backendFileLvl.SetLevel(level, "")
				logging.SetBackend(backendLvl, backendFileLvl)
				Log.logFile = logFile
			}
		}

		if debugMode {
			StartServer()
		}
		Log.Notice("logger initialized")
	})
	return Log
}

func (log *CTopLogger) tail(stopCh <-chan struct{}) chan string {
	stream := make(chan string)

	go func() {
		defer close(stream)
		log.backend.Lock()
		node := log.backend.Head()
		log.backend.Unlock()

		for node == nil {
			if exited.Load() {
				return
			}
			select {
			case <-stopCh:
				return
			case <-time.After(50 * time.Millisecond):
			}
			log.backend.Lock()
			node = log.backend.Head()
			log.backend.Unlock()
		}

		for {
			if exited.Load() {
				return
			}
			var msg string
			log.backend.Lock()
			if node.Record != nil {
				msg = node.Record.Formatted(0)
			}
			log.backend.Unlock()

			if msg != "" {
				select {
				case <-stopCh:
					return
				case stream <- msg:
				}
			}
			for {
				if exited.Load() {
					return
				}
				log.backend.Lock()
				nnode := node.Next()
				log.backend.Unlock()
				if nnode != nil {
					node = nnode
					break
				}
				select {
				case <-stopCh:
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
	}()

	return stream
}

func (log *CTopLogger) Exit() {
	exited.Store(true)
	if log.logFile != nil {
		_ = log.logFile.Close()
	}
	StopServer()
}

func debugMode() bool       { return os.Getenv("CTOP_DEBUG") == "1" }
func debugModeTCP() bool    { return os.Getenv("CTOP_DEBUG_TCP") == "1" }
func debugModeFile() string { return os.Getenv("CTOP_DEBUG_FILE") }
