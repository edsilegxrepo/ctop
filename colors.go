// colors.go bridges the application theme subsystem to the global TermUI theme configuration.
package main

import (
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

var (
	// ColorMap aliases the active color palette definition from the theme package.
	ColorMap = theme.ColorMap
	// InvertColorMap toggles light/dark color mappings across widgets.
	InvertColorMap = theme.InvertColorMap
)

// initTheme synchronizes termui's global theme variables with the ctop color palette.
func initTheme() {
	// Sync global widget themes in termui v3 if needed
	ui.Theme.Block.Title = theme.Style("label.fg")
	ui.Theme.Block.Border = theme.Style("border.fg")
	ui.Theme.Paragraph.Text = theme.Style("par.text.fg")
}
