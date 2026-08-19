package widgets

import (
	"image"
	"strings"
	"sync"

	"github.com/edsilegx/ctop/pkg/sanitize"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
	"github.com/mattn/go-runewidth"
	tb "github.com/nsf/termbox-go"
)

type ToggleText interface {
	// returns text for toggle on/off
	Toggle(on bool) string
}

type TextView struct {
	ui.Block
	mu          sync.Mutex
	inputStream <-chan ToggleText
	render      chan bool
	toggleState bool
	filterStr   string
	paused      bool
	Text        []ToggleText // all the text
	TextOut     []string     // text to be displayed
	TextStyle   ui.Style
	padding     Padding
}

func NewTextView(lines <-chan ToggleText) *TextView {
	w, h := theme.TermDimensions()
	t := &TextView{
		Block:       *ui.NewBlock(),
		inputStream: lines,
		render:      make(chan bool, 10),
		Text:        []ToggleText{},
		TextOut:     []string{},
		TextStyle:   theme.Style2("menu.text.fg", "menu.text.bg"),
		padding:     Padding{4, 2},
	}

	t.BorderStyle = theme.Style("menu.border.fg")
	t.TitleStyle = theme.Style("menu.label.fg")
	t.SetRect(0, 0, w, h)

	t.readInputLoop()
	t.renderLoop()
	return t
}

// Resize adjusts view according to window size
func (t *TextView) Resize() {
	if tb.IsInit {
		ui.Clear()
	}
	w, h := theme.TermDimensions()
	t.SetRect(0, 0, w, h)
	t.queueRender()
}

// SetRect sets block boundaries
func (t *TextView) SetRect(x1, y1, x2, y2 int) {
	t.Block.SetRect(x1, y1, x2, y2)
}

// Toggle toggles text display format
func (t *TextView) Toggle() {
	t.mu.Lock()
	t.toggleState = !t.toggleState
	t.recomputeTextOut()
	t.mu.Unlock()
	t.queueRender()
}

// Pause pauses automatic background redraws
func (t *TextView) Pause() {
	t.mu.Lock()
	t.paused = true
	t.mu.Unlock()
}

// Resume resumes automatic background redraws
func (t *TextView) Resume() {
	t.mu.Lock()
	t.paused = false
	t.recomputeTextOut()
	t.mu.Unlock()
	t.queueRender()
}

// IsPaused returns whether background redraws are paused
func (t *TextView) IsPaused() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.paused
}

// SetFilter sets the active filter substring
func (t *TextView) SetFilter(f string) {
	t.mu.Lock()
	t.filterStr = f
	t.recomputeTextOut()
	t.mu.Unlock()
	t.queueRender()
}

// Filter returns current filter substring
func (t *TextView) Filter() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.filterStr
}

func (t *TextView) queueRender() {
	select {
	case t.render <- true:
	default:
	}
}

// RecomputeTextOut calculates displayed lines based on dimensions, filter, and wrap
func (t *TextView) RecomputeTextOut() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recomputeTextOut()
}

func (t *TextView) recomputeTextOut() {
	maxWidth := (t.Inner.Max.X - t.Inner.Min.X) - (t.padding[0] * 2)
	height := (t.Inner.Max.Y - t.Inner.Min.Y) - (t.padding[1] * 2)
	if maxWidth <= 0 || height <= 0 {
		return
	}
	t.TextOut = []string{}
	for i := len(t.Text) - 1; i >= 0; i-- {
		raw := sanitize.StripANSI(t.Text[i].Toggle(t.toggleState))
		if t.filterStr != "" && !strings.Contains(strings.ToLower(raw), strings.ToLower(t.filterStr)) {
			continue
		}
		lines := splitLine(raw, maxWidth)
		for j := len(lines) - 1; j >= 0; j-- {
			t.TextOut = append([]string{lines[j]}, t.TextOut...)
			if len(t.TextOut) >= height {
				break
			}
		}
		if len(t.TextOut) >= height {
			break
		}
	}
}

func (t *TextView) Draw(buf *ui.Buffer) {
	t.Block.Draw(buf)

	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.TextOut) == 0 {
		msg := "(no logs available)"
		if t.filterStr != "" {
			msg = "(no logs matching filter)"
		}
		buf.SetString(msg, theme.Style("menu.text.fg"), image.Pt(t.Inner.Min.X+t.padding[0], t.Inner.Min.Y+t.padding[1]))
		return
	}

	x := t.Inner.Min.X + t.padding[0]
	y := t.Inner.Min.Y + t.padding[1]

	tsStyle := ui.NewStyle(ui.ColorCyan, theme.Color("bg"))

	for _, line := range t.TextOut {
		if y >= t.Inner.Max.Y-t.padding[1] {
			break
		}
		currX := x
		runes := []rune(line)
		isTsLine := t.toggleState && len(runes) >= 25 && runes[4] == '-' && runes[7] == '-' && runes[10] == ' ' && runes[13] == ':'
		for idx, ch := range runes {
			if currX >= t.Inner.Max.X-t.padding[0] {
				break
			}
			cellStyle := t.TextStyle
			if isTsLine && idx < 23 {
				cellStyle = tsStyle
			}
			buf.SetCell(ui.NewCell(ch, cellStyle), image.Pt(currX, y))
			currX += runewidth.RuneWidth(ch)
		}
		y++
	}
}

func (t *TextView) renderLoop() {
	go func() {
		for range t.render {
			t.mu.Lock()
			if t.paused {
				t.mu.Unlock()
				continue
			}
			t.recomputeTextOut()
			t.mu.Unlock()
			if tb.IsInit {
				ui.Render(t)
			}
		}
	}()
}

func (t *TextView) readInputLoop() {
	go func() {
		for line := range t.inputStream {
			t.mu.Lock()
			t.Text = append(t.Text, line)
			t.mu.Unlock()
			t.queueRender()
		}
	}()
}

func splitLine(line string, lineSize int) []string {
	if line == "" || lineSize <= 0 {
		return []string{}
	}

	var lines []string
	for {
		if len(line) <= lineSize {
			lines = append(lines, line)
			return lines
		}
		lines = append(lines, line[:lineSize])
		line = line[lineSize:]
	}
}
