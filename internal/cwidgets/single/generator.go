package single

import (
	"fmt"
	"image"
	"strings"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

type genLine struct {
	isHeader bool
	text     string
}

// Generator widget displays generated docker run command and docker-compose.yml snippet.
type Generator struct {
	ui.Block
	RunCmd  string
	Compose string
	Offset  int
}

// NewGenerator constructs a new container generator widget.
func NewGenerator() *Generator {
	g := &Generator{
		Block:  *ui.NewBlock(),
		Offset: 0,
	}
	g.Title = "RECREATE & COMPOSE GENERATOR"
	g.BorderStyle = theme.Style("border.fg")
	g.TitleStyle = theme.Style("label.fg")
	g.SetRect(0, 0, colWidth[0], 6)
	return g
}

// Set updates the generated command strings.
func (w *Generator) Set(runCmd, compose string) {
	w.RunCmd = runCmd
	w.Compose = compose
	w.Offset = 0
}

func (w *Generator) buildLines() []genLine {
	var lines []genLine

	// Section 1: Docker Run Command
	lines = append(lines, genLine{isHeader: true, text: "[ Equivalent Docker Run Command ]"})
	if w.RunCmd != "" {
		for _, line := range strings.Split(w.RunCmd, "\n") {
			lines = append(lines, genLine{text: line})
		}
	} else {
		lines = append(lines, genLine{text: "No run command generated."})
	}

	lines = append(lines, genLine{}) // blank line

	// Section 2: docker-compose.yml
	lines = append(lines, genLine{isHeader: true, text: "[ Equivalent docker-compose.yml ]"})
	if w.Compose != "" {
		for _, line := range strings.Split(w.Compose, "\n") {
			lines = append(lines, genLine{text: line})
		}
	} else {
		lines = append(lines, genLine{text: "No docker-compose specification generated."})
	}

	return lines
}

// Up scrolls content up
func (w *Generator) Up() {
	if w.Offset > 0 {
		w.Offset--
	}
}

// Down scrolls content down
func (w *Generator) Down() {
	lines := w.buildLines()
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	if visibleH <= 0 {
		visibleH = 1
	}
	maxOffset := len(lines) - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if w.Offset < maxOffset {
		w.Offset++
	}
}

// PgUp scrolls one page up
func (w *Generator) PgUp() {
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	step := visibleH / 2
	if step < 1 {
		step = 1
	}
	w.Offset -= step
	if w.Offset < 0 {
		w.Offset = 0
	}
}

// PgDown scrolls one page down
func (w *Generator) PgDown() {
	lines := w.buildLines()
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	if visibleH <= 0 {
		visibleH = 1
	}
	maxOffset := len(lines) - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}
	step := visibleH / 2
	if step < 1 {
		step = 1
	}
	w.Offset += step
	if w.Offset > maxOffset {
		w.Offset = maxOffset
	}
}

// GetHeight returns required vertical lines.
func (w *Generator) GetHeight() int {
	return len(w.buildLines()) + 2
}

// Draw renders the formatted run command and compose yaml.
func (w *Generator) Draw(buf *ui.Buffer) {
	lines := w.buildLines()
	visibleH := w.Inner.Max.Y - w.Inner.Min.Y
	if visibleH <= 0 {
		visibleH = 1
	}

	maxOffset := len(lines) - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if w.Offset > maxOffset {
		w.Offset = maxOffset
	}

	if len(lines) > visibleH {
		endLine := w.Offset + visibleH
		if endLine > len(lines) {
			endLine = len(lines)
		}
		w.Title = fmt.Sprintf("RECREATE & COMPOSE GENERATOR [%d-%d/%d | ▲▼/PgUp/PgDn]", w.Offset+1, endLine, len(lines))
	} else {
		w.Title = "RECREATE & COMPOSE GENERATOR"
	}

	w.Block.Draw(buf)

	subHeaderStyle := theme.Style("status.warn")
	cmdStyle := theme.Style("par.text.fg")

	y := w.Inner.Min.Y

	endIdx := w.Offset + visibleH
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	for idx := w.Offset; idx < endIdx; idx++ {
		if y >= w.Inner.Max.Y {
			break
		}
		line := lines[idx]
		if line.isHeader {
			buf.SetString(line.text, subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
		} else if line.text != "" {
			maxW := w.Inner.Max.X - w.Inner.Min.X - 3
			txt := line.text
			if maxW > 0 {
				if len(txt) > maxW {
					txt = txt[:maxW-1] + "…"
				}
				buf.SetString(txt, cmdStyle, image.Pt(w.Inner.Min.X+2, y))
			}
		}
		y++
	}
}
