// Package menu provides customizable interactive modal menus, selectable item lists, tooltips, and sorting dialogs.
// Objective: Offer flexible list selection components for sorting, filtering, columns, and contextual actions.
package menu

import (
	"image"
	"sort"
	"strings"

	"github.com/edsilegx/ctop/internal/theme"
	ui "github.com/gizak/termui/v3"
)

type Padding [2]int // x,y padding

// Menu represents a modal menu window with item selection, keyboard scrolling, and tooltip display.
type Menu struct {
	ui.Block
	SortItems    bool   // enable automatic sorting of menu items
	Selectable   bool   // whether menu is navigable
	SubText      string // optional text to display before items
	TextStyle    ui.Style
	SelectedText ui.Style
	cursorPos    int
	items        Items
	padding      Padding
	toolTip      *ToolTip
}

func NewMenu() *Menu {
	m := &Menu{
		Block:        *ui.NewBlock(),
		TextStyle:    theme.Style2("menu.text.fg", "menu.text.bg"),
		SelectedText: theme.Style2("par.text.hi", "menu.text.fg"),
		cursorPos:    0,
		padding:      Padding{3, 1},
	}
	m.BorderStyle = theme.Style("menu.border.fg")
	m.TitleStyle = theme.Style("menu.label.fg")
	m.calcSize()
	return m
}

// AddItems appends items to Menu
func (m *Menu) AddItems(items ...Item) {
	m.items = append(m.items, items...)
	m.refresh()
}

// DelItem removes menu item by value or label
func (m *Menu) DelItem(s string) (success bool) {
	for n, i := range m.items {
		if i.Val == s || i.Label == s {
			m.items = append(m.items[:n], m.items[n+1:]...)
			success = true
			m.refresh()
			break
		}
	}
	return success
}

// ClearItems removes all current menu items
func (m *Menu) ClearItems() {
	m.items = m.items[:0]
}

// SetCursor moves cursor to a position by Item value or label
func (m *Menu) SetCursor(s string) (success bool) {
	for n, i := range m.items {
		if !i.Separator && (i.Val == s || i.Label == s) {
			m.cursorPos = n
			return true
		}
	}
	return false
}

// SetToolTip sets an optional tooltip string to show at bottom of screen
func (m *Menu) SetToolTip(lines ...string) {
	m.toolTip = NewToolTip(lines...)
}

func (m *Menu) SelectedItem() Item {
	if len(m.items) == 0 {
		return Item{}
	}
	if m.cursorPos >= len(m.items) {
		m.cursorPos = len(m.items) - 1
	}
	for m.cursorPos < len(m.items) && m.items[m.cursorPos].Separator {
		m.cursorPos++
	}
	if m.cursorPos >= len(m.items) {
		for m.cursorPos > 0 && m.items[m.cursorPos].Separator {
			m.cursorPos--
		}
	}
	return m.items[m.cursorPos]
}

func (m *Menu) SelectedValue() string {
	return m.SelectedItem().Val
}

func (m *Menu) Draw(buf *ui.Buffer) {
	m.Block.Draw(buf)

	y := m.Inner.Min.Y + m.padding[1]

	if m.SubText != "" {
		x := m.Inner.Min.X + m.padding[0]
		buf.SetString(m.SubText, m.TextStyle, image.Pt(x, y))
		y += 2
	}

	for n, item := range m.items {
		if item.Separator {
			continue
		}
		x := m.Inner.Min.X + m.padding[0]
		style := m.TextStyle
		txt := item.Text()
		if strings.HasPrefix(txt, "──") {
			style = theme.Style("label.fg")
			avail := m.Inner.Max.X - m.padding[0] - x - len(txt)
			if avail > 0 {
				txt = txt + " " + strings.Repeat("─", avail-1)
			}
		} else if m.Selectable && n == m.cursorPos {
			style = m.SelectedText
		}
		buf.SetString(txt, style, image.Pt(x, y+n))
	}

	if m.toolTip != nil {
		m.toolTip.Draw(buf)
	}
}

func (m *Menu) Up() {
	if m.cursorPos > 0 {
		m.cursorPos--
		for m.cursorPos > 0 && m.items[m.cursorPos].Separator {
			m.cursorPos--
		}
		if m.items[m.cursorPos].Separator {
			m.cursorPos = 0
		}
	}
}

func (m *Menu) Down() {
	if m.cursorPos < (len(m.items) - 1) {
		m.cursorPos++
		for m.cursorPos < len(m.items)-1 && m.items[m.cursorPos].Separator {
			m.cursorPos++
		}
		if m.items[m.cursorPos].Separator {
			m.cursorPos = len(m.items) - 1
		}
	}
}

// Sort menu items (if enabled) and re-calculate window size
func (m *Menu) refresh() {
	if m.SortItems {
		sort.Sort(m.items)
	}
	m.calcSize()
}

// Set width and height based on menu items
func (m *Menu) calcSize() {
	minWidth := 7
	for _, i := range m.items {
		if i.Separator {
			continue
		}
		s := i.Text()
		if len(s) > minWidth {
			minWidth = len(s)
		}
	}

	height := len(m.items)
	if m.SubText != "" {
		if len(m.SubText) > minWidth {
			minWidth = len(m.SubText)
		}
		height += 2
	}

	totalWidth := minWidth + (m.padding[0] * 2) + 2
	totalHeight := height + (m.padding[1] * 2) + 2

	termW, termH := theme.TermDimensions()
	if totalWidth > termW {
		totalWidth = termW
	}
	if totalHeight > termH {
		totalHeight = termH
	}

	m.SetRect(1, 1, 1+totalWidth, 1+totalHeight)
}
