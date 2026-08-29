// Package web provides an embedded real-time HTTP server, Server-Sent Events (SSE) broadcaster,
// HTML5 visualization dashboard, and read-only REST telemetry APIs for ctop.
//
// SAFETY GUARANTEE:
// This package is strictly READ-ONLY. No mutating Docker commands, lifecycle controls,
// exec sessions, file uploads, or container modifications are exposed under any circumstance.
package web

import "time"

// MountInfo represents container volume and bind mount configuration.
type MountInfo struct {
	Destination string `json:"destination"`
	Source      string `json:"source"`
	Type        string `json:"type"`
	Mode        string `json:"mode"`
	Driver      string `json:"driver,omitempty"`
}

// NetworkInfo represents container network interface configuration.
type NetworkInfo struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	Gateway   string `json:"gateway"`
	Mac       string `json:"mac"`
	PrefixLen int    `json:"prefix_len"`
}

// ContainerSnapshot represents a point-in-time telemetry snapshot for a single container.
type ContainerSnapshot struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	State        string            `json:"state"`
	Health       string            `json:"health,omitempty"`
	Host         string            `json:"host,omitempty"`
	Created      string            `json:"created,omitempty"`
	Uptime       string            `json:"uptime,omitempty"`
	CPUUtil      int               `json:"cpu_util"`
	MemUsage     int64             `json:"mem_usage"`
	MemLimit     int64             `json:"mem_limit"`
	MemPercent   int               `json:"mem_percent"`
	MemRss       int64             `json:"mem_rss,omitempty"`
	MemCache     int64             `json:"mem_cache,omitempty"`
	NetRx        int64             `json:"net_rx"`
	NetTx        int64             `json:"net_tx"`
	NetRxRate    int64             `json:"net_rx_rate"`
	NetTxRate    int64             `json:"net_tx_rate"`
	IOBytesRead  int64             `json:"io_bytes_read"`
	IOBytesWrite int64             `json:"io_bytes_write"`
	IORateRead   int64             `json:"io_rate_read"`
	IORateWrite  int64             `json:"io_rate_write"`
	Pids         int               `json:"pids"`
	IPs          string            `json:"ips,omitempty"`
	Ports        string            `json:"ports,omitempty"`
	WebPort      string            `json:"web_port,omitempty"`
	Command      string            `json:"command,omitempty"`
	Entrypoint   string            `json:"entrypoint,omitempty"`
	WorkDir      string            `json:"workdir,omitempty"`
	User         string            `json:"user,omitempty"`
	RestartPol   string            `json:"restart_policy,omitempty"`
	MemLimitStr  string            `json:"mem_limit_str,omitempty"`
	Env          []string          `json:"env,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Mounts       []MountInfo       `json:"mounts,omitempty"`
	Networks     []NetworkInfo     `json:"networks,omitempty"`
	Timestamp    time.Time         `json:"timestamp"`
}

// SystemMetrics represents aggregated cluster or host-level telemetry across all containers.
type SystemMetrics struct {
	TotalContainers   int       `json:"total_containers"`
	RunningContainers int       `json:"running_containers"`
	PausedContainers  int       `json:"paused_containers"`
	StoppedContainers int       `json:"stopped_containers"`
	TotalCPUUtil      int       `json:"total_cpu_util"`
	TotalMemUsage     int64     `json:"total_mem_usage"`
	TotalMemLimit     int64     `json:"total_mem_limit"`
	TotalNetRxRate    int64     `json:"total_net_rx_rate"`
	TotalNetTxRate    int64     `json:"total_net_tx_rate"`
	TotalIORateRead   int64     `json:"total_io_rate_read"`
	TotalIORateWrite  int64     `json:"total_io_rate_write"`
	UptimeSeconds     int64     `json:"uptime_seconds"`
	Timestamp         time.Time `json:"timestamp"`
}

// TelemetryEvent represents a real-time event broadcasted over Server-Sent Events (SSE).
type TelemetryEvent struct {
	Type       string              `json:"type"` // "metrics", "heartbeat", "initial"
	Timestamp  string              `json:"timestamp"`
	System     SystemMetrics       `json:"system"`
	Containers []ContainerSnapshot `json:"containers,omitempty"`
}

// HealthStatus represents the readiness and health status response.
type HealthStatus struct {
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"`
	Timestamp time.Time `json:"timestamp"`
}

// AggregateSnapshots calculates cluster or host totals from container snapshots.
func AggregateSnapshots(snapshots []ContainerSnapshot) SystemMetrics {
	sys := SystemMetrics{
		TotalContainers: len(snapshots),
		Timestamp:       time.Now().UTC(),
	}
	for _, c := range snapshots {
		switch c.State {
		case "running":
			sys.RunningContainers++
			sys.TotalCPUUtil += c.CPUUtil
			sys.TotalMemUsage += c.MemUsage
			sys.TotalMemLimit += c.MemLimit
			sys.TotalNetRxRate += c.NetRxRate
			sys.TotalNetTxRate += c.NetTxRate
			sys.TotalIORateRead += c.IORateRead
			sys.TotalIORateWrite += c.IORateWrite
		case "paused":
			sys.PausedContainers++
			sys.TotalMemUsage += c.MemUsage
			sys.TotalMemLimit += c.MemLimit
		default:
			sys.StoppedContainers++
		}
	}
	return sys
}
