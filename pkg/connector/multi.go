package connector

import (
	"fmt"
	"strings"
	"sync"

	"github.com/edsilegx/ctop/pkg/container"
)

// MultiConnector aggregates container streams from multiple underlying Connectors (e.g. multiple Docker hosts).
type MultiConnector struct {
	connectors map[string]Connector
	closed     chan struct{}
	closeOnce  sync.Once
	lock       sync.RWMutex
}

// NewMultiConnector creates an empty MultiConnector ready to host multiple connector instances.
func NewMultiConnector() *MultiConnector {
	return &MultiConnector{
		connectors: make(map[string]Connector),
		closed:     make(chan struct{}),
	}
}

// AddConnector registers a named connector (e.g. host name or endpoint) into the aggregator.
func (mc *MultiConnector) AddConnector(hostID string, conn Connector) {
	mc.lock.Lock()
	defer mc.lock.Unlock()
	mc.connectors[hostID] = conn
}

// All returns a combined, sorted list of containers across all monitored hosts.
func (mc *MultiConnector) All() container.Containers {
	mc.lock.RLock()
	defer mc.lock.RUnlock()

	var all container.Containers
	for _, conn := range mc.connectors {
		all = append(all, conn.All()...)
	}
	all.Sort()
	return all
}

// Get finds a container by its ID across all connected hosts.
func (mc *MultiConnector) Get(id string) (*container.Container, bool) {
	mc.lock.RLock()
	defer mc.lock.RUnlock()

	for _, conn := range mc.connectors {
		if c, found := conn.Get(id); found {
			return c, true
		}
	}
	return nil, false
}

// Wait blocks until the multi-connector is closed.
func (mc *MultiConnector) Wait() struct{} {
	return <-mc.closed
}

// Close terminates all underlying connections.
func (mc *MultiConnector) Close() {
	mc.closeOnce.Do(func() {
		close(mc.closed)
	})
}

// ParseHostSpec parses a host connection argument (e.g. "ssh://user@host:2222", "tcp://host:2375", "local")
// and returns the endpoint and clean host identifier label.
func ParseHostSpec(spec string) (endpoint, hostID string) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "local" {
		return ResolveDockerEndpoint(), "local"
	}
	if strings.HasPrefix(spec, "ssh://") {
		// Clean host identifier without port for compact UI display
		hostID = strings.TrimPrefix(spec, "ssh://")
		if idx := strings.LastIndex(hostID, ":"); idx != -1 {
			hostID = hostID[:idx]
		}
		return spec, hostID
	}
	if strings.HasPrefix(spec, "tcp://") || strings.HasPrefix(spec, "http://") || strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "unix://") {
		parts := strings.Split(spec, "://")
		hostID = parts[len(parts)-1]
		return spec, hostID
	}
	return spec, spec
}

// NewMultiDockerConnector initializes multiple Docker host connectors from a list of host specifications.
func NewMultiDockerConnector(hostSpecs ...string) (*MultiConnector, error) {
	if len(hostSpecs) == 0 {
		hostSpecs = []string{"local"}
	}

	mc := NewMultiConnector()
	for _, spec := range hostSpecs {
		endpoint, hostID := ParseHostSpec(spec)
		var (
			conn Connector
			err  error
		)
		if endpoint == "" || spec == "local" {
			conn, err = NewDocker()
		} else {
			conn, err = NewDockerWithEndpoint(endpoint, hostID)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to connect to host %s (%s): %w", hostID, spec, err)
		}
		mc.AddConnector(hostID, conn)
	}
	return mc, nil
}
