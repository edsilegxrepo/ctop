package single

import (
	"context"
	"fmt"
	"image"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/prober"
	ui "github.com/gizak/termui/v3"
)

// NetworkInfo represents interface configuration for a network
type NetworkInfo struct {
	Name    string
	IP      string
	Gateway string
	MAC     string
	Subnet  string
}

// ProbeResult alias to prober.ProbeResult
type ProbeResult = prober.ProbeResult

// Network widget displays container network adapters, addresses, port mappings, and live TCP probes.
type Network struct {
	ui.Block
	Networks []NetworkInfo
	Ports    string
	IPs      string
	Probes   []ProbeResult
	ProbeSeq int
	mu       sync.Mutex
	probing  bool
	cancel   context.CancelFunc
}

// NewNetwork constructs a new Network inspection widget.
func NewNetwork() *Network {
	nw := &Network{
		Block:    *ui.NewBlock(),
		Networks: []NetworkInfo{},
		Probes:   []ProbeResult{},
	}
	nw.Title = "NETWORKING & PORTS [p: re-probe]"
	nw.BorderStyle = theme.Style("border.fg")
	nw.TitleStyle = theme.Style("label.fg")
	nw.SetRect(0, 0, colWidth[0], 6)
	return nw
}

// StopProbes aborts any ongoing network probe goroutines
func (w *Network) StopProbes() {
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.probing = false
	w.mu.Unlock()
}

// Set parses the serialized networks and port information
func (w *Network) Set(networkStr, portsStr, ipsStr string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Ports = portsStr
	w.IPs = ipsStr
	w.Networks = []NetworkInfo{}

	if strings.TrimSpace(networkStr) != "" {
		entries := strings.Split(networkStr, ";;")
		for _, entry := range entries {
			if strings.TrimSpace(entry) == "" {
				continue
			}
			parts := strings.Split(entry, ":::")
			n := NetworkInfo{
				Name: parts[0],
			}
			if len(parts) > 1 {
				n.IP = parts[1]
			}
			if len(parts) > 2 {
				n.Gateway = parts[2]
			}
			if len(parts) > 3 {
				n.MAC = parts[3]
			}
			if len(parts) > 4 {
				n.Subnet = parts[4]
			}
			w.Networks = append(w.Networks, n)
		}
	}
}

// RunProbes asynchronously executes TCP reachability tests against unique external mapped ports and internal container endpoints in parallel
func (w *Network) RunProbes() {
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.probing = true

	portsVal := w.Ports
	networksVal := make([]NetworkInfo, len(w.Networks))
	copy(networksVal, w.Networks)
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			w.probing = false
			w.mu.Unlock()
		}()
		type probeTask struct {
			label  string
			target string
			isGw   bool
		}

		var tasks []probeTask
		seenTargets := make(map[string]bool)

		// 1. Unique External Host Ports (from portsVal)
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
						var label string
						if hostIP == "::" || hostIP == "::1" || hostIP == "[::]" {
							if prober.SupportsIPv6() {
								hostIP = "::1"
								label = "External (IPv6)"
							} else {
								hostIP = "127.0.0.1"
								label = "External (IPv4 Fallback)"
							}
						} else {
							hostIP = "127.0.0.1"
							label = "External (IPv4)"
						}
						target := net.JoinHostPort(hostIP, port)
						if !seenTargets[target] {
							seenTargets[target] = true
							tasks = append(tasks, probeTask{label: label, target: target, isGw: false})
						}
					}
				}
			}
		}

		// 2. Unique Container Internal Ports
		uniqueContPorts := make(map[string]bool)
		if strings.TrimSpace(portsVal) != "" {
			for _, line := range strings.Split(portsVal, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if strings.Contains(line, "->") {
					parts := strings.Split(line, "->")
					contPart := strings.TrimSpace(parts[1])
					port := strings.Split(contPart, "/")[0]
					if port != "" {
						uniqueContPorts[port] = true
					}
				} else if strings.Contains(line, "/") {
					port := strings.Split(line, "/")[0]
					if port != "" {
						uniqueContPorts[port] = true
					}
				}
			}
		}

		// Add internal targets per network interface
		for _, n := range networksVal {
			if n.IP != "" {
				for port := range uniqueContPorts {
					target := net.JoinHostPort(n.IP, port)
					if !seenTargets[target] {
						seenTargets[target] = true
						tasks = append(tasks, probeTask{
							label:  fmt.Sprintf("Internal (%s)", n.Name),
							target: target,
							isGw:   false,
						})
					}
				}

				// 3. Bridge Gateway
				if n.Gateway != "" {
					if !seenTargets[n.Gateway] {
						seenTargets[n.Gateway] = true
						tasks = append(tasks, probeTask{
							label:  fmt.Sprintf("Gateway (%s)", n.Name),
							target: n.Gateway,
							isGw:   true,
						})
					}
				}
			}
		}

		// Run all probes concurrently
		var wg sync.WaitGroup
		resultsMu := sync.Mutex{}
		results := make([]ProbeResult, len(tasks))

		for idx, task := range tasks {
			wg.Add(1)
			go func(i int, t probeTask) {
				defer wg.Done()
				if t.isGw {
					// Gateway probe
					gwAddr := net.JoinHostPort(t.target, "53")
					res := prober.ProbeTCP(ctx, t.label, gwAddr, 250*time.Millisecond)
					status := "REACHABLE"
					success := true
					if !res.Success {
						status = "CONFIGURED"
					}
					resultsMu.Lock()
					results[i] = ProbeResult{
						Label:    t.label,
						Target:   t.target,
						Status:   status,
						Duration: res.Duration,
						Success:  success,
					}
					resultsMu.Unlock()
				} else {
					res := prober.ProbeTCP(ctx, t.label, t.target, 350*time.Millisecond)
					status := res.Status
					if res.Success && strings.HasPrefix(t.label, "Internal") {
						status = "REACHABLE"
					}
					resultsMu.Lock()
					results[i] = ProbeResult{
						Label:    t.label,
						Target:   t.target,
						Status:   status,
						Duration: res.Duration,
						Success:  res.Success,
					}
					resultsMu.Unlock()
				}
			}(idx, task)
		}

		wg.Wait()

		select {
		case <-ctx.Done():
			return
		default:
		}

		w.mu.Lock()
		w.ProbeSeq++
		seq := w.ProbeSeq
		now := time.Now()
		for i := range results {
			results[i].Seq = seq
			results[i].Timestamp = now
		}
		w.Probes = results
		w.mu.Unlock()
	}()
}

// GetHeight calculates required widget height
func (w *Network) GetHeight() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := 4 // title + borders
	if len(w.Networks) > 0 {
		h += len(w.Networks) + 2
	} else if w.IPs != "" {
		h += len(strings.Split(w.IPs, "\n")) + 2
	} else {
		h += 2
	}

	if w.Ports != "" {
		h += len(strings.Split(w.Ports, "\n")) + 2
	} else {
		h += 2
	}

	numProbes := len(w.Probes)
	if numProbes > 0 {
		h += numProbes + 2
	} else {
		h += 3
	}
	return h
}

// Draw renders formatted network adapters, port tables, and probe results
func (w *Network) Draw(buf *ui.Buffer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Block.Draw(buf)

	headerStyle := theme.Style("label.fg")
	keyStyle := theme.Style("label.fg")
	valStyle := theme.Style("par.text.fg")
	subHeaderStyle := theme.Style("status.warn")

	y := w.Inner.Min.Y

	// Section 1: Attached Networks
	buf.SetString("[ Attached Networks ]", subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
	y++

	if len(w.Networks) > 0 {
		header := fmt.Sprintf("%-18s %-18s %-18s %s", "NETWORK", "IP ADDRESS", "GATEWAY", "MAC ADDRESS")
		buf.SetString(header, headerStyle, image.Pt(w.Inner.Min.X+1, y))
		y++

		for _, n := range w.Networks {
			if y >= w.Inner.Max.Y {
				break
			}
			buf.SetString(fmt.Sprintf("%-18s", n.Name), keyStyle, image.Pt(w.Inner.Min.X+1, y))
			buf.SetString(fmt.Sprintf("%-18s", n.IP), valStyle, image.Pt(w.Inner.Min.X+20, y))
			buf.SetString(fmt.Sprintf("%-18s", n.Gateway), valStyle, image.Pt(w.Inner.Min.X+39, y))
			buf.SetString(n.MAC, valStyle, image.Pt(w.Inner.Min.X+58, y))
			y++
		}
	} else if strings.TrimSpace(w.IPs) != "" {
		lines := strings.Split(w.IPs, "\n")
		for _, l := range lines {
			if y >= w.Inner.Max.Y {
				break
			}
			buf.SetString(l, valStyle, image.Pt(w.Inner.Min.X+2, y))
			y++
		}
	} else {
		buf.SetString("No network interfaces attached (e.g. host mode or isolated).", valStyle, image.Pt(w.Inner.Min.X+2, y))
		y++
	}

	y++ // gap

	// Section 2: Port Bindings & Forwarding
	if y < w.Inner.Max.Y {
		buf.SetString("[ Port Mappings & Forwarding ]", subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
		y++

		if strings.TrimSpace(w.Ports) != "" {
			ports := strings.Split(w.Ports, "\n")
			for _, p := range ports {
				if y >= w.Inner.Max.Y {
					break
				}
				buf.SetString("• "+p, valStyle, image.Pt(w.Inner.Min.X+2, y))
				y++
			}
		} else {
			buf.SetString("No ports exposed or published to host.", valStyle, image.Pt(w.Inner.Min.X+2, y))
			y++
		}
	}

	y++ // gap

	// Section 3: Live Connectivity & Port Probes
	if y < w.Inner.Max.Y {
		interval := config.GetVal("probeInterval")
		if interval == "" {
			interval = "5s"
		}
		title := fmt.Sprintf("[ Live TCP Port & Connectivity Probes ] (Auto-probes every %s, press 'p' to re-probe)", interval)
		buf.SetString(title, subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
		y++

		probes := make([]ProbeResult, len(w.Probes))
		copy(probes, w.Probes)
		isProbing := w.probing

		if isProbing && len(probes) == 0 {
			buf.SetString("  Probing network endpoints...", theme.Style("label.fg"), image.Pt(w.Inner.Min.X+2, y))
		} else if len(probes) > 0 {
			for _, pr := range probes {
				if y >= w.Inner.Max.Y {
					break
				}
				statusColor := theme.Style("status.ok")
				badge := fmt.Sprintf("[✓ %s (%.1fms)]", pr.Status, float64(pr.Duration.Microseconds())/1000.0)
				if !pr.Success {
					statusColor = theme.Style("status.danger")
					badge = fmt.Sprintf("[✗ %s]", pr.Status)
				}
				dateStr := pr.Timestamp.Format("2006-01-02 15:04:05")
				if w.Inner.Dx() < 110 {
					dateStr = pr.Timestamp.Format("15:04:05")
				}
				meta := ""
				if pr.Seq > 0 {
					meta = fmt.Sprintf("#%d %s", pr.Seq, dateStr)
				}
				buf.SetString(fmt.Sprintf("  • %-20s: %-21s ──► ", pr.Label, pr.Target), valStyle, image.Pt(w.Inner.Min.X+2, y))
				buf.SetString(fmt.Sprintf("%-24s", badge), statusColor, image.Pt(w.Inner.Min.X+50, y))
				if meta != "" {
					buf.SetString(meta, theme.Style("label.fg"), image.Pt(w.Inner.Min.X+76, y))
				}
				y++
			}
		} else {
			buf.SetString("  No active endpoints to probe (press 'p' to test).", valStyle, image.Pt(w.Inner.Min.X+2, y))
		}
	}
}
