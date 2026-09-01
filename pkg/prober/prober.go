// Package prober performs non-blocking TCP network reachability probes against container endpoints.
package prober

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// ProbeResult holds individual endpoint reachability telemetry
type ProbeResult struct {
	Label     string        // e.g. "External (IPv4)", "Internal IP", "Gateway"
	Target    string        // e.g. "127.0.0.1:8080"
	Status    string        // "OPEN", "CLOSED", "TIMEOUT"
	Duration  time.Duration // Round-trip connection time
	Success   bool          // True if connection succeeded
	Seq       int           // Probe cycle sequence number
	Timestamp time.Time     // Timestamp of probe completion
}

// ProbeTCP performs a single TCP dial against target within specified timeout.
func ProbeTCP(ctx context.Context, label, target string, timeout time.Duration) ProbeResult {
	start := time.Now()
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	dur := time.Since(start)

	if err == nil {
		_ = conn.Close()
		return ProbeResult{
			Label:    label,
			Target:   target,
			Status:   "OPEN",
			Duration: dur,
			Success:  true,
		}
	}

	status := "CLOSED"
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		status = "TIMEOUT"
	}

	return ProbeResult{
		Label:    label,
		Target:   target,
		Status:   status,
		Duration: dur,
		Success:  false,
	}
}

// TargetTask defines a probe target endpoint
type TargetTask struct {
	Label  string
	Target string
}

// ExtractProbeTargets parses Docker serialized port mappings and network interface definitions.
func ExtractProbeTargets(portsVal, networkStr string) []TargetTask {
	var tasks []TargetTask
	seen := make(map[string]bool)

	// 1. External Host Ports
	if strings.TrimSpace(portsVal) != "" {
		for _, line := range strings.Split(portsVal, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.Contains(line, "->") {
				parts := strings.Split(line, "->")
				hostPart := strings.TrimSpace(parts[0])
				lastColon := strings.LastIndex(hostPart, ":")
				if lastColon != -1 {
					hostIP := hostPart[:lastColon]
					port := hostPart[lastColon+1:]
					label := "External (IPv4)"
					if hostIP == "::" || hostIP == "::1" || hostIP == "[::]" {
						hostIP = "::1"
						label = "External (IPv6)"
					} else {
						hostIP = "127.0.0.1"
					}
					target := net.JoinHostPort(hostIP, port)
					if !seen[target] {
						seen[target] = true
						tasks = append(tasks, TargetTask{Label: label, Target: target})
					}
				}
			}
		}
	}

	// 2. Container Internal & Gateway Endpoints
	if strings.TrimSpace(networkStr) != "" {
		for _, entry := range strings.Split(networkStr, ";;") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			parts := strings.Split(entry, ":::")
			if len(parts) > 1 && parts[1] != "" {
				ip := parts[1]
				target := net.JoinHostPort(ip, "80")
				if !seen[target] {
					seen[target] = true
					tasks = append(tasks, TargetTask{Label: fmt.Sprintf("%s (IP)", parts[0]), Target: target})
				}
			}
			if len(parts) > 2 && parts[2] != "" {
				gw := parts[2]
				target := net.JoinHostPort(gw, "53")
				if !seen[target] {
					seen[target] = true
					tasks = append(tasks, TargetTask{Label: fmt.Sprintf("%s (Gateway)", parts[0]), Target: target})
				}
			}
		}
	}

	return tasks
}
