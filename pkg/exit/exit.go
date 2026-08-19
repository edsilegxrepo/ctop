// Package exit defines standard UNIX exit code constants for application termination scenarios.
// Objective: Standardize process return codes for CLI scripting, monitoring, and error diagnosis.
package exit

const (
	// ExitSuccess indicates a successful, clean termination.
	ExitSuccess = 0
	// ExitGeneral indicates an unhandled runtime error or panic.
	ExitGeneral = 1
	// ExitUsage indicates invalid command-line flags or arguments.
	ExitUsage = 2
	// ExitConfig indicates configuration file loading or validation errors.
	ExitConfig = 3
	// ExitConnector indicates container connector initialization or connection failures.
	ExitConnector = 4
	// ExitUI indicates terminal UI initialization or rendering subsystem failures.
	ExitUI = 5
)
