# Changelog

All notable changes to `ctop` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.9.3] - 2026-09-01

### Added
- **Mandatory 64-Character Base62 Authentication Token**: Upgraded `--web-auth-token` to generate and enforce 64-character alphanumeric tokens (`[a-zA-Z0-9]`, ~381-bit entropy) with automatic `0400` permission file storage in `~/.config/ctop/token`.
- **Persistent Token Mode (`--persistent-token`)**: Added option used exclusively with `--web-auth-token` to prevent token regeneration across restarts, autogenerating the token once and preserving it on disk.
- **In-Terminal Web Service Inspector (Tab 9 / WebView)**: Embedded interactive HTTP/HTTPS prober in TUI (`[9]` / `W`) and Web Dashboard Tab 9 supporting Rendered ANSI HTML, sorted HTTP headers, raw body viewing, dynamic port cycling (`n`/`p`), and custom subpath probing (`g`).
- **Pure-Go HTML AST Parser & Terminal Renderer (`pkg/htmlrender`)**: Zero-dependency HTML tokenizer and DOM walker with Unicode box-drawing table formatting, runewidth-aware word-wrapping, and strict anti-XSS tag stripping (`<script>`, `<style>`, `<svg>`).
- **Automated Service Discovery & Bounded HTTP Prober (`pkg/serviceprobe`)**: Automated endpoint resolution from port mappings, internal bridge IPs, and ENV declarations with 2MB response ceilings, 3-redirect limits, and Slowloris timeout protection.
- **Thread-Safe Daily-Rotated NDJSON Audit Logging (`pkg/audit`)**: Added `--audit-log <path>` flag generating immutable access and compliance audit trails with automatic midnight file rotation and zero-secret leakage.
- **CLI Service Subcommand Dispatcher (`pkg/service`)**: Integrated `ctop service <action>` (`install`, `uninstall`, `status`, `generate`) for managing background systemd telemetry daemons.
- **Automated Reverse Proxy E2E Test Suite (`tests/nginx/`)**: Added 16-scenario automated NGINX integration test harness validating SSL termination, subpath prefixes, live SSE feeds, token auth, session cookies, and daily audit rotation.

### Security & Performance
- **Zero-Leak Remote Security Invariant**: Remote/proxied requests strictly require Transport Encryption (TLS or HTTPS reverse proxy) and authentication; unencrypted remote requests are rejected with `403 Forbidden`.
- **Multi-Hop IP Spoofing Defense**: Rate limiting and audit logging isolate true client IPs from multi-hop `X-Forwarded-For` proxy chains.
- **Non-Blocking SSE Broadcaster**: Ring buffer ensures stalled or slow subscribers are dropped without delaying remaining connected clients.
- **Connector Shutdown Hardening**: Guarded all background event dispatcher sends against `cm.closed` to prevent goroutine shutdown deadlocks.
- **OS-Specific Cgroup v2 Optimization**: Guarded `/sys/fs/cgroup` directory walks on Linux only to eliminate unnecessary syscalls on Windows.
- **Standardized Process Exit Codes (`pkg/exit`)**: Added `ExitService` (9) and `ExitDaemonStartup` (10) for automated process supervision.

---

## [v0.9.2] - 2026-08-31

### Added
- **Live TCP Endpoint Prober**: Embedded network prober in both Web Dashboard and TUI Networking tab with status indicators, latency tracking, auto-refresh intervals (3s, 5s, 10s, 30s), and zero-flicker table updates.
- **In-Container File Explorer & Preview**: Full recursive file tree browser with deep search, breadcrumb navigation, and in-place file content preview in both TUI (`[F]`) and Web Dashboard.
- **Container Filesystem Diff Viewer**: Interactive visual layer mutation and filesystem change tracker (`Added`, `Changed`, `Deleted`) in both TUI (`[D]`) and Web Dashboard.
- **Image Metadata Details Tab**: Dedicated image inspection tab (`[i]`) presenting base architecture, virtual size, layer counts, author metadata, and build directives.
- **Streamlined Report Viewer Popup**: Interactive unselected text report preview modal in Web Dashboard with direct clipboard copy and plain-text (`.txt`) file download.
- **Systemd Service Generator (`pkg/service`)**: Built-in systemd unit generator (`ctop service generate`) for deploying headless ctop telemetry daemons.
- **Enhanced TUI List Navigation**: Full support for `<PageUp>`, `<PageDown>`, `<Home>`, `<End>`, `<C-u>`, `<C-d>`, `g`, and `G` shortcuts across File Explorer and large container views.

### Changed
- **Web Navigation Menu**: Added a drop-down Tabs Menu for rapid direct tab jumping in the Web Dashboard.

---

## [v0.9.1] - 2026-08-28

### Added
- **Embedded Web Telemetry Dashboard (`--web`)**: Built-in, zero-dependency, strictly read-only HTML5 Canvas 2D dashboard and Server-Sent Events (SSE) streaming server (`/api/v1/stream`).
- **Interactive Container Drill-Down Inspector**: Per-container drill-down modal with 4 real-time sparklines (CPU, Memory, Net, Disk I/O), rolling 5-sample running history table, and 5 inspection tabs (`Overview`, `Volumes`, `Networking`, `Process & Env`, `In-Container Top`).
- **Reverse Proxy Subpath Routing (`--url-prefix`)**: Added `--url-prefix <path>` CLI flag (e.g. `/probe`) with dynamic asset routing and base path injection for NGINX, Caddy, and Traefik reverse proxies.
- **Headless Telemetry Daemon Mode (`--headless`)**: Pure background monitoring daemon mode without terminal UI initialization.
- **Data Export & Clipboard Reporting**: One-click pretty-formatted JSON downloads (`/api/v1/export`) and formatted plain-text ASCII report clipboard copy for cluster and single-container views.
- **Automated Secret & Credential Sanitization**: Automated regex filtering in `pkg/sanitize` stripping passwords, API keys, tokens, certificates, DSNs, and AWS credentials from web endpoints.

### Changed
- **TUI Header Modernization**: Refactored compact grid headers to `MEM (Alloc / Total)`, `NET (Rx / Tx)`, and `IO (Reads / Writes)`.
- **Categorized CLI Help**: Structured `--help` output into 6 operational categories matching documentation.
- **Architecture Modularization**: Decoupled core into headless public packages (`pkg/*`) and private visual components (`internal/*`).
- **Stopped Container Uptime Formatting**: Blanked uptime duration (`—`) for stopped and exited containers.
- **Comprehensive Test Suite**: Reached **84.5% total repository statement coverage** across all 23 packages with expanded unit, edge-case, and E2E integration test suites.

### Security
- **Strict Read-Only Web Surface**: Enforced read-only HTTP semantics across all endpoints with zero mutating Docker operations or command execution paths.

---

## [v0.9.0] - 2026-08-18

- **Multi-Class Container Inspector**: Categorized container inspection with dedicated views for **Overview & Metrics** (`[o]`), **Volumes & Mounts** (`[v]`), **Networking & Ports** (`[n]`), **Process & Environment** (`[E]`), and **Labels & Compose** (`[L]`).
- **Interactive Tab Navigation**: Instant tab switching via `<Tab>`, `<Shift+Tab>`, number keys `1-5`, or class hotkeys (`o`, `v`, `n`, `E`, `L`) inside the single view.
- **Enhanced Storage & Runtime Inspection**: Full tabular view of host-to-container mount destinations, source volumes/binds, access modes (`rw`/`ro`), command entrypoints, working directory, user/UID, exit codes, restart policies, and resource limits.
- **High-Coverage Test Suite**: Comprehensive unit, integration, and mock test coverage across all 17 packages achieving **80.7% total repository statement coverage**.
- **Modern Theme Subsystem**: Dedicated `theme` package with safe terminal dimension detection and light/dark palette abstraction.
- **Portability Support**: Native Windows console and Linux (RHEL & Ubuntu) support with zero root/sudo privilege requirements.
- **Comprehensive Documentation**: Complete operational [README.md](README.md), architecture diagrams in [ARCHITECTURE.md](ARCHITECTURE.md), and categorized test catalog in [TESTING.md](TESTING.md).

### Changed
- **Branding & Module Migration**: Migrated Go module identifier and internal import paths to `github.com/edsilegx/ctop`.
- **Dependency Upgrades**: Upgraded `github.com/opencontainers/runc` to `v1.5.1`, `github.com/opencontainers/selinux` to `v1.15.1`, `github.com/fsouza/go-dockerclient` to `v1.13.2`, `github.com/BurntSushi/toml` to `v1.6.0`, `golang.org/x/net` to `v0.58.0`, and `golang.org/x/sys` to `v0.47.0`.
- **Hardened Compilation Flags**: Enabled `-trimpath -buildmode=pie` Position Independent Executable generation across all targets.
- **Unprivileged Installer**: Re-engineered [install.sh](install.sh) to install directly into user space (`~/.local/bin/ctop`) with automatic temporary file cleanup traps.

### Fixed
- **Deadlock Elimination**: Fixed recursive mutex self-deadlock in custom `termui.Drawable` widgets by isolating widget state locks from block drawing passes.
- **Docker Uptime Calculation**: Resolved 292-year duration calculation overflow by checking `insp.State.StartedAt.IsZero()` for unstarted containers.
- **Integer Overflow Protection (G115)**: Added safe bounds checking with `math.MaxInt64` / `math.MaxInt` across all Docker and runC metric collectors.
- **Thread-Safety & Race Conditions**: Added fine-grained synchronization across `container.Container`, `logging.safeMemoryBackend`, and `widgets.TextView`.

### Security
- **Strict Permission Enforcement**: Hardened configuration directory (`0700`) and file creation (`0600`) permissions in [config/file.go](config/file.go).
- **Gosec & Semgrep Clean**: Passed full security scanning suite with zero outstanding issues.
- **TruffleHog Verified**: Verified 0 secrets or sensitive credentials across the entire repository history.

---

## [v0.8.2] - 2025-10-30

### Added
- **Architecture Documentation**: Added initial `ARCHITECTURE.md` describing system design, component boundaries, and event pipelines.
- **Linting & Modernization Guide**: Added `LINTING.md` documenting static analysis baselines.

### Changed
- **Static Analysis Compliance**: Fixed 51 issues flagged by `golangci-lint` and `go vet` (unkeyed struct literals, unreachable code, nil dereference checks).
- **Module Maintenance**: Synchronized Go module dependencies and standardized gofmt formatting.

---

## [v0.8.1] - 2025-07-25

### Security
- **runc Vulnerability Remediation**: Upgraded `github.com/opencontainers/runc` to `1.1.14` to remediate critical container escape vulnerabilities.
- **Supply Chain Sync**: Re-synced module graph and cleaned unused indirect requirements.

---

## [v0.8.0] - 2025-07-25

### Added
- **Windows Shell Support**: Added interactive container shell execution support for Windows platforms.
- **TUI Suspension**: Implemented terminal UI suspension and restoration during interactive container shell sessions.

---

## [v0.0.0] - 2025-07-20

### Functional baseline
- Import ctop repository from https://github.com/bcicen/ctop
