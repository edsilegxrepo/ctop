package widgets

import (
	"image"
	"strings"

	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

var input_chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-_./:~@+=, #$%&*()[]{}<>?!"

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
		MaxLen:    256,
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
	titleLen := len(i.Title)
	needed := 72
	if titleLen+8 > needed {
		needed = titleLen + 8
	}
	width := needed + (i.padding[0] * 2) + 2
	if width > termW-2 {
		width = termW - 2
	}
	if width < 20 {
		width = termW
	}

	x0 := (termW - width) / 2
	if x0 < 0 {
		x0 = 0
	}
	height := 3
	y1 := (termH - height) / 2
	if y1 < 0 {
		y1 = 0
	}
	i.SetRect(x0, y1, x0+width, y1+height)
}

func (i *Input) Align() {
	w, h := theme.TermDimensions()
	i.calcSize(w, h)
}

func (i *Input) Draw(buf *ui.Buffer) {
	w, h := theme.TermDimensions()
	if i.Rectangle.Dx() < len(i.Title)+6 || i.Rectangle.Dx() < 50 {
		i.calcSize(w, h)
	}
	i.Block.Draw(buf)
	x := i.Inner.Min.X + i.padding[0]
	y := i.Inner.Min.Y
	txt := i.Data
	maxVisible := (i.Inner.Max.X - i.Inner.Min.X) - (i.padding[0] * 2)
	if maxVisible > 0 && len(txt) > maxVisible {
		txt = txt[len(txt)-maxVisible:]
	}
	buf.SetString(txt, i.TextStyle, image.Pt(x, y))
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
	}
}
