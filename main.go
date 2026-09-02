// Package main is the entry point for ctop, a top-like real-time container metrics monitor.
//
// Objective:
//
//	Parse command-line options, initialize configuration, logging, and theme engines, coordinate
//	container connector discovery (Docker, runc, multi-host), spin up optional background web/headless servers,
//	and drive the interactive terminal event loop.
//
// Core Components:
//   - main(): CLI entrypoint orchestrating subcommands (--service, --update, --version, --help), web daemon initialization, and TUI startup.
//   - GridCursor: Thread-safe cursor managing filtered container navigation and viewport offsets.
//   - CompactGrid / CTopHeader / StatusLine / ErrorView: Core TermUI widgets rendering the primary screen layout.
//   - ConnectorSuper: Connector supervisor discovering and polling container runtimes.
//   - WebServer: Optional HTTP/JSON + SSE live dashboard daemon.
//
// Functionality:
//   - Command-line argument parsing and validation.
//   - Multi-connector daemon discovery and context aggregation.
//   - Headless background service execution with systemd integration.
//   - Graceful shutdown on SIGINT/SIGTERM with terminal restoration.
//
// Data Flow:
//
//	CLI Flags -> Config/Theme/Logging -> Connector Supervision -> GridCursor -> TermUI Render Loop / Web Broadcaster.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/edsilegx/ctop/internal/cwidgets/compact"
	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/internal/widgets"
	"github.com/edsilegx/ctop/pkg/audit"
	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/connector"
	"github.com/edsilegx/ctop/pkg/container"
	"github.com/edsilegx/ctop/pkg/exit"
	"github.com/edsilegx/ctop/pkg/logging"
	"github.com/edsilegx/ctop/pkg/service"
	"github.com/edsilegx/ctop/pkg/update"
	"github.com/edsilegx/ctop/pkg/web"
	ui "github.com/gizak/termui/v3"
	tb "github.com/nsf/termbox-go"
)

var (
	// Version metadata populated during compilation
	build     = "none"
	version   = "dev-build"
	goVersion = runtime.Version()

	// Global application state singletons
	log      *logging.CTopLogger  // Global logger instance
	cursor   *GridCursor          // Interactive container list selection and pagination cursor
	cGrid    *compact.CompactGrid // Terminal grid renderer for compact container row widgets
	header   *widgets.CTopHeader  // Top application status header
	status   *widgets.StatusLine  // Bottom status line for notification messages and errors
	errView  *widgets.ErrorView   // Modal connection error viewer
	uiEvents <-chan ui.Event      // Asynchronous UI keyboard and resize event stream

	versionStr = fmt.Sprintf("ctop version %v, build %v %v", version, build, goVersion)
)

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// main parses CLI arguments, initializes subsystems, and starts the primary render loop.
func main() {
	defer panicExit()

	if len(os.Args) > 1 && os.Args[1] == "update" {
		if err := update.Run(version); err != nil {
			fmt.Fprintf(os.Stderr, "update error: %v\n", err)
			os.Exit(exit.ExitGeneral)
		}
		os.Exit(exit.ExitSuccess)
	}

	if len(os.Args) > 1 && os.Args[1] == "service" {
		if err := service.Run(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "service error: %v\n", err)
			os.Exit(exit.ExitService)
		}
		os.Exit(exit.ExitSuccess)
	}

	// parse command line arguments
	var (
		versionFlag         bool
		helpFlag            bool
		filterFlag          string
		activeOnlyFlag      bool
		sortFieldFlag       string
		reverseSortFlag     bool
		invertFlag          bool
		iconsFlag           string
		readOnlyFlag        bool
		downloadDirFlag     string
		connectorFlag       string
		tlsVerifyFlag       bool
		tlsCAFlag           string
		tlsCertFlag         string
		tlsKeyFlag          string
		cumulativeFlag      bool
		rateFlag            bool
		webFlag             string
		urlPrefixFlag       string
		webAuthTokenFlag    bool
		persistentTokenFlag bool
		webTLSCertFlag      string
		webTLSKeyFlag       string
		auditLogFlag        string
		headlessFlag        bool
		hostFlags           stringSlice
	)

	flag.BoolVar(&versionFlag, "version", false, "output version information and exit")
	flag.BoolVar(&helpFlag, "help", false, "display this help dialog")
	flag.StringVar(&filterFlag, "filter", "", "filter containers by name, ID regex, or structured query")
	flag.BoolVar(&activeOnlyFlag, "active", false, "show active (running) containers only (default: shows all)")
	flag.StringVar(&sortFieldFlag, "sort", "", "select container sort field")
	flag.BoolVar(&reverseSortFlag, "reverse", false, "reverse container sort order")
	flag.BoolVar(&invertFlag, "invert", false, "invert default colors for light terminal backgrounds")
	flag.StringVar(&iconsFlag, "icons", "", "icon style to use ('unicode' or 'nerd')")
	flag.BoolVar(&readOnlyFlag, "read-only", false, "read-only inspection mode (disables state modifications)")
	flag.StringVar(&downloadDirFlag, "download-dir", "", "default host directory for container file downloads")
	flag.StringVar(&connectorFlag, "connector", "docker", "container connector to use")
	flag.BoolVar(&tlsVerifyFlag, "tls-verify", false, "enforce TLS verification when connecting to Docker daemon")
	flag.StringVar(&tlsCAFlag, "tls-ca", "", "path to CA certificate file for TLS/mTLS verification")
	flag.StringVar(&tlsCertFlag, "tls-cert", "", "path to client TLS certificate file for mTLS authentication")
	flag.StringVar(&tlsKeyFlag, "tls-key", "", "path to client TLS private key file for mTLS authentication")
	flag.BoolVar(&cumulativeFlag, "cumulative", false, "show cumulative lifetime metrics (total bytes) instead of real-time throughput rates")
	flag.BoolVar(&rateFlag, "rate", false, "show real-time throughput rates (bytes/sec) for network and I/O (default)")
	flag.StringVar(&webFlag, "web", "", "start embedded read-only web dashboard and REST/SSE API on specified address (e.g. ':9090')")
	flag.StringVar(&urlPrefixFlag, "url-prefix", "", "Base URL subpath when running behind reverse proxies (e.g. /probe)")
	flag.BoolVar(&webAuthTokenFlag, "web-auth-token", false, "enforce web authentication token (auto-generated in ~/.config/ctop/token)")
	flag.BoolVar(&persistentTokenFlag, "persistent-token", false, "persist authentication token across restarts (requires --web-auth-token)")
	flag.StringVar(&webTLSCertFlag, "web-tls-cert", "", "path to server TLS certificate PEM file for web HTTPS")
	flag.StringVar(&webTLSKeyFlag, "web-tls-key", "", "path to server TLS private key PEM file for web HTTPS")
	flag.StringVar(&auditLogFlag, "audit-log", "", "path to audit log file (records all events and access in NDJSON with daily rotation)")
	flag.BoolVar(&headlessFlag, "headless", false, "run in headless daemon mode without terminal UI (requires --web)")
	flag.Var(&hostFlags, "host", "Docker host endpoint(s) to connect to (can be specified multiple times)")
	flag.Usage = printHelp
	flag.Parse()

	connector.SetGlobalTLSConfig(connector.TLSConfig{
		Verify: tlsVerifyFlag,
		CA:     tlsCAFlag,
		Cert:   tlsCertFlag,
		Key:    tlsKeyFlag,
	})

	if versionFlag {
		fmt.Println(versionStr)
		os.Exit(exit.ExitSuccess)
	}

	if helpFlag {
		printHelp()
		os.Exit(exit.ExitSuccess)
	}

	if persistentTokenFlag && !webAuthTokenFlag {
		fmt.Fprintf(os.Stderr, "error: --persistent-token requires --web-auth-token\n")
		os.Exit(exit.ExitUsage)
	}

	if persistentTokenFlag && webFlag == "" {
		fmt.Fprintf(os.Stderr, "error: --persistent-token requires --web <address>\n")
		os.Exit(exit.ExitUsage)
	}

	// init logger
	log = logging.Init()

	// init audit logger if requested
	if auditLogFlag != "" {
		if _, err := audit.Init(auditLogFlag); err != nil {
			fmt.Fprintf(os.Stderr, "error initializing audit logger: %v\n", err)
			os.Exit(exit.ExitGeneral)
		}
		defer audit.Close()
		audit.LogApp("startup", audit.LevelInfo, map[string]interface{}{
			"version":  versionStr,
			"headless": headlessFlag,
			"web":      webFlag,
			"pid":      os.Getpid(),
		})
	}

	// init global config and read config file if exists
	config.Init()
	if err := config.Read(); err != nil {
		log.Warningf("reading config: %s", err)
	}

	// override default config values with command line flags
	if filterFlag != "" {
		config.Update("filterStr", filterFlag)
	}

	if downloadDirFlag != "" {
		config.Update("downloadDir", downloadDirFlag)
	}

	if readOnlyFlag {
		config.UpdateSwitch("readOnly", true)
	}

	// Ensure all containers (running, paused, stopped) are shown by default unless -a/--active flag is passed
	if activeOnlyFlag {
		config.UpdateSwitch("allContainers", false)
	} else {
		config.UpdateSwitch("allContainers", true)
	}

	if sortFieldFlag != "" {
		validSort(sortFieldFlag)
		config.Update("sortField", sortFieldFlag)
	}

	if reverseSortFlag {
		config.Toggle("sortReversed")
	}

	if cumulativeFlag {
		config.UpdateSwitch("rateMode", false)
	} else if rateFlag {
		config.UpdateSwitch("rateMode", true)
	}

	if iconsFlag != "" {
		config.Update("icons", iconsFlag)
	}
	theme.SetIconStyle(config.GetVal("icons"))

	// init connector
	var (
		cSuper *connector.ConnectorSuper
		err    error
	)
	if len(hostFlags) > 0 {
		if len(hostFlags) > 1 {
			config.ColumnToggle("host")
		}
		cSuper = connector.NewConnectorSuper(func() (connector.Connector, error) {
			return connector.NewMultiDockerConnector(hostFlags...)
		})
	} else {
		cSuper, err = connector.ByName(connectorFlag)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "permission denied") {
				fmt.Fprintf(os.Stderr, "error connecting to Docker socket: permission denied (ensure current user belongs to the 'docker' group)\n")
				os.Exit(exit.ExitDockerPermission)
			}
			if strings.Contains(errStr, "tls") || strings.Contains(errStr, "certificate") {
				fmt.Fprintf(os.Stderr, "error initializing TLS connection: %v\n", err)
				os.Exit(exit.ExitDockerTLS)
			}
			fmt.Fprintf(os.Stderr, "error initializing connector '%s': %v\n", connectorFlag, err)
			os.Exit(exit.ExitConnector)
		}
	}
	cursor = &GridCursor{cSuper: cSuper}

	// start web server if requested
	var webSrv *web.Server
	if webFlag != "" {
		srv, cleanup, err := startWebServer(webFlag, version, urlPrefixFlag, cSuper, WebOptions{
			URLPrefix:       urlPrefixFlag,
			AuthToken:       webAuthTokenFlag,
			PersistentToken: persistentTokenFlag,
			TLSCert:         webTLSCertFlag,
			TLSKey:          webTLSKeyFlag,
			AuditLog:        auditLogFlag,
		})
		if err != nil {
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "address already in use") || strings.Contains(errStr, "only one usage of each socket address") {
				fmt.Fprintf(os.Stderr, "error starting web dashboard: address %s is already in use by another process\n", webFlag)
				os.Exit(exit.ExitPortInUse)
			}
			fmt.Fprintf(os.Stderr, "error starting web dashboard: %v\n", err)
			os.Exit(exit.ExitGeneral)
		}
		defer cleanup()
		webSrv = srv
		proto := "http"
		if webTLSCertFlag != "" && webTLSKeyFlag != "" {
			proto = "https"
		}
		log.Infof("embedded web dashboard listening on %s://%s (read-only)", proto, srv.Addr())
	}

	// Headless daemon mode
	if headlessFlag {
		if webFlag == "" {
			fmt.Fprintf(os.Stderr, "--headless mode requires --web <address>\n")
			os.Exit(exit.ExitUsage)
		}
		proto := "http"
		if webTLSCertFlag != "" && webTLSKeyFlag != "" {
			proto = "https"
		}
		listenAddr := webFlag
		if webSrv != nil {
			listenAddr = webSrv.Addr()
		}
		fmt.Printf("ctop running in headless mode (read-only web dashboard on %s://%s). Press Ctrl+C to exit.\n", proto, listenAddr)
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		fmt.Println("\nshutting down ctop headless daemon...")
		return
	}

	// init ui
	if invertFlag {
		InvertColorMap()
	}
	initTheme()
	if err := ui.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "error initializing terminal UI: %v\n", err)
		os.Exit(exit.ExitUI)
	}
	tb.SetInputMode(tb.InputEsc)
	tb.HideCursor()
	uiEvents = ui.PollEvents()

	defer Shutdown()
	cGrid = compact.NewCompactGrid()
	header = widgets.NewCTopHeader()
	status = widgets.NewStatusLine()
	errView = widgets.NewErrorView()

	for {
		exitRequested := Display()
		if exitRequested {
			return
		}
	}
}

func Shutdown() {
	audit.LogApp("shutdown", audit.LevelInfo, map[string]interface{}{
		"pid": os.Getpid(),
	})
	audit.Close()
	if log != nil {
		log.Notice("shutting down")
		log.Exit()
	}
	ui.Close()
}

// ensure a given sort field is valid
func validSort(s string) {
	if _, ok := container.Sorters[s]; !ok {
		fmt.Fprintf(os.Stderr, "invalid sort field: %s\n", s)
		os.Exit(exit.ExitUsage)
	}
}

func panicExit() {
	if r := recover(); r != nil {
		Shutdown()
		fmt.Fprintf(os.Stderr, "fatal runtime error: %v\n", r)
		os.Exit(exit.ExitGeneral)
	}
}

var helpMsg = `ctop - interactive container viewer

usage:
  ctop [options]
  ctop [command]

commands:
  update                 Check and install the latest ctop release
  service [action]       Manage systemd background service (install|uninstall|status|generate)

options:
  General & Help:
    --version            output version information and exit
    --help               display this help dialog

  Container Discovery & Filtering:
    --filter string      filter containers by name, ID regex, or structured query
    --active             show active (running) containers only (default: shows all)
    --sort string        select container sort field (cpu, mem, mem %, net, io, pids, name, state, uptime, compose)
    --reverse            reverse container sort order

  Display, Theme & Metrics Mode:
    --icons string       icon glyph style to use ('unicode' or 'nerd') (default: "unicode")
    --invert             invert default colors for light terminal backgrounds
    --rate               show real-time throughput rates (bytes/sec) for network and I/O (default: true)
    --cumulative         show cumulative lifetime metrics (total bytes) instead of real-time rates

  Web Dashboard, TLS & Auth Security:
    --web string         start embedded read-only web dashboard and REST/SSE API on specified address (e.g. ':9090')
    --url-prefix string  Base URL subpath when running behind reverse proxies (e.g. /probe)
    --web-auth-token     enforce web authentication token (auto-generated in ~/.config/ctop/token)
    --persistent-token   persist authentication token across restarts (requires --web-auth-token)
    --web-tls-cert string path to server TLS certificate PEM file for web HTTPS
    --web-tls-key string  path to server TLS private key PEM file for web HTTPS
    --audit-log string   path to audit log file (records all events and access in NDJSON with daily rotation)
    --headless           run in headless daemon mode without terminal UI (requires --web)

  Remote Hosts & TLS Security:
    --host string        Docker host endpoint(s) to monitor (can be specified multiple times)
    --tls-verify         enforce TLS verification when connecting to Docker daemon
    --tls-ca string      path to CA certificate file for TLS/mTLS verification (e.g. ~/.docker/ca.pem)
    --tls-cert string    path to client TLS certificate file for mTLS authentication (e.g. ~/.docker/cert.pem)
    --tls-key string     path to client TLS private key file for mTLS authentication (e.g. ~/.docker/key.pem)

  Engine Connector & Operation Mode:
    --connector string   container connector to use (default: "docker")
    --read-only          read-only inspection mode (disables state modifications)
    --download-dir string default host directory for container file downloads and log exports (default: ".")`

func printHelp() {
	fmt.Println(helpMsg)
	fmt.Printf("\navailable connectors:\n  ")
	fmt.Println(strings.Join(connector.Enabled(), ", "))
}
