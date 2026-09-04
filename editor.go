package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/edsilegx/ctop/pkg/container"
	tb "github.com/nsf/termbox-go"
)

// EditContainerFile downloads a container file to a temporary file,
// opens it with the host's preferred $EDITOR, and uploads the file back to the container if modified.
// Returns whether the file was modified and any error encountered.
func EditContainerFile(c *container.Container, containerPath string) (bool, error) {
	if c == nil {
		return false, fmt.Errorf("container is nil")
	}
	if containerPath == "" {
		return false, fmt.Errorf("file path is required")
	}

	ext := filepath.Ext(containerPath)
	base := strings.TrimSuffix(filepath.Base(containerPath), ext)
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("ctop-edit-%s-*%s", base, ext))
	if err != nil {
		return false, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	// Download file from container into tmpPath
	if _, err := c.Download(containerPath, tmpPath); err != nil {
		// Fallback to ReadFile if direct download had an issue
		if content, rErr := c.ReadFile(containerPath, 10*1024*1024); rErr == nil {
			if wErr := os.WriteFile(tmpPath, []byte(content), 0o600); wErr != nil {
				return false, fmt.Errorf("failed to write temp file for editing: %w", wErr)
			}
		} else {
			return false, fmt.Errorf("failed to retrieve file for editing: %w", err)
		}
	}

	initialInfo, err := os.Stat(tmpPath)
	if err != nil {
		return false, err
	}
	initialMod := initialInfo.ModTime()
	initialSize := initialInfo.Size()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad.exe"
		} else {
			for _, ed := range []string{"nano", "vim", "vi"} {
				if _, err := exec.LookPath(ed); err == nil {
					editor = ed
					break
				}
			}
			if editor == "" {
				editor = "vi"
			}
		}
	}

	parts := strings.Fields(editor)
	if len(parts) == 0 {
		parts = []string{"vi"}
	}

	editorBin, err := exec.LookPath(parts[0])
	if err != nil {
		return false, fmt.Errorf("editor executable %q not found in PATH: %w", parts[0], err)
	}
	cleanBin := filepath.Clean(editorBin)

	wasInit := tb.IsInit
	if wasInit {
		tb.Close()
	}

	// #nosec G204,G702 -- Launching user-configured $EDITOR or $VISUAL interactive editor requires dynamic command execution after LookPath verification
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- dynamic editor command execution after LookPath verification
	cmd := exec.Command(cleanBin, append(parts[1:], tmpPath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	execErr := cmd.Run()

	if wasInit {
		_ = tb.Init()
		tb.SetInputMode(tb.InputEsc)
		tb.HideCursor()
		_ = tb.Sync()
	}

	if execErr != nil {
		return false, fmt.Errorf("editor (%s) exited with error: %w", editor, execErr)
	}

	newInfo, err := os.Stat(tmpPath)
	if err != nil {
		return false, err
	}
	if newInfo.ModTime().Equal(initialMod) && newInfo.Size() == initialSize {
		return false, nil
	}

	if err := c.Upload(tmpPath, containerPath); err != nil {
		return false, fmt.Errorf("failed to upload edited file: %w", err)
	}

	return true, nil
}
