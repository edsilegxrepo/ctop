# ctop - Top-like Interface for Container Metrics

`ctop` provides a concise, real-time, top-like overview of container performance metrics. It offers a live, interactive terminal interface to monitor CPU, memory, network, and disk I/O metrics across multiple container runtimes directly from your terminal.

---

## Table of Contents
1. [Application Overview & Objectives](#1-application-overview--objectives)
2. [Security Assessment](#2-security-assessment)
3. [Code Quality & Architecture Review](#3-code-quality--architecture-review)
4. [Command Line Arguments](#4-command-line-arguments)
5. [Usage & Deployment Guide](#5-usage--deployment-guide)
   - [Standalone Binary (Linux RHEL/Ubuntu)](#standalone-binary)
   - [Runtime Connector Configuration](#runtime-connector-configuration)
   - [Diagnostics & Debug Logging](#diagnostics--debug-logging)
   - [Terminal UI Output Samples](#terminal-ui-output-samples)
6. [Interactive Controls, Status & Health Indicators](#6-interactive-controls-status--health-indicators)
   - [Keybindings & Shortcuts](#keybindings--shortcuts)
   - [Status & Health Indicator Legend](#status--health-indicator-legend)
7. [Associated Documentation](#7-associated-documentation)

---

## 1. Application Overview & Objectives

### Objectives
- **Real-Time Visibility**: Continuously collect and display low-overhead CPU, memory, network I/O, disk I/O, and PID metrics for all running and exited containers.
- **Runtime Portability**: Modular connector architecture supporting Docker engines, Linux `runC` cgroups, and offline synthetic test harnesses.
- **Interactive TUI**: Intuitive, high-performance terminal UI with instant search filtering, multi-metric column sorting, detailed single-container inspection, log multiplexing, and shell execution.
- **Minimal Footprint**: Low memory footprint (~15MB RSS) and minimal CPU overhead with zero external runtime dependencies.

---

## 2. Security Assessment

`ctop` is engineered with defense-in-depth security principles across its operational lifecycle:

### a. Encryption in Transit
- **Docker Daemon TLS / mTLS**: When connecting over remote TCP endpoints (`tcp://<host>:2376`), `ctop` enforces TLS v1.2+ encryption with mutual client certificate verification when `DOCKER_TLS_VERIFY=1` and `DOCKER_CERT_PATH` are supplied.
- **Local IPC Isolation**: Local communication uses UNIX domain sockets (`/var/run/docker.sock`) or Windows Named Pipes (`npipe:////./pipe/docker_engine`), bypassing network stacks and preventing remote packet interception.
- **Remote Log Streaming**: Optional remote logging sockets support authenticated and encrypted loopback or private network bindings.

### b. Secret & Credential Management
- **Zero Plaintext Storage**: `ctop` does not store, cache, or persist daemon passwords, API tokens, or registry credentials to disk.
- **Automatic Secret Masking**: Sensitive container environment variables (passwords, API tokens, private keys, certificates, database URLs, DSNs) are obfuscated by default (`•••••••••••• [masked]`) in the Process & Env inspector to prevent shoulder surfing and accidental disclosure. Pressing `u` toggles mask visibility.
- **Environment Ingestion**: Connection parameters are sourced securely from runtime environment variables (`DOCKER_HOST`, `DOCKER_CERT_PATH`, `DOCKER_TLS_VERIFY`).
- **Configuration Permissions**: User preferences written to disk (`~/.config/ctop/config`) only contain display preferences (column ordering, sort fields, filter strings) and use standard file permissions (`0600`/`0700`).

### c. Authentication & Authorization (RBAC)
- **Engine Authentication**: Fully supports Docker daemon client certificates (`ca.pem`, `cert.pem`, `key.pem`).
- **Access Segregation**:
  - **Read-Only Mode**: In monitoring-only environments, mounting `/var/run/docker.sock:ro` provides full telemetry, container inspection, and log viewing while strictly preventing container state modifications (Start, Stop, Pause, Exec).
  - **Administrative Controls**: Lifecycle commands (`Pause`, `Unpause`, `Restart`, `Stop`, `Exec`) are explicitly dispatched only upon user confirmation (`[y/N]` prompt) in interactive menus.

### d. Current & Non-Vulnerable Dependencies
- All third-party libraries are actively maintained and pinned to secure versions:
  - `github.com/fsouza/go-dockerclient`: Modern Docker engine API client with context cancellation.
  - `github.com/gizak/termui/v3`: Modernized, thread-safe terminal UI framework.
  - `github.com/opencontainers/runc/libcontainer`: Upstream standard container runtime primitives.
  - `github.com/BurntSushi/toml`: Secure TOML parser with strict bounds checking.
- Zero known CVE vulnerabilities across all dependencies.

### e. Unprivileged Execution Context
- `ctop` executes entirely unprivileged and does not require root permissions. Standard user accounts only require membership in the `docker` user group (`sudo usermod -aG docker $USER`) or a configured sudo/socket access policy to interact with the container runtime socket (`/var/run/docker.sock`).
- When deployed in containerized environments, `ctop` can execute as a non-root user (`nobody` or custom UID) with read-only access to the Docker socket.

### f. Web Telemetry & Zero-Leak Security Guard
- **Zero-Leak Dual-Channel Architecture**:
  - **Local Loopback Auto-Unlock**: Direct local access on `127.0.0.1` / `localhost` without reverse proxy headers bypasses password prompts for instant local convenience.
  - **Remote Access Invariant**: Remote or proxied access strictly requires **Transport Encryption (TLS 1.2+ or Secure Reverse Proxy) + Web Authentication Token** (`--web-auth-token`). Unencrypted remote requests are rejected with `403 Forbidden`.
  - **Reverse Proxy Loopback Guard**: Requests arriving at `127.0.0.1` that contain proxy forwarding headers (`X-Forwarded-For`, `X-Real-IP`, `X-Forwarded-Proto`, `X-Forwarded-Host`) are classified as **remote**, strictly enforcing TLS and authentication.
- **Strict Query Parameter Deprecation**: Following RFC 6750 §5.3 and OWASP guidelines, query parameter tokens (`?token=` or `?auth=`) are strictly rejected with `401 Unauthorized` to prevent credential leakage in browser histories, proxy access logs, and referrer headers.
- **REST / SSE Dual Channel**:
  - **REST API / SSE Streams / CLI**: Authenticated via standard `Authorization: Bearer <token>` headers.
  - **Web Dashboard UI**: Authenticated via an ephemeral, in-memory `ctop_session` cookie (`HttpOnly; SameSite=Strict; Secure; Max-Age=86400`).
- **Memory-Bounded Ephemeral Session Store**: Thread-safe in-memory session store bounded at 100 concurrent sessions with Least Recently Used (LRU) eviction, a 24-hour absolute TTL, and a 2-hour idle timeout. Sessions are wiped automatically on daemon restart.
- **Sliding-Window Login Rate Limiter**: Max 5 failed login attempts per client IP per minute; subsequent attempts receive `429 Too Many Requests` with `Retry-After: 60` headers.
- **Constant-Time Verification**: All token and session validations enforce `crypto/subtle.ConstantTimeCompare` to eliminate timing side-channel attacks.
- **Filesystem Security**: Tokens are stored in `~/.config/ctop/token` with strict owner-only permissions (`0400` file, `0700` directory) and cleaned up automatically on daemon shutdown.

---

## 3. Code Quality & Architecture Review

`ctop` follows clean Go engineering practices and rigorous quality standards:

- **High Test Coverage**: **84.5% statement coverage** across all 23 packages with fast execution speed, zero race conditions, zero deadlocks, and zero test artifact pollution.
- **Thread Safety**: Fully verified under `go test -race`. All shared structs (`container.Container`, `logging.safeMemoryBackend`, `widgets.TextView`) utilize fine-grained read/write mutexes (`sync.RWMutex`, `sync.Mutex`) or atomic primitives (`sync/atomic.Bool`).
- **Deadlock-Free Design**: Clean separation between custom widget state locks and `termui.Block` draw routines, eliminating recursive mutex lockups.
- **Platform Support**: Fully portable across **Windows** (native console & WSL) and **Linux** (**RHEL** and **Ubuntu**).

For architectural diagrams and deep-dive design choices, see [ARCHITECTURE.md](ARCHITECTURE.md).
For test specifications, defect logs, and coverage reports, see [TESTING.md](TESTING.md).

---

## 4. Command Line Arguments

`ctop` can be configured at startup using the following command-line flags, organized by operational category:

#### General & Help
| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--version` | `bool` | `false` | Output version information and exit. |
| `--help` | `bool` | `false` | Display help dialog and list available connectors. |

#### Container Discovery & Filtering
| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--filter` | `string` | `""` | Filter containers by name, ID regex, or structured key-value queries (`status=`, `health=`, `name=`, `image=`, `env=`, labels). |
| `--active` | `bool` | `false` | Show active (running) containers only (default: shows all). |
| `--sort` | `string` | `""` | Select container sort field (`cpu`, `mem`, `mem %`, `net`, `io`, `pids`, `name`, `state`, `uptime`, `compose`). |
| `--reverse` | `bool` | `false` | Reverse container sort order. |

#### Display, Theme & Metrics Mode
| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--icons` | `string` | `unicode` | Icon glyph style to use (`unicode` or `nerd` for Nerd Font symbols). |
| `--invert` | `bool` | `false` | Invert default terminal color palette (for light terminal backgrounds). |
| `--rate` | `bool` | `true` | Show real-time throughput rates (`bytes/sec`) for network and I/O (default). |
| `--cumulative` | `bool` | `false` | Show cumulative lifetime metrics (total bytes) instead of real-time throughput rates. |

#### Web Dashboard, TLS & Auth Security
| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--web` | `string` | `""` | Start embedded read-only web dashboard and REST/SSE API on specified address (e.g. `:9090` or `127.0.0.1:9090`). |
| `--url-prefix` | `string` | `""` | Base URL subpath when running behind reverse proxies (e.g. `/probe`). |
| `--web-auth-token` | `bool` | `false` | Enforce 64-character web authentication token (automatically generated and stored in `~/.config/ctop/token` with `0400` permissions). |
| `--persistent-token` | `bool` | `false` | Prevent token regeneration on startup; autogenerates token once and persists across restarts (requires `--web-auth-token`). |
| `--web-tls-cert` | `string` | `""` | Path to server TLS certificate PEM file for web HTTPS. |
| `--web-tls-key` | `string` | `""` | Path to server TLS private key PEM file for web HTTPS. |
| `--audit-log` | `string` | `""` | Path to daily-rotated NDJSON audit log file (e.g. `/var/log/ctop/audit.ndjson`). |
| `--headless` | `bool` | `false` | Run in headless daemon mode without terminal UI (requires `--web`). |

#### Remote Hosts & Docker Daemon TLS Security
| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--host` | `string` | `""` | Docker host endpoint(s) to monitor (can be specified multiple times: `local`, `tcp://`, `ssh://`, `unix://`). |
| `--tls-verify` | `bool` | `false` | Enforce strict TLS certificate verification when connecting to remote Docker daemons. |
| `--tls-ca` | `string` | `""` | Path to custom CA certificate file for TLS/mTLS verification (e.g. `~/.docker/ca.pem`). |
| `--tls-cert` | `string` | `""` | Path to client TLS certificate file for mTLS authentication (e.g. `~/.docker/cert.pem`). |
| `--tls-key` | `string` | `""` | Path to client TLS private key file for mTLS authentication (e.g. `~/.docker/key.pem`). |

#### Engine Connector & Operation Mode
| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--connector` | `string` | `docker` | Container engine connector to use (`docker`, `runc`, `mock`). |
| `--read-only` | `bool` | `false` | Read-only inspection mode (disables container lifecycle mutations). |
| `--download-dir` | `string` | `"."` | Default host destination directory for file downloads and log exports. |

### CLI Subcommands

| Command | Description | Example |
| :--- | :--- | :--- |
| `ctop update` | Query latest GitHub releases, verify assets, and perform atomic in-place binary upgrade. | `ctop update` |
| `ctop service <action>` | Generate and manage background daemon systemd service units (`install`, `uninstall`, `status`, `generate`). | `ctop service generate` |

### Web Telemetry & Headless Daemon

`ctop` includes a built-in, zero-dependency, **strictly read-only** embedded web dashboard and real-time Server-Sent Events (SSE) telemetry API:

```bash
# Launch ctop TUI and serve web dashboard simultaneously on port 9090
ctop --web :9090

# Run ctop as a headless background monitoring daemon with auto-generated 64-character token
ctop --headless --web 127.0.0.1:9090 --web-auth-token

# Run ctop with native TLS 1.2+ encryption, web authentication token, and daily audit logging
ctop --headless --web :9443 \
     --web-tls-cert /path/to/server.crt \
     --web-tls-key /path/to/server.key \
     --web-auth-token \
     --audit-log /var/log/ctop/audit.ndjson
```

#### REST & SSE API Surface (Read-Only)

- **`GET /`**: Interactive HTML5 Canvas 2D telemetry dashboard (includes Security Guard unlock modal).
- **`POST /api/v1/auth/login`**: Exchange 64-character token for an authenticated `ctop_session` cookie (rate limited to 5 attempts/min per IP).
- **`POST /api/v1/auth/logout`**: Revoke active session and clear cookie.
- **`GET /api/v1/auth/status`**: Check authentication state and loopback bypass status.
- **`GET /api/v1/health`**: Service liveness and readiness probe.
- **`GET /api/v1/metrics`**: Aggregated cluster and host resource telemetry JSON.
- **`GET /api/v1/containers`**: List of active container snapshots.
- **`GET /api/v1/containers/{id}`**: Single container telemetry details.
- **`GET /api/v1/containers/{id}/top`**: In-container running process table.
- **`GET /api/v1/containers/{id}/diff`**: Writable layer filesystem change set.
- **`GET /api/v1/containers/{id}/files`**: In-container directory listings (`?path=/...`).
- **`GET /api/v1/containers/{id}/endpoints`**: Candidate container HTTP/HTTPS endpoints discovered by in-engine prober.
- **`GET /api/v1/containers/{id}/proxy`**: Secure server-side container web proxy gateway (`?port=80&path=/&format=json|html`).
- **`GET /api/v1/containers/{id}/probes`**: In-container network and exposed port reachability probes.
- **`GET /api/v1/stream`**: Real-time Server-Sent Events (SSE) stream (`text/event-stream`).
- **`GET /api/v1/export`**: Complete telemetry snapshot JSON export (`?container=<id>`).

---

## 5. Usage & Deployment Guide

### Standalone Binary

#### Download Pre-Compiled Binary (Linux RHEL / Ubuntu)
```bash
# Download pre-compiled binary to user bin directory
mkdir -p ~/.local/bin
wget https://github.com/edsilegx/ctop/releases/download/v<release>/ctop-<release>-linux-amd64 -O ~/.local/bin/ctop
chmod +x ~/.local/bin/ctop

# Run ctop
ctop
```

#### Run with Options
```bash
# Run with default settings (Docker connector, show all containers)
ctop

# Show only active containers, sorted by memory usage
ctop --active --sort mem

# Show only running containers
ctop --active

# Filter containers by name (e.g. containers with "app" in the name)
ctop --filter app

# Sort containers by CPU usage in descending order
ctop --sort cpu --reverse

# Use the runC connector instead of Docker
ctop --connector runc

# Multi-host container monitoring across local engine and remote servers
ctop --host local --host tcp://192.168.1.50:2375 --host ssh://deploy@prod-server1.internal

# Secure mTLS container monitoring with explicit client certificates
ctop --host tcp://prod-node1.internal:2376 --tls-verify --tls-ca ~/.docker/ca.pem --tls-cert ~/.docker/cert.pem --tls-key ~/.docker/key.pem

# Filter containers using structured multi-field query
ctop --filter "status=running env=production image=redis"

# Enable modern Nerd Font icon glyphs
ctop --icons nerd

# Invert color scheme for light terminal backgrounds
ctop --invert

# Self-update to latest release
ctop update
```

### Multi-Host Monitoring Engine

`ctop` can aggregate real-time telemetry from multiple local and remote Docker daemons simultaneously in a single unified view. When multiple `--host` flags are specified, the dynamic `HOST` column is automatically enabled:

```bash
# Monitor local daemon + staging swarm + production cluster
ctop --host local --host tcp://10.0.0.12:2375 --host ssh://ubuntu@production.infra.net:2222
```

### Docker Context Auto-Resolution

`ctop` natively mirrors the Docker CLI's context resolution hierarchy. It automatically detects and connects to the active Docker context configured in `~/.docker/config.json` (`currentContext`) or the `DOCKER_CONTEXT` environment variable without requiring manual socket path configuration:
- **Colima**: Automatically resolves `unix://$HOME/.colima/default/docker.sock`.
- **Rancher Desktop**: Automatically resolves Rancher's isolated socket metadata.
- **Docker Desktop**: Resolves named desktop and cloud contexts.

### Structured Multi-Field Filtering Syntax

In addition to standard substring and regex searches, `ctop` supports structured key-value filter tokens that can be combined with space-separated `AND` logic:

| Filter Syntax | Description | Example |
| :--- | :--- | :--- |
| `status=<state>` / `state=<state>` | Matches container state (`running`, `paused`, `exited`, `created`) | `status=running` |
| `health=<status>` | Matches container healthcheck status (`healthy`, `unhealthy`, `starting`) | `health=healthy` |
| `name=<substr>` | Matches container name substring | `name=api` |
| `image=<substr>` / `ancestor=<substr>` | Matches container image name | `image=postgres` |
| `compose=<project>` / `project=<name>` | Matches Docker Compose stack project name | `project=backend` |
| `<label_key>=<value>` | Matches specific container label or metadata key-value pair | `environment=prod` |

*Example combined query:*
```bash
# Display only healthy running containers in the prod environment with 'api' in the name
ctop --filter "status=running health=healthy name=api environment=prod"
```

### Runtime Connector Configuration

`ctop` dynamically connects to supported container runtimes via standard environment variables:

| Runtime | Environment Variable | Default Value | Description |
| :--- | :--- | :--- | :--- |
| **Docker** | `DOCKER_HOST` | `unix:///var/run/docker.sock` | Daemon socket path or TCP address (`tcp://<host>:2376`). |
| **Docker** | `DOCKER_CONTEXT` | `""` | Docker CLI context name (e.g. `colima`, `desktop-linux`). |
| **Docker** | `DOCKER_TLS_VERIFY` | `0` | Enable mutual TLS verification for TCP endpoints. |
| **Docker** | `DOCKER_CERT_PATH` | `""` | Directory containing `ca.pem`, `cert.pem`, and `key.pem`. |
| **runC** | `RUNC_ROOT` | `/run/runc` | Path to runC container state root directory. |
| **runC** | `RUNC_SYSTEMD_CGROUP` | `false` | When set to `1` or `true`, enables systemd cgroups integration. |

---

### Embedded Web Telemetry Dashboard & REST/SSE API

`ctop` includes a zero-dependency, real-time embedded web telemetry server and browser dashboard accessible via the `--web` CLI option.

#### 1. Starting the Web Server
```bash
# Start ctop with terminal UI + web dashboard on port 9090
ctop --web :9090

# Start ctop in headless daemon mode (ideal for Docker containers, background services, or CI/CD)
ctop --web 0.0.0.0:9090 --headless

# Run behind a reverse proxy subpath (e.g. https://metrics.internal/probe)
ctop --web :9090 --url-prefix /probe
```

#### 2. Web Dashboard Features
- **Cluster Summary Cards**: Total, running, paused, and stopped containers, aggregated CPU %, total memory usage/limit, and network & disk I/O throughput rates.
- **HTML5 Canvas 2D Sparklines**: Real-time streaming graphs for Cluster CPU utilization, Memory allocation, and Network throughput (Rx/Tx).
- **Interactive Container Drill-Down Modal**: Click any container row to open a full glassmorphic inspector:
  - **4 Real-Time Sparkline Charts**: CPU %, Memory allocation, Network throughput (Rx/Tx), and Disk I/O rate (Read/Write).
  - **Running Telemetry History**: Fixed-width rolling 5-sample live history table.
  - **6 Inspection Tabs**:
    - `[o] Overview & Metrics`: Live charts, 5-sample running history table, and runtime specs (Created, Uptime, Command, Entrypoint, User, IPs, Ports).
    - `[v] Volumes & Mounts`: Destination, Source, Type (`volume`/`bind`), Mode (`rw`/`ro`), and Driver.
    - `[n] Networking & Ports`: Network interfaces (IP, Gateway, MAC, CIDR prefix) and Port Forwarding cards.
    - `[E] Process & Env`: Searchable environment variable key-value table with one-click copy buttons.
    - `[P] In-Container Top`: Live process table queried from `/api/v1/containers/{id}/top`.
    - `[w] Web & Probes`: Zero-dependency embedded web prober, sandboxed IFrame preview, live response headers, and raw payload viewer.
  - **Keyboard Navigation**: Press `Esc` to close modal, or `o`, `v`, `n`, `e`, `i`, `p`, `d`, `f`, `w` to switch inspection tabs.

#### 3. Telemetry Export & Interactive Reports
- **Cluster & Container JSON Export (`📥 Export JSON`)**: Downloads pretty-formatted (2-space indented) JSON containing complete system metrics, container metadata, and running telemetry samples.
- **Interactive Plain-Text Report Viewer (`📋 View Report`)**: Opens a dedicated report viewer popup displaying the complete structured ASCII-aligned telemetry report, with one-click actions to copy directly to clipboard (`📋 Copy to Clipboard`) or download as a `.txt` file (`📥 Download .txt`).

#### 4. Automatic Secret & Credential Sanitization
To prevent accidental credential disclosure on shared monitoring dashboards:
- All environment variables and container labels matching sensitive patterns (`PASS`, `SECRET`, `KEY`, `TOKEN`, `AUTH`, `CERT`, `CRED`, `PRIVATE`, `DATABASE_URL`, `DB_URL`, `DSN`, `AWS_`, `ACCESS_KEY`, `SESSION_TOKEN`, `APIKEY`) are **strictly filtered out and excluded** from the web server payloads and browser dashboard.

#### 5. REST & SSE API Reference (Read-Only)

All endpoints strictly enforce `GET`/`HEAD` read-only access (mutating requests return `405 Method Not Allowed`, with the exception of authenticated session login/logout `POST` handlers):

| Endpoint | Method | Auth Required | Description |
| :--- | :---: | :---: | :--- |
| `/` | `GET` | Cookie / Local | Interactive Web Dashboard SPA (Zero-Leak Unlock Modal). |
| `/api/v1/auth/status` | `GET` | No | Check auth status (`authenticated`, `auth_enabled`, `direct_local`). |
| `/api/v1/auth/login` | `POST` | No | Submit 64-character token to establish `ctop_session` cookie (Rate limited: 5 attempts/min). |
| `/api/v1/auth/logout` | `POST` | Cookie | Terminate active web session and clear cookie. |
| `/api/v1/health` | `GET` | No | Server liveness probe, version, and uptime. |
| `/api/v1/metrics` | `GET` | Yes (Remote) | Aggregated cluster-wide CPU, memory, network, and disk I/O metrics. |
| `/api/v1/containers` | `GET` | Yes (Remote) | Array of all container metadata and telemetry snapshots. |
| `/api/v1/containers/{id}` | `GET` | Yes (Remote) | Detailed telemetry and inspect metadata for a specific container. |
| `/api/v1/containers/{id}/top` | `GET` | Yes (Remote) | In-container running process table. |
| `/api/v1/containers/{id}/diff` | `GET` | Yes (Remote) | Writable layer filesystem change set. |
| `/api/v1/containers/{id}/files` | `GET` | Yes (Remote) | In-container directory listings (`?path=/...`). |
| `/api/v1/containers/{id}/endpoints` | `GET` | Yes (Remote) | Candidate container HTTP/HTTPS endpoints discovered by in-engine prober. |
| `/api/v1/containers/{id}/proxy` | `GET` | Yes (Remote) | Secure server-side container web proxy gateway (`?port=80&path=/&format=json|html`). |
| `/api/v1/containers/{id}/probes` | `GET` | Yes (Remote) | In-container network and exposed port reachability probes. |
| `/api/v1/export` | `GET` | Yes (Remote) | Pretty JSON download of cluster telemetry (supports `?container=<id>`). |
| `/api/v1/stream` | `GET` | Yes (Remote) | High-throughput Server-Sent Events (SSE) live telemetry feed. |

#### 6. Zero-Leak Security Guard & TLS Testing Guide

For full technical specifications and threat models, see [docs/SECGUARD.md](docs/SECGUARD.md).

##### A. Automated Unit & Integration Tests
```bash
# Run web server unit tests (constant-time compare, rate limiter, session store, TLS config)
go test -v ./pkg/web/...

# Run live bridge end-to-end integration tests
go test -v -run "TestWebBridge" .
```

##### B. Testing Native TLS & Auth Token with cURL and Web Browser

1. **Test self-signed certificates:**
   Pre-generated test certificates are available in `tests/tls/server.crt` and `tests/tls/server.key` (or generate fresh ones with `openssl req -x509 -newkey rsa:4096 -nodes -keyout tests/tls/server.key -out tests/tls/server.crt -days 365 -subj "/CN=localhost"`).

2. **Start `ctop` in headless mode with TLS and Web Auth Token:**
   ```bash
   ctop --headless --web :9443 \
        --web-tls-cert tests/tls/server.crt \
        --web-tls-key tests/tls/server.key \
        --web-auth-token
   ```
   *`ctop` automatically generates a 64-character Base62 alphanumeric token in `~/.config/ctop/token` (`0400` permissions).*

3. **Test with cURL (Bearer Header):**
   ```bash
   TOKEN=$(cat ~/.config/ctop/token)

   # Unauthenticated request -> 401 Unauthorized
   curl -k -i https://localhost:9443/api/v1/containers

   # Authenticated request with Bearer header -> 200 OK
   curl -k -i -H "Authorization: Bearer $TOKEN" https://localhost:9443/api/v1/containers

   # Deprecated URL query parameter (?token=...) -> 401 Unauthorized
   curl -k -i https://localhost:9443/api/v1/containers?token=$TOKEN

   # Live Real-Time SSE Stream
   curl -k -N -H "Authorization: Bearer $TOKEN" https://localhost:9443/api/v1/stream
   ```

4. **Test with Web Browser Dashboard:**
   - Open **`https://localhost:9443/`** in your browser.
   - The **Security Guard** modal will prompt for the bearer token from `~/.config/ctop/token`.
   - Incorrect entries trigger error feedback (5 failed attempts trigger rate-limiting: `429 Too Many Requests`).
   - Entering the correct token unlocks the dashboard, sets the `ctop_session` cookie (`HttpOnly; SameSite=Strict; Secure`), and streams live metrics.
   - Click the **🔒 Logout** button in the top navigation bar to terminate the active session.

##### C. Testing Behind a Reverse Proxy

When running `ctop` behind a reverse proxy (e.g. NGINX, Traefik, Caddy, AWS ALB):
- Direct local requests on `127.0.0.1` without forwarding headers automatically unlock the local dashboard.
- Any request with proxy headers (`X-Forwarded-For`, `X-Real-IP`, `X-Forwarded-Proto`) is classified as **remote**, strictly enforcing HTTPS and authentication.

**Simulate with cURL:**
```bash
# 1. Local Direct Access (Bypasses UI prompt)
curl -i http://localhost:9090/api/v1/containers
# -> 200 OK

# 2. Remote Access without HTTPS (Rejected)
curl -i -H "X-Forwarded-For: 203.0.113.10" http://localhost:9090/api/v1/containers
# -> 403 Forbidden ("TLS encryption required")

# 3. Remote Access with HTTPS & Bearer Token (Authorized)
curl -i -H "X-Forwarded-For: 203.0.113.10" \
        -H "X-Forwarded-Proto: https" \
        -H "Authorization: Bearer $TOKEN" \
        http://localhost:9090/api/v1/containers
# -> 200 OK
```

#### 7. Reverse Proxy Configuration Examples

**NGINX (with SSL Termination & SSE Streaming):**
```nginx
server {
    listen 443 ssl http2;
    server_name telemetry.example.com;

    ssl_certificate     /etc/ssl/certs/example.crt;
    ssl_certificate_key /etc/ssl/private/example.key;
    ssl_protocols       TLSv1.2 TLSv1.3;

    location /probe/ {
        proxy_pass http://127.0.0.1:9090/probe/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection '';
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding off;
    }
}
```

**Caddy:**
```caddy
telemetry.example.com {
    handle_path /probe/* {
        reverse_proxy 127.0.0.1:9090 {
            header_up X-Forwarded-Proto "https"
        }
    }
}
```

---

### Compliance & NDJSON Audit Logging (`--audit-log`)

`ctop` provides structured, immutable audit logging in **Newline Delimited JSON (NDJSON)** format with automatic **daily file rotation**. When configured via `--audit-log <path>`, all security, access, authentication, container lifecycle, and daemon events are recorded with sub-millisecond timestamps and client attribution.

#### 1. Enabling Audit Logging
```bash
# Start ctop with daily rotated audit logging
ctop --headless --web :9443 \
     --web-tls-cert tests/tls/server.crt \
     --web-tls-key tests/tls/server.key \
     --web-auth-token \
     --audit-log /var/log/ctop/audit.ndjson
```
*`ctop` automatically writes active daily records to `/var/log/ctop/audit-YYYY-MM-DD.ndjson` and rotates file descriptors at midnight.*

#### 2. Audit Event Schema & Categories
Every line in the audit log is a complete, single JSON object:

| Category | Actions Recorded | Description |
| :--- | :--- | :--- |
| **`access`** | `http_request`, `sse_connect`, `sse_disconnect` | HTTP method, route path, status code, latency (ms), client IP, TLS cipher/version, and auth mode. |
| **`auth`** | `login_success`, `login_failure`, `logout`, `rate_limited` | Authentication attempts, session token issuance, rate limit triggers, and session revocations. |
| **`container`** | `container_start`, `container_stop`, `container_inspect` | Container lifecycle transitions and inspect operations. |
| **`app`** | `startup`, `shutdown` | Daemon initialization, version information, PID, and termination. |

*Sample NDJSON Audit Record:*
```json
{"timestamp":"2026-09-01T20:00:00.123Z","level":"INFO","category":"access","action":"http_request","client_ip":"192.168.1.100","method":"GET","path":"/api/v1/containers","status":200,"duration_ms":1.45,"auth":{"type":"bearer","authenticated":true,"token_prefix":"9469..."},"details":{"tls":"TLSv1.3","user_agent":"curl/7.76.1"}}
```

---

### Diagnostics & Debug Logging

`ctop` includes an internal diagnostic logging engine for troubleshooting without interrupting the terminal interface:

#### 1. File Logging
Stream internal operational logs directly to a log file:
```bash
CTOP_DEBUG=1 CTOP_DEBUG_FILE=ctop.log ctop
```

#### 2. Local Socket & TCP Stream
Stream live diagnostic logs via loopback TCP socket or local UNIX domain socket:
```bash
# Start ctop with TCP log listener
CTOP_DEBUG=1 CTOP_DEBUG_TCP=1 ctop

# In a separate terminal, follow log stream
curl -s localhost:9000
```

---

### Terminal UI Output Samples

#### 1. Compact Grid View (Default)
```text
ctop - 18:50:00        3 containers (3 running)                  filter: 
─────────────────────────────────────────────────────────────────────────────
NAME                 CID          CPU       MEM (Alloc / Total)   NET (Rx / Tx)   IO (Reads / Writes) PIDS
● web-frontend       c8a412f10a8b [ 12%]   142.5M / 2.0G [ 7%]   1.2M / 4.8M     12K / 45K            8
● redis-cache        91b34e12c019 [  2%]    45.1M / 1.0G [ 4%]   8.5M / 1.1M      0B / 120K           4
● postgres-db        f419c83a992e [  8%]   512.0M / 8.0G [ 6%]   3.4M / 9.2M     45M / 12M           16
─────────────────────────────────────────────────────────────────────────────
[a] all [f] filter [s] sort [g] group [m] mode [c] columns [l] logs [o] open [e] exec [U] tune [h] help [q] quit
```

#### 2. Multi-Tab Container Inspector (`[o]` key)
```text
web-frontend (c8a412f10a8b) - Up 3 hours                                    [q] back
─────────────────────────────────────────────────────────────────────────────
CPU Util: 12% ───┐                     Memory: 142.5M / 2.0G (7%) ───┐
 20% |           |                      20% |                        |
 10% |  /\__/\_  |                      10% |  ──────────────        |
  0% └──┴──────┴─┘                       0% └──┴────────────┴────────┘

NETWORK I/O                            DISK I/O
  RX: 1.2 MB (4.5 KB/s)                  Read:  12.0 KB (0 B/s)
  TX: 4.8 MB (18.2 KB/s)                 Write: 45.0 KB (2.1 KB/s)

MEMORY BREAKDOWN                       METADATA
  RSS: 110.2 MB                          Image: node:18-alpine
  Cache: 32.3 MB                         IPs:   172.17.0.2
  Swap: 0 B | Kernel: 4.2 MB             Ports: 0.0.0.0:8080->8080/tcp
```

The multi-tab inspector provides deep inspection across 12 specialized tabs:
- **`[1]` Overview & Metrics**: Real-time telemetry sparklines (CPU, Memory, Net Rx/Tx, Disk I/O), memory breakdown (RSS, Cache, Swap, Kernel Memory, OOM Kill detection), and container metadata.
- **`[2]` Live Logs**: Real-time container stdout/stderr log stream viewer with timestamp toggle, keyword filtering, and disk export.
- **`[3]` Volumes & Mounts**: Storage bindings table showing Destination path, Source path, Mount Type (`volume`/`bind`/`tmpfs`), and Access Mode (`rw`/`ro`).
- **`[4]` Networking & Ports**: Network interface table (Name, IP, Gateway, MAC, Subnet), published host port bindings (`0.0.0.0:8080 -> 80/tcp`), and live TCP reachability probes for external host and internal container endpoints (`[p]` to re-probe).
- **`[5]` Process & Env**: Runtime execution parameters, Linux Capabilities (`CapAdd`/`CapDrop`), Security Options (Seccomp, AppArmor), Healthcheck probe timeline, and environment variables with sensitive variable masking (`[u]` to toggle).
- **`[6]` Image Details**: Detailed container image metadata, layer hierarchy, labels, and tags.
- **`[7]` In-Container Top**: Live running process table inside the container namespace (`PID`, `USER`, `TIME`, `CMD`).
- **`[8]` Filesystem Diff**: Real-time filesystem changes on the writable layer with Added (`[A]`), Changed (`[C]`), and Deleted (`[D]`) status indicators.
- **`[9]` Recreate / Compose**: Equivalent `docker run` command and `docker-compose.yml` specification generator (`[9]` or `G`).
- **`[0]` Labels & Compose**: Docker Compose orchestration tags and container labels (`[0]`).
- **`[F]` In-Container Files**: Interactive directory browser with text preview (`<Enter>`/`<Space>`), host download (`[d]`), host upload (`[u]`), in-container file editing with `$EDITOR` (`[e]`/`[E]`), file deletion (`[x]`/`[X]`/`<Delete>`), filter (`[/]`), deep search (`[f]`), and clear (`[c]`).
- **`[W]` Web Services**: Interactive in-terminal web service inspector & live HTTP/HTTPS prober (`[W]` or `w`). Features rendered ANSI HTML, sorted HTTP response headers, raw response body, port cycling (`n` next, `p` prev), and custom subpath prompt (`g`).

*Navigation:* Use `<Tab>` / `<Shift+Tab>`, number keys `1-9`/`0`/`F`/`W`, or class hotkeys (`o`, `l`, `v`, `n`, `E`, `i`, `P`, `D`, `G`, `0`, `F`, `W`) to switch views. In Web tab: `n`/`p` cycles ports, `g` prompts for target URL/path, `1-3` switches view modes. In File Explorer: `d` downloads to host, `u` uploads from host, `e` edits via host `$EDITOR`, `x` deletes file, `D` customizes download directory, `/` filters, `f` deep-searches, `c` clears filter/search, `Home`/`End`/`PgUp`/`PgDown` navigates. In Network tab: `p` runs live TCP port probes. Use `↑`/`↓` to scroll.

#### 3. Log Stream Drawer (`[l]` key)
```text
Logs: web-frontend (c8a412f10a8b) ───────────────── [t] time [/] filter [s] save [D] dir [q] exit
2026-08-18T18:48:12Z [info] HTTP GET /api/v1/health 200 OK 4ms
2026-08-18T18:48:25Z [info] Database connection pool verified healthy
2026-08-18T18:49:01Z [info] Handling websocket broadcast to 12 clients
```
*Tip:* Press `s` inside the log viewer to export active logs to disk, and press `D` to change the destination directory on-the-fly.

#### 4. Live Resource Hot-Tuning (`[U]` key)
Directly adjust container limits and policies without container restarts or downtime:
- **Memory Limit**: Modify live memory ceiling in MB (e.g. `512`, `1024`, or `0` for unlimited).
- **CPU Quota**: Allocate CPU cores (e.g. `1.5` for 1.5 cores, `0.5` for half a core).
- **Restart Policy**: Switch between `always`, `unless-stopped`, `on-failure`, and `no`.

---

## 6. Interactive Controls, Status & Health Indicators

### Keybindings & Shortcuts

| Key Binding | Action |
| :--- | :--- |
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `PageUp` / `Ctrl+u` | Jump up one page |
| `PageDown` / `Ctrl+d` | Jump down one page |
| `<Enter>` | Open container action menu (Inspectors, File Explorer, Logs, Lifecycle, Resource Tuning, Exec, Tools) |
| `a` | Toggle display of inactive / stopped containers |
| `f` | Open interactive filter prompt |
| `g` | Toggle Docker Compose project stack grouping |
| `s` | Open sort selection menu (`cpu`, `mem`, `mem %`, `net`, `io`, `pids`, `name`, `state`, `uptime`, `compose`) |
| `r` | Reverse active sort order |
| `c` | Open column configuration menu |
| `o` | Open multi-tab container inspector (12 tabs: Overview, Logs, Mounts, Network, Env, Image, Top, Diff, Recreate, Labels, Files, Web) |
| `v` | Open volumes & mounts inspector directly (Tab 3) |
| `n` | Open networking & ports inspector directly (Tab 4) |
| `p` | Re-run live TCP port reachability probes (in Network tab) |
| `F` | Open interactive in-container file explorer & text previewer directly (Tab F) |
| `W` / `w` | Open in-terminal web service inspector directly (Tab W) |
| `l` | Open live container log drawer (`t` timestamps, `/` filter, `s` save, `D` dir, `q` close) |
| `X` / `x` | Export container diagnostic report directly (JSON/Text) |
| `U` | Open live resource hot-tuning dialog (Memory limit MB, CPU quota, Restart policy) |
| `k` (in menu) | Open granular POSIX signal menu inside container action menu (`<Enter>` -> `[k]`) |
| `e` | Open interactive shell inside selected container |
| `e` / `E` (explorer) | Edit container file with host `$EDITOR` (in File Explorer) |
| `x` / `X` (explorer) | Delete container file with confirmation (in File Explorer) |
| `w` | Open web port in default browser (first mapped HTTP port) with clean screen restoration |
| `u` | Toggle secret masking in Environment inspector / Upload in File Explorer |
| `d` | Download selected container file/directory to host (in File Explorer) |
| `D` | Set target host download / export directory (in File Explorer & Log Viewer) |
| `S` | Save active settings to configuration file (`~/.config/ctop/config` or `~/.ctop`) |
| `H` | Toggle ctop status header |
| `h` / `?` | Open interactive help dialog |
| `q` / `<Escape>` / `Ctrl+c` | Close modal / Exit ctop |

### Status & Health Indicator Legend

The compact grid view prefixes container names with dynamic state and health badges:

#### Container State
| Unicode | Nerd Font | Color | Description |
| :---: | :---: | :--- | :--- |
| `►` | `` / `` | Green | Container is currently running and active. |
| `■` | `` | Red | Container is stopped / exited. |
| `‖` | `` | Yellow | Container execution is paused. |
| `◉` | `` | Default | Container has been created but not started. |

#### Container Health Check
If a container is configured with a Docker health check, a health badge appears beside the state indicator:
| Unicode | Nerd Font | Color | Description |
| :---: | :---: | :--- | :--- |
| `☼` | `` | Green | Health check status is `healthy` (all probes passing). |
| `◌` | `` | Yellow | Health check status is `starting` (grace period). |
| `⚠` | `` | Red | Health check status is `unhealthy` (probes failing). |

---

## 7. Associated Documentation

- **[ARCHITECTURE.md](ARCHITECTURE.md)**: Detailed system architecture, component contracts, data flow diagrams, and concurrency models.
- **[docs/SECGUARD.md](docs/SECGUARD.md)**: Zero-Leak Web Authentication, TLS Enforcement & Security Guard Specification.
- **[docs/DESIGN.md](docs/DESIGN.md)**: Headless engine architecture, REST/WebSocket streaming API, and modular package decoupling specification.
- **[docs/MODERNIZATION.md](docs/MODERNIZATION.md)**: Go modernization roadmap, dependency upgrades (`termui` v3, `runc` v1.3), and static analysis remediation record.
- **[TESTING.md](TESTING.md)**: Complete test strategy, categorized test catalog (14 groups), 10 identified defect resolutions, and coverage verification report.
- **[CHANGELOG.md](CHANGELOG.md)**: Chronological record of all version releases, feature additions, defect fixes, and security remediations.
- **[SECURITY.md](SECURITY.md)**: Security vulnerability disclosure policy and reporting protocols.
