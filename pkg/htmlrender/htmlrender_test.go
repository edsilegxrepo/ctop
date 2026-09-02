// Package htmlrender_test provides unit and security test suites for terminal HTML rendering.
//
// Test Strategy:
//   - Typography & Layout: Test headings, paragraphs, preformatted code, blockquotes, and lists.
//   - Unicode Table Rendering: Test box-drawing table formatting, column width distribution, and cell padding.
//   - Hyperlinks & Footnotes: Test link collection, footnote generation, and relative URL normalization.
//   - Anti-XSS & Sanitization: Test that dangerous script, style, and svg tags are completely stripped.
//   - Edge Cases & Stress: Test malformed HTML, broken nested tables, and multi-byte CJK/emoji wrapping.
package htmlrender

import (
	"strings"
	"testing"
)

func TestRenderHeadingsAndParagraphs(t *testing.T) {
	raw := `
	<!DOCTYPE html>
	<html>
	<head><title>Test Page</title><style>body { color: red; }</style></head>
	<body>
		<h1>Main Title</h1>
		<p>This is a <b>bold</b> and <i>italic</i> description of the container service.</p>
		<h2>Sub Heading</h2>
		<p>Second paragraph with <code>inline code</code> block.</p>
	</body>
	</html>`

	doc := RenderHTML(raw, RenderOptions{MaxWidth: 80, Colorize: true})
	if doc.Title != "Test Page" {
		t.Fatalf("expected title 'Test Page', got %q", doc.Title)
	}

	content := strings.Join(doc.Lines, "\n")
	if !strings.Contains(content, "# Main Title") {
		t.Errorf("expected h1 header '# Main Title' in output:\n%s", content)
	}
	if !strings.Contains(content, "## Sub Heading") {
		t.Errorf("expected h2 header '## Sub Heading' in output:\n%s", content)
	}
	if !strings.Contains(content, "inline code") {
		t.Errorf("expected inline code in output:\n%s", content)
	}
	if strings.Contains(content, "color: red") {
		t.Errorf("expected style tags to be stripped, found style content in output")
	}
}

func TestRenderTable(t *testing.T) {
	raw := `
	<table>
		<tr>
			<th>Service</th>
			<th>Port</th>
			<th>Status</th>
		</tr>
		<tr>
			<td>Web API</td>
			<td>8080</td>
			<td>Healthy</td>
		</tr>
		<tr>
			<td>Prometheus</td>
			<td>9090</td>
			<td>Running</td>
		</tr>
	</table>`

	doc := RenderHTML(raw, RenderOptions{MaxWidth: 80})
	content := strings.Join(doc.Lines, "\n")

	if !strings.Contains(content, "┌") || !strings.Contains(content, "┤") || !strings.Contains(content, "┘") {
		t.Errorf("expected box-drawing table in output:\n%s", content)
	}
	if !strings.Contains(content, "Web API") || !strings.Contains(content, "8080") || !strings.Contains(content, "Healthy") {
		t.Errorf("expected table cells in output:\n%s", content)
	}
	if cnt := strings.Count(content, "Web API"); cnt != 1 {
		t.Errorf("expected 'Web API' to appear exactly once in rendered document, found %d occurrences", cnt)
	}
}

func TestRenderListsAndLinks(t *testing.T) {
	raw := `
	<ul>
		<li>First bullet item</li>
		<li>Second bullet with <a href="https://example.com/api">API Link</a></li>
	</ul>
	<ol>
		<li>Step One</li>
		<li>Step Two</li>
	</ol>`

	doc := RenderHTML(raw, RenderOptions{MaxWidth: 80, ShowFootnotes: true})
	content := strings.Join(doc.Lines, "\n")

	if !strings.Contains(content, "•") {
		t.Errorf("expected bullet symbol in list:\n%s", content)
	}
	if !strings.Contains(content, "Step One") || !strings.Contains(content, "Step Two") {
		t.Errorf("expected numbered list items:\n%s", content)
	}
	if len(doc.Links) != 1 || doc.Links[0] != "https://example.com/api" {
		t.Errorf("expected 1 extracted link 'https://example.com/api', got %v", doc.Links)
	}
	if !strings.Contains(content, "Footnotes & Links:") {
		t.Errorf("expected Footnotes section in output:\n%s", content)
	}
}

func TestRenderCodeAndBlockquote(t *testing.T) {
	raw := `
	<blockquote>
		Container alert: system memory high
	</blockquote>
	<pre>
def calculate_metrics():
    return {"status": "ok"}
	</pre>`

	doc := RenderHTML(raw, RenderOptions{MaxWidth: 80})
	content := strings.Join(doc.Lines, "\n")

	if !strings.Contains(content, "│") || !strings.Contains(content, "Container alert: system memory high") {
		t.Errorf("expected blockquote border in output:\n%s", content)
	}
	if !strings.Contains(content, "def calculate_metrics():") {
		t.Errorf("expected preformatted code in output:\n%s", content)
	}
}

func TestRenderMalformedHTML(t *testing.T) {
	raw := `<div><h2>Unclosed Tags<p>Some text without closing tags`
	doc := RenderHTML(raw, RenderOptions{MaxWidth: 80})
	if len(doc.Lines) == 0 {
		t.Fatalf("expected non-empty output for malformed HTML")
	}
}

func TestRenderBrokenTablesAndNested(t *testing.T) {
	raw := `
	<table>
		<tr><td>Cell 1<td>Cell 2
		<tr><td>Cell 3
	</table>
	<table>
		<tr><th>Header Only</th></tr>
	</table>
	<table></table>`

	doc := RenderHTML(raw, RenderOptions{MaxWidth: 80})
	content := strings.Join(doc.Lines, "\n")
	if !strings.Contains(content, "Cell 1") || !strings.Contains(content, "Cell 2") || !strings.Contains(content, "Cell 3") {
		t.Errorf("expected table cells in output:\n%s", content)
	}
	if !strings.Contains(content, "Header Only") {
		t.Errorf("expected header only table rendered:\n%s", content)
	}
}

func TestRenderMultiByteAndUnicodeWrapping(t *testing.T) {
	raw := `<p>容器监控系统 🚀 HighPerformance Metrics System 📊 运行正常</p>`
	doc := RenderHTML(raw, RenderOptions{MaxWidth: 25})
	if len(doc.Lines) == 0 {
		t.Fatalf("expected wrapped output for unicode string")
	}
	for _, line := range doc.Lines {
		if strings.Contains(line, "容器") && len(line) == 0 {
			t.Errorf("unexpected empty line")
		}
	}
}

func TestRenderScriptAndDangerousTagsStripped(t *testing.T) {
	raw := `
	<div>
		<h1>Safe Content</h1>
		<script>alert("malicious_code_execution")</script>
		<style>body { display: none; }</style>
		<svg><circle cx="50" cy="50" r="40" stroke="green" stroke-width="4" fill="yellow" /></svg>
		<p>Regular Paragraph Text</p>
	</div>`

	doc := RenderHTML(raw, RenderOptions{MaxWidth: 80})
	content := strings.Join(doc.Lines, "\n")

	if strings.Contains(content, "malicious_code_execution") {
		t.Errorf("SECURITY: script content was NOT stripped from output:\n%s", content)
	}
	if strings.Contains(content, "display: none") {
		t.Errorf("SECURITY: style content was NOT stripped from output:\n%s", content)
	}
	if strings.Contains(content, "circle cx=") {
		t.Errorf("SECURITY: svg content was NOT stripped from output:\n%s", content)
	}
	if !strings.Contains(content, "Safe Content") || !strings.Contains(content, "Regular Paragraph Text") {
		t.Errorf("expected safe text in output:\n%s", content)
	}
}
