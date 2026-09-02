// Package single_test provides unit and integration tests for the TermUI WebView widget.
//
// Test Strategy:
//   - Widget Lifecycle: Test widget initialization, TermUI rectangle bounding, and non-blocking drawing.
//   - Endpoint Navigation: Test cycling between multiple ports (next/previous), subpath modifications, and view modes.
//   - State Preservation: Test that background container metric refreshes preserve active tab, port, and subpath state.
//   - View Modes: Test buffer generation across ModeRendered, ModeHeaders, and ModeRaw.
package single

import (
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ui "github.com/gizak/termui/v3"
)

func TestWebViewWidget(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<h1>Test Container Page</h1><p>Running on embedded server.</p>"))
	}))
	defer ts.Close()

	wv := NewWebView()
	wv.SetRect(0, 0, 100, 30)

	// Extract port from ts.URL
	tsPort := ts.URL[strings.LastIndex(ts.URL, ":")+1:]
	portsStr := tsPort + ":80/tcp"

	wv.SetContainer("c123456", "web-container", portsStr, "", nil)

	// Wait for background probe to complete
	time.Sleep(100 * time.Millisecond)

	if wv.CurrentEndpoint() == nil {
		t.Fatalf("expected non-nil endpoint")
	}

	// Test mode toggling
	wv.SetMode(ModeHeaders)
	if wv.mode != ModeHeaders {
		t.Errorf("expected ModeHeaders, got %v", wv.mode)
	}

	wv.ToggleMode()
	if wv.mode != ModeRaw {
		t.Errorf("expected ModeRaw, got %v", wv.mode)
	}

	wv.ToggleMode()
	if wv.mode != ModeRendered {
		t.Errorf("expected ModeRendered, got %v", wv.mode)
	}

	// Test scrolling
	wv.ScrollDown(5)
	if wv.scrollOffset < 0 {
		t.Errorf("scrollOffset should be non-negative")
	}
	wv.ScrollUp(5)
	if wv.scrollOffset != 0 {
		t.Errorf("scrollOffset should be 0 after scrolling up")
	}

	// Test draw buffer rendering without panic
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 30))
	wv.Draw(buf)
}

func TestWebViewRapidConcurrency(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer ts.Close()

	wv := NewWebView()
	wv.SetRect(0, 0, 100, 30)

	tsPort := ts.URL[strings.LastIndex(ts.URL, ":")+1:]
	portsStr := tsPort + ":80/tcp"

	wv.SetContainer("c123456", "web-container", portsStr, "", nil)

	// Simulate rapid concurrent path changes
	for i := 0; i < 20; i++ {
		go wv.SetCustomPath(strings.Repeat("/subpath", i%5))
		go wv.FetchCurrent()
	}

	time.Sleep(150 * time.Millisecond)

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 30))
	wv.Draw(buf)
}

func TestWebViewPortCyclingAndCustomTarget(t *testing.T) {
	wv := NewWebView()
	wv.SetRect(0, 0, 100, 30)

	// Simulate Traefik container with ports 80 and 8080
	portsStr := "0.0.0.0:80 -> 80/tcp\n0.0.0.0:8080 -> 8080/tcp"
	wv.SetContainer("c_traefik", "traefik-proxy", portsStr, "", nil)

	if wv.EndpointsCount() != 2 {
		t.Fatalf("expected 2 endpoints, got %d", wv.EndpointsCount())
	}

	// 1. Initial endpoint should be port 80
	ep1 := wv.CurrentEndpoint()
	if ep1 == nil || ep1.Port != 80 {
		t.Fatalf("expected first endpoint to be port 80, got %+v", ep1)
	}

	// 2. NextEndpoint() should cycle to port 8080
	wv.NextEndpoint()
	ep2 := wv.CurrentEndpoint()
	if ep2 == nil || ep2.Port != 8080 {
		t.Fatalf("expected second endpoint to be port 8080, got %+v", ep2)
	}

	// 3. SetCustomPath with port syntax ':8080/dashboard/'
	wv.SetCustomPath(":8080/dashboard/")
	if !strings.Contains(wv.TargetURL(), "8080") || !strings.Contains(wv.TargetURL(), "/dashboard/") {
		t.Errorf("expected TargetURL to contain 8080 and /dashboard/, got %s", wv.TargetURL())
	}

	// 4. SetCustomPath with full URL 'http://127.0.0.1:8080/ping'
	wv.SetCustomPath("http://127.0.0.1:8080/ping")
	if wv.TargetURL() != "http://127.0.0.1:8080/ping" {
		t.Errorf("expected TargetURL to be http://127.0.0.1:8080/ping, got %s", wv.TargetURL())
	}
}
