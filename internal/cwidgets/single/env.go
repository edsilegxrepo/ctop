package single

import (
	"image"
	"regexp"
	"strings"

	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/sanitize"
	ui "github.com/gizak/termui/v3"
)

var envPattern = regexp.MustCompile(`(?P<KEY>[^=]+)=(?P<VALUE>.*)`)

type Env struct {
	ui.Block
	Rows   [][]string
	data   map[string]string
	raw    string
	Masked bool
}

func NewEnv() *Env {
	env := &Env{
		Block:  *ui.NewBlock(),
		Rows:   [][]string{},
		data:   make(map[string]string),
		Masked: true,
	}
	env.Title = "ENVIRONMENT VARIABLES [🔒 Secrets Masked - Press 'u' to reveal]"
	env.BorderStyle = theme.Style("border.fg")
	env.TitleStyle = theme.Style("label.fg")
	env.SetRect(0, 0, colWidth[0], 4)
	return env
}

func (w *Env) ToggleMask() {
	w.Masked = !w.Masked
	if w.Masked {
		w.Title = "ENVIRONMENT VARIABLES [🔒 Secrets Masked - Press 'u' to reveal]"
	} else {
		w.Title = "ENVIRONMENT VARIABLES [🔓 Secrets Unmasked - Press 'u' to mask]"
	}
	w.rebuild()
}

func (w *Env) Set(allEnvs string) {
	w.raw = allEnvs
	w.rebuild()
}

func (w *Env) rebuild() {
	envs := strings.Split(w.raw, ";")
	w.Rows = [][]string{}
	w.data = make(map[string]string)
	for _, env := range envs {
		match := envPattern.FindStringSubmatch(env)
		if len(match) == 3 {
			key := match[1]
			value := match[2]
			w.data[key] = value

			displayVal := value
			if w.Masked && sanitize.IsSensitiveKey(key) && len(value) > 0 {
				displayVal = "•••••••••••• [masked]"
			}
			w.Rows = append(w.Rows, mkInfoRows(key, displayVal)...)
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

	keyStyle := theme.Style("grid.header.fg")
	valStyle := theme.Style("par.text.fg")
	maskedStyle := ui.NewStyle(theme.Color("status.warn"))
	sepStyle := theme.Style("border.fg")
	sepCell := ui.NewCell(ui.VERTICAL_LINE, sepStyle)

	maxKeyLen := 15
	for _, row := range w.Rows {
		if len(row[0]) > maxKeyLen {
			maxKeyLen = len(row[0])
		}
	}
	col0Width := maxKeyLen + 2
	maxAllowedCol0 := (w.Inner.Max.X - w.Inner.Min.X) * 45 / 100
	if maxAllowedCol0 > 15 && col0Width > maxAllowedCol0 {
		col0Width = maxAllowedCol0
	}

	sepX := w.Inner.Min.X + col0Width
	valX := sepX + 2

	y := w.Inner.Min.Y
	for _, row := range w.Rows {
		if y >= w.Inner.Max.Y {
			break
		}

		key := row[0]
		val := row[1]

		maxKeyW := sepX - w.Inner.Min.X - 2
		if maxKeyW > 0 && len(key) > maxKeyW {
			key = key[:maxKeyW]
		}

		buf.SetString(key, keyStyle, image.Pt(w.Inner.Min.X+1, y))
		buf.SetCell(sepCell, image.Pt(sepX, y))

		style := valStyle
		if strings.Contains(val, "[masked]") {
			style = maskedStyle
		}

		maxValW := w.Inner.Max.X - valX
		if maxValW > 0 {
			if len(val) > maxValW {
				val = val[:maxValW]
			}
			buf.SetString(val, style, image.Pt(valX, y))
		}

		y++
	}
}
