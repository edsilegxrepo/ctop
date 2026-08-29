package single

import (
	"image"
	"strings"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

// Generator widget displays generated docker run command and docker-compose.yml snippet.
type Generator struct {
	ui.Block
	RunCmd  string
	Compose string
}

// NewGenerator constructs a new container generator widget.
func NewGenerator() *Generator {
	g := &Generator{
		Block: *ui.NewBlock(),
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
}

// GetHeight returns required vertical lines.
func (w *Generator) GetHeight() int {
	h := 4
	if w.RunCmd != "" {
		h += len(strings.Split(w.RunCmd, "\n")) + 2
	}
	if w.Compose != "" {
		h += len(strings.Split(w.Compose, "\n")) + 2
	}
	return h
}

// Draw renders the formatted run command and compose yaml.
func (w *Generator) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	subHeaderStyle := theme.Style("status.warn")
	cmdStyle := theme.Style("par.text.fg")

	y := w.Inner.Min.Y

	// Section 1: Docker Run Command
	buf.SetString("[ Equivalent Docker Run Command ]", subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
	y++

	if w.RunCmd != "" {
		for _, line := range strings.Split(w.RunCmd, "\n") {
			if y >= w.Inner.Max.Y {
				break
			}
			maxW := w.Inner.Max.X - w.Inner.Min.X - 3
			if maxW > 0 {
				if len(line) > maxW {
					line = line[:maxW]
				}
				buf.SetString(line, cmdStyle, image.Pt(w.Inner.Min.X+2, y))
			}
			y++
		}
	} else {
		buf.SetString("No run command generated.", cmdStyle, image.Pt(w.Inner.Min.X+2, y))
		y++
	}

	y++ // gap

	// Section 2: docker-compose.yml
	buf.SetString("[ Equivalent docker-compose.yml ]", subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
	y++

	if w.Compose != "" {
		for _, line := range strings.Split(w.Compose, "\n") {
			if y >= w.Inner.Max.Y {
				break
			}
			maxW := w.Inner.Max.X - w.Inner.Min.X - 3
			if maxW > 0 {
				if len(line) > maxW {
					line = line[:maxW]
				}
				buf.SetString(line, cmdStyle, image.Pt(w.Inner.Min.X+2, y))
			}
			y++
		}
	} else {
		buf.SetString("No docker-compose specification generated.", cmdStyle, image.Pt(w.Inner.Min.X+2, y))
	}
}
