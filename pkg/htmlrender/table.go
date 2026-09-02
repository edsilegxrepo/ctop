// Package htmlrender provides terminal ANSI/text rendering for HTML documents and responses.
package htmlrender

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// TableCell represents a single table header or data cell.
type TableCell struct {
	Text     string
	IsHeader bool
	Align    string // "left", "center", "right"
}

// TableRow represents a row of cells.
type TableRow struct {
	Cells []TableCell
}

// Table represents an extracted HTML table structure.
type Table struct {
	Rows [][]TableCell
}

// RenderTable formats a Table into an aligned Unicode/ASCII box-drawing grid.
func (t *Table) Render(maxWidth int) []string {
	if len(t.Rows) == 0 {
		return nil
	}

	// 1. Calculate column count
	numCols := 0
	for _, row := range t.Rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	if numCols == 0 {
		return nil
	}

	// 2. Compute maximum content width per column
	colWidths := make([]int, numCols)
	for _, row := range t.Rows {
		for c, cell := range row {
			w := runewidth.StringWidth(strings.TrimSpace(cell.Text))
			if w > colWidths[c] {
				colWidths[c] = w
			}
		}
	}

	// 3. Ensure columns fit within maxWidth
	totalWidth := 1 // leading border
	for _, w := range colWidths {
		if w < 3 {
			w = 3 // Minimum column width
		}
		totalWidth += w + 3 // padding + trailing border
	}

	if maxWidth > 20 && totalWidth > maxWidth {
		// Proportionally shrink columns
		excess := totalWidth - maxWidth
		for i := range colWidths {
			if colWidths[i] > 10 && excess > 0 {
				shrink := excess / 2
				if shrink > colWidths[i]-8 {
					shrink = colWidths[i] - 8
				}
				colWidths[i] -= shrink
				excess -= shrink
			}
		}
	}

	var lines []string

	// Helper for horizontal border lines
	buildBorder := func(left, mid, right, fill string) string {
		var sb strings.Builder
		sb.WriteString(left)
		for i, w := range colWidths {
			sb.WriteString(strings.Repeat(fill, w+2))
			if i < len(colWidths)-1 {
				sb.WriteString(mid)
			}
		}
		sb.WriteString(right)
		return sb.String()
	}

	topBorder := buildBorder("┌", "┬", "┐", "─")
	midBorder := buildBorder("├", "┼", "┤", "─")
	botBorder := buildBorder("└", "┴", "┘", "─")

	lines = append(lines, topBorder)

	for rIdx, row := range t.Rows {
		var rowBuf strings.Builder
		rowBuf.WriteString("│")
		for c := 0; c < numCols; c++ {
			text := ""
			isHeader := false
			if c < len(row) {
				text = strings.TrimSpace(row[c].Text)
				isHeader = row[c].IsHeader
			}
			w := colWidths[c]
			textWidth := runewidth.StringWidth(text)
			if textWidth > w {
				text = runewidth.Truncate(text, w-1, "…")
				textWidth = runewidth.StringWidth(text)
			}
			padding := w - textWidth
			if padding < 0 {
				padding = 0
			}

			rowBuf.WriteString(" ")
			if isHeader {
				rowBuf.WriteString("\033[1;36m" + text + "\033[0m")
			} else {
				rowBuf.WriteString(text)
			}
			rowBuf.WriteString(strings.Repeat(" ", padding) + " │")
		}
		lines = append(lines, rowBuf.String())

		if rIdx == 0 && len(t.Rows) > 1 {
			lines = append(lines, midBorder)
		}
	}

	lines = append(lines, botBorder)
	return lines
}
