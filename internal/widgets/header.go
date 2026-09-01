// Package widgets provides custom TermUI presentation components (Header, StatusLine, ErrorView, TextView, Input).
//
// Objective:
//
//	Render terminal headers, status notifications, scrollable text buffers, and prompt inputs.
//
// Core Components:
//   - CTopHeader: Application header rendering clock, total container counts, and active filter strings.
//   - StatusLine: Ephemeral footer status bar displaying success/error feedback messages.
//   - ErrorView: Full-screen error viewport used during daemon disconnection.
//   - Input: Interactive text input box for modal prompt queries.
//   - TextView: Scrollable multiline text viewer.
package widgets

import (
	"fmt"
	"image"
	"time"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

// CTopHeader displays the top application bar showing live clock, total/filtered container count, and active filter text.
type CTopHeader struct {
	ui.Block
	TimeText   string
	CountText  string
	FilterText string
	Style      ui.Style
}

func NewCTopHeader() *CTopHeader {
	h := &CTopHeader{
		Block:     *ui.NewBlock(),
		CountText: "-",
		Style:     theme.Style2("header.fg", "header.bg"),
	}
	h.Border = false
	h.Align()
	return h
}

func (c *CTopHeader) Draw(buf *ui.Buffer) {
	c.TimeText = timeStr()
	buf.Fill(ui.NewCell(' ', c.Style), c.Rectangle)

	// Draw Time
	buf.SetString(c.TimeText, c.Style, image.Pt(c.Min.X+2, c.Min.Y))

	// Draw Count
	if c.CountText != "" {
		buf.SetString(c.CountText, c.Style, image.Pt(c.Min.X+24, c.Min.Y))
	}

	// Draw Filter
	if c.FilterText != "" {
		buf.SetString(c.FilterText, c.Style, image.Pt(c.Min.X+40, c.Min.Y))
	}
}

func (c *CTopHeader) Align() {
	w, _ := theme.TermDimensions()
	c.SetRect(0, 0, w, 1)
}

func (c *CTopHeader) Height() int {
	return 1
}

func (c *CTopHeader) SetCount(val int) {
	c.CountText = fmt.Sprintf("%d containers", val)
}

func (c *CTopHeader) SetFilter(val string) {
	if val == "" {
		c.FilterText = ""
	} else {
		c.FilterText = fmt.Sprintf("filter: %s", val)
	}
}

func timeStr() string {
	ts := time.Now().Local().Format("15:04:05 MST")
	return fmt.Sprintf("ctop - %s", ts)
}
