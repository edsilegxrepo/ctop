// icons.go manages glyph rendering sets supporting both standard Unicode and Nerd Font icon suites.
//
// Objective:
//
//	Provide visually appealing status indicators, health symbols, and file type icons across diverse terminal font capabilities.
//
// Functionality:
//   - Toggles between IconStyleUnicode and IconStyleNerd.
//   - StatusGlyph / HealthGlyph / ExtGlyph: Resolves container states, health statuses, and file extensions to runes.
package theme

import (
	"sync"
)

// IconStyle represents the active icon rendering set ("unicode" vs "nerd").
type IconStyle string

const (
	IconStyleUnicode IconStyle = "unicode"
	IconStyleNerd    IconStyle = "nerd"
)

var (
	iconMu      sync.RWMutex
	activeStyle = IconStyleUnicode

	// UnicodeGlyphs defines the standard fallback characters supported in all terminals.
	UnicodeGlyphs = map[string]rune{
		"state.created":    '◉',
		"state.running":    '►',
		"state.exited":     '■',
		"state.paused":     '‖',
		"state.dead":       '✖',
		"state.restarting": '↻',
		"health.healthy":   '☼',
		"health.unhealthy": '⚠',
		"health.starting":  '◌',
		"action.start":     '▶',
		"action.stop":      '■',
		"action.pause":     '❚',
		"action.restart":   '↻',
		"action.remove":    '🗑',
		"docker.whale":     '⚓',
	}

	// NerdGlyphs defines rich icons for terminals running a patched Nerd Font.
	NerdGlyphs = map[string]rune{
		"state.created":    '\uf1ce', //  circle-o-notch
		"state.running":    '\uf04b', //  play
		"state.exited":     '\uf04d', //  stop
		"state.paused":     '\uf04c', //  pause
		"state.dead":       '\uf00d', //  times / x
		"state.restarting": '\uf021', //  refresh
		"health.healthy":   '\uf058', //  check-circle
		"health.unhealthy": '\uf071', //  exclamation-triangle
		"health.starting":  '\uf110', //  spinner
		"action.start":     '\uf04b', //  play
		"action.stop":      '\uf04d', //  stop
		"action.pause":     '\uf04c', //  pause
		"action.restart":   '\uf021', //  refresh
		"action.remove":    '\uf1f8', //  trash
		"docker.whale":     '\uf308', //  docker whale icon
	}
)

// SetIconStyle sets the icon style ("unicode" or "nerd").
func SetIconStyle(style string) {
	iconMu.Lock()
	defer iconMu.Unlock()
	if style == string(IconStyleNerd) {
		activeStyle = IconStyleNerd
	} else {
		activeStyle = IconStyleUnicode
	}
}

// GetIconStyle returns the currently configured IconStyle.
func GetIconStyle() IconStyle {
	iconMu.RLock()
	defer iconMu.RUnlock()
	return activeStyle
}

// Glyph returns the rune for the given icon key according to the active icon style.
func Glyph(key string) rune {
	iconMu.RLock()
	defer iconMu.RUnlock()

	if activeStyle == IconStyleNerd {
		if r, ok := NerdGlyphs[key]; ok {
			return r
		}
	}
	if r, ok := UnicodeGlyphs[key]; ok {
		return r
	}
	return ' '
}

// StatusGlyph returns the status rune for a container lifecycle state.
func StatusGlyph(state string) rune {
	return Glyph("state." + state)
}

// HealthGlyph returns the health rune for a container health state.
func HealthGlyph(health string) rune {
	return Glyph("health." + health)
}
