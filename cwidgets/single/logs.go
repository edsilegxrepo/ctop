package single

import (
	"time"

	"github.com/bcicen/ctop/models"
	"github.com/bcicen/ctop/theme"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type LogLines struct {
	ts   []time.Time
	data []string
}

func NewLogLines(max int) *LogLines {
	ll := &LogLines{
		ts:   make([]time.Time, max),
		data: make([]string, max),
	}
	return ll
}

func (ll *LogLines) getLines(start, end int) []string {
	if start < 0 {
		start = 0
	}
	if end < 0 || end > len(ll.data) {
		return ll.data[start:]
	}
	return ll.data[start:end]
}

func (ll *LogLines) add(l models.Log) {
	if len(ll.data) == cap(ll.data) {
		ll.data = append(ll.data[:0], ll.data[1:]...)
		ll.ts = append(ll.ts[:0], ll.ts[1:]...)
	}
	ll.ts = append(ll.ts, l.Timestamp)
	ll.data = append(ll.data, l.Message)
	log.Debugf("recorded log line: %v", l)
}

type Logs struct {
	*widgets.List
	lines *LogLines
}

func NewLogs(stream chan models.Log) *Logs {
	w, h := theme.TermDimensions()
	p := widgets.NewList()
	p.TextStyle = theme.Style("par.text.fg")
	p.BorderStyle = theme.Style("border.fg")
	p.TitleStyle = theme.Style("label.fg")
	p.Title = "LOGS"
	p.SetRect(0, h/2, w, h)

	i := &Logs{p, NewLogLines(4098)}
	go func() {
		for line := range stream {
			i.lines.add(line)
			ui.Render(i)
		}
	}()
	return i
}

func (w *Logs) Align() {
	termW, termH := theme.TermDimensions()
	w.SetRect(colWidth[0], 0, termW, termH)
}

func (w *Logs) Draw(buf *ui.Buffer) {
	height := w.Max.Y - w.Min.Y
	maxLines := height - 2
	if maxLines < 0 {
		maxLines = 0
	}
	offset := len(w.lines.data) - maxLines
	if offset < 0 {
		offset = 0
	}
	w.Rows = w.lines.getLines(offset, -1)
	w.List.Draw(buf)
}
