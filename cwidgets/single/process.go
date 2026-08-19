package single

import (
	"image"
	"strings"

	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

// Process widget displays container runtime configuration, commands, user, and resource limits.
type Process struct {
	ui.Block
	Rows [][]string
	data map[string]string
}

// NewProcess constructs a new Process & Runtime inspection widget.
func NewProcess() *Process {
	p := &Process{
		Block: *ui.NewBlock(),
		Rows:  [][]string{},
		data:  make(map[string]string),
	}
	p.Title = "PROCESS & RUNTIME"
	p.BorderStyle = theme.Style("border.fg")
	p.TitleStyle = theme.Style("label.fg")
	p.SetRect(0, 0, colWidth[0], 6)
	return p
}

var displayProcess = []string{
	"entrypoint", "cmd", "workdir", "user",
	"restartPolicy", "exitCode", "oomKilled",
	"memLimit", "cpuLimit", "pidsLimit",
	"privileged", "readonlyRootfs",
}

var processLabels = map[string]string{
	"entrypoint":     "Entrypoint",
	"cmd":            "Command",
	"workdir":        "Working Dir",
	"user":           "User / UID",
	"restartPolicy":  "Restart Policy",
	"exitCode":       "Exit Code",
	"oomKilled":      "OOM Killed",
	"memLimit":       "Memory Limit",
	"cpuLimit":       "CPU Allocation",
	"pidsLimit":      "PIDs Limit",
	"privileged":     "Privileged",
	"readonlyRootfs": "Readonly Rootfs",
}

// Set updates process metadata values
func (w *Process) Set(k, v string) {
	if strings.TrimSpace(v) == "" {
		delete(w.data, k)
	} else {
		w.data[k] = v
	}

	w.Rows = [][]string{}
	for _, key := range displayProcess {
		if val, ok := w.data[key]; ok {
			label := processLabels[key]
			if label == "" {
				label = key
			}
			w.Rows = append(w.Rows, mkInfoRows(label, val)...)
		}
	}
}

// GetHeight returns required widget lines
func (w *Process) GetHeight() int {
	if len(w.Rows) == 0 {
		return 4
	}
	return len(w.Rows) + 2
}

// Draw renders process configuration table
func (w *Process) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	keyStyle := theme.Style("header.fg")
	valStyle := theme.Style("par.text.fg")
	sepStyle := theme.Style("border.fg")
	sepCell := ui.NewCell(ui.VERTICAL_LINE, sepStyle)

	col0Width := 16
	sepX := w.Inner.Min.X + col0Width
	valX := sepX + 2

	y := w.Inner.Min.Y

	if len(w.Rows) == 0 {
		buf.SetString("No process runtime parameters available.", valStyle, image.Pt(w.Inner.Min.X+2, y+1))
		return
	}

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
