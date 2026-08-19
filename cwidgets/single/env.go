package single

import (
	"image"
	"regexp"
	"strings"

	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

var envPattern = regexp.MustCompile(`(?P<KEY>[^=]+)=(?P<VALUE>.*)`)

type Env struct {
	ui.Block
	Rows [][]string
	data map[string]string
}

func NewEnv() *Env {
	env := &Env{
		Block: *ui.NewBlock(),
		Rows:  [][]string{},
		data:  make(map[string]string),
	}
	env.Title = "ENV"
	env.BorderStyle = theme.Style("border.fg")
	env.TitleStyle = theme.Style("label.fg")
	env.SetRect(0, 0, colWidth[0], 4)
	return env
}

func (w *Env) Set(allEnvs string) {
	envs := strings.Split(allEnvs, ";")
	w.Rows = [][]string{}
	for _, env := range envs {
		match := envPattern.FindStringSubmatch(env)
		if len(match) == 3 {
			key := match[1]
			value := match[2]
			w.data[key] = value
			w.Rows = append(w.Rows, mkInfoRows(key, value)...)
		}
	}

	h := len(w.Rows) + 2
	w.SetRect(w.Min.X, w.Min.Y, w.Min.X+colWidth[0], w.Min.Y+h)
}

func (w *Env) GetHeight() int {
	return len(w.Rows) + 2
}

func (w *Env) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	keyStyle := theme.Style("par.text.fg")
	valStyle := theme.Style("par.text.fg")
	sepStyle := theme.Style("border.fg")
	sepCell := ui.NewCell(ui.VERTICAL_LINE, sepStyle)

	col0Width := 20
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
