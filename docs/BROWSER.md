# Zero-Dependency Embedded Container Web & HTML Inspector: Architectural Design

## Executive Summary

`ctop` is an interactive container monitoring and telemetry system built for operational efficiency in developer workstations, servers, CI/CD runners, and edge deployments. Previously, container web inspection relied on external operating system browsers (`browser.OpenURL` / `xdg-open`), which failed completely in headless environments, SSH sessions, containerized deployments, and restricted networks.

This document defines the complete architectural design for replacing external browser dependencies with a **zero-dependency, pure-Go embedded HTTP prober, terminal HTML-to-ANSI rendering engine, and web dashboard proxy inspector**.

---

## 1. Problem Analysis: The Flaws of External Browser Launching

```
               ┌─────────────────────────────────────────────────────────────┐
               │              Current External Browser Flow (Broken)         │
               └──────────────────────────────┬──────────────────────────────┘
                                              │
                    ┌─────────────────────────┴─────────────────────────┐
                    ▼                                                   ▼
       ┌────────────────────────┐                          ┌────────────────────────┐
       │     SSH / Headless     │                          │     Local Browser      │
       ├────────────────────────┤                          ├────────────────────────┤
       │ x  xdg-open: not found │                          │ x  Private IP blocked  │
       │ x  No DISPLAY/Wayland  │                          │ x  Mixed content block │
       │ x  Silent failure      │                          │ x  Zero debug metadata │
       └────────────────────────┘                          └────────────────────────┘
```

1. **Headless & SSH Failure**: Server-side terminals lack X11, Wayland, or desktop managers. Launching external commands emits silent errors or corrupts terminal states.
2. **Network Routing Blindspots**: Containers frequently bind to internal bridge networks (e.g., `172.17.0.2:8080`) or unmapped container ports that cannot be reached directly by the host desktop browser without manual port forwarding.
3. **Mixed-Content & Private Network Barriers**: When `ctop`'s Web Dashboard runs over HTTPS, opening external HTTP container URLs triggers modern browser security restrictions (Mixed Content Blocking and Private Network Access specifications).
4. **Lack of Operational Telemetry**: A desktop browser does not surface Time to First Byte (TTFB), raw HTTP headers, SSL/TLS handshake metadata, or formatted ASCII payloads necessary for debugging services.

---

## 2. Architectural Vision: In-Engine Web & HTML Inspection

The redesigned subsystem operates entirely within `ctop`'s binary boundaries across both interfaces:

```
┌───────────────────────────────────────────────────────────────────────────────────────────────┐
│                               Container HTTP Service Discovery                                │
│                     (Inspects Bindings, Exposed Ports, IPAddresses, and Envs)                 │
└───────────────────────────────────────────────┬───────────────────────────────────────────────┘
                                                │
                     ┌──────────────────────────┴──────────────────────────┐
                     ▼                                                     ▼
     ┌────────────────────────────────┐                    ┌────────────────────────────────┐
     │          Terminal UI           │                    │         Web Dashboard          │
     │     Embedded HTML Inspector    │                    │     Proxy & Sandbox Gateway    │
     ├────────────────────────────────┤                    ├────────────────────────────────┤
     │ • Port Selection Dialog        │                    │ • Server-Side Proxy Gateway    │
     │ • Pure-Go HTML DOM Parser      │                    │ • Sandboxed IFrame Preview     │
     │ • ANSI Colors, Tables & Links  │                    │ • Syntax Highlighted Source    │
     │ • 3-Mode View (DOM/Header/Raw) │                    │ • Live Latency & Probe Cards   │
     │ • Interactive Path Navigation  │                    │ • One-Click Health Endpoints   │
     └────────────────────────────────┘                    └────────────────────────────────┘
```

---

## 3. Terminal UI (TUI) Embedded Web Inspector

### 3.1 Endpoint & Port Auto-Discovery

When an operator triggers the Web Inspector via keybinding (`[w]` for Web View or `[o]` drilldown), `ctop` analyzes the container metadata:

1. **Port Identification**:
   - Inspects `NetworkSettings.Ports` (e.g. `80/tcp`, `443/tcp`, `8080/tcp`, `3000/tcp`, `9090/tcp`).
   - Fallbacks to container environment variables (`PORT`, `HTTP_PORT`, `APP_PORT`).
2. **Discovered Endpoint Selection Dialog**:
   - If multiple HTTP ports are active, a floating selector modal is presented:
     ```text
     ┌────────────────── Select Service Endpoint ──────────────────┐
     │                                                             │
     │  ● [1] 80/tcp   -> 127.0.0.1:8080   (HTTP NGINX Web Root)   │
     │    [2] 9090/tcp -> 127.0.0.1:9090   (Prometheus Metrics)    │
     │    [3] 8081/tcp -> (Internal Only)  (Healthz / Diagnostics) │
     │                                                             │
     │  [Enter] Inspect Endpoint   [g] Custom URL   [Esc] Cancel   │
     └─────────────────────────────────────────────────────────────┘
     ```

### 3.2 Pure-Go Terminal HTML-to-ANSI Rendering Engine (`pkg/htmlrender`)

The HTML renderer parses raw HTML into styled, structured terminal lines without requiring webkit, chromium, or external binaries:

1. **Visual Hierarchy & Headings**:
   - `<h1>` to `<h6>`: Bold styled text with hierarchical ANSI color tokens and horizontal dividers.
2. **Paragraphs & Typography**:
   - `<p>`, `<span>`, `<div>`: Soft word-wrapping to active terminal columns.
   - `<b>`, `<strong>`: Bold text styling.
   - `<i>`, `<em>`: Italicized / inverted styling.
   - `<code>`: Distinct background color syntax highlights.
   - `<pre>`: Monospaced code fence preserving linebreaks and indentation.
   - `<blockquote>`: Left-bordered terminal quotation lines (`│ ` prefix).
3. **Lists**:
   - `<ul>` / `<li>`: Bulleted items (`• item`).
   - `<ol>` / `<li>`: Numbered items (`1. item`).
4. **Tables**:
   - `<table>`, `<tr>`, `<th>`, `<td>`: Aligned box-drawing grid formatters with column auto-sizing and padding.
5. **Hyperlinks & Navigation**:
   - `<a>`: Underlined colored text with OSC 8 terminal clickable hyperlink sequences and a numbered footnote reference table at the bottom of the document (`[1] https://...`).

### 3.3 Three Inspection Modes

The TUI Web Inspector provides three operational tabs toggled via `Tab` or number keys `[1]`, `[2]`, `[3]`:

```text
┌─ web-frontend (c8a412f10a8b) ────────────────────────────── [1] Rendered  [2] Headers  [3] Raw ─┐
│ URL: http://127.0.0.1:8080/          Status: 200 OK          Latency: 3.4ms       Size: 1.2 KB  │
├─────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                 │
│  Welcome to Container Application Service                                                       │
│  ─────────────────────────────────────────────────────────────────────────────                  │
│                                                                                                 │
│  System Status: Operational                                                                     │
│                                                                                                 │
│  Available API Endpoints:                                                                       │
│  ┌──────────────────────┬────────┬───────────────────────────┐                                  │
│  │ Route Path           │ Method │ Description               │                                  │
│  ├──────────────────────┼────────┼───────────────────────────┤                                  │
│  │ /api/v1/health       │ GET    │ Service health check probe│                                  │
│  │ /api/v1/metrics      │ GET    │ Prometheus metric stream  │                                  │
│  │ /api/v1/status       │ GET    │ Cluster node info         │                                  │
│  └──────────────────────┴────────┴───────────────────────────┘                                  │
│                                                                                                 │
│  Footnotes / Links:                                                                             │
│  [1] Documentation: https://docs.example.internal                                               │
│                                                                                                 │
├─────────────────────────────────────────────────────────────────────────────────────────────────┤
│ [g] Goto Path   [r] Reload   [/] Search   [Tab] Switch View   [PageUp/Dn] Scroll   [q] Back     │
└─────────────────────────────────────────────────────────────────────────────────────────────────┘
```

- **Mode 1: Rendered HTML (`[1]`)**: Formatted document layout with ANSI styling and ASCII tables.
- **Mode 2: Response Headers (`[2]`)**: HTTP Protocol (`HTTP/1.1` / `HTTP/2`), Status line, Round-Trip Latency, and full header dictionary (`Content-Type`, `Server`, `Set-Cookie`, `Cache-Control`).
- **Mode 3: Raw Source (`[3]`)**: Line-numbered raw payload with JSON/HTML syntax coloring.

### 3.4 Interactive Navigation Bar

- `g`: Prompts for custom subpath navigation (e.g. `/healthz`, `/metrics`, `/swagger/index.html`).
- `r`: Triggers an immediate HTTP request cycle and recalculates round-trip latency.
- `/`: Interactive in-page text search and match highlighting.
- `q` / `Esc`: Closes inspector and returns smoothly to compact grid.

---

## 4. Web Dashboard Embedded Service Inspector

### 4.1 Server-Side Proxy Gateway (`/api/v1/containers/{id}/proxy`)

To resolve mixed-content blocks and internal IP unreachable errors in web browsers, `ctop`'s backend acts as an authenticated reverse proxy:

1. **Proxy Flow**:
   - Browser requests: `GET /api/v1/containers/{id}/proxy?port=8080&path=/api/health`
   - `ctop` backend connects directly to container IP/socket: `http://172.17.0.2:8080/api/health`
   - Response streams back through `ctop`'s existing HTTPS session.
2. **Security & Sandbox Isolation**:
   - `Content-Security-Policy: sandbox allow-scripts allow-forms; default-src 'self'`
   - Strips malicious frame-busting headers (`X-Frame-Options`) on proxied internal previews to enable dashboard iframe containment while preventing top-level navigation hijacks.

### 4.2 Web Dashboard UI Components

1. **Sandboxed Service Preview Tab**:
   - Embedded responsive IFrame displaying the live container web application.
2. **One-Click Quick Probes**:
   - Instant probe buttons for standard endpoints:
     `[/]` `[/health]` `[/healthz]` `[/metrics]` `[/api/v1/status]` `[/swagger/index.html]`
3. **Live Latency & Health Sparkline**:
   - Historical graph displaying probe response latency over time.
4. **Header & Payload Inspector**:
   - Accordion displaying formatted request/response headers, status codes, and raw responses.

---

## 5. Security & Isolation Controls

1. **Server-Side Request Forgery (SSRF) Protection**:
   - Probing is restricted exclusively to container network interfaces (container IP addresses, container port bindings, or localhost forwards). Requests to cloud metadata endpoints (`169.254.169.254`) or unauthorized host networks are blocked.
2. **Strict Timeouts & Resource Bounds**:
   - Connect Timeout: **2 seconds**.
   - Read Timeout: **5 seconds**.
   - Max Payload Size: **2 MB** (prevents memory exhaustion on large asset downloads).
3. **NDJSON Audit Trail Integration**:
   - Every internal web probe and proxy request is recorded in `ctop`'s audit log (`pkg/audit`) with category `access` and action `container_web_probe`.

---

## 6. Technical Package Structure

```
pkg/
├── htmlrender/               # Pure-Go Terminal HTML-to-ANSI Rendering Engine
│   ├── parser.go             # HTML5 tokenization and DOM tree builder (x/net/html)
│   ├── renderer.go           # ANSI styling, layout, headers, paragraphs, lists
│   ├── table.go              # ASCII / Unicode box-drawing table formatter
│   └── htmlrender_test.go    # Unit tests for HTML tags, formatting, and edge cases
├── serviceprobe/             # In-Engine HTTP & Service Probing Client
│   ├── probe.go              # Low-timeout HTTP client with latency measurement
│   ├── discover.go           # Container port & IP extraction algorithms
│   └── probe_test.go         # Unit tests with mock HTTP test servers
internal/cwidgets/single/
│   ├── webview.go            # TUI interactive full-screen web inspector widget
│   └── webview_test.go       # TUI widget lifecycle and keyboard navigation tests
pkg/web/
│   ├── proxy_handler.go      # Backend proxy endpoint (/api/v1/containers/{id}/proxy)
│   └── dashboard.html        # Web dashboard service preview tab & probe UI
```

---

## 7. Implementation Roadmap

| Milestone | Target Component | Scope of Work |
| :---: | :--- | :--- |
| **Phase 1** | **`pkg/htmlrender`** | • Implement HTML DOM parser using `golang.org/x/net/html`.<br>• Add ANSI renderers for headings, text styling, code blocks, lists, and tables.<br>• Unit test matrix verifying diverse HTML structures. |
| **Phase 2** | **`pkg/serviceprobe`** | • Build container IP/Port auto-discovery engine.<br>• Implement bound HTTP client with timeout, latency timer, and header capture. |
| **Phase 3** | **TUI Web Inspector** | • Create full-screen interactive widget in `internal/cwidgets/single/webview.go`.<br>• Implement Rendered, Headers, and Raw mode switching (`Tab` key).<br>• Add path input dialog (`g` key) and live reload (`r` key). |
| **Phase 4** | **Web Dashboard Proxy** | • Implement `/api/v1/containers/{id}/proxy` backend handler in `pkg/web`.<br>• Add sandboxed iframe preview and quick-probe buttons in web dashboard modal. |
| **Phase 5** | **E2E Validation & Docs** | • End-to-end integration tests in WSL.<br>• Update `README.md` and user manual. |
