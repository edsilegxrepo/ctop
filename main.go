// Package main is the entry point for ctop, a top-like real-time container metrics monitor.
// Objective: Parse CLI arguments, initialize configuration/logging/UI, coordinate container connector
// discovery, and drive the interactive terminal event loop.
// Data Flow: CLI Flags -> Config/Theme -> Connector -> GridCursor -> TermUI Render Loop.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/edsilegx/ctop/config"
	"github.com/edsilegx/ctop/connector"
	"github.com/edsilegx/ctop/container"
	"github.com/edsilegx/ctop/cwidgets/compact"
	"github.com/edsilegx/ctop/logging"
	"github.com/edsilegx/ctop/pkg/exit"
	"github.com/edsilegx/ctop/widgets"
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

// main parses CLI arguments, initializes subsystems, and starts the primary render loop.
func main() {
	defer panicExit()

	// parse command line arguments
	var (
		versionFlag     = flag.Bool("v", false, "output version information and exit")
		helpFlag        = flag.Bool("h", false, "display this help dialog")
		filterFlag      = flag.String("f", "", "filter containers")
		activeOnlyFlag  = flag.Bool("a", false, "show active containers only")
		sortFieldFlag   = flag.String("s", "", "select container sort field")
		reverseSortFlag = flag.Bool("r", false, "reverse container sort order")
		invertFlag      = flag.Bool("i", false, "invert default colors")
		readOnlyFlag    = flag.Bool("ro", false, "read-only inspection mode (disables state modifications)")
		downloadDirFlag = flag.String("download-dir", "", "default host directory for container file downloads")
		connectorFlag   = flag.String("connector", "docker", "container connector to use")
	)
	flag.Parse()

	if *versionFlag {
		fmt.Println(versionStr)
		os.Exit(exit.ExitSuccess)
	}

	if *helpFlag {
		printHelp()
		os.Exit(exit.ExitSuccess)
	}

	// init logger
	log = logging.Init()

	// init global config and read config file if exists
	config.Init()
	if err := config.Read(); err != nil {
		log.Warningf("reading config: %s", err)
	}

	// override default config values with command line flags
	if *filterFlag != "" {
		config.Update("filterStr", *filterFlag)
	}

	if *downloadDirFlag != "" {
		config.Update("downloadDir", *downloadDirFlag)
	}

	if *readOnlyFlag {
		config.UpdateSwitch("readOnly", true)
	}

	// Ensure all containers (running, paused, stopped) are shown by default unless -a flag is passed
	if *activeOnlyFlag {
		config.UpdateSwitch("allContainers", false)
	} else {
		config.UpdateSwitch("allContainers", true)
	}

	if *sortFieldFlag != "" {
		validSort(*sortFieldFlag)
		config.Update("sortField", *sortFieldFlag)
	}

	if *reverseSortFlag {
		config.Toggle("sortReversed")
	}

	// init ui
	if *invertFlag {
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
	// init grid, cursor, header
	cSuper, err := connector.ByName(*connectorFlag)
	if err != nil {
		Shutdown()
		fmt.Fprintf(os.Stderr, "error initializing connector '%s': %v\n", *connectorFlag, err)
		os.Exit(exit.ExitConnector)
	}
	cursor = &GridCursor{cSuper: cSuper}
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

usage: ctop [options]

options:
`

func printHelp() {
	fmt.Println(helpMsg)
	flag.PrintDefaults()
	fmt.Printf("\navailable connectors: ")
	fmt.Println(strings.Join(connector.Enabled(), ", "))
}
