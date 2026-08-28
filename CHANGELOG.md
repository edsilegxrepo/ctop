# Changelog

All notable changes to `ctop` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
