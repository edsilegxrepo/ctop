package single

import (
	"fmt"
	"image"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/edsilegx/ctop/internal/cwidgets"
	"github.com/edsilegx/ctop/internal/theme"
	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/models"
	ui "github.com/gizak/termui/v3"
	"github.com/mattn/go-runewidth"
)

// InlineEditMode represents the active inline text entry mode in File Explorer.
type InlineEditMode int

const (
	EditModeNone InlineEditMode = iota
	EditModeTargetDir
	EditModeFilter
	EditModeDeepSearch
	EditModeUpload
	EditModeConfirmDelete
	EditModeConfirmEdit
)

var iconSpacingRegex = regexp.MustCompile(`([ℹ🗑✏📁📄📤⬇🔍🔎✔❌])([ \t]{0,1})([^\s])`)

// ensureIconSpacing ensures that unicode icons/emojis have at least 2 spaces after them
// so that wide terminal glyphs do not collide with adjacent text in terminal emulators.
func ensureIconSpacing(s string) string {
	return iconSpacingRegex.ReplaceAllString(s, "${1}  ${3}")
}

// Explorer widget displays interactive container filesystem tree and file preview.
type Explorer struct {
	ui.Block
	CurrentDir        string
	Entries           []models.FileInfo
	Filter            string
	CursorPos         int
	ScrollOffset      int
	Previewing        bool
	PreviewTxt        string
	PreviewOffset     int
	PreviewPath       string
	StatusMsg         string
	StatusIsErr       bool
	HostDownloadDir   string
	EditMode          InlineEditMode
	InputBuffer       string
	IsDeepSearch      bool
	DeepSearchTerm    string
	EditingTargetDir  bool
	EditDirBuffer     string
	PendingActionItem models.FileInfo
	empty             bool
}

// NewExplorer constructs a new in-container file explorer widget.
func NewExplorer() *Explorer {
	exp := &Explorer{
		Block:           *ui.NewBlock(),
		CurrentDir:      "/",
		Entries:         []models.FileInfo{},
		Filter:          "",
		CursorPos:       0,
		HostDownloadDir: config.GetDownloadDir(),
		empty:           true,
	}
	exp.Title = "CONTAINER EXPLORER [Enter: Open | v: View | e: Edit | x: Delete | /: Filter | f: Search | c: Clear | d: Download | u: Upload | q: Exit]"
	exp.BorderStyle = theme.Style("border.fg")
	exp.TitleStyle = theme.Style("label.fg")
	exp.SetRect(0, 0, colWidth[0], 6)
	return exp
}

// SetDownloadDir sets the active host destination directory for downloads.
func (w *Explorer) SetDownloadDir(dir string) {
	if dir == "" {
		dir = config.GetDownloadDir()
	}
	w.HostDownloadDir = dir
}

// StartEditTargetDir enters inline host target directory editing mode.
func (w *Explorer) StartEditTargetDir() {
	w.EditMode = EditModeTargetDir
	w.EditingTargetDir = true
	w.InputBuffer = w.HostDownloadDir
	w.EditDirBuffer = w.HostDownloadDir
}

// StartEditFilter enters inline filter editing mode.
func (w *Explorer) StartEditFilter() {
	w.EditMode = EditModeFilter
	w.InputBuffer = w.Filter
}

// StartEditDeepSearch enters inline deep search editing mode.
func (w *Explorer) StartEditDeepSearch() {
	w.EditMode = EditModeDeepSearch
	w.InputBuffer = ""
}

// StartEditUpload enters inline host upload path editing mode.
func (w *Explorer) StartEditUpload() {
	w.EditMode = EditModeUpload
	w.InputBuffer = ""
}

// StartConfirmDelete enters inline confirmation mode for deleting a file.
func (w *Explorer) StartConfirmDelete(item models.FileInfo) {
	w.EditMode = EditModeConfirmDelete
	w.PendingActionItem = item
	w.StatusMsg = ""
}

// StartConfirmEdit enters inline confirmation mode for editing a file.
func (w *Explorer) StartConfirmEdit(item models.FileInfo) {
	w.EditMode = EditModeConfirmEdit
	w.PendingActionItem = item
	w.StatusMsg = ""
}

// CancelEdit exits any inline editing mode without saving.
func (w *Explorer) CancelEdit() {
	w.EditMode = EditModeNone
	w.EditingTargetDir = false
	w.InputBuffer = ""
	w.EditDirBuffer = ""
	w.PendingActionItem = models.FileInfo{}
}

// CancelEditTargetDir exits inline editing without saving (legacy alias).
func (w *Explorer) CancelEditTargetDir() {
	w.CancelEdit()
}

// ApplyEditTargetDir applies the edited host target directory and saves to config.
func (w *Explorer) ApplyEditTargetDir() string {
	w.EditMode = EditModeNone
	w.EditingTargetDir = false
	newDir := strings.TrimSpace(w.InputBuffer)
	if newDir == "" {
		newDir = strings.TrimSpace(w.EditDirBuffer)
	}
	if newDir != "" {
		config.SetDownloadDir(newDir)
		w.SetDownloadDir(config.GetDownloadDir())
	}
	w.InputBuffer = ""
	w.EditDirBuffer = ""
	w.PendingActionItem = models.FileInfo{}
	return w.HostDownloadDir
}

// IsEditing returns true if any inline editing is active.
func (w *Explorer) IsEditing() bool {
	return w.EditMode != EditModeNone || w.EditingTargetDir
}

// SetDeepSearch updates the explorer to reflect active deep search results.
func (w *Explorer) SetDeepSearch(term string) {
	w.IsDeepSearch = true
	w.DeepSearchTerm = term
	if w.CursorPos >= len(w.TotalItems()) {
		w.CursorPos = 0
	}
}

// ClearFilterAndSearch clears active filter and deep search states.
func (w *Explorer) ClearFilterAndSearch() {
	w.Filter = ""
	w.IsDeepSearch = false
	w.DeepSearchTerm = ""
	if w.CursorPos >= len(w.TotalItems()) {
		w.CursorPos = 0
	}
}

// EditKeyPress processes a keystroke during inline editing (target dir, filter, or search).
// Returns done=true when finished (Enter or Esc), applied=true if confirmed, the mode, and the entered value.
func (w *Explorer) EditKeyPress(keyID string) (done bool, applied bool, mode InlineEditMode, value string) {
	mode = w.EditMode
	if mode == EditModeNone && w.EditingTargetDir {
		mode = EditModeTargetDir
	}

	if mode == EditModeConfirmDelete || mode == EditModeConfirmEdit {
		if keyID == "y" || keyID == "Y" || keyID == "<Enter>" {
			item := w.PendingActionItem
			m := mode
			w.CancelEdit()
			return true, true, m, item.Path
		}
		if keyID == "n" || keyID == "N" || keyID == "<Escape>" || keyID == "<C-c>" {
			m := mode
			w.CancelEdit()
			return true, false, m, ""
		}
		return false, false, mode, ""
	}

	if keyID == "<Escape>" || keyID == "<C-c>" {
		w.CancelEdit()
		return true, false, mode, ""
	}
	if keyID == "<Enter>" {
		val := strings.TrimSpace(w.InputBuffer)
		if val == "" && w.EditDirBuffer != "" && mode == EditModeTargetDir {
			val = strings.TrimSpace(w.EditDirBuffer)
		}
		m := mode
		w.EditMode = EditModeNone
		w.EditingTargetDir = false
		w.InputBuffer = ""
		w.EditDirBuffer = ""

		switch m {
		case EditModeTargetDir:
			if val != "" {
				config.SetDownloadDir(val)
				w.SetDownloadDir(config.GetDownloadDir())
			}
		case EditModeFilter:
			w.SetFilter(val)
		}
		return true, true, m, val
	}
	if keyID == "<Backspace>" || keyID == "<C-<Backspace>>" || keyID == "<C-h>" {
		bufStr := w.InputBuffer
		if bufStr == "" && w.EditDirBuffer != "" {
			bufStr = w.EditDirBuffer
		}
		runes := []rune(bufStr)
		if len(runes) > 0 {
			w.InputBuffer = string(runes[:len(runes)-1])
			w.EditDirBuffer = w.InputBuffer
		}
		return false, false, mode, ""
	}
	if keyID == "<C-u>" {
		w.InputBuffer = ""
		w.EditDirBuffer = ""
		return false, false, mode, ""
	}
	if keyID == "<Space>" {
		w.InputBuffer += " "
		w.EditDirBuffer = w.InputBuffer
		return false, false, mode, ""
	}
	if len(keyID) == 1 {
		w.InputBuffer += keyID
		w.EditDirBuffer = w.InputBuffer
		return false, false, mode, ""
	}
	return false, false, mode, ""
}

// EditDirKeyPress processes a keystroke during inline host target directory editing (legacy helper).
func (w *Explorer) EditDirKeyPress(keyID string) (done bool, applied bool) {
	d, a, _, _ := w.EditKeyPress(keyID)
	return d, a
}

// Set updates current directory and its entries.
func (w *Explorer) Set(dirPath string, entries []models.FileInfo) {
	w.CurrentDir = dirPath
	w.Entries = entries
	w.empty = len(entries) == 0
	if w.CursorPos >= len(w.TotalItems()) {
		w.CursorPos = 0
		w.ScrollOffset = 0
	}
}

// SetFilter sets the active wildcard or substring name filter.
func (w *Explorer) SetFilter(filterStr string) {
	w.Filter = strings.TrimSpace(filterStr)
	if w.CursorPos >= len(w.TotalItems()) {
		w.CursorPos = 0
		w.ScrollOffset = 0
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
	w.PreviewOffset = 0
	if item, ok := w.Selected(); ok {
		w.PreviewPath = item.Path
	}
}

// SetWidth sets explorer width
func (w *Explorer) SetWidth(width int) {
	// width is handled during SetRect
}

// ClearPreview closes text content preview.
func (w *Explorer) ClearPreview() {
	w.Previewing = false
	w.PreviewTxt = ""
	w.PreviewOffset = 0
	w.PreviewPath = ""
}

func (w *Explorer) previewVisibleRows() int {
	v := w.Inner.Max.Y - w.Inner.Min.Y - 2
	if v < 1 {
		return 1
	}
	return v
}

func (w *Explorer) previewMaxOffset() int {
	lines := strings.Split(w.PreviewTxt, "\n")
	v := w.previewVisibleRows()
	maxOffset := len(lines) - v
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

// PreviewUp scrolls preview pane up by one line.
func (w *Explorer) PreviewUp() {
	if w.PreviewOffset > 0 {
		w.PreviewOffset--
	}
}

// PreviewDown scrolls preview pane down by one line.
func (w *Explorer) PreviewDown() {
	if w.PreviewOffset < w.previewMaxOffset() {
		w.PreviewOffset++
	}
}

// PreviewHome jumps to top of the preview pane.
func (w *Explorer) PreviewHome() {
	w.PreviewOffset = 0
}

// PreviewEnd jumps to bottom of the preview pane.
func (w *Explorer) PreviewEnd() {
	w.PreviewOffset = w.previewMaxOffset()
}

// PreviewPgUp scrolls preview pane up by page.
func (w *Explorer) PreviewPgUp(step int) {
	if step <= 0 {
		step = w.previewVisibleRows()
	}
	w.PreviewOffset -= step
	if w.PreviewOffset < 0 {
		w.PreviewOffset = 0
	}
}

// PreviewPgDown scrolls preview pane down by page.
func (w *Explorer) PreviewPgDown(step int) {
	if step <= 0 {
		step = w.previewVisibleRows()
	}
	w.PreviewOffset += step
	maxOff := w.previewMaxOffset()
	if w.PreviewOffset > maxOff {
		w.PreviewOffset = maxOff
	}
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
	w.ScrollOffset = 0
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
	if w.StatusMsg != "" || w.EditMode == EditModeConfirmDelete || w.EditMode == EditModeConfirmEdit {
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
		titlePath := w.PreviewPath
		if titlePath == "" {
			titlePath = w.CurrentDir
		}

		lines := strings.Split(w.PreviewTxt, "\n")
		totalLines := len(lines)
		visibleRows := w.previewVisibleRows()
		maxOff := w.previewMaxOffset()
		if w.PreviewOffset > maxOff {
			w.PreviewOffset = maxOff
		}
		if w.PreviewOffset < 0 {
			w.PreviewOffset = 0
		}

		endLine := w.PreviewOffset + visibleRows
		if endLine > totalLines {
			endLine = totalLines
		}
		startLine := w.PreviewOffset + 1
		if totalLines == 0 {
			startLine = 0
		}

		title := fmt.Sprintf("[ File: %s ] [Lines %d-%d/%d]  (▲▼/Home/End: scroll | Enter/Esc: return)", titlePath, startLine, endLine, totalLines)
		buf.SetString(title, previewHeaderStyle, image.Pt(w.Inner.Min.X+1, y))
		y++
		sepLine := strings.Repeat("─", max(10, w.Inner.Max.X-w.Inner.Min.X-2))
		buf.SetString(sepLine, theme.Style("border.fg"), image.Pt(w.Inner.Min.X+1, y))
		y++

		for i := w.PreviewOffset; i < totalLines; i++ {
			if y >= w.Inner.Max.Y {
				break
			}
			l := lines[i]
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

	// Confirmation prompt or Status banner above Directory
	if w.EditMode == EditModeConfirmDelete {
		confirmTxt := fmt.Sprintf("🗑  Delete %s? [y: confirm, n/Esc: cancel]", w.PendingActionItem.Name)
		dangerStyle := theme.Style("status.danger")
		buf.SetString(confirmTxt, dangerStyle, image.Pt(w.Inner.Min.X+1, y))
		y++
	} else if w.EditMode == EditModeConfirmEdit {
		confirmTxt := fmt.Sprintf("✏  Edit %s? [y: confirm, n/Esc: cancel]", w.PendingActionItem.Name)
		warnStyle := theme.Style("status.warn")
		buf.SetString(confirmTxt, warnStyle, image.Pt(w.Inner.Min.X+1, y))
		y++
	} else if w.StatusMsg != "" {
		bannerStyle := theme.Style("status.ok")
		if w.StatusIsErr {
			bannerStyle = theme.Style("status.danger")
		}
		buf.SetString(ensureIconSpacing(w.StatusMsg), bannerStyle, image.Pt(w.Inner.Min.X+1, y))
		y++
	}

	// Breadcrumb, Filter, and Host target bar
	filterBadge := ""
	if w.Filter != "" {
		filterBadge = fmt.Sprintf("   🔍  Filter: [%s] [c: clear]", w.Filter)
	} else if w.IsDeepSearch {
		filterBadge = fmt.Sprintf("   🔎  Search: %q (%d items) [c: clear]", w.DeepSearchTerm, len(w.TotalItems()))
	}

	if w.EditMode == EditModeTargetDir || (w.EditMode == EditModeNone && w.EditingTargetDir) {
		leftPart := fmt.Sprintf("📁  Directory: %s%s   ⬇  Host Target: ", w.CurrentDir, filterBadge)
		x := w.Inner.Min.X + 1
		buf.SetString(leftPart, headerStyle, image.Pt(x, y))
		x += runewidth.StringWidth(leftPart)

		availForInput := (w.Inner.Max.X - 1) - x - 30
		inputTxt := w.InputBuffer
		if inputTxt == "" && w.EditDirBuffer != "" {
			inputTxt = w.EditDirBuffer
		}
		if availForInput > 10 && runewidth.StringWidth(inputTxt) > availForInput {
			r := []rune(inputTxt)
			for len(r) > 0 && runewidth.StringWidth(string(r)) > availForInput {
				r = r[1:]
			}
			inputTxt = string(r)
		}

		editBox := fmt.Sprintf("[%s_]", inputTxt)
		buf.SetString(editBox, selectedStyle, image.Pt(x, y))
		x += runewidth.StringWidth(editBox)

		hintStyle := theme.Style("status.warn")
		hint := "  (Enter: apply, Esc: cancel)"
		if x+runewidth.StringWidth(hint) <= w.Inner.Max.X {
			buf.SetString(hint, hintStyle, image.Pt(x, y))
		}
	} else if w.EditMode == EditModeFilter {
		leftPart := fmt.Sprintf("📁  Directory: %s   ⬇  Host Target: %s [D: change]   ", w.CurrentDir, w.HostDownloadDir)
		x := w.Inner.Min.X + 1
		buf.SetString(leftPart, headerStyle, image.Pt(x, y))
		x += runewidth.StringWidth(leftPart)

		label := "🔍  Filter: "
		buf.SetString(label, headerStyle, image.Pt(x, y))
		x += runewidth.StringWidth(label)

		availForInput := (w.Inner.Max.X - 1) - x - 30
		inputTxt := w.InputBuffer
		if availForInput > 8 && runewidth.StringWidth(inputTxt) > availForInput {
			r := []rune(inputTxt)
			for len(r) > 0 && runewidth.StringWidth(string(r)) > availForInput {
				r = r[1:]
			}
			inputTxt = string(r)
		}

		editBox := fmt.Sprintf("[%s_]", inputTxt)
		buf.SetString(editBox, selectedStyle, image.Pt(x, y))
		x += runewidth.StringWidth(editBox)

		hintStyle := theme.Style("status.warn")
		hint := "  (Enter: apply, Esc: cancel)"
		if x+runewidth.StringWidth(hint) <= w.Inner.Max.X {
			buf.SetString(hint, hintStyle, image.Pt(x, y))
		}
	} else if w.EditMode == EditModeDeepSearch {
		leftPart := fmt.Sprintf("📁  Directory: %s   ⬇  Host Target: %s [D: change]   ", w.CurrentDir, w.HostDownloadDir)
		x := w.Inner.Min.X + 1
		buf.SetString(leftPart, headerStyle, image.Pt(x, y))
		x += runewidth.StringWidth(leftPart)

		label := "🔎  Search: "
		buf.SetString(label, headerStyle, image.Pt(x, y))
		x += runewidth.StringWidth(label)

		availForInput := (w.Inner.Max.X - 1) - x - 30
		inputTxt := w.InputBuffer
		if availForInput > 8 && runewidth.StringWidth(inputTxt) > availForInput {
			r := []rune(inputTxt)
			for len(r) > 0 && runewidth.StringWidth(string(r)) > availForInput {
				r = r[1:]
			}
			inputTxt = string(r)
		}

		editBox := fmt.Sprintf("[%s_]", inputTxt)
		buf.SetString(editBox, selectedStyle, image.Pt(x, y))
		x += runewidth.StringWidth(editBox)

		hintStyle := theme.Style("status.warn")
		hint := "  (Enter: search, Esc: cancel)"
		if x+runewidth.StringWidth(hint) <= w.Inner.Max.X {
			buf.SetString(hint, hintStyle, image.Pt(x, y))
		}
	} else if w.EditMode == EditModeUpload {
		leftPart := fmt.Sprintf("📁  Directory: %s   ⬇  Host Target: %s [D: change]   ", w.CurrentDir, w.HostDownloadDir)
		x := w.Inner.Min.X + 1
		buf.SetString(leftPart, headerStyle, image.Pt(x, y))
		x += runewidth.StringWidth(leftPart)

		label := "📤  Upload: "
		buf.SetString(label, headerStyle, image.Pt(x, y))
		x += runewidth.StringWidth(label)

		availForInput := (w.Inner.Max.X - 1) - x - 30
		inputTxt := w.InputBuffer
		if availForInput > 8 && runewidth.StringWidth(inputTxt) > availForInput {
			r := []rune(inputTxt)
			for len(r) > 0 && runewidth.StringWidth(string(r)) > availForInput {
				r = r[1:]
			}
			inputTxt = string(r)
		}

		editBox := fmt.Sprintf("[%s_]", inputTxt)
		buf.SetString(editBox, selectedStyle, image.Pt(x, y))
		x += runewidth.StringWidth(editBox)

		hintStyle := theme.Style("status.warn")
		hint := "  (Enter: upload, Esc: cancel)"
		if x+runewidth.StringWidth(hint) <= w.Inner.Max.X {
			buf.SetString(hint, hintStyle, image.Pt(x, y))
		}
	} else {
		pathInfo := fmt.Sprintf("📁  Directory: %s   ⬇  Host Target: %s [D: change]%s", w.CurrentDir, w.HostDownloadDir, filterBadge)
		buf.SetString(pathInfo, headerStyle, image.Pt(w.Inner.Min.X+1, y))
	}
	y++

	// Table header
	header := fmt.Sprintf("      %-28s %-12s %-10s %s", "NAME", "PERMISSIONS", "SIZE", "MODIFIED")
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

	if w.ScrollOffset < 0 {
		w.ScrollOffset = 0
	}
	if w.CursorPos < w.ScrollOffset {
		w.ScrollOffset = w.CursorPos
	}
	if w.CursorPos >= w.ScrollOffset+visibleRows {
		w.ScrollOffset = w.CursorPos - visibleRows + 1
	}
	maxOffset := len(items) - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if w.ScrollOffset > maxOffset {
		w.ScrollOffset = maxOffset
	}

	offset := w.ScrollOffset
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

		line := fmt.Sprintf("%s  %-28s %-12s %-10s %s", icon, name, item.Mode, sizeStr, item.ModTime)

		if isSelected {
			buf.SetString(fmt.Sprintf("► %s", line), selectedStyle, image.Pt(w.Inner.Min.X+1, y))
		} else {
			buf.SetString(fmt.Sprintf("  %s", line), style, image.Pt(w.Inner.Min.X+1, y))
		}

		y++
	}
}
