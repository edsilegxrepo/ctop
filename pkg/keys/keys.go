// Package keys defines keyboard shortcut bindings and key event matching for terminal interactions.
// Objective: Centralize TermUI key code mapping to abstract navigation and hotkeys.
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
