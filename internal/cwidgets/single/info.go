package single

import (
	"image"
	"strings"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

var displayInfo = []string{"id", "name", "image", "ports", "IPs", "state", "created", "uptime", "health", "restartPolicy", "exitCode", "oomKilled"}

type Info struct {
	ui.Block
	Rows [][]string
	data map[string]string
}

func NewInfo() *Info {
	info := &Info{
		Block: *ui.NewBlock(),
		Rows:  [][]string{},
		data:  make(map[string]string),
	}
	info.Title = "INFO"
	info.BorderStyle = theme.Style("border.fg")
	info.TitleStyle = theme.Style("label.fg")
	info.SetRect(0, 0, colWidth[0], 4)
	return info
}

func (w *Info) Set(k, v string) {
	w.data[k] = v

	// rebuild rows
	w.Rows = [][]string{}
	for _, k := range displayInfo {
		if v, ok := w.data[k]; ok {
			w.Rows = append(w.Rows, mkInfoRows(k, v)...)
		}
	}

	h := len(w.Rows) + 2
	w.SetRect(w.Min.X, w.Min.Y, w.Min.X+colWidth[0], w.Min.Y+h)
}

func (w *Info) GetHeight() int {
	return len(w.Rows) + 2
}

func (w *Info) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	keyStyle := theme.Style("par.text.fg")
	valStyle := theme.Style("par.text.fg")
	sepStyle := theme.Style("border.fg")
	sepCell := ui.NewCell(ui.VERTICAL_LINE, sepStyle)

	col0Width := 10
	sepX := w.Inner.Min.X + col0Width
	valX := sepX + 2

	y := w.Inner.Min.Y
	for _, row := range w.Rows {
		if y >= w.Inner.Max.Y {
			break
		}

		key := row[0]
		val := row[1]

		buf.SetString(key, keyStyle, image.Pt(w.Inner.Min.X+1, y))
		buf.SetCell(sepCell, image.Pt(sepX, y))

		maxValW := w.Inner.Max.X - valX
		if maxValW > 0 {
			if len(val) > maxValW {
				val = val[:maxValW]
			}
			buf.SetString(val, valStyle, image.Pt(valX, y))
		}

		y++
	}
}

// Build row(s) from a key and value string
func mkInfoRows(k, v string) (rows [][]string) {
	lines := strings.Split(v, "\n")

	// initial row with field name
	rows = append(rows, []string{k, lines[0]})

	// append any additional lines in separate row
	if len(lines) > 1 {
		for _, line := range lines[1:] {
			if line != "" {
				rows = append(rows, []string{"", line})
			}
		}
	}

	return rows
}
