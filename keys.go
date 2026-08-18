package main

// Common action keybindings for termui v3 event IDs
var keyMap = map[string][]string{
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

// IsKeyMatch checks if a key ID matches one of the mapped actions
func IsKeyMatch(action, keyID string) bool {
	for _, k := range keyMap[action] {
		if k == keyID {
			return true
		}
	}
	return false
}
