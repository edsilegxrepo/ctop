# List of Go modules modernization status

All pinned modules have been successfully refactored and upgraded:

| Module | Status | Previous | Current |
|---|---|---|---|
| github.com/gizak/termui/v3 | Upgraded | v2.3.1-0.20180817033724-8d4faad06196+incompatible | v3.1.0 |
| github.com/opencontainers/runc | Upgraded | v1.1.14 | v1.3.0 |
| github.com/cilium/ebpf | Upgraded | v0.12.3 | v0.19.0 |

Notes:
- All pinned modules have been modernized and upgraded to their latest supported versions.
- `github.com/gizak/termui/v3` has been fully migrated across all widget, layout, and event subsystems.
- `github.com/opencontainers/runc` (`v1.3.0`) and `github.com/cilium/ebpf` (`v0.19.0`) have been integrated with `libcontainer.Load` and `opencontainers/cgroups`.

## Migration and module modernization

This section outlines the detailed architectural analysis, technical requirements, breaking changes, and a step-by-step phased roadmap to modernize the codebase and safely upgrade all pinned dependencies.

---

### 1. Dependency Analysis & Breaking Changes

```
ctop
├── github.com/opencontainers/runc (v1.1.14 -> v1.3.0) [Direct]
│   ├── github.com/cilium/ebpf (v0.12.3 -> v0.19.0) [Indirect]
│   └── github.com/opencontainers/cgroups (v0.0.1) [Indirect / Extracted]
└── github.com/gizak/termui (v2.3.1 -> v3.1.0 / v3) [Direct]
    └── github.com/nsf/termbox-go (Migrated / Decoupled)
```

#### 1.1 `github.com/cilium/ebpf` (`v0.12.3` &rarr; `v0.19.0`)
- **Dependency Classification**: Indirect / Transitive dependency.
- **Why It Was Pinned**: During prior dependency upgrade cycles, upgrading `runc` caused build breakages. Because `runc v1.1.14` depends on older eBPF definitions, `cilium/ebpf` had to be downgraded and version-pinned in lockstep.
- **Application Code Impact**: Zero direct imports exist in `ctop`.
- **Modernization Requirement**: Upgrading `github.com/opencontainers/runc` to `v1.3.0` automatically resolves and upgrades `cilium/ebpf` to `v0.19.0` without requiring direct code modifications for eBPF itself.

---

#### 1.2 `github.com/opencontainers/runc` (`v1.1.14` &rarr; `v1.3.0`)
- **Dependency Classification**: Direct dependency.
- **Impacted Files**:
  - `connector/runc.go`
  - `connector/collector/runc.go`

##### Major Breaking Changes:
1. **Removal of `libcontainer.Factory`**:
   - *Previous behavior*: Containers were instantiated by creating a factory with `libcontainer.New(rootPath)` and then calling `factory.Load(containerID)`.
   - *Modernized API*: The `Factory` interface was removed. Container loading is now handled via package-level `libcontainer.Load(rootPath, containerID)`.
2. **Cgroups Package Relocation**:
   - *Previous behavior*: Types were imported from `github.com/opencontainers/runc/libcontainer/cgroups`.
   - *Modernized API*: Cgroups stats and managers were extracted into a standalone module: `github.com/opencontainers/cgroups`. All references to `*cgroups.Stats` must import `github.com/opencontainers/cgroups`.
3. **Concrete Type Replacement (`libcontainer.Container`)**:
   - *Previous behavior*: `libcontainer.Container` was an interface.
   - *Modernized API*: `libcontainer.Container` is now a concrete struct (`*libcontainer.Container`). Methods like `.Config()` return `configs.Config` value structures directly.

##### Detailed Refactoring Plan:
- In `connector/runc.go`:
  - Remove `factory libcontainer.Factory` from `type Runc struct`.
  - Update `NewRunc()`: Eliminate `libcontainer.New(opts.root)` call.
  - Update `GetLibc(id string) *libcontainer.Container`: Replace `cm.factory.Load(id)` with `libcontainer.Load(cm.opts.root, id)`.
  - Update all map references from `map[string]libcontainer.Container` to `map[string]*libcontainer.Container`.
- In `connector/collector/runc.go`:
  - Replace import `github.com/opencontainers/runc/libcontainer/cgroups` with `github.com/opencontainers/cgroups`.
  - Update `libc` field in `type Runc struct` to `*libcontainer.Container`.
  - In `ReadCPU`, `ReadMem`, and `ReadIO`, update parameter type `stats *cgroups.Stats` to reference `github.com/opencontainers/cgroups.Stats`.

---

#### 1.3 `github.com/gizak/termui` (`v2.3.1` &rarr; `v3.1.0` / `v3`)
- **Dependency Classification**: Direct core dependency.
- **Impacted Areas**:
  - **Core Application & Event Loop**: `main.go`, `grid.go`, `cursor.go`, `keys.go`, `colors.go`, `menus.go`, `debug.go`
  - **Base Widgets**: `widgets/view.go`, `widgets/header.go`, `widgets/status.go`, `widgets/input.go`, `widgets/error.go`, `widgets/menu/main.go`, `widgets/menu/tooltip.go`
  - **Compact Grid Components**: `cwidgets/compact/grid.go`, `cwidgets/compact/row.go`, `cwidgets/compact/header.go`, `cwidgets/compact/gauge.go`, `cwidgets/compact/status.go`, `cwidgets/compact/text.go`, `cwidgets/compact/column.go`
  - **Single Container Detail Components**: `cwidgets/single/main.go`, `cwidgets/single/cpu.go`, `cwidgets/single/mem.go`, `cwidgets/single/net.go`, `cwidgets/single/io.go`, `cwidgets/single/info.go`, `cwidgets/single/logs.go`, `cwidgets/single/env.go`, `cwidgets/single/hist.go`

##### Major Breaking Changes:
1. **Module Path and Imports**:
   - `github.com/gizak/termui` &rarr; `github.com/gizak/termui/v3`
   - Built-in widget primitives moved to `github.com/gizak/termui/v3/widgets`.
2. **Geometry and Layout Model**:
   - *termui v2*: Widgets inherited from `ui.Block` with explicit `X`, `Y`, `Width`, `Height` positioning and implemented `ui.Bufferer` (`Buffer() ui.Buffer`).
   - *termui v3*: Widgets implement the `termui.Drawable` interface (`Draw(buf *termui.Buffer)`, `SetRect(x1, y1, x2, y2 int)`, `GetRect() image.Rectangle`). Coordinates are defined as bounding rectangles rather than top-left origin + width/height.
3. **Styling and Color Subsystem**:
   - *termui v2*: Colors and text attributes were based on `ui.Attribute` and `termbox.Attribute` using global color mappings (`ui.ColorMap`, `ui.ColorDefault`, `ui.ModifierBold`).
   - *termui v3*: Native `termui.Style`, `termui.Color`, and `termui.Modifier` types. Supports inline bracket formatting tags: `"[text](fg:color,bg:color,mod:bold)"`.
4. **Event Handling and Main Loop**:
   - *termui v2*: String path routing (`ui.Handle("/sys/kbd/<escape>", ...)`), internal event dispatcher with `ui.Loop()` / `ui.StopLoop()`.
   - *termui v3*: Clean channel-based polling model via `termui.PollEvents() <-chan termui.Event`. Events are handled via switch statements on `e.Type` (`termui.KeyboardEvent`, `termui.ResizeEvent`) and `e.ID` (`"<Escape>"`, `"<Enter>"`, `"<Resize>"`).
5. **Decoupling from Direct `termbox-go` Calls**:
   - Direct dependencies on `github.com/nsf/termbox-go` in `main.go` and `colors.go` can be eliminated or abstracted through termui v3's backend.

---

### 2. Phased Implementation Roadmap

```
┌────────────────────────────────────────────────────────┐
│ Phase 1: runc & cilium/ebpf Modernization              │
│ - Upgrade go.mod dependencies                          │
│ - Refactor connector/runc.go & connector/collector/    │
│ - Build and unit test runc connector                   │
└──────────────────────────┬─────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────┐
│ Phase 2: termui v3 Core & Event Dispatch Migration     │
│ - Introduce github.com/gizak/termui/v3                 │
│ - Refactor colors.go (termui.Style & Color)            │
│ - Refactor keys.go & main event loop (PollEvents)      │
└──────────────────────────┬─────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────┐
│ Phase 3: Widget Hierarchy & View Porting               │
│ - Port base widgets (header, status, input, menus)     │
│ - Port CompactGrid (rows, gauges, columns, text)       │
│ - Port SingleView (cpu, mem, io, net, logs, charts)    │
└──────────────────────────┬─────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────┐
│ Phase 4: Quality Assurance & Dependency Unpinning      │
│ - Run go vet ./... & golangci-lint run ./...           │
│ - Execute full test suite                              │
│ - Update GOMOD-PINNED.md & SECURITY-COMPLIANCE.md      │
└────────────────────────────────────────────────────────┘
```

#### Phase 1: `opencontainers/runc` and `cilium/ebpf` Upgrade
1. **Module Upgrade**:
   ```bash
   go get github.com/opencontainers/runc@v1.3.0 github.com/cilium/ebpf@v0.19.0
   go mod tidy
   ```
2. **Connector Code Modernization**:
   - Update `connector/runc.go` to use `libcontainer.Load`.
   - Update `connector/collector/runc.go` to use `github.com/opencontainers/cgroups`.
3. **Verification**:
   - Validate compilation with `go build -v ./connector/...`.

#### Phase 2: `termui v3` Core Infrastructure & Dispatch Modernization
1. **Module Upgrade**:
   ```bash
   go get github.com/gizak/termui/v3@v3.1.0
   ```
2. **Color & Theme System Modernization**:
   - Rewrite `colors.go` to define `termui.Style` and `termui.Color` constants.
   - Implement color inversion helper using native `termui.Style` modifications.
3. **Keybinding and Event Loop Modernization**:
   - Update `keys.go` with normalized key identifier strings from termui v3.
   - Refactor `main.go` and `grid.go` to replace `ui.Handle` / `ui.Loop` with a `for e := range ui.PollEvents()` event select loop.

#### Phase 3: Widget and View Porting
1. **Base Widgets (`widgets/`)**:
   - Port `widgets/header.go`, `widgets/status.go`, `widgets/error.go`, `widgets/input.go`, and `widgets/menu/main.go` to implement `termui.Drawable`.
   - Replace old `ui.NewPar` and `ui.NewList` with `termui/v3/widgets.Paragraph` and `termui/v3/widgets.List`.
2. **Compact View (`cwidgets/compact/`)**:
   - Update `CompactGrid`, `Row`, `Gauge`, and `Status` widgets to use `SetRect` bounding boxes and `Draw(buf *termui.Buffer)`.
   - Refactor gauge rendering to use `termui/v3/widgets.Gauge` or custom buffer drawing routines.
3. **Single View (`cwidgets/single/`)**:
   - Migrate `SingleView` metrics panes (`cpu.go`, `mem.go`, `net.go`, `io.go`, `logs.go`, `info.go`, `env.go`).
   - Migrate historical sparklines / line charts (`hist.go`, `sparkline.go`) to `termui/v3/widgets.Plot` or `termui/v3/widgets.Sparkline`.

#### Phase 4: Verification, Linting, & Unpinning
1. **Static Analysis & Compilation**:
   ```bash
   go build -v ./...
   go vet ./...
   golangci-lint run ./...
   go test -v ./...
   ```
2. **Documentation Update**:
   - Remove unpinned dependencies from the table in `GOMOD-PINNED.md`.
   - Record resolved vulnerability statuses in `SECURITY-COMPLIANCE.md`.

---

### 3. Verification & Validation Protocol

| Validation Step | Command / Procedure | Acceptance Criteria |
|---|---|---|
| **Compilation** | `go build -v ./...` | Clean build with zero errors or unresolved symbols across all packages. |
| **Static Analysis** | `go vet ./...` | Zero findings reported. |
| **Linter Compliance** | `golangci-lint run ./...` | 0 issues across all enabled linters (`govet`, `errcheck`, `staticcheck`, `unused`, etc.). |
| **Unit Tests** | `go test -v ./...` | All tests pass. |
| **Interactive Terminal Test** | Run `ctop` under Docker & runc connectors | Responsive UI, functioning cursor navigation, container sorting/filtering, menu overlay, single container drill-down, and clean exit without terminal corruption. |

