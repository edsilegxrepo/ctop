package main

import (
	"github.com/bcicen/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

var (
	ColorMap       = theme.ColorMap
	InvertColorMap = theme.InvertColorMap
)

func initTheme() {
	// Sync global widget themes in termui v3 if needed
	ui.Theme.Block.Title = theme.Style("label.fg")
	ui.Theme.Block.Border = theme.Style("border.fg")
	ui.Theme.Paragraph.Text = theme.Style("par.text.fg")
}
