package compact

import (
	"image"

	"github.com/edsilegx/ctop/models"
	"github.com/edsilegx/ctop/theme"
	ui "github.com/gizak/termui/v3"
)

// Status indicator
type Status struct {
	ui.Block
	statusRune  rune
	statusStyle ui.Style
	healthRune  rune
	healthStyle ui.Style
}

func NewStatus() CompactCol {
	s := &Status{
		Block:       *ui.NewBlock(),
		statusRune:  ' ',
		statusStyle: theme.Style("fg"),
		healthRune:  ' ',
		healthStyle: theme.Style("fg"),
	}
	s.Border = false
	return s
}

func (s *Status) Draw(buf *ui.Buffer) {
	s.Block.Draw(buf)
	x := s.Min.X
	y := s.Min.Y

	if s.statusRune != 0 && s.statusRune != ' ' {
		buf.SetCell(ui.NewCell(s.statusRune, s.statusStyle), image.Pt(x, y))
	}
	if s.healthRune != 0 && s.healthRune != ' ' {
		buf.SetCell(ui.NewCell(s.healthRune, s.healthStyle), image.Pt(x+1, y))
	}
}

func (s *Status) SetMeta(m models.Meta) {
	s.setState(m.Get("state"))
	s.setHealth(m.Get("health"))
}

func (s *Status) SetX(x int) {
	s.SetRect(x, s.Min.Y, x+s.FixedWidth(), s.Min.Y+1)
}

func (s *Status) SetY(y int) {
	s.SetRect(s.Min.X, y, s.Min.X+s.FixedWidth(), y+1)
}

func (s *Status) SetWidth(w int) {
	s.SetRect(s.Min.X, s.Min.Y, s.Min.X+w, s.Min.Y+1)
}

// Status implements CompactCol
func (s *Status) Reset()                    {}
func (s *Status) SetMetrics(models.Metrics) {}
func (s *Status) Highlight()                {}
func (s *Status) UnHighlight()              {}
func (s *Status) Header() string            { return "" }
func (s *Status) FixedWidth() int           { return 2 }

func (s *Status) setState(val string) {
	if val == "" {
		return
	}

	style := theme.Style("fg")
	var mark rune

	switch val {
	case "created":
		mark = '◉'
	case "running":
		mark = '►'
		style = theme.Style("status.ok")
	case "exited":
		mark = '■'
		style = theme.Style("status.danger")
	case "paused":
		mark = '‖'
		style = theme.Style("status.warn")
	default:
		mark = ' '
		log.Warningf("unknown status string: \"%v\"", val)
	}

	s.statusRune = mark
	s.statusStyle = style
}

func (s *Status) setHealth(val string) {
	if val == "" {
		return
	}

	style := theme.Style("fg")
	var mark rune

	switch val {
	case "healthy":
		mark = '☼'
		style = theme.Style("status.ok")
	case "unhealthy":
		mark = '⚠'
		style = theme.Style("status.danger")
	case "starting":
		mark = '◌'
		style = theme.Style("status.warn")
	default:
		mark = ' '
		log.Warningf("unknown health state string: \"%v\"", val)
	}

	s.healthRune = mark
	s.healthStyle = style
}
