// Package serviceprobe_test provides unit and resilience test suites for service discovery and HTTP probing.
//
// Test Strategy:
//   - Endpoint Discovery: Test single-line and multiline Docker port notations, container IP fallback, and ENV heuristics.
//   - HTTP Probing: Test live httptest.Server responses, header parsing, HTML/JSON detection, and latency tracking.
//   - Resilience & Edge Cases: Test raw binary sockets (MySQL/Redis), Slowloris timeout bounds, redirect limits, and oversized payload ceilings.
package serviceprobe

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDiscoverEndpoints(t *testing.T) {
	portsStr := "0.0.0.0:8080->80/tcp, 0.0.0.0:8443->443/tcp, 9090/tcp"
	netStr := "bridge:::172.17.0.5:::172.17.0.1:::02:42:ac:11:00:05:::16"
	envs := []string{"PORT=3000", "PATH=/usr/bin"}

	endpoints := DiscoverEndpoints(portsStr, netStr, envs)
	if len(endpoints) < 3 {
		t.Fatalf("expected at least 3 endpoints, found %d: %+v", len(endpoints), endpoints)
	}

	found80 := false
	found443 := false
	found3000 := false

	for _, ep := range endpoints {
		if ep.Port == 80 {
			found80 = true
			if ep.Protocol != "http" || ep.HostPort != 8080 {
				t.Errorf("unexpected endpoint for port 80: %+v", ep)
			}
		}
		if ep.Port == 443 {
			found443 = true
			if ep.Protocol != "https" || ep.HostPort != 8443 {
				t.Errorf("unexpected endpoint for port 443: %+v", ep)
			}
		}
		if ep.Port == 3000 {
			found3000 = true
		}
	}

	if !found80 || !found443 || !found3000 {
		t.Errorf("missing expected ports (80=%v, 443=%v, 3000=%v)", found80, found443, found3000)
	}
}

func TestDiscoverEndpointsMultiline(t *testing.T) {
	// Real-world Docker formatted multiline ports string (e.g. Traefik)
	portsStr := "0.0.0.0:80 -> 80/tcp\n0.0.0.0:8080 -> 8080/tcp\n:::443 -> 443/tcp"
	endpoints := DiscoverEndpoints(portsStr, "", nil)

	if len(endpoints) != 3 {
		t.Fatalf("expected 3 discovered endpoints for multiline port mappings, got %d: %+v", len(endpoints), endpoints)
	}
	if endpoints[0].Port != 80 || endpoints[1].Port != 8080 || endpoints[2].Port != 443 {
		t.Errorf("unexpected discovered port list: %+v", endpoints)
	}
}

func TestProbeHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Custom-Header", "ctop-test")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "<html><body><h1>Container App</h1></body></html>")
	}))
	defer ts.Close()

	ctx := context.Background()
	res := ProbeHTTP(ctx, ts.URL, 2*time.Second)

	if res.Error != "" {
		t.Fatalf("unexpected probe error: %s", res.Error)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}
	if !res.IsHTML {
		t.Errorf("expected is_html to be true")
	}
	if res.Headers["X-Custom-Header"] != "ctop-test" {
		t.Errorf("expected custom header in response")
	}
	if !strings.Contains(res.Body, "Container App") {
		t.Errorf("expected body to contain 'Container App', got %q", res.Body)
	}
	if res.Duration <= 0 {
		t.Errorf("expected positive duration measurement")
	}
}

func TestProbeHTTPError(t *testing.T) {
	ctx := context.Background()
	// Non-existent loopback port
	res := ProbeHTTP(ctx, "http://127.0.0.1:59999", 500*time.Millisecond)

	if res.Error == "" {
		t.Fatalf("expected error for unreachable target port, got none")
	}
}

func TestProbeHTTPRedirectLimit(t *testing.T) {
	// Circular redirect server
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ts.URL+"/loop", http.StatusFound)
	}))
	defer ts.Close()

	ctx := context.Background()
	res := ProbeHTTP(ctx, ts.URL, 2*time.Second)

	// Bounded redirect policy returns last response without hanging or infinite loop
	if res.StatusCode != http.StatusFound {
		t.Errorf("expected redirect status 302 Found after reaching limit, got %d", res.StatusCode)
	}
}

func TestProbeHTTP_NonHTTPBinaryService(t *testing.T) {
	// Mock a non-HTTP binary service (e.g. MySQL / Redis / PostgreSQL handshake)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer func() { _ = l.Close() }()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			// Send raw non-HTTP binary payload
			_, _ = conn.Write([]byte{0x4a, 0x00, 0x00, 0x00, 0x0a, '5', '.', '7', '.', '3', '4', 0x00, 0xff, 0xfe})
			_ = conn.Close()
		}
	}()

	ctx := context.Background()
	res := ProbeHTTP(ctx, "http://"+l.Addr().String(), 1*time.Second)

	// Must handle non-HTTP binary response gracefully without panic or unhandled error crash
	if res.Error == "" {
		t.Errorf("expected protocol error for non-HTTP binary response, got none (status: %d)", res.StatusCode)
	}
}

func TestProbeHTTP_SlowlorisTimeout(t *testing.T) {
	// Mock a Slowloris-style hanging server
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer func() { _ = l.Close() }()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			// Accept and sleep without responding
			time.Sleep(3 * time.Second)
			_ = conn.Close()
		}
	}()

	ctx := context.Background()
	start := time.Now()
	res := ProbeHTTP(ctx, "http://"+l.Addr().String(), 500*time.Millisecond)
	elapsed := time.Since(start)

	if res.Error == "" {
		t.Errorf("expected timeout error for stalled server, got none")
	}
	if elapsed > 1500*time.Millisecond {
		t.Errorf("probe took %v, expected timeout enforcement within ~500ms", elapsed)
	}
}

func TestProbeHTTP_OversizedBodyLimit(t *testing.T) {
	// Server sending 5MB of repeated data
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		chunk := bytes.Repeat([]byte("A"), 1024*1024) // 1MB
		for i := 0; i < 5; i++ {
			_, _ = w.Write(chunk)
		}
	}))
	defer ts.Close()

	ctx := context.Background()
	res := ProbeHTTP(ctx, ts.URL, 3*time.Second)

	if res.Error != "" {
		t.Fatalf("unexpected probe error: %s", res.Error)
	}
	if len(res.Body) > 2*1024*1024+1024 {
		t.Errorf("expected body length <= 2MB, got %d bytes", len(res.Body))
	}
}
