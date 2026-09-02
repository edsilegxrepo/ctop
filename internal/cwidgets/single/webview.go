// Package single implements the full-screen, multi-tab container inspection view.
//
// Objective:
//
//	Provide an interactive, in-terminal web service inspector widget for exploring live HTTP/HTTPS
//	endpoints on monitored containers, rendering rich ANSI HTML, deterministic headers, and raw payloads.
//
// Core Components:
//   - WebView: Thread-safe TermUI widget managing endpoint rotation, subpath routing, and viewport scrolling.
//   - ViewMode: Mode enum supporting [1] Rendered DOM, [2] Sorted HTTP Headers, and [3] Raw Response Body.
//   - probeSeq: Monotonic async sequence counter protecting against out-of-order redraw race conditions.
//
// Functionality:
//   - Dynamic port cycling ([n] next port, [p] previous port, [g] go to target port/subpath).
//   - State preservation across 1-second background container metric ticks.
//   - Deterministic alphabetical header sorting eliminating redraw visual rotation.
//
// Data Flow:
//
//	Container Meta -> DiscoverEndpoints() -> Background ProbeHTTP() -> htmlrender.RenderHTML() -> TermUI Buffer.
package single

import (
	"context"
	"fmt"
	"image"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/htmlrender"
	"github.com/edsilegx/ctop/pkg/serviceprobe"
	ui "github.com/gizak/termui/v3"
	"github.com/mattn/go-runewidth"
)

// ViewMode defines the active inspection rendering mode.
type ViewMode int

const (
	ModeRendered ViewMode = 0
	ModeHeaders  ViewMode = 1
	ModeRaw      ViewMode = 2
)

// WebView is an embedded terminal web and HTML inspector widget.
type WebView struct {
	ui.Block
	mu            sync.RWMutex
	containerID   string
	containerName string
	endpoints     []serviceprobe.Endpoint
	selectedEp    int
	customPath    string
	mode          ViewMode
	probeRes      *serviceprobe.HTTPProbeResult
	renderedDoc   *htmlrender.Document
	scrollOffset  int
	isLoading     bool
	probeSeq      int64
	lastProbedAt  time.Time
}

// NewWebView constructs a new WebView widget.
func NewWebView() *WebView {
	w := &WebView{
		Block:      *ui.NewBlock(),
		mode:       ModeRendered,
		customPath: "/",
	}
	w.Border = true
	w.Title = " Embedded Container Web & HTML Inspector "
	return w
}

// SetContainer loads endpoints and triggers initial probe for the container if changed.
func (w *WebView) SetContainer(id, name, portsStr, netStr string, envs []string) {
	w.mu.Lock()
	sameContainer := (w.containerID == id && w.containerID != "")
	w.containerID = id
	w.containerName = name
	newEndpoints := serviceprobe.DiscoverEndpoints(portsStr, netStr, envs)

	if sameContainer {
		// Retain existing selected port and state across periodic metric updates
		currentPort := -1
		if w.selectedEp >= 0 && w.selectedEp < len(w.endpoints) {
			currentPort = w.endpoints[w.selectedEp].Port
		}
		w.endpoints = newEndpoints
		if currentPort > 0 {
			for i, ep := range w.endpoints {
				if ep.Port == currentPort {
					w.selectedEp = i
					break
				}
			}
		}
		if w.selectedEp >= len(w.endpoints) {
			w.selectedEp = 0
		}
		w.mu.Unlock()
		return
	}

	// New container initialization
	w.endpoints = newEndpoints
	w.selectedEp = 0
	w.scrollOffset = 0
	w.customPath = "/"
	w.probeRes = nil
	w.renderedDoc = nil
	w.mu.Unlock()

	w.FetchCurrent()
}

// CurrentEndpoint returns the currently selected endpoint.
func (w *WebView) CurrentEndpoint() *serviceprobe.Endpoint {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(w.endpoints) == 0 || w.selectedEp < 0 || w.selectedEp >= len(w.endpoints) {
		return nil
	}
	ep := w.endpoints[w.selectedEp]
	return &ep
}

// EndpointsCount returns the total number of discovered endpoints.
func (w *WebView) EndpointsCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.endpoints)
}

// targetURLLocked builds the full target URL without acquiring locks.
func (w *WebView) targetURLLocked() string {
	if len(w.endpoints) == 0 || w.selectedEp < 0 || w.selectedEp >= len(w.endpoints) {
		return ""
	}

	baseURL := w.endpoints[w.selectedEp].URL
	subpath := strings.TrimSpace(w.customPath)
	if subpath == "" {
		subpath = "/"
	}
	if !strings.HasPrefix(subpath, "/") {
		subpath = "/" + subpath
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	u.Path = subpath
	return u.String()
}

// TargetURL builds the full target URL for the active endpoint and custom subpath.
func (w *WebView) TargetURL() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.targetURLLocked()
}

// FetchCurrent initiates a background probe to the current target URL.
func (w *WebView) FetchCurrent() {
	targetURL := w.TargetURL()
	if targetURL == "" {
		return
	}

	w.mu.Lock()
	w.probeSeq++
	seq := w.probeSeq
	w.isLoading = true
	w.mu.Unlock()

	go func(target string, currentSeq int64) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		res := serviceprobe.ProbeHTTP(ctx, target, 3*time.Second)

		doc := htmlrender.RenderHTML(res.Body, htmlrender.RenderOptions{
			MaxWidth:      80,
			ShowFootnotes: true,
			Colorize:      true,
		})

		w.mu.Lock()
		if w.probeSeq == currentSeq {
			w.probeRes = res
			w.renderedDoc = doc
			w.isLoading = false
			w.lastProbedAt = time.Now()
		}
		w.mu.Unlock()
	}(targetURL, seq)
}

// NextEndpoint selects the next discovered endpoint.
func (w *WebView) NextEndpoint() {
	w.mu.Lock()
	if len(w.endpoints) > 1 {
		w.selectedEp = (w.selectedEp + 1) % len(w.endpoints)
		w.scrollOffset = 0
	}
	w.mu.Unlock()
	w.FetchCurrent()
}

// PrevEndpoint selects the previous discovered endpoint.
func (w *WebView) PrevEndpoint() {
	w.mu.Lock()
	if len(w.endpoints) > 1 {
		w.selectedEp = (w.selectedEp - 1 + len(w.endpoints)) % len(w.endpoints)
		w.scrollOffset = 0
	}
	w.mu.Unlock()
	w.FetchCurrent()
}

// SetCustomPath sets a custom path, port, or URL and triggers reload.
func (w *WebView) SetCustomPath(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		input = "/"
	}

	w.mu.Lock()

	// 1. Check if input is a full URL (e.g. http://127.0.0.1:8080/dashboard/)
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		if u, err := url.Parse(input); err == nil {
			host := u.Hostname()
			port := 80
			if u.Scheme == "https" {
				port = 443
			}
			if u.Port() != "" {
				if p, err := strconv.Atoi(u.Port()); err == nil {
					port = p
				}
			}
			subpath := u.RequestURI()
			if subpath == "" {
				subpath = "/"
			}

			found := -1
			for i, ep := range w.endpoints {
				if ep.HostPort == port || ep.Port == port {
					found = i
					break
				}
			}
			if found >= 0 {
				w.selectedEp = found
			} else {
				w.endpoints = append(w.endpoints, serviceprobe.Endpoint{
					Port:        port,
					HostIP:      host,
					HostPort:    port,
					Protocol:    u.Scheme,
					URL:         fmt.Sprintf("%s://%s:%d/", u.Scheme, host, port),
					Description: fmt.Sprintf("Custom Port %d", port),
					IsExposed:   true,
				})
				w.selectedEp = len(w.endpoints) - 1
			}
			w.customPath = subpath
			w.scrollOffset = 0
			w.mu.Unlock()
			w.FetchCurrent()
			return
		}
	}

	// 2. Check if input specifies a port (e.g. ":8080", "8080", ":8080/dashboard", "8080/ping")
	rawInput := strings.TrimPrefix(input, ":")

	parts := strings.SplitN(rawInput, "/", 2)
	if portNum, err := strconv.Atoi(parts[0]); err == nil && portNum > 0 && portNum <= 65535 {
		subpath := "/"
		if len(parts) == 2 && parts[1] != "" {
			subpath = "/" + parts[1]
		}

		found := -1
		for i, ep := range w.endpoints {
			if ep.Port == portNum || ep.HostPort == portNum {
				found = i
				break
			}
		}

		if found >= 0 {
			w.selectedEp = found
		} else {
			defaultHost := "127.0.0.1"
			if len(w.endpoints) > 0 {
				defaultHost = w.endpoints[0].HostIP
			}
			proto := "http"
			if portNum == 443 || portNum == 8443 {
				proto = "https"
			}
			w.endpoints = append(w.endpoints, serviceprobe.Endpoint{
				Port:        portNum,
				HostIP:      defaultHost,
				HostPort:    portNum,
				Protocol:    proto,
				URL:         fmt.Sprintf("%s://%s:%d/", proto, defaultHost, portNum),
				Description: fmt.Sprintf("Port %d", portNum),
				IsExposed:   true,
			})
			w.selectedEp = len(w.endpoints) - 1
		}
		w.customPath = subpath
		w.scrollOffset = 0
		w.mu.Unlock()
		w.FetchCurrent()
		return
	}

	// 3. Standard subpath (e.g. "/ping", "/dashboard/")
	if !strings.HasPrefix(input, "/") {
		input = "/" + input
	}
	w.customPath = input
	w.scrollOffset = 0
	w.mu.Unlock()
	w.FetchCurrent()
}

// SetMode switches between Rendered, Headers, and Raw modes.
func (w *WebView) SetMode(m ViewMode) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.mode = m
	w.scrollOffset = 0
}

// ToggleMode cycles through the 3 view modes.
func (w *WebView) ToggleMode() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.mode = (w.mode + 1) % 3
	w.scrollOffset = 0
}

// ScrollUp scrolls the active view up.
func (w *WebView) ScrollUp(lines int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.scrollOffset -= lines
	if w.scrollOffset < 0 {
		w.scrollOffset = 0
	}
}

// ScrollDown scrolls the active view down.
func (w *WebView) ScrollDown(lines int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.scrollOffset += lines
}

// Draw renders the full-screen terminal HTML/HTTP view.
func (w *WebView) Draw(buf *ui.Buffer) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.Block.Draw(buf)
	minX := w.Inner.Min.X
	maxX := w.Inner.Max.X
	minY := w.Inner.Min.Y
	maxY := w.Inner.Max.Y

	if maxX <= minX || maxY <= minY {
		return
	}

	contentWidth := maxX - minX

	// 1. Status Bar Header
	statusLineY := minY
	targetURL := w.targetURLLocked()

	statusStyle := ui.NewStyle(theme.Color("status.ok"), theme.Color("bg"), ui.ModifierBold)
	infoStyle := ui.NewStyle(theme.Color("fg"), theme.Color("bg"))
	tabActiveStyle := ui.NewStyle(theme.Color("status.warn"), theme.Color("bg"), ui.ModifierBold)
	tabInactiveStyle := ui.NewStyle(theme.Color("fg.dim"), theme.Color("bg"))

	// Format top info bar
	statusText := "Connecting..."
	latencyText := ""
	if w.isLoading {
		statusText = "Loading..."
	} else if w.probeRes != nil {
		if w.probeRes.Error != "" {
			statusText = "Connection Failed"
			statusStyle = ui.NewStyle(theme.Color("status.danger"), theme.Color("bg"), ui.ModifierBold)
		} else {
			statusText = fmt.Sprintf("%d %s", w.probeRes.StatusCode, w.probeRes.StatusText)
			latencyText = fmt.Sprintf("⏱️ %.1fms", w.probeRes.DurationMS)
			if w.probeRes.StatusCode >= 400 {
				statusStyle = ui.NewStyle(theme.Color("status.warn"), theme.Color("bg"), ui.ModifierBold)
			}
		}
	}

	var portTag string
	if len(w.endpoints) > 1 {
		portTag = fmt.Sprintf("[Port %d/%d] ", w.selectedEp+1, len(w.endpoints))
	}

	urlStr := fmt.Sprintf("URL: %s  %s", targetURL, portTag)
	buf.SetString(urlStr, infoStyle, image.Pt(minX, statusLineY))
	badgeX := minX + runewidth.StringWidth(urlStr)
	badgeStr := fmt.Sprintf("[%s]", statusText)
	buf.SetString(badgeStr, statusStyle, image.Pt(badgeX, statusLineY))
	if latencyText != "" {
		latX := badgeX + runewidth.StringWidth(badgeStr) + 1
		buf.SetString(latencyText, infoStyle, image.Pt(latX, statusLineY))
	}

	// Format tabs indicator on top right
	modeLabels := []string{"[1] Rendered", "[2] Headers", "[3] Raw"}
	tabX := maxX - 35
	if tabX > badgeX+20 {
		for i, label := range modeLabels {
			st := tabInactiveStyle
			if ViewMode(i) == w.mode {
				st = tabActiveStyle
			}
			buf.SetString(label, st, image.Pt(tabX, statusLineY))
			tabX += len(label) + 1
		}
	}

	// Divider line
	divY := minY + 1
	for x := minX; x < maxX; x++ {
		buf.SetCell(ui.NewCell('─', ui.NewStyle(theme.Color("border"))), image.Pt(x, divY))
	}

	// 2. Main Content Area
	bodyMinY := minY + 2
	bodyMaxY := maxY - 2
	availHeight := bodyMaxY - bodyMinY

	if availHeight <= 0 {
		return
	}

	var contentLines []string

	if len(w.endpoints) == 0 {
		contentLines = []string{
			"No HTTP/web ports discovered on this container.",
			"Press 'g' to enter a custom URL or port (e.g. http://127.0.0.1:8080/).",
		}
	} else if w.probeRes == nil && w.isLoading {
		contentLines = []string{"Fetching container web endpoint payload..."}
	} else if w.probeRes != nil && w.probeRes.Error != "" {
		contentLines = []string{
			fmt.Sprintf("Connection Failed: %s", w.probeRes.Error),
			"",
			"Troubleshooting suggestions:",
			"  • Verify the container process is actively listening on this port.",
			"  • Press 'g' to change the URL path or port.",
			"  • Press 'n' to cycle to other container network endpoints.",
			"  • Press 'r' to retry connection.",
		}
	} else if w.probeRes != nil {
		switch w.mode {
		case ModeRendered:
			if w.renderedDoc != nil {
				contentLines = w.renderedDoc.Lines
			}
		case ModeHeaders:
			contentLines = append(contentLines, fmt.Sprintf("Protocol: %s", w.probeRes.Proto))
			contentLines = append(contentLines, fmt.Sprintf("Status:   %d %s", w.probeRes.StatusCode, w.probeRes.StatusText))
			contentLines = append(contentLines, fmt.Sprintf("Latency:  %.2f ms", w.probeRes.DurationMS))
			contentLines = append(contentLines, fmt.Sprintf("Size:     %d bytes", w.probeRes.BodySize))
			contentLines = append(contentLines, "")
			contentLines = append(contentLines, "--- Response Headers ---")
			var headerKeys []string
			for k := range w.probeRes.Headers {
				headerKeys = append(headerKeys, k)
			}
			sort.Strings(headerKeys)
			for _, k := range headerKeys {
				contentLines = append(contentLines, fmt.Sprintf("%-24s: %s", k, w.probeRes.Headers[k]))
			}
		case ModeRaw:
			rawLines := strings.Split(w.probeRes.Body, "\n")
			for i, line := range rawLines {
				contentLines = append(contentLines, fmt.Sprintf("%4d │ %s", i+1, line))
			}
		}
	}

	// Clamp scroll offset
	maxScroll := len(contentLines) - availHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if w.scrollOffset > maxScroll {
		w.scrollOffset = maxScroll
	}

	// Render visible slice
	for i := 0; i < availHeight; i++ {
		lineIdx := w.scrollOffset + i
		y := bodyMinY + i
		if lineIdx < len(contentLines) {
			rawLine := contentLines[lineIdx]
			cleanLine := rawLine
			if runewidth.StringWidth(cleanLine) > contentWidth {
				cleanLine = runewidth.Truncate(cleanLine, contentWidth, "…")
			}
			lineStyle := infoStyle
			if strings.HasPrefix(cleanLine, "Connection Failed:") {
				lineStyle = ui.NewStyle(theme.Color("status.danger"), theme.Color("bg"), ui.ModifierBold)
			}
			buf.SetString(cleanLine, lineStyle, image.Pt(minX, y))
		}
	}

	// 3. Bottom Hotkey Navigation Bar
	bottomY := maxY - 1
	for x := minX; x < maxX; x++ {
		buf.SetCell(ui.NewCell('─', ui.NewStyle(theme.Color("border"))), image.Pt(x, bottomY-1))
	}

	navStr := "[g] Goto Path  [r] Reload  [n] Cycle Port  [Tab] View Mode  [PageUp/Dn] Scroll  [q] Back"
	buf.SetString(navStr, ui.NewStyle(theme.Color("status.ok"), theme.Color("bg"), ui.ModifierBold), image.Pt(minX, bottomY))
}
