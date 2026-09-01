// Package theme provides visual styling, color palette mappings, style builders, and terminal dimension helpers.
// Objective: Centralize TermUI colors, styling primitives, and headless terminal sizing.
package theme

import (
	"sync"

	ui "github.com/gizak/termui/v3"
	tb "github.com/nsf/termbox-go"
)

var (
	inverted bool
	themeMu  sync.RWMutex

	// ColorMap defines the default dark-theme color palette for all ctop UI components.
	ColorMap = map[string]ui.Color{
		"fg":                 ui.ColorWhite,
		"bg":                 ui.ColorClear,
		"block.bg":           ui.ColorClear,
		"border.bg":          ui.ColorClear,
		"border.fg":          ui.ColorWhite,
		"label.bg":           ui.ColorClear,
		"label.fg":           ui.ColorCyan,
		"menu.text.fg":       ui.ColorWhite,
		"menu.text.bg":       ui.ColorClear,
		"menu.border.fg":     ui.ColorCyan,
		"menu.label.fg":      ui.ColorCyan,
		"header.fg":          ui.ColorBlack,
		"header.bg":          ui.ColorWhite,
		"gauge.bar.bg":       ui.ColorGreen,
		"gauge.percent.fg":   ui.ColorWhite,
		"linechart.axes.fg":  ui.ColorClear,
		"linechart.line.fg":  ui.ColorGreen,
		"mbarchart.bar.bg":   ui.ColorGreen,
		"mbarchart.num.fg":   ui.ColorWhite,
		"mbarchart.text.fg":  ui.ColorWhite,
		"par.text.fg":        ui.ColorWhite,
		"par.text.bg":        ui.ColorClear,
		"par.text.hi":        ui.ColorBlack,
		"sparkline.line.fg":  ui.ColorGreen,
		"sparkline.title.fg": ui.ColorWhite,
		"status.ok":          ui.ColorGreen,
		"status.healthy":     ui.ColorGreen,
		"status.warn":        ui.Color(130), // Calm Muted Corporate Copper-Orange (#AF5F00)
		"status.danger":      ui.ColorRed,
		"status.error":       ui.ColorRed,
		"grid.header.fg":     ui.ColorCyan,
		"cursor.bg":          ui.Color(24),
		"cursor.fg":          ui.ColorWhite,
	}
)

// Color returns the Color associated with the given key
func Color(k string) ui.Color {
	themeMu.RLock()
	defer themeMu.RUnlock()
	if c, ok := ColorMap[k]; ok {
		return c
	}
	return ui.ColorWhite
}

// Style returns a termui.Style with the given foreground key
func Style(fgKey string) ui.Style {
	return ui.NewStyle(Color(fgKey))
}

// Style2 returns a termui.Style with the given foreground and background keys
func Style2(fgKey, bgKey string) ui.Style {
	return ui.NewStyle(Color(fgKey), Color(bgKey))
}

// InvertColorMap inverts the foreground colors for light backgrounds
func InvertColorMap() {
	themeMu.Lock()
	defer themeMu.Unlock()
	if inverted {
		return
	}
	inverted = true
	for k, v := range ColorMap {
		if v == ui.ColorWhite {
			ColorMap[k] = ui.ColorBlack
		}
	}
	ColorMap["grid.header.fg"] = ui.ColorBlue
	ColorMap["cursor.bg"] = ui.Color(153)
	ColorMap["cursor.fg"] = ui.ColorBlack
	ColorMap["par.text.hi"] = ui.ColorWhite
	ColorMap["header.fg"] = ui.ColorWhite
	ColorMap["header.bg"] = ui.ColorBlack
}

// TermDimensions returns terminal width and height without calling tb.Sync()
func TermDimensions() (int, int) {
	if !tb.IsInit {
		return 80, 24
	}
	return tb.Size()
}

// SyncTerm syncs termbox internal buffer on resize
func SyncTerm() {
	if tb.IsInit {
		_ = tb.Sync()
	}
}

// SafeClear safely clears the terminal screen if termbox is initialized
func SafeClear() {
	if tb.IsInit {
		ui.Clear()
	}
}
