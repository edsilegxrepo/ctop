package single

import (
	"fmt"
	"image"
	"path"
	"path/filepath"
	"strings"

	"github.com/edsilegx/ctop/internal/cwidgets"
	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/models"
	ui "github.com/gizak/termui/v3"
)

// Explorer widget displays interactive container filesystem tree and file preview.
type Explorer struct {
	ui.Block
	CurrentDir      string
	Entries         []models.FileInfo
	Filter          string
	CursorPos       int
	Previewing      bool
	PreviewTxt      string
	StatusMsg       string
	StatusIsErr     bool
	HostDownloadDir string
	empty           bool
}

// NewExplorer constructs a new in-container file explorer widget.
func NewExplorer() *Explorer {
	exp := &Explorer{
		Block:           *ui.NewBlock(),
		CurrentDir:      "/",
		Entries:         []models.FileInfo{},
		Filter:          "",
		CursorPos:       0,
		HostDownloadDir: ".",
		empty:           true,
	}
	exp.Title = "IN-CONTAINER FILE EXPLORER [Enter: Open | v: View | /: Filter | f: Deep Search | d: Download | u: Upload | D: Target | q: Exit]"
	exp.BorderStyle = theme.Style("border.fg")
	exp.TitleStyle = theme.Style("label.fg")
	exp.SetRect(0, 0, colWidth[0], 6)
	return exp
}

// SetDownloadDir sets the active host destination directory for downloads.
func (w *Explorer) SetDownloadDir(dir string) {
	if dir == "" {
		dir = "."
	}
	w.HostDownloadDir = dir
}

// Set updates current directory and its entries.
func (w *Explorer) Set(dirPath string, entries []models.FileInfo) {
	w.CurrentDir = dirPath
	w.Entries = entries
	w.empty = len(entries) == 0
	if w.CursorPos >= len(w.TotalItems()) {
		w.CursorPos = 0
	}
}

// SetFilter sets the active wildcard or substring name filter.
func (w *Explorer) SetFilter(filterStr string) {
	w.Filter = strings.TrimSpace(filterStr)
	if w.CursorPos >= len(w.TotalItems()) {
		w.CursorPos = 0
	}
}

func (w *Explorer) matchFilter(name string) bool {
	if w.Filter == "" {
		return true
	}
	p := strings.ToLower(w.Filter)
	target := strings.ToLower(name)

	// If pattern contains wildcard * or ?
	if strings.ContainsAny(p, "*?") {
		if matched, err := path.Match(p, target); err == nil && matched {
			return true
		}
		if matched, err := filepath.Match(p, target); err == nil && matched {
			return true
		}
		return false
	}

	// Substring match
	return strings.Contains(target, p)
}

// SetStatus sets a temporary status/notification message.
func (w *Explorer) SetStatus(msg string, isErr bool) {
	w.StatusMsg = msg
	w.StatusIsErr = isErr
}

// SetPreview displays text content in preview modal.
func (w *Explorer) SetPreview(content string) {
	w.Previewing = true
	w.PreviewTxt = content
}

// SetWidth sets explorer width
func (w *Explorer) SetWidth(width int) {
	// width is handled during SetRect
}

// ClearPreview closes text content preview.
func (w *Explorer) ClearPreview() {
	w.Previewing = false
	w.PreviewTxt = ""
}

func (w *Explorer) TotalItems() []models.FileInfo {
	var items []models.FileInfo
	if w.CurrentDir != "/" && w.CurrentDir != "" {
		parentPath := path.Dir(w.CurrentDir)
		if parentPath == "" {
			parentPath = "/"
		}
		items = append(items, models.FileInfo{
			Name:  "..",
			Path:  parentPath,
			IsDir: true,
			Mode:  "drwxr-xr-x",
		})
	}
	for _, entry := range w.Entries {
		if w.matchFilter(entry.Name) {
			items = append(items, entry)
		}
	}
	return items
}

// Selected returns the currently highlighted FileInfo.
func (w *Explorer) Selected() (models.FileInfo, bool) {
	items := w.TotalItems()
	if len(items) == 0 || w.CursorPos < 0 || w.CursorPos >= len(items) {
		return models.FileInfo{}, false
	}
	return items[w.CursorPos], true
}

func (w *Explorer) Up() {
	if w.CursorPos > 0 {
		w.CursorPos--
	}
}

func (w *Explorer) Down() {
	items := w.TotalItems()
	if w.CursorPos < len(items)-1 {
		w.CursorPos++
	}
}

func (w *Explorer) Home() {
	w.CursorPos = 0
}

func (w *Explorer) End() {
	items := w.TotalItems()
	if len(items) > 0 {
		w.CursorPos = len(items) - 1
	}
}

func (w *Explorer) PgUp(step int) {
	if step <= 0 {
		step = 10
	}
	w.CursorPos -= step
	if w.CursorPos < 0 {
		w.CursorPos = 0
	}
}

func (w *Explorer) PgDown(step int) {
	items := w.TotalItems()
	if step <= 0 {
		step = 10
	}
	if len(items) == 0 {
		w.CursorPos = 0
		return
	}
	w.CursorPos += step
	if w.CursorPos >= len(items) {
		w.CursorPos = len(items) - 1
	}
}

// GetHeight returns required vertical lines.
func (w *Explorer) GetHeight() int {
	if w.Previewing {
		return len(strings.Split(w.PreviewTxt, "\n")) + 6
	}
	items := w.TotalItems()
	base := 6
	if w.StatusMsg != "" {
		base += 2
	}
	if len(items) == 0 {
		return base
	}
	return len(items) + base
}

// Draw renders directory listing, status banner, and previewer.
func (w *Explorer) Draw(buf *ui.Buffer) {
	w.Block.Draw(buf)

	headerStyle := theme.Style("label.fg")
	dirStyle := theme.Style("status.warn")
	fileStyle := theme.Style("par.text.fg")
	selectedStyle := theme.Style2("par.text.hi", "label.fg")
	previewHeaderStyle := theme.Style("status.warn")
	previewTextStyle := theme.Style("par.text.fg")

	y := w.Inner.Min.Y

	// If in preview mode, render text content view
	if w.Previewing {
		buf.SetString(fmt.Sprintf("[ File Preview: %s ]  (Press Enter/Esc to return)", w.CurrentDir), previewHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
		y++
		sepLine := strings.Repeat("─", max(10, w.Inner.Max.X-w.Inner.Min.X-2))
		buf.SetString(sepLine, theme.Style("border.fg"), image.Pt(w.Inner.Min.X+1, y))
		y++

		lines := strings.Split(w.PreviewTxt, "\n")
		for _, l := range lines {
			if y >= w.Inner.Max.Y {
				break
			}
			maxW := w.Inner.Max.X - w.Inner.Min.X - 3
			if maxW > 0 {
				if len(l) > maxW {
					l = l[:maxW]
				}
				buf.SetString(l, previewTextStyle, image.Pt(w.Inner.Min.X+2, y))
			}
			y++
		}
		return
	}

	// Status banner if present
	if w.StatusMsg != "" {
		bannerStyle := theme.Style("status.ok")
		if w.StatusIsErr {
			bannerStyle = theme.Style("status.danger")
		}
		buf.SetString(w.StatusMsg, bannerStyle, image.Pt(w.Inner.Min.X+1, y))
		y++
	}

	// Breadcrumb, Filter, and Host target bar
	filterBadge := ""
	if w.Filter != "" {
		filterBadge = fmt.Sprintf("   🔍 Filter: [%s]", w.Filter)
	}
	pathInfo := fmt.Sprintf("📁 Directory: %s%s   ⬇ Host Target: %s [D: change]", w.CurrentDir, filterBadge, w.HostDownloadDir)
	buf.SetString(pathInfo, headerStyle, image.Pt(w.Inner.Min.X+1, y))
	y++

	// Table header
	header := fmt.Sprintf("   %-30s %-12s %-10s %s", "NAME", "PERMISSIONS", "SIZE", "MODIFIED")
	buf.SetString(header, theme.Style("label.fg"), image.Pt(w.Inner.Min.X+1, y))
	y++

	sepLine := strings.Repeat("─", max(10, w.Inner.Max.X-w.Inner.Min.X-2))
	buf.SetString(sepLine, theme.Style("border.fg"), image.Pt(w.Inner.Min.X+1, y))
	y++

	items := w.TotalItems()
	if len(items) == 0 {
		buf.SetString("   (empty directory)", fileStyle, image.Pt(w.Inner.Min.X+1, y))
		return
	}

	visibleRows := w.Inner.Max.Y - y
	if visibleRows <= 0 {
		return
	}

	offset := 0
	if w.CursorPos >= visibleRows {
		offset = w.CursorPos - visibleRows + 1
	}
	end := offset + visibleRows
	if end > len(items) {
		end = len(items)
	}

	for idx := offset; idx < end; idx++ {
		item := items[idx]
		isSelected := idx == w.CursorPos
		icon := "📄"
		style := fileStyle
		name := item.Name
		if item.IsDir {
			icon = "📁"
			style = dirStyle
			if item.Name != ".." {
				name += "/"
			}
		}

		if len(name) > 28 {
			name = name[:25] + "..."
		}

		sizeStr := "-"
		if !item.IsDir {
			sizeStr = cwidgets.ByteFormat64(item.Size)
		}

		line := fmt.Sprintf("%s %-28s %-12s %-10s %s", icon, name, item.Mode, sizeStr, item.ModTime)

		if isSelected {
			buf.SetString(fmt.Sprintf("► %s", line), selectedStyle, image.Pt(w.Inner.Min.X+1, y))
		} else {
			buf.SetString(fmt.Sprintf("  %s", line), style, image.Pt(w.Inner.Min.X+1, y))
		}

		y++
	}
}
