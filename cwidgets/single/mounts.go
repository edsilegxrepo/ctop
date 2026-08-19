package single

import (
	"fmt"
	"image"
	"strings"

	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

// Mount represents a single storage volume or bind mount attached to a container.
type Mount struct {
	Destination string
	Source      string
	Type        string
	Mode        string
	Propagation string
}

// Mounts widget displays container storage volumes, bind mounts, and tmpfs mappings.
type Mounts struct {
	ui.Block
	Rows   []Mount
	raw    string
	empty  bool
	Width  int
}

// NewMounts constructs a new Mounts inspection widget.
func NewMounts() *Mounts {
	m := &Mounts{
		Block: *ui.NewBlock(),
		Rows:  []Mount{},
		empty: true,
	}
	m.Title = "VOLUMES & MOUNTS"
	m.BorderStyle = theme.Style("border.fg")
	m.TitleStyle = theme.Style("label.fg")
	m.SetRect(0, 0, colWidth[0], 6)
	return m
}

// Set parses the serialized mounts string (delimited by ';;' and ':::')
func (w *Mounts) Set(mountStr string) {
	w.raw = mountStr
	w.Rows = []Mount{}

	if strings.TrimSpace(mountStr) == "" {
		w.empty = true
		return
	}

	entries := strings.Split(mountStr, ";;")
	for _, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		parts := strings.Split(entry, ":::")
		m := Mount{
			Destination: parts[0],
		}
		if len(parts) > 1 {
			m.Source = parts[1]
		}
		if len(parts) > 2 && parts[2] != "" {
			m.Type = parts[2]
		} else {
			m.Type = "volume"
		}
		if len(parts) > 3 && parts[3] != "" {
			m.Mode = parts[3]
		} else {
			m.Mode = "rw"
		}
		if len(parts) > 4 {
			m.Propagation = parts[4]
		}
		w.Rows = append(w.Rows, m)
	}

	w.empty = len(w.Rows) == 0
}

// GetHeight returns required vertical lines for table and borders
func (w *Mounts) GetHeight() int {
	if w.empty {
		return 5
	}
	return len(w.Rows)*2 + 4 // each mount row + source sub-row + header + borders
}

// Draw renders the formatted mounts table
func (w *Mounts) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	headerStyle := theme.Style("label.fg")
	destStyle := theme.Style("par.text.fg")
	srcStyle := theme.Style("par.text.fg")
	typeStyle := theme.Style("header.fg")
	modeStyle := theme.Style("status.warn")
	roStyle := theme.Style("status.danger")

	y := w.Inner.Min.Y

	if w.empty {
		buf.SetString("No volumes or storage mounts attached.", theme.Style("par.text.fg"), image.Pt(w.Inner.Min.X+2, y+1))
		return
	}

	// Table Header
	header := fmt.Sprintf("%-35s %-10s %-6s %s", "DESTINATION", "TYPE", "MODE", "SOURCE")
	buf.SetString(header, headerStyle, image.Pt(w.Inner.Min.X+1, y))
	y++

	// Separator line
	sepLine := strings.Repeat("─", max(10, w.Inner.Max.X-w.Inner.Min.X-2))
	buf.SetString(sepLine, theme.Style("border.fg"), image.Pt(w.Inner.Min.X+1, y))
	y++

	for _, m := range w.Rows {
		if y >= w.Inner.Max.Y {
			break
		}

		mModeStyle := modeStyle
		if m.Mode == "ro" {
			mModeStyle = roStyle
		}

		// Destination
		dest := m.Destination
		if len(dest) > 34 {
			dest = dest[:31] + "..."
		}
		buf.SetString(dest, destStyle, image.Pt(w.Inner.Min.X+1, y))

		// Type
		buf.SetString(fmt.Sprintf("%-10s", m.Type), typeStyle, image.Pt(w.Inner.Min.X+37, y))

		// Mode
		buf.SetString(fmt.Sprintf("%-6s", m.Mode), mModeStyle, image.Pt(w.Inner.Min.X+48, y))

		// Source (Host path / volume name)
		src := m.Source
		maxSrcLen := w.Inner.Max.X - (w.Inner.Min.X + 55)
		if maxSrcLen > 0 {
			if len(src) > maxSrcLen {
				src = src[:maxSrcLen]
			}
			buf.SetString(src, srcStyle, image.Pt(w.Inner.Min.X+55, y))
		}

		y++
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
