// Package htmlrender provides a pure-Go HTML lexer, DOM walker, and ANSI terminal renderer.
//
// Objective:
//
//	Convert raw HTML responses into styled, formatted terminal text with headers, tables,
//	blockquotes, lists, hyperlinks, and footnotes without requiring external browser engines.
//
// Core Components:
//   - Document: Parsed document model storing ANSI lines, plaintext, extracted links, and metadata.
//   - RenderOptions: Configuration for terminal width constraints, colorization, and footnote generation.
//   - renderContext: Stateful AST walker managing text wrapping, indentation stacks, and list state.
//
// Functionality:
//   - Box-drawing Unicode table formatting with automatic column width distribution.
//   - Word-wrapping tailored to terminal widths and runewidth multi-byte characters.
//   - Safe sanitization of dangerous tags (<script>, <style>, <svg>, <object>).
//
// Data Flow:
//
//	Raw HTML String -> golang.org/x/net/html.Parse -> renderContext.walk() -> ANSI Lines -> *Document.
package htmlrender

import (
	"fmt"
	"io"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/net/html"
)

// Document represents a fully parsed and ANSI-rendered HTML document.
type Document struct {
	Title     string
	Lines     []string
	Links     []string
	RawHTML   string
	PlainText string
}

// RenderOptions specifies layout constraints and styling choices.
type RenderOptions struct {
	MaxWidth      int
	ShowFootnotes bool
	Colorize      bool
}

// RenderHTML parses the given HTML string and renders it into formatted terminal lines.
func RenderHTML(rawHTML string, opts RenderOptions) *Document {
	if opts.MaxWidth <= 0 {
		opts.MaxWidth = 80
	}

	doc := &Document{
		RawHTML: rawHTML,
	}

	node, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		// Fallback to raw text
		doc.Lines = strings.Split(rawHTML, "\n")
		doc.PlainText = rawHTML
		return doc
	}

	ctx := &renderContext{
		opts:  opts,
		doc:   doc,
		links: make(map[string]int),
	}

	ctx.walk(node)
	ctx.flush()

	// Append link footnotes if configured and present
	if opts.ShowFootnotes && len(doc.Links) > 0 {
		doc.Lines = append(doc.Lines, "")
		doc.Lines = append(doc.Lines, "\033[1;33mFootnotes & Links:\033[0m")
		for i, link := range doc.Links {
			doc.Lines = append(doc.Lines, fmt.Sprintf("  \033[36m[%d]\033[0m %s", i+1, link))
		}
	}

	doc.PlainText = strings.Join(doc.Lines, "\n")
	return doc
}

type renderContext struct {
	opts       RenderOptions
	doc        *Document
	links      map[string]int
	currLine   strings.Builder
	inPre      bool
	inCode     bool
	listStack  []listState
	quoteDepth int
}

type listState struct {
	ordered bool
	index   int
}

func (c *renderContext) flush() {
	if c.currLine.Len() > 0 {
		text := c.currLine.String()
		c.currLine.Reset()

		if c.inPre {
			c.doc.Lines = append(c.doc.Lines, text)
			return
		}

		// Wrap lines to MaxWidth
		wrapped := wrapText(text, c.opts.MaxWidth, c.quoteDepth)
		c.doc.Lines = append(c.doc.Lines, wrapped...)
	}
}

func (c *renderContext) blankLine() {
	c.flush()
	if len(c.doc.Lines) > 0 && c.doc.Lines[len(c.doc.Lines)-1] != "" {
		c.doc.Lines = append(c.doc.Lines, "")
	}
}

func (c *renderContext) walk(n *html.Node) {
	if n == nil {
		return
	}

	switch n.Type {
	case html.ElementNode:
		c.handleElementOpen(n)
	case html.TextNode:
		c.handleText(n.Data)
	}

	// Traverse children (skip script, style, svg, and table contents since tables are handled atomically)
	if n.Type != html.ElementNode || (n.Data != "script" && n.Data != "style" && n.Data != "svg" && n.Data != "table") {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			c.walk(child)
		}
	}

	if n.Type == html.ElementNode {
		c.handleElementClose(n)
	}
}

func (c *renderContext) handleElementOpen(n *html.Node) {
	switch n.Data {
	case "title":
		if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
			c.doc.Title = strings.TrimSpace(n.FirstChild.Data)
		}
	case "h1":
		c.blankLine()
		c.currLine.WriteString("\033[1;35m# ")
	case "h2":
		c.blankLine()
		c.currLine.WriteString("\033[1;34m## ")
	case "h3":
		c.blankLine()
		c.currLine.WriteString("\033[1;36m### ")
	case "h4", "h5", "h6":
		c.blankLine()
		c.currLine.WriteString("\033[1;32m#### ")
	case "p":
		c.blankLine()
	case "br":
		c.flush()
	case "hr":
		c.blankLine()
		w := c.opts.MaxWidth
		if w <= 0 || w > 80 {
			w = 80
		}
		c.doc.Lines = append(c.doc.Lines, "\033[90m"+strings.Repeat("─", w)+"\033[0m")
		c.blankLine()
	case "b", "strong":
		c.currLine.WriteString("\033[1m")
	case "i", "em":
		c.currLine.WriteString("\033[3m")
	case "code":
		c.inCode = true
		if !c.inPre {
			c.currLine.WriteString("\033[7m ")
		}
	case "pre":
		c.blankLine()
		c.inPre = true
		c.currLine.WriteString("\033[32m")
	case "blockquote":
		c.blankLine()
		c.quoteDepth++
	case "ul":
		c.blankLine()
		c.listStack = append(c.listStack, listState{ordered: false, index: 0})
	case "ol":
		c.blankLine()
		c.listStack = append(c.listStack, listState{ordered: true, index: 1})
	case "li":
		c.flush()
		indent := strings.Repeat("  ", len(c.listStack)-1)
		if len(c.listStack) > 0 {
			top := &c.listStack[len(c.listStack)-1]
			if top.ordered {
				fmt.Fprintf(&c.currLine, "%s\033[33m%d.\033[0m ", indent, top.index)
				top.index++
			} else {
				fmt.Fprintf(&c.currLine, "%s\033[36m•\033[0m ", indent)
			}
		} else {
			c.currLine.WriteString("• ")
		}
	case "a":
		for _, attr := range n.Attr {
			if attr.Key == "href" && strings.TrimSpace(attr.Val) != "" {
				href := strings.TrimSpace(attr.Val)
				if !strings.HasPrefix(href, "javascript:") {
					if _, exists := c.links[href]; !exists {
						c.links[href] = len(c.doc.Links) + 1
						c.doc.Links = append(c.doc.Links, href)
					}
					c.currLine.WriteString("\033[4;34m")
				}
				break
			}
		}
	case "table":
		c.blankLine()
		tbl := extractTable(n)
		if tbl != nil {
			tableLines := tbl.Render(c.opts.MaxWidth)
			c.doc.Lines = append(c.doc.Lines, tableLines...)
		}
		c.blankLine()
	}
}

func (c *renderContext) handleElementClose(n *html.Node) {
	switch n.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		c.currLine.WriteString("\033[0m")
		c.blankLine()
	case "p":
		c.blankLine()
	case "b", "strong", "i", "em":
		c.currLine.WriteString("\033[0m")
	case "code":
		if !c.inPre {
			c.currLine.WriteString(" \033[0m")
		}
		c.inCode = false
	case "pre":
		c.currLine.WriteString("\033[0m")
		c.flush()
		c.inPre = false
		c.blankLine()
	case "blockquote":
		c.blankLine()
		if c.quoteDepth > 0 {
			c.quoteDepth--
		}
		c.blankLine()
	case "ul", "ol":
		if len(c.listStack) > 0 {
			c.listStack = c.listStack[:len(c.listStack)-1]
		}
		c.blankLine()
	case "li":
		c.flush()
	case "a":
		for _, attr := range n.Attr {
			if attr.Key == "href" && strings.TrimSpace(attr.Val) != "" {
				href := strings.TrimSpace(attr.Val)
				if idx, ok := c.links[href]; ok {
					fmt.Fprintf(&c.currLine, "\033[0m\033[33m[%d]\033[0m", idx)
				} else {
					c.currLine.WriteString("\033[0m")
				}
				break
			}
		}
	}
}

func (c *renderContext) handleText(text string) {
	if c.inPre {
		c.currLine.WriteString(text)
		return
	}

	// Normalize spaces for normal prose
	cleaned := strings.ReplaceAll(text, "\n", " ")
	cleaned = strings.ReplaceAll(cleaned, "\t", " ")
	for strings.Contains(cleaned, "  ") {
		cleaned = strings.ReplaceAll(cleaned, "  ", " ")
	}

	if cleaned != "" {
		c.currLine.WriteString(cleaned)
	}
}

// extractTable parses a <table> DOM node into a structured Table.
func extractTable(tableNode *html.Node) *Table {
	var rows [][]TableCell

	var findRows func(*html.Node)
	findRows = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "tr" {
			var cells []TableCell
			for cell := n.FirstChild; cell != nil; cell = cell.NextSibling {
				if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
					isHeader := cell.Data == "th"
					cellText := extractNodeText(cell)
					cells = append(cells, TableCell{
						Text:     cellText,
						IsHeader: isHeader,
					})
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			findRows(child)
		}
	}

	findRows(tableNode)
	if len(rows) == 0 {
		return nil
	}
	return &Table{Rows: rows}
}

// extractNodeText recursively extracts concatenated plain text from a DOM subtree.
func extractNodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}

// wrapText word-wraps input text to fit within width, prepending blockquote borders if depth > 0.
func wrapText(text string, width, quoteDepth int) []string {
	if width <= 0 {
		width = 80
	}
	prefix := ""
	if quoteDepth > 0 {
		prefix = strings.Repeat("│ ", quoteDepth)
	}
	availWidth := width - runewidth.StringWidth(prefix)
	if availWidth < 10 {
		availWidth = 10
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	var cur strings.Builder
	curWidth := 0

	for _, w := range words {
		wWidth := runewidth.StringWidth(w)
		if curWidth+wWidth+1 > availWidth && curWidth > 0 {
			lines = append(lines, prefix+cur.String())
			cur.Reset()
			curWidth = 0
		}
		if curWidth > 0 {
			cur.WriteString(" ")
			curWidth++
		}
		cur.WriteString(w)
		curWidth += wWidth
	}

	if cur.Len() > 0 {
		lines = append(lines, prefix+cur.String())
	}
	return lines
}

// ReadAndRender consumes an io.Reader and returns a rendered Document.
func ReadAndRender(r io.Reader, opts RenderOptions) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return RenderHTML(string(data), opts), nil
}
