package compact

import (
	"image"

	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/models"
	ui "github.com/gizak/termui/v3"
)

// Status indicator
type Status struct {
	ui.Block
	statusRune  rune
	statusStyle ui.Style
	healthRune  rune
	healthStyle ui.Style
	highlighted bool
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
	height := s.Max.Y - s.Min.Y
	y := s.Min.Y + (height-1)/2

	if s.highlighted {
		hiCell := ui.NewCell(' ', ui.NewStyle(theme.Color("cursor.fg"), theme.Color("cursor.bg")))
		buf.Fill(hiCell, s.Rectangle)
	}

	if s.statusRune != 0 && s.statusRune != ' ' {
		st := s.statusStyle
		if s.highlighted {
			st.Bg = theme.Color("cursor.bg")
		}
		buf.SetCell(ui.NewCell(s.statusRune, st), image.Pt(x, y))
	}
	if s.healthRune != 0 && s.healthRune != ' ' {
		ht := s.healthStyle
		if s.highlighted {
			ht.Bg = theme.Color("cursor.bg")
		}
		buf.SetCell(ui.NewCell(s.healthRune, ht), image.Pt(x+1, y))
	}
}

func (s *Status) SetMeta(m models.Meta) {
	s.setState(m.Get("state"))
	s.setHealth(m.Get("health"))
}

func (s *Status) SetX(x int) {
	s.SetRect(x, s.Min.Y, x+s.FixedWidth(), s.Max.Y)
}

func (s *Status) SetY(y int) {
	s.SetRect(s.Min.X, y, s.Min.X+s.FixedWidth(), y+(s.Max.Y-s.Min.Y))
}

func (s *Status) SetWidth(w int) {
	s.SetRect(s.Min.X, s.Min.Y, s.Min.X+w, s.Max.Y)
}

// Status implements CompactCol
func (s *Status) Reset()                    {}
func (s *Status) SetMetrics(models.Metrics) {}
func (s *Status) Highlight()                { s.highlighted = true }
func (s *Status) UnHighlight()              { s.highlighted = false }
func (s *Status) Header() string            { return "" }
func (s *Status) FixedWidth() int           { return 2 }

func (s *Status) setState(val string) {
	if val == "" {
		return
	}

	style := theme.Style("fg")
	mark := theme.StatusGlyph(val)

	switch val {
	case "running":
		style = theme.Style("status.ok")
	case "exited", "dead":
		style = theme.Style("status.danger")
	case "paused", "restarting":
		style = theme.Style("status.warn")
	case "created":
		style = theme.Style("fg")
	default:
		if mark == ' ' {
			log.Warningf("unknown status string: \"%v\"", val)
		}
	}

	s.statusRune = mark
	s.statusStyle = style
}

func (s *Status) setHealth(val string) {
	if val == "" {
		return
	}

	style := theme.Style("fg")
	mark := theme.HealthGlyph(val)

	switch val {
	case "healthy":
		style = theme.Style("status.ok")
	case "unhealthy":
		style = theme.Style("status.danger")
	case "starting":
		style = theme.Style("status.warn")
	default:
		if mark == ' ' {
			log.Warningf("unknown health state string: \"%v\"", val)
		}
	}

	s.healthRune = mark
	s.healthStyle = style
}
