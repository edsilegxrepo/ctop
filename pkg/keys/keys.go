// Package keys defines keyboard shortcut bindings and key event matching for terminal interactions.
//
// Objective:
//
//	Centralize TermUI key code mapping to abstract navigation and hotkeys into logical action names.
//
// Core Components:
//   - KeyMap: Canonical map binding logical action names ("up", "down", "pgup", "pgdown", "exit", "help") to termui key identifiers.
//   - IsKeyMatch: Action predicate matching incoming event IDs against registered key chords.
package keys

// KeyMap maps standard terminal actions to corresponding termui/termbox key event identifiers.
var KeyMap = map[string][]string{
	"up": {
		"<Up>",
		"k",
	},
	"down": {
		"<Down>",
		"j",
	},
	"pgup": {
		"<PageUp>",
		"<C-u>",
		"<Previous>",
	},
	"pgdown": {
		"<PageDown>",
		"<C-d>",
		"<Next>",
	},
	"exit": {
		"q",
		"<C-c>",
		"<Escape>",
	},
	"help": {
		"h",
		"?",
	},
	"home": {
		"<Home>",
		"g",
		"<C-a>",
	},
	"end": {
		"<End>",
		"G",
		"<C-e>",
	},
	"enter": {
		"<Enter>",
	},
}

// IsKeyMatch checks if a key ID matches one of the mapped actions.
func IsKeyMatch(action, keyID string) bool {
	keys, ok := KeyMap[action]
	if !ok {
		return false
	}
	for _, k := range keys {
		if k == keyID {
			return true
		}
	}
	return false
}
