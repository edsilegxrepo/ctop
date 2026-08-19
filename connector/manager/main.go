// Package manager defines container lifecycle control interfaces (Start, Stop, Pause, Unpause, Restart, Exec).
// Objective: Abstract runtime lifecycle operations across Docker, runC, and Mock backends.
package manager

import "errors"

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
}
