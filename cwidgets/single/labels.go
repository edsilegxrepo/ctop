package single

import (
	"image"
	"sort"
	"strings"

	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

// Labels widget displays Docker Compose tags and container labels.
type Labels struct {
	ui.Block
	ComposeRows [][]string
	GeneralRows [][]string
	empty       bool
}

// NewLabels constructs a new Labels & Compose inspection widget.
func NewLabels() *Labels {
	l := &Labels{
		Block:       *ui.NewBlock(),
		ComposeRows: [][]string{},
		GeneralRows: [][]string{},
		empty:       true,
	}
	l.Title = "LABELS & COMPOSE"
	l.BorderStyle = theme.Style("border.fg")
	l.TitleStyle = theme.Style("label.fg")
	l.SetRect(0, 0, colWidth[0], 6)
	return l
}

// Set parses the serialized labels string (delimited by ';;' and '=')
func (w *Labels) Set(labelsStr string) {
	w.ComposeRows = [][]string{}
	w.GeneralRows = [][]string{}

	if strings.TrimSpace(labelsStr) == "" {
		w.empty = true
		return
	}

	entries := strings.Split(labelsStr, ";;")
	var composeKeys []string
	var generalKeys []string
	labelMap := make(map[string]string)

	for _, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		k := parts[0]
		v := ""
		if len(parts) > 1 {
			v = parts[1]
		}
		labelMap[k] = v

		if strings.HasPrefix(k, "com.docker.compose.") {
			composeKeys = append(composeKeys, k)
		} else {
			generalKeys = append(generalKeys, k)
		}
	}

	sort.Strings(composeKeys)
	sort.Strings(generalKeys)

	for _, k := range composeKeys {
		shortKey := strings.TrimPrefix(k, "com.docker.compose.")
		w.ComposeRows = append(w.ComposeRows, []string{shortKey, labelMap[k]})
	}

	for _, k := range generalKeys {
		w.GeneralRows = append(w.GeneralRows, []string{k, labelMap[k]})
	}

	w.empty = len(w.ComposeRows) == 0 && len(w.GeneralRows) == 0
}

// GetHeight returns required lines
func (w *Labels) GetHeight() int {
	if w.empty {
		return 5
	}
	h := 2 // borders
	if len(w.ComposeRows) > 0 {
		h += len(w.ComposeRows) + 2
	}
	if len(w.GeneralRows) > 0 {
		h += len(w.GeneralRows) + 2
	}
	return h
}

// Draw renders Compose and General container labels
func (w *Labels) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	keyStyle := theme.Style("header.fg")
	valStyle := theme.Style("par.text.fg")
	subHeaderStyle := theme.Style("status.warn")
	sepStyle := theme.Style("border.fg")
	sepCell := ui.NewCell(ui.VERTICAL_LINE, sepStyle)

	y := w.Inner.Min.Y

	if w.empty {
		buf.SetString("No labels or orchestration tags configured.", valStyle, image.Pt(w.Inner.Min.X+2, y+1))
		return
	}

	// Section 1: Docker Compose Metadata
	if len(w.ComposeRows) > 0 {
		buf.SetString("[ Docker Compose Context ]", subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
		y++

		col0Width := 20
		sepX := w.Inner.Min.X + col0Width
		valX := sepX + 2

		for _, row := range w.ComposeRows {
			if y >= w.Inner.Max.Y {
				break
			}
			buf.SetString(row[0], keyStyle, image.Pt(w.Inner.Min.X+1, y))
			buf.SetCell(sepCell, image.Pt(sepX, y))

			maxValW := w.Inner.Max.X - valX
			if maxValW > 0 {
				val := row[1]
				if len(val) > maxValW {
					val = val[:maxValW]
				}
				buf.SetString(val, valStyle, image.Pt(valX, y))
			}
			y++
		}
		y++ // gap
	}

	// Section 2: General Labels
	if len(w.GeneralRows) > 0 && y < w.Inner.Max.Y {
		buf.SetString("[ Container Labels ]", subHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
		y++

		col0Width := 30
		sepX := w.Inner.Min.X + col0Width
		valX := sepX + 2

		for _, row := range w.GeneralRows {
			if y >= w.Inner.Max.Y {
				break
			}
			k := row[0]
			if len(k) > col0Width-2 {
				k = k[:col0Width-5] + "..."
			}
			buf.SetString(k, keyStyle, image.Pt(w.Inner.Min.X+1, y))
			buf.SetCell(sepCell, image.Pt(sepX, y))

			maxValW := w.Inner.Max.X - valX
			if maxValW > 0 {
				val := row[1]
				if len(val) > maxValW {
					val = val[:maxValW]
				}
				buf.SetString(val, valStyle, image.Pt(valX, y))
			}
			y++
		}
	}
}
