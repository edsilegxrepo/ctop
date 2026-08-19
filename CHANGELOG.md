# Changelog

All notable changes to `ctop` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v0.9.0] - 2026-08-18

### Added
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

## [v0.7.7] - 2022-03-23

### Changed
- **Go 1.18 Toolchain**: Upgraded build toolchain to support Go 1.18.
- **runc Update**: Bumped `github.com/opencontainers/runc` to `v1.1.0` to incorporate updated `golang.org/x/sys` primitives.

---

## [v0.7.6] - 2021-06-11

### Changed
- **Dependency Upgrades**: Upgraded `github.com/opencontainers/runc` and supporting container libraries.
- **Documentation**: Updated installation instructions and usage examples.

---

## [v0.7.5] - 2020-11-06

### Added
- **GitHub CLI Integration**: Integrated `gh release` tooling into the release automation pipeline.

### Fixed
- **Makefile Targets**: Fixed release directory packaging and checksum generation.

---

## [v0.7.4] - 2020-10-25

### Changed
- **Runtime Updates**: Updated Go version and dependencies for `runc v1.0.0-rc92`.
- **Documentation**: Refreshed CLI options and connector configuration documentation.

---

## [v0.7.3] - 2020-01-03

### Fixed
- **Stability Improvements**: Resolved edge cases in container metrics streaming and log listener disconnects.

---

## [v0.7.2] - 2019-01-24

### Added
- **Navigation Shortcuts**: Enhanced keybindings and single-container view controls.

---

## [v0.7.1] - 2018-03-09

### Fixed
- **Collector Stability**: Improved error handling and reconnect backoff for container engine collectors.

---

## [v0.7.0] - 2018-01-11

### Added
- **Multi-Runtime Connectors**: Introduced modular connector architecture supporting both **Docker** and native **runC** containers.
- **Dynamic Connector Selection**: Added `-connector` CLI argument and automatic runtime detection.

---

## [v0.6.1] - 2017-07-12

### Fixed
- **Release Automation**: Fixed release directory naming and binary archive generation.

---

## [v0.6.0] - 2017-06-12

### Added
- **Single Container Inspection**: Added detailed single-container view with real-time CPU/memory histograms, environment inspection, and log tailing.
- **Interactive Shell Exec**: Added container shell execution shortcut (`e`).

---

## [v0.5.1] - 2017-03-21

### Fixed
- **Terminal Resize Handling**: Improved grid dynamic reflow on terminal window resize events.

---

## [v0.5.0] - 2017-03-15

### Added
- **Inverted Color Scheme**: Added `-i` flag for high visibility on light terminal backgrounds.
- **Regex Filter**: Added `-f` flag and interactive `/` filter dialog for filtering containers by name.
- **Sort Options**: Added container sorting by CPU, memory, network, I/O, and uptime (`-s`, `-r`).

---

## [v0.4.1] - 2017-03-10

### Changed
- **Build Pipeline**: Embedded version and git commit metadata into binary via linker flags.

---

## [v0.4.0] - 2017-03-08

### Added
- **Status Indicators**: Added dynamic running/paused/stopped container state badges.
- **Interactive Menus**: Added sort menu, help modal (`?` / `h`), and configuration persistence (`~/.config/ctop/config`).

---

## [v0.1.0] - 2016-11-06

### Added
- **Initial Release**: Top-like real-time resource monitor for Docker containers.
- **Core Telemetry**: CPU utilization, memory limits, and network throughput metrics streaming.
