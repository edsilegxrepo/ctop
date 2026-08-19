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

---

## 3. Code Quality & Architecture Review

`ctop` follows clean Go engineering practices and rigorous quality standards:

- **High Test Coverage**: **83.9% statement coverage** across all 17 packages with fast execution speed, zero race conditions, zero deadlocks, and zero test artifact pollution.
- **Thread Safety**: Fully verified under `go test -race`. All shared structs (`container.Container`, `logging.safeMemoryBackend`, `widgets.TextView`) utilize fine-grained read/write mutexes (`sync.RWMutex`, `sync.Mutex`) or atomic primitives (`sync/atomic.Bool`).
- **Deadlock-Free Design**: Clean separation between custom widget state locks and `termui.Block` draw routines, eliminating recursive mutex lockups.
- **Platform Support**: Fully portable across **Windows** (native console & WSL) and **Linux** (**RHEL** and **Ubuntu**).

For architectural diagrams and deep-dive design choices, see [ARCHITECTURE.md](ARCHITECTURE.md).
For test specifications, defect logs, and coverage reports, see [TESTING.md](TESTING.md).

---

## 4. Command Line Arguments

`ctop` can be configured at startup using the following command-line flags:

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-v` | `bool` | `false` | Output version information and exit. |
| `-h` | `bool` | `false` | Display help dialog and list available connectors. |
| `-f` | `string` | `""` | Filter containers by name or ID regex. |
| `-a` | `bool` | `false` | Show active (running) containers only (default: shows all). |
| `-s` | `string` | `""` | Select container sort field (`cpu`, `mem`, `mem %`, `net`, `io`, `pids`, `name`, `state`, `uptime`). |
| `-r` | `bool` | `false` | Reverse container sort order. |
| `-i` | `bool` | `false` | Invert default terminal color palette (for light terminal backgrounds). |
| `-ro` | `bool` | `false` | Read-only inspection mode (disables container lifecycle mutations). |
| `-download-dir` | `string` | `"."` | Default host destination directory for file downloads and log exports. |
| `-connector` | `string` | `docker` | Container engine connector to use (`docker`, `runc`, `mock`). |

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
# Show only active containers, sorted by memory usage
ctop -a -s mem

# Filter containers with 'redis' in the name
ctop -f redis

# Invert color scheme for light terminals
ctop -i
```

### Runtime Connector Configuration

`ctop` dynamically connects to supported container runtimes via standard environment variables:

| Runtime | Environment Variable | Default Value | Description |
| :--- | :--- | :--- | :--- |
| **Docker** | `DOCKER_HOST` | `unix:///var/run/docker.sock` | Daemon socket path or TCP address (`tcp://<host>:2376`). |
| **Docker** | `DOCKER_TLS_VERIFY` | `0` | Enable mutual TLS verification for TCP endpoints. |
| **Docker** | `DOCKER_CERT_PATH` | `""` | Directory containing `ca.pem`, `cert.pem`, and `key.pem`. |
| **runC** | `RUNC_ROOT` | `/run/runc` | Path to runC container state root directory. |
| **runC** | `RUNC_SYSTEMD_CGROUP` | `false` | When set to `1` or `true`, enables systemd cgroups integration. |

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
NAME                 CID          CPU       MEM               NET RX/TX     IO R/W      PIDS
● web-frontend       c8a412f10a8b [ 12%]   142.5M / 2.0G [ 7%]  1.2M / 4.8M   12K / 45K    8
● redis-cache        91b34e12c019 [  2%]    45.1M / 1.0G [ 4%]  8.5M / 1.1M    0B / 120K   4
● postgres-db        f419c83a992e [  8%]   512.0M / 8.0G [ 6%]  3.4M / 9.2M   45M / 12M   16
─────────────────────────────────────────────────────────────────────────────
[a] all [f] filter [s] sort [g] group [c] columns [l] logs [o] open [e] exec [U] tune [h] help [q] quit
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

The multi-tab inspector provides deep inspection across 9 specialized tabs:
- **`[1]` Overview & Metrics**: Real-time telemetry sparklines (CPU, Memory, Net Rx/Tx, Disk I/O), memory breakdown (RSS, Cache, Swap, Kernel Memory, OOM Kill detection), and container metadata.
- **`[2]` Volumes & Mounts**: Storage bindings table showing Destination path, Source path, Mount Type (`volume`/`bind`/`tmpfs`), and Access Mode (`rw`/`ro`).
- **`[3]` Networking & Ports**: Network interface table (Name, IP, Gateway, MAC, Subnet), published host port bindings (`0.0.0.0:8080 -> 80/tcp`), and live TCP reachability probes for external host and internal container endpoints (`[p]` to re-probe).
- **`[4]` Process & Env**: Runtime execution parameters, Linux Capabilities (`CapAdd`/`CapDrop`), Security Options (Seccomp, AppArmor), Healthcheck probe timeline, and environment variables with sensitive variable masking (`[u]` to toggle).
- **`[5]` In-Container Top**: Live running process table inside the container namespace (`PID`, `USER`, `TIME`, `CMD`).
- **`[6]` Filesystem Diff**: Real-time filesystem changes on the writable layer with Added (`[A]`), Changed (`[C]`), and Deleted (`[D]`) status indicators.
- **`[7]` Recreate / Compose**: Equivalent `docker run` command and `docker-compose.yml` specification generator.
- **`[8]` Labels & Compose**: Docker Compose orchestration tags and container labels.
- **`[9]` In-Container Files**: Interactive directory browser, in-TUI file previewer (`<Enter>`/`<Space>`), host download exporter (`[d]`), and host file uploader (`[u]` to upload host files/directories into the container).

*Navigation:* Use `<Tab>` / `<Shift+Tab>`, number keys `1-9`, or class hotkeys (`o`, `v`, `n`, `E`, `P`, `D`, `G`, `L`, `F`) to switch views. Use `u` to toggle environment variable secret masks. In File Explorer: `d` downloads to host, `u` uploads from host, `D` customizes download directory. In Network tab: `p` runs live TCP port probes. Use `↑`/`↓` to scroll.

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
| `o` | Open multi-tab container inspector (Overview, Mounts, Network, Env, Top, Diff, Recreate, Labels, Files) |
| `v` | Open volumes & mounts inspector directly |
| `n` | Open networking & ports inspector directly |
| `p` | Re-run live TCP port reachability probes (in Network tab) |
| `F` | Open interactive in-container file explorer & text previewer directly |
| `l` | Open live container log drawer (`t` timestamps, `/` filter, `s` save, `D` dir, `q` close) |
| `U` | Open live resource hot-tuning dialog (Memory limit MB, CPU quota, Restart policy) |
| `k` | Open granular POSIX signal menu (`SIGHUP`, `SIGQUIT`, `SIGUSR1`, `SIGUSR2`, `SIGTERM`, `SIGKILL`) |
| `e` | Open interactive shell inside selected container |
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
| Indicator | Color | Description |
| :--- | :--- | :--- |
| `►` | Green | Container is currently running and active. |
| `■` | Red | Container is stopped / exited. |
| `‖` | Yellow | Container execution is paused. |
| `◉` | Default | Container has been created but not started. |

#### Container Health Check
If a container is configured with a Docker health check, a health badge appears beside the state indicator:
| Health Indicator | Color | Description |
| :--- | :--- | :--- |
| `☼` | Green | Health check status is `healthy` (all probes passing). |
| `◌` | Yellow | Health check status is `starting` (grace period). |
| `⚠` | Red | Health check status is `unhealthy` (probes failing). |

---

## 7. Associated Documentation

- **[ARCHITECTURE.md](ARCHITECTURE.md)**: Detailed system architecture, component contracts, data flow diagrams, and concurrency models.
- **[TESTING.md](TESTING.md)**: Complete test strategy, categorized test catalog (14 groups), 10 identified defect resolutions, and coverage verification report.
- **[CHANGELOG.md](CHANGELOG.md)**: Chronological record of all version releases, feature additions, defect fixes, and security remediations.
- **[SECURITY.md](SECURITY.md)**: Security vulnerability disclosure policy and reporting protocols.
