package md2pdf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	markdown "github.com/ideras/md-to-pdf"
)

func TestConvertFile_Headings(t *testing.T) {
	md := `# Heading 1
## Heading 2
### Heading 3
#### Heading 4
`
	pdfPath := convert(t, "headings", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_Paragraphs(t *testing.T) {
	md := `This is a normal paragraph with some text.

Another paragraph here with more content.
`
	pdfPath := convert(t, "paragraphs", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_BoldAndItalic(t *testing.T) {
	md := `This has **bold text** and *italic text* and ***bold italic***.
`
	pdfPath := convert(t, "emphasis", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_UnorderedList(t *testing.T) {
	md := `- First item
- Second item
- Third item with **bold**
`
	pdfPath := convert(t, "ulist", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_OrderedList(t *testing.T) {
	md := `1. First step
2. Second step
3. Third step
`
	pdfPath := convert(t, "olist", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_NestedList(t *testing.T) {
	md := `- Item 1
  - Nested 1.1
  - Nested 1.2
- Item 2
  1. Ordered nested
  2. Another
`
	pdfPath := convert(t, "nested", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_CodeBlock(t *testing.T) {
	md := "```go\nfunc hello() {\n    fmt.Println(\"hi\")\n}\n```\n"
	pdfPath := convert(t, "codeblock", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_InlineCode(t *testing.T) {
	md := "Use `os.ReadFile` to read files.\n"
	pdfPath := convert(t, "inlinecode", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_ThematicBreak(t *testing.T) {
	md := "Before the break.\n\n---\n\nAfter the break.\n"
	pdfPath := convert(t, "hr", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_Blockquote(t *testing.T) {
	md := "> This is a blockquote.\n> Multiple lines.\n"
	pdfPath := convert(t, "blockquote", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_EmptyDocument(t *testing.T) {
	md := ""
	pdfPath := convert(t, "empty", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_FileNotFound(t *testing.T) {
	err := markdown.ConvertFile("/nonexistent/file.md", "/tmp/out.pdf")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read markdown file")
}

func TestConvert_WritesPDFToWriter(t *testing.T) {
	var output bytes.Buffer
	err := markdown.Convert([]byte("# Streamed PDF\n"), &output)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(output.Bytes(), []byte("%PDF-")))
}

func TestConvert_NilWriter(t *testing.T) {
	err := markdown.Convert([]byte("# No output\n"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writer is nil")
}

func TestConvertFile_Table(t *testing.T) {
	md := `| Name     | Age | City     |
|----------|-----|----------|
| Alice    | 25  | New York |
| Bob      | 30  | London   |
| Charlie  | 35  | Tokyo    |
`
	pdfPath := convert(t, "table", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_TableWithAlignment(t *testing.T) {
	md := `| Left | Center | Right |
| :--- | :----: | ----: |
| a    | b      | c     |
| d    | e      | f     |
`
	pdfPath := convert(t, "table_align", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_TableNoHeader(t *testing.T) {
	md := `| a | b |
|---|---|
| c | d |
| e | f |
`
	pdfPath := convert(t, "table_noheader", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_EmptyTable(t *testing.T) {
	md := `Before table.

| a | b |
|---|---|

After table.
`
	pdfPath := convert(t, "table_empty", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_SyntaxHighlightedGo(t *testing.T) {
	md := "```go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello world\")\n}\n```\n"
	pdfPath := convert(t, "syntax_go", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_SyntaxHighlightedPython(t *testing.T) {
	md := "```python\ndef hello(name):\n    # This is a comment\n    print(f\"Hello, {name}!\")\n    return True\n```\n"
	pdfPath := convert(t, "syntax_python", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_SyntaxHighlightedJavaScript(t *testing.T) {
	md := "```javascript\nfunction add(a, b) {\n    /* sum two numbers */\n    return a + b;\n}\n\nconst result = add(1, 2);\nconsole.log(result);\n```\n"
	pdfPath := convert(t, "syntax_js", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_SyntaxHighlightedUnknownLang(t *testing.T) {
	// Unknown language falls back to plain code block
	md := "```foobarlang123\nfoo bar baz\n```\n"
	pdfPath := convert(t, "syntax_unknown", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_SyntaxHighlightedNoLang(t *testing.T) {
	// Code block without language annotation renders as plain
	md := "```\nplain code block\n```\n"
	pdfPath := convert(t, "syntax_nolang", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_AssemblyCode(t *testing.T) {
	md := "```asm\nmul $s0, $s1\nmflo $t1\nadd $t0, $t1, $s2\n```\n"
	// Assembly uses GAS lexer which produces fine-grained tokens (one per word).
	// The renderer must group tokens on the same line — 3 input lines → 3 PDF lines.
	pdfPath := convert(t, "assembly", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_SoftLineBreakJoinsWithSpace(t *testing.T) {
	// Paragraph lines without blank line between them should join with spaces
	md := "La diferencia es que ASCII solo\npuede representar 128 caracteres\nbasicos, ya sean letras o\nnumeros\n"
	pdfPath := convert(t, "softbreak", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_EmojiDoesNotPanic(t *testing.T) {
	md := "# 📘 Heading\n\nParagraph with 👩‍🎓 and 🧪 emoji.\n\n| Col |\n|---|\n| 🟡 |\n"
	pdfPath := convert(t, "emoji", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_ComplexDocument(t *testing.T) {
	md := "# Report\n\n## Results\n\n| Student | Score | Grade |\n|---------|-------|-------|\n| Alice   | 95    | A     |\n| Bob     | 87    | B+    |\n| Charlie | 73    | C     |\n\n## Code\n\n```python\ndef calc():\n    # compute average\n    total = 95 + 87 + 73\n    return total / 3\n```\n"
	pdfPath := convert(t, "complex_doc", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_TableAndCodeTogether(t *testing.T) {
	md := `# Report

| Student | Grade |
|---------|-------|
| Alice   | 95    |
| Bob     | 87    |

## Code Example

` + "```python\nprint(\"done\")\n```\n"
	pdfPath := convert(t, "table_code", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_InlineHTMLBrAndSub(t *testing.T) {
	// Exercises the exact HTML pattern from the real evaluation Markdown:
	// **Title**<br><sub>subtitle</sub> inside a table cell.
	md := `# Evaluation

| Criterion | Score | Comment |
|-----------|-------|---------|
| **Sintaxis RE/flex correcta**<br><sub>La expresión regular está bien formada.</sub> | 4.2 / 6.2 | Válida pero subóptima. |
| **Aceptación de casos válidos**<br><sub>Reconoce: 0, 7, 42.</sub> | 4.2 / 6.2 | Acepta bien. |

Paragraph with a manual break here.<br>Continued on next line.

Text with <sub>subscript</sub> inside a paragraph.
`
	pdfPath := convert(t, "inline_html", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_InlineHTMLUnknownTagsIgnored(t *testing.T) {
	// Unknown HTML tags should be silently ignored, not panic.
	md := "Paragraph with <span>a span</span> and a <strong>strong</strong> tag.\n"
	pdfPath := convert(t, "unknown_html_tags", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_BlockquoteSimple(t *testing.T) {
	md := `Normal paragraph before.

> This is a simple blockquote line.

Normal paragraph after.
`
	pdfPath := convert(t, "blockquote_simple", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_BlockquoteMultiLine(t *testing.T) {
	// Multiple consecutive > lines form one paragraph; the wrapped continuation
	// must stay indented (not snap back to the left margin).
	md := `> First line of a multi-line blockquote.
> Second line of the same blockquote.
> Third line continues here with enough text to force a wrap onto the next line of the PDF.

Back to normal.
`
	pdfPath := convert(t, "blockquote_multiline", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_BlockquoteWithEmphasis(t *testing.T) {
	// Bold and italic inside a blockquote must not break the italic baseline.
	md := "> A blockquote with **bold** and *italic* and normal text all on one line.\n\nNormal after.\n"
	pdfPath := convert(t, "blockquote_emphasis", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_BlockquoteLong(t *testing.T) {
	// A long blockquote whose text wraps — wrapped lines must stay indented.
	md := "> A longer blockquote that has quite a bit of text in it and should probably wrap onto the next line when the content is long enough to exceed the available width of the page.\n"
	pdfPath := convert(t, "blockquote_long", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_BlockquoteNested(t *testing.T) {
	md := "> Outer blockquote.\n>\n>> Nested inner blockquote.\n\nNormal.\n"
	pdfPath := convert(t, "blockquote_nested", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_HardLineBreak(t *testing.T) {
	// Two trailing spaces create a hard line break in Markdown.
	// The progress bar must appear on a separate line from "Progreso:".
	md := "**Progreso:** `83.3 / 90`  \n\u2588\u2588\u2588\u2591\u2591 `92.56%`\n"
	pdfPath := convert(t, "hard_line_break", md)
	assertPDFValid(t, pdfPath)
}

func TestConvertFile_HardLineBreakInParagraph(t *testing.T) {
	// Hard break inside a longer paragraph.
	md := "First line ends here.  \nSecond line starts here.\n"
	pdfPath := convert(t, "hard_line_break_para", md)
	assertPDFValid(t, pdfPath)
}

// --- helpers ---

func convert(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	mdPath := filepath.Join(dir, name+".md")
	pdfPath := filepath.Join(dir, name+".pdf")

	err := os.WriteFile(mdPath, []byte(content), 0644)
	require.NoError(t, err)

	err = markdown.ConvertFile(mdPath, pdfPath)
	require.NoError(t, err)
	return pdfPath
}

func assertPDFValid(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.NotZero(t, info.Size(), "PDF file should not be empty")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, len(data) > 4, "PDF file too small")
	assert.Equal(t, "%PDF", string(data[:4]), "file should start with PDF header")
}
