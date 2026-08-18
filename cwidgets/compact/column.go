package compact

import (
	"github.com/bcicen/ctop/config"
	"github.com/bcicen/ctop/models"
	ui "github.com/gizak/termui/v3"
)

var (
	allCols = map[string]NewCompactColFn{
		"status":  NewStatus,
		"name":    NewNameCol,
		"id":      NewCIDCol,
		"image":   NewImageCol,
		"ports":   NewPortsCol,
		"IPs":     NewIpsCol,
		"created": NewCreatedCol,
		"cpu":     NewCPUCol,
		"cpus":    NewCpuScaledCol,
		"mem":     NewMemCol,
		"net":     NewNetCol,
		"io":      NewIOCol,
		"pids":    NewPIDCol,
		"uptime":  NewUptimeCol,
	}
)

type NewCompactColFn func() CompactCol

func newRowWidgets() []CompactCol {
	enabled := config.EnabledColumns()
	cols := make([]CompactCol, len(enabled))

	for n, name := range enabled {
		wFn, ok := allCols[name]
		if !ok {
			panic("no such widget name: " + name)
		}
		cols[n] = wFn()
	}

	return cols
}

type CompactCol interface {
	ui.Drawable
	Reset()
	Header() string  // header text to display for column
	FixedWidth() int // fixed width size. if == 0, width is automatically calculated
	Highlight()
	UnHighlight()
	SetMeta(models.Meta)
	SetMetrics(models.Metrics)
	SetY(y int)
	SetX(x int)
	SetWidth(w int)
}
