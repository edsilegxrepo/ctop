// Package manager defines container lifecycle control interfaces (Start, Stop, Pause, Unpause, Restart, Exec).
// Objective: Abstract runtime lifecycle operations across Docker, runC, and Mock backends.
package manager

import (
	"errors"

	"github.com/edsilegx/ctop/pkg/models"
)

var ErrActionNotImpl = errors.New("action not implemented")

// Manager defines the interface for interacting with container processes and runtime lifecycles.
type Manager interface {
	Start() error
	Stop() error
	Remove() error
	Pause() error
	Unpause() error
	Restart() error
	Exec(cmd []string) error
	Kill(signal string) error
	Top(args string) (models.TopResult, error)
	Changes() ([]models.Change, error)
	ReadDir(path string) ([]models.FileInfo, error)
	ReadFile(path string, maxBytes int64) (string, error)
	Download(srcPath, dstPath string) (int64, error)
	Upload(srcHostPath, dstContainerPath string) error
	UpdateResources(memoryMB int64, cpus float64, restartPolicy string) error
}
