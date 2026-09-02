// Package serviceprobe provides in-engine HTTP probing, service discovery, and endpoint resolution for containers.
//
// Objective:
//
//	Automatically discover candidate HTTP/HTTPS endpoints from container port mappings, networks,
//	and environment variables, enabling embedded TUI rendering and dashboard service inspection.
//
// Core Components:
//   - Endpoint: Model representing a candidate web service target with host/container ports, protocol, and URL.
//   - DiscoverEndpoints: Orchestrates heuristics across published ports, container IP bridges, and ENV declarations.
//   - parsePortPart: Normalizes Docker port string formats (arrows, colons, slashes, IPv6 brackets).
//
// Functionality:
//   - Multi-format Docker port string parsing (e.g., '0.0.0.0:80 -> 80/tcp', ':::443 -> 443/tcp').
//   - Container IP extraction from internal bridge network metadata when host ports are unmapped.
//   - Standard web service port identification (80, 443, 8080, 8443, 3000, 5000, 9090, 8000).
//
// Data Flow:
//
//	Container Metadata (Ports, Networks, Envs) -> DiscoverEndpoints() -> Ordered []Endpoint -> WebView / REST Proxy.
package serviceprobe

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Endpoint represents an accessible network/web service endpoint on a container.
type Endpoint struct {
	Port        int    `json:"port"`
	HostIP      string `json:"host_ip,omitempty"`
	HostPort    int    `json:"host_port,omitempty"`
	Protocol    string `json:"protocol"` // "http" or "https"
	URL         string `json:"url"`
	Description string `json:"description"`
	IsExposed   bool   `json:"is_exposed"`
}

// DiscoverEndpoints inspects raw port mappings, network strings, and environment variables
// to discover all candidate HTTP/web endpoints for a container.
func DiscoverEndpoints(portsStr, netStr string, envs []string) []Endpoint {
	var endpoints []Endpoint
	seenPorts := make(map[string]bool)

	// 1. Inspect Port Mappings (e.g. "8080:80/tcp", "0.0.0.0:8080 -> 80/tcp", "80/tcp\n8080/tcp")
	rawParts := strings.FieldsFunc(portsStr, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})

	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		hostPort, containerPort, isTCP := parsePortPart(part)
		if !isTCP || containerPort <= 0 {
			continue
		}

		key := fmt.Sprintf("%d:%d", hostPort, containerPort)
		if seenPorts[key] {
			continue
		}
		seenPorts[key] = true

		proto := "http"
		if containerPort == 443 || containerPort == 8443 || containerPort == 9443 || hostPort == 443 || hostPort == 8443 {
			proto = "https"
		}

		targetHost := "127.0.0.1"
		targetPort := hostPort
		isExposed := true

		if targetPort <= 0 {
			// Container port not mapped to host port -> check container IP
			containerIP := extractFirstContainerIP(netStr)
			if containerIP != "" {
				targetHost = containerIP
				targetPort = containerPort
			} else {
				targetHost = "127.0.0.1"
				targetPort = containerPort
				isExposed = false
			}
		}

		url := fmt.Sprintf("%s://%s:%d/", proto, targetHost, targetPort)
		desc := describePort(containerPort)

		endpoints = append(endpoints, Endpoint{
			Port:        containerPort,
			HostIP:      targetHost,
			HostPort:    targetPort,
			Protocol:    proto,
			URL:         url,
			Description: desc,
			IsExposed:   isExposed,
		})
	}

	// 2. Inspect Environment Variables (e.g. PORT=3000, HTTP_PORT=8080)
	for _, env := range envs {
		env = strings.TrimSpace(env)
		if strings.HasPrefix(env, "PORT=") || strings.HasPrefix(env, "HTTP_PORT=") || strings.HasPrefix(env, "APP_PORT=") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				if portNum, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && portNum > 0 && portNum <= 65535 {
					key := fmt.Sprintf("env:%d", portNum)
					if !seenPorts[key] {
						seenPorts[key] = true
						url := fmt.Sprintf("http://127.0.0.1:%d/", portNum)
						endpoints = append(endpoints, Endpoint{
							Port:        portNum,
							HostIP:      "127.0.0.1",
							HostPort:    portNum,
							Protocol:    "http",
							URL:         url,
							Description: fmt.Sprintf("Env %s", parts[0]),
							IsExposed:   false,
						})
					}
				}
			}
		}
	}

	// 3. Fallback: If no ports detected, check container IP on default port 80
	if len(endpoints) == 0 {
		containerIP := extractFirstContainerIP(netStr)
		if containerIP != "" {
			endpoints = append(endpoints, Endpoint{
				Port:        80,
				HostIP:      containerIP,
				HostPort:    80,
				Protocol:    "http",
				URL:         fmt.Sprintf("http://%s:80/", containerIP),
				Description: "Default Container Web Port",
				IsExposed:   false,
			})
		}
	}

	return endpoints
}

// parsePortPart extracts host port, container port, and protocol from port binding strings.
func parsePortPart(part string) (hostPort, containerPort int, isTCP bool) {
	isTCP = true
	if strings.Contains(part, "/udp") {
		isTCP = false
	}
	part = strings.TrimSuffix(strings.TrimSuffix(part, "/tcp"), "/udp")
	part = strings.TrimSpace(part)

	// Formats: "0.0.0.0:8080 -> 80", "0.0.0.0:8080->80", "127.0.0.1:8080:80", "8080:80", "80"
	part = strings.ReplaceAll(part, " -> ", ":")
	part = strings.ReplaceAll(part, "->", ":")

	rawTokens := strings.Split(part, ":")
	var tokens []string
	for _, t := range rawTokens {
		t = strings.TrimSpace(t)
		t = strings.Trim(t, "[]")
		if t != "" {
			tokens = append(tokens, t)
		}
	}

	if len(tokens) == 0 {
		return 0, 0, isTCP
	}

	if len(tokens) == 1 {
		if p, err := strconv.Atoi(tokens[0]); err == nil {
			return 0, p, isTCP
		}
	} else if len(tokens) == 2 {
		// "8080:80"
		h, err1 := strconv.Atoi(tokens[0])
		c, err2 := strconv.Atoi(tokens[1])
		if err1 == nil && err2 == nil {
			return h, c, isTCP
		}
		// "0.0.0.0:80" or "localhost:80"
		if err2 == nil {
			return 0, c, isTCP
		}
		if err1 == nil {
			return 0, h, isTCP
		}
	} else if len(tokens) >= 3 {
		// "0.0.0.0:8080:80" or ":::8080:80"
		h, err1 := strconv.Atoi(tokens[len(tokens)-2])
		c, err2 := strconv.Atoi(tokens[len(tokens)-1])
		if err1 == nil && err2 == nil {
			return h, c, isTCP
		}
		if err2 == nil {
			return 0, c, isTCP
		}
	}
	return 0, 0, isTCP
}

// extractFirstContainerIP extracts the first non-loopback IP from raw network strings.
func extractFirstContainerIP(netStr string) string {
	if netStr == "" {
		return ""
	}
	// Net string format: "bridge:::172.17.0.2:::172.17.0.1:::...;;..."
	networks := strings.Split(netStr, ";;")
	for _, n := range networks {
		fields := strings.Split(n, ":::")
		if len(fields) >= 2 {
			ipStr := strings.TrimSpace(fields[1])
			if ip := net.ParseIP(ipStr); ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				return ipStr
			}
		}
	}
	return ""
}

// describePort returns a friendly description for well-known port numbers.
func describePort(port int) string {
	switch port {
	case 80:
		return "HTTP Web Root"
	case 443:
		return "HTTPS Secure Web"
	case 8080, 8000:
		return "HTTP Application Service"
	case 3000:
		return "Node / Frontend App"
	case 5000:
		return "Flask / Python API"
	case 8008, 8888:
		return "Web Dashboard / Admin"
	case 9090:
		return "Prometheus Metrics / Web"
	case 9000:
		return "FastCGI / SonarQube"
	case 5601:
		return "Kibana Dashboard"
	case 3306, 5432, 6379, 27017:
		return "Database Port"
	default:
		return fmt.Sprintf("TCP Port %d", port)
	}
}
