// Package service provides system service generation and management helpers for background daemon operation.
//
// Objective:
//
//	Generate and inspect systemd service unit configurations for headless background telemetry collection.
//
// Core Components:
//   - SystemdUnitTemplate: Standard unit definition specifying restart policies, environment variables, and web flags.
//   - GenerateSystemdUnit: Populates the unit template with the absolute binary path.
package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const SystemdUnitTemplate = `[Unit]
Description=ctop - Container Top & Monitoring Telemetry Daemon
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=%s --headless --web :9090 --web-auth-token auto
Restart=always
RestartSec=5s
LimitNOFILE=65536
Environment=CTOP_DOWNLOAD_DIR=/var/log/ctop

[Install]
WantedBy=multi-user.target
`

// GenerateSystemdUnit returns a standard systemd service unit string for the specified binary path.
func GenerateSystemdUnit(binPath string) string {
	if binPath == "" {
		binPath = "/usr/local/bin/ctop"
	}
	return fmt.Sprintf(SystemdUnitTemplate, binPath)
}

// Run processes the 'ctop service' CLI commands (install, uninstall, status, generate).
func Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing service subcommand. Usage: ctop service [install|uninstall|status|generate]")
	}

	subcmd := strings.ToLower(args[0])
	switch subcmd {
	case "generate":
		execPath, err := os.Executable()
		if err != nil {
			execPath = "/usr/local/bin/ctop"
		}
		fmt.Println(GenerateSystemdUnit(execPath))
		return nil

	case "install":
		if runtime.GOOS != "linux" {
			fmt.Printf("Service installation is natively supported on Linux (systemd).\n")
			fmt.Printf("For %s, run ctop in daemon mode with:\n  ctop --headless --web :9090\n", runtime.GOOS)
			return nil
		}
		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to determine executable path: %w", err)
		}
		unitContent := GenerateSystemdUnit(execPath)
		unitPath := "/etc/systemd/system/ctop.service"

		// #nosec G306 -- Systemd service units in /etc/systemd/system require 0644 world-readable permissions for systemctl discovery
		if err := os.WriteFile(filepath.Clean(unitPath), []byte(unitContent), 0o644); err != nil {
			return fmt.Errorf("failed to write %s (ensure sudo/root permissions): %w", unitPath, err)
		}
		fmt.Printf("✓ Installed systemd service to %s\n", unitPath)
		fmt.Printf("Run the following commands to enable and start ctop:\n")
		fmt.Printf("  sudo systemctl daemon-reload\n")
		fmt.Printf("  sudo systemctl enable --now ctop\n")
		return nil

	case "uninstall":
		if runtime.GOOS != "linux" {
			return fmt.Errorf("service uninstallation is only supported on Linux")
		}
		unitPath := "/etc/systemd/system/ctop.service"
		if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", unitPath, err)
		}
		fmt.Printf("✓ Removed systemd service from %s\n", unitPath)
		fmt.Printf("Run 'sudo systemctl daemon-reload' to complete removal.\n")
		return nil

	case "status":
		fmt.Printf("ctop daemon service configuration:\n")
		execPath, _ := os.Executable()
		fmt.Printf("  Executable: %s\n", execPath)
		fmt.Printf("  Target OS:  %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("  Default Unit Path: /etc/systemd/system/ctop.service\n\n")
		fmt.Printf("Unit Definition:\n%s\n", GenerateSystemdUnit(execPath))
		return nil

	default:
		return fmt.Errorf("unknown service subcommand %q. Available: install, uninstall, status, generate", subcmd)
	}
}
