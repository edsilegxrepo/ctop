// Package models defines core telemetry, container metadata, and log data structures.
// Objective: Provide lightweight, standardized domain types for container metrics, log lines, and metadata.
// Data Flow: Connector Telemetry Stream -> Models (Metrics, Log, Meta) -> Container -> UI Widgets.
package models

import "time"

// Log represents a single timestamped container log entry.
type Log struct {
	Timestamp time.Time
	Message   string
}

// Meta holds arbitrary key-value metadata strings associated with a container.
type Meta map[string]string

// NewMeta returns an initialized Meta map.
// An optional series of key, values may be provided to populate the map prior to returning
func NewMeta(kvs ...string) Meta {
	m := make(Meta)

	var i int
	for i < len(kvs)-1 {
		m[kvs[i]] = kvs[i+1]
		i += 2
	}

	return m
}

func (m Meta) Get(k string) string {
	if s, ok := m[k]; ok {
		return s
	}
	return ""
}

type Metrics struct {
	NCpus        int
	CPUUtil      int
	NetTx        int64
	NetRx        int64
	MemLimit     int64
	MemPercent   int
	MemUsage     int64
	MemRss       int64
	MemCache     int64
	MemSwap      int64
	MemKernel    int64
	IOBytesRead  int64
	IOBytesWrite int64
	NetTxRate    int64 // bytes per second (EMA smoothed)
	NetRxRate    int64 // bytes per second (EMA smoothed)
	IORateRead   int64 // bytes per second (EMA smoothed)
	IORateWrite  int64 // bytes per second (EMA smoothed)
	Pids         int
}

func NewMetrics() Metrics {
	return Metrics{
		CPUUtil:      -1,
		NetTx:        -1,
		NetRx:        -1,
		NetTxRate:    -1,
		NetRxRate:    -1,
		MemUsage:     -1,
		MemPercent:   -1,
		MemRss:       -1,
		MemCache:     -1,
		MemSwap:      -1,
		MemKernel:    -1,
		IOBytesRead:  -1,
		IOBytesWrite: -1,
		IORateRead:   -1,
		IORateWrite:  -1,
		Pids:         -1,
	}
}

// TopResult holds process table output from container runtime top
type TopResult struct {
	Titles    []string
	Processes [][]string
}

// Change represents a filesystem modification entry (Added, Modified, Deleted)
type Change struct {
	Path string
	Kind int // 0: Modified (C), 1: Added (A), 2: Deleted (D)
}

// FileInfo represents file or directory metadata inside a container
type FileInfo struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	Mode    string
	ModTime string
}

// EMA calculates an Exponential Moving Average (default alpha 0.3) for telemetry smoothing.
type EMA struct {
	Alpha       float64
	Initialized bool
	Value       float64
}

// NewEMA creates an initialized EMA filter with the given alpha smoothing factor.
func NewEMA(alpha float64) *EMA {
	if alpha <= 0.0 || alpha > 1.0 {
		alpha = 0.3 // default smoothing factor
	}
	return &EMA{Alpha: alpha}
}

// Update incorporates a new sample into the smoothed average and returns the updated value.
func (e *EMA) Update(sample float64) float64 {
	if !e.Initialized {
		e.Value = sample
		e.Initialized = true
		return sample
	}
	e.Value = (e.Alpha * sample) + ((1.0 - e.Alpha) * e.Value)
	return e.Value
}
