package widgets

import (
	"image"
	"strings"

	"github.com/bcicen/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

var (
	input_chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-_./:"
)

type Padding [2]int // x,y padding

type Input struct {
	ui.Block
	Data      string
	MaxLen    int
	TextStyle ui.Style
	stream    chan string // stream text as it changes
	padding   Padding
}

func NewInput() *Input {
	w, h := theme.TermDimensions()
	i := &Input{
		Block:     *ui.NewBlock(),
		MaxLen:    30,
		TextStyle: theme.Style2("menu.text.fg", "menu.text.bg"),
		padding:   Padding{2, 1},
	}
	i.Title = "Filter"
	i.BorderStyle = theme.Style("menu.border.fg")
	i.TitleStyle = theme.Style("menu.label.fg")
	i.calcSize(w, h)
	return i
}

func (i *Input) calcSize(termW, termH int) {
	width := i.MaxLen + (i.padding[0] * 2) + 2
	height := 3
	if width > termW {
		width = termW
	}
	y1 := termH - height
	if y1 < 0 {
		y1 = 0
	}
	i.SetRect(0, y1, width, termH)
}

func (i *Input) Draw(buf *ui.Buffer) {
	i.Block.Draw(buf)
	x := i.Inner.Min.X + i.padding[0]
	y := i.Inner.Min.Y
	buf.SetString(i.Data, i.TextStyle, image.Pt(x, y))
}

func (i *Input) Stream() chan string {
	i.stream = make(chan string)
	return i.stream
}

func (i *Input) KeyPress(keyID string) {
	if keyID == "<Backspace>" || keyID == "<C-<Backspace>>" || keyID == "<C-h>" {
		if len(i.Data) > 0 {
			i.Data = i.Data[:len(i.Data)-1]
			if i.stream != nil {
				i.stream <- i.Data
			}
		}
		ui.Render(i)
		return
	}
	if len(i.Data) >= i.MaxLen {
		return
	}
	if len(keyID) == 1 && strings.Contains(input_chars, keyID) {
		i.Data += keyID
		if i.stream != nil {
			i.stream <- i.Data
		}
		ui.Render(i)
	}
}
