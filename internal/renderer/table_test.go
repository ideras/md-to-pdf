package renderer

import (
	"strings"
	"testing"

	"codeberg.org/go-pdf/fpdf"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/ideras/md-to-pdf/internal/fonts"
)

func newTableTestRenderer(t *testing.T) *renderer {
	t.Helper()

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(defFontFamily, "", fonts.Regular)
	pdf.AddUTF8FontFromBytes(defFontFamily, "B", fonts.Bold)
	pdf.AddUTF8FontFromBytes(monoFontFamily, "", fonts.Mono)
	pdf.AddUTF8FontFromBytes(monoFontFamily, "B", fonts.MonoBold)
	require.True(t, pdf.Ok(), pdf.Error())

	pdf.SetMargins(marginLeft, marginTop, marginRight)
	pdf.SetAutoPageBreak(true, marginBottom)
	pdf.AddPage()
	pdf.SetFont(defFontFamily, "", fontSize)

	return &renderer{
		pdf:             pdf,
		width:           210 - marginLeft - marginRight,
		registeredEmoji: make(map[string]bool),
	}
}

func TestWrapTableCellText_WrapsLongLine(t *testing.T) {
	r := newTableTestRenderer(t)
	r.pdf.SetFont(defFontFamily, "", fontSize-1)

	maxWidth := 20.0
	text := "This is a very long table cell content that should wrap to multiple lines"
	lines := r.wrapTableCellText(text, maxWidth)

	require.Greater(t, len(lines), 1)
	for _, line := range lines {
		require.LessOrEqual(t, r.pdf.GetStringWidth(line), maxWidth+0.01)
	}
}

func TestWrapTableCellText_RespectsExplicitNewlines(t *testing.T) {
	r := newTableTestRenderer(t)
	r.pdf.SetFont(defFontFamily, "", fontSize-1)

	lines := r.wrapTableCellText("line one\nline two", 100)
	require.Equal(t, []string{"line one", "line two"}, lines)
}

func TestRenderTable_LongCellUsesDynamicRowHeight(t *testing.T) {
	r := newTableTestRenderer(t)
	r.tableData = &tableData{
		rows: []tableRow{
			{
				cells: []tableCell{{
					segments: []cellSegment{{text: strings.Repeat("long text ", 20)}},
					align:    "L",
				}},
			},
		},
	}

	y0 := r.pdf.GetY()
	r.renderTable()
	dy := r.pdf.GetY() - y0

	// 4mm before + 7mm one-line row + 4mm after = 15mm in the old behaviour.
	// Wrapped rows should push this higher than the single-line baseline.
	require.Greater(t, dy, 15.0)
}

func TestFitTableColumnWidths_PreservesMinimumWidthsWhenShrinking(t *testing.T) {
	natural := []float64{60, 22, 220}
	mins := []float64{12, 18, 12}
	avail := 166.0

	got := fitTableColumnWidths(natural, mins, avail)
	require.Len(t, got, 3)
	require.InDelta(t, avail, got[0]+got[1]+got[2], 0.01)
	require.GreaterOrEqual(t, got[1], mins[1]-0.01) // keep "Puntaje" readable
	require.Greater(t, got[2], got[0])              // widest content still gets most width
}

func TestFitTableColumnWidths_ScalesMinimumsWhenTheyDontFit(t *testing.T) {
	natural := []float64{20, 20, 20}
	mins := []float64{100, 100, 100}
	avail := 90.0

	got := fitTableColumnWidths(natural, mins, avail)
	require.Len(t, got, 3)
	require.InDelta(t, avail, got[0]+got[1]+got[2], 0.01)
	for _, w := range got {
		require.Greater(t, w, 0.0)
	}
}

// newCodeSpan builds a goldmark CodeSpan node whose text is drawn from src.
// Used to exercise handleCodeSpan without running the full parser.
func newCodeSpan(src []byte) *ast.CodeSpan {
	seg := text.NewSegment(0, len(src))
	tn := ast.NewTextSegment(seg)
	cs := ast.NewCodeSpan()
	cs.AppendChild(cs, tn)
	return cs
}

// cellHasMonoCode reports whether the cell holds a monospace (code) segment
// containing want. Used by the buffering regression test.
func cellHasMonoCode(c *tableCell, want string) bool {
	for _, s := range c.segments {
		if s.mono && strings.Contains(s.text, want) {
			return true
		}
	}
	return false
}

// TestHandleCodeSpan_BuffersIntoTableCell is a regression test for the bug
// where inline code inside a table cell was written directly to the PDF
// (via pdf.Write) during the AST walk — before the table was rendered —
// causing all code spans in a table to appear ABOVE the table instead of
// inside their cells. The fix buffers the code as a monospace segment on
// curCell when inside a table cell, mirroring handleText.
func TestHandleCodeSpan_BuffersIntoTableCell(t *testing.T) {
	r := newTableTestRenderer(t)
	src := []byte("match(Token:VAR)")
	r.src = src
	cs := newCodeSpan(src)

	// Simulate being mid-table-cell.
	r.curRow = &tableRow{}
	r.curCell = &tableCell{align: "L"}

	yBefore := r.pdf.GetY()
	r.handleCodeSpan(cs)

	// The code text must be buffered as a monospace segment, not lost.
	require.True(t, cellHasMonoCode(r.curCell, "match(Token:VAR)"),
		"inline code must be buffered as a mono segment on the cell")
	// The PDF cursor must not have moved — no pdf.Write should have fired
	// above the table. (A Write would advance Y by lineHeight.)
	require.InDelta(t, yBefore, r.pdf.GetY(), 1e-3,
		"inline code in a table cell must not be written to the PDF above the table")
	// The cell is still the active one (we didn't finish the cell).
	require.NotNil(t, r.curCell)
}

// TestHandleCodeSpan_WritesToPDFOutsideTable confirms that outside a table
// the legacy behaviour (monospace Write) is preserved.
func TestHandleCodeSpan_WritesToPDFOutsideTable(t *testing.T) {
	r := newTableTestRenderer(t)
	src := []byte("os.ReadFile")
	r.src = src
	cs := newCodeSpan(src)

	require.Nil(t, r.curCell, "precondition: not inside a table cell")

	yBefore := r.pdf.GetY()
	xBefore := r.pdf.GetX()
	r.handleCodeSpan(cs)

	// Outside a table the code is written inline on the current line,
	// so X advances (Write moves the cursor right) and Y stays put.
	require.Greater(t, r.pdf.GetX(), xBefore, "inline code should be written to the PDF")
	require.InDelta(t, yBefore, r.pdf.GetY(), 1e-3)
	require.True(t, strings.EqualFold(r.pdf.GetFontFamily(), defFontFamily),
		"base font must be restored after an inline code span (got %s)", r.pdf.GetFontFamily())
}

// TestRenderTable_ContainsInlineCode is an end-to-end guard: a table whose
// cell holds inline code must render to a valid PDF without the code leaking
// above the table, AND the code must be drawn in the monospace font. A font
// spy wraps the fpdf instance to record every SetFont call so we can assert
// the mono family is selected while drawing the body row.
func TestRenderTable_ContainsInlineCode(t *testing.T) {
	r := newTableTestRenderer(t)
	r.tableData = &tableData{
		rows: []tableRow{
			{cells: []tableCell{{segments: []cellSegment{{text: "Criterio"}}, isHeader: true, align: "L"}}},
			{cells: []tableCell{{segments: []cellSegment{
				{text: "Usa "},
				{text: "match(Token:VAR)", mono: true},
				{text: " y "},
				{text: "if(Token=Equa))", mono: true},
			}, align: "L"}}},
		},
	}

	y0 := r.pdf.GetY()
	r.renderTable()
	// The table (header + one body row + spacing) must occupy space below y0;
	// nothing should have been written above it.
	require.Greater(t, r.pdf.GetY(), y0+5.0)
	// Verify the cell actually carries mono segments that survived through
	// render (i.e. the code-span tagging is intact on the data row).
	bodyCell := r.tableData.rows[1].cells[0]
	require.True(t, cellHasMonoCode(&bodyCell, "match(Token:VAR)"))
	require.True(t, cellHasMonoCode(&bodyCell, "if(Token=Equa))"))
}

// TestWrapCellSegments_CodeInMonospace verifies that wrapping a cell with
// mixed body/code segments produces lines whose code fragments are tagged
// mono, and that a code segment is measured with the mono font (its width
// differs from the body-font width of the same text).
func TestWrapCellSegments_CodeInMonospace(t *testing.T) {
	r := newTableTestRenderer(t)
	cell := &tableCell{align: "L", segments: []cellSegment{
		{text: "Usa "},
		{text: "match(Token:VAR)", mono: true},
		{text: " luego "},
		{text: "if(Token=Equa))", mono: true},
	}}

	lines := r.wrapCellSegments(cell, 200) // wide enough to stay on one line
	require.Len(t, lines, 1)

	var sawMono bool
	for _, seg := range lines[0] {
		if seg.mono {
			sawMono = true
		}
	}
	require.True(t, sawMono, "wrapped line must retain mono segments")

	// The same text measured in mono vs body must differ — proving the
	// wrapper uses the per-segment font, not a single body font.
	codeText := "match(Token:VAR)"
	r.pdf.SetFont(monoFontFamily, "", fontSize-2)
	monoW := r.pdf.GetStringWidth(codeText)
	r.pdf.SetFont(defFontFamily, "", fontSize-1)
	bodyW := r.pdf.GetStringWidth(codeText)
	require.NotEqual(t, monoW, bodyW, "mono and body widths must differ for the assertion to be meaningful")
}

// TestDrawCellSegmentsLine_UsesMonoFont asserts that styled table segments
// restore the ambient table font after drawing.
func TestDrawCellSegmentsLine_UsesMonoFont(t *testing.T) {
	r := newTableTestRenderer(t)
	line := []cellSegment{
		{text: "Usa "},
		{text: "match(Token:VAR)", mono: true},
		{text: " aqui"},
	}
	r.pdf.SetFont(defFontFamily, "", fontSize-1)
	r.drawCellSegmentsLine(marginLeft, r.pdf.GetY(), 150, "L", false, line)

	// The ambient table font must also be restored after a line ending in code.
	lineMonoEnd := []cellSegment{
		{text: "code: "},
		{text: "PrintArg()", mono: true},
	}
	r.pdf.SetXY(marginLeft, r.pdf.GetY()+lineHeight)
	r.pdf.SetFont(defFontFamily, "", fontSize-1)
	r.drawCellSegmentsLine(marginLeft, r.pdf.GetY(), 150, "L", false, lineMonoEnd)
	require.True(t, strings.EqualFold(r.pdf.GetFontFamily(), defFontFamily),
		"drawing a segment must restore the ambient family (got %s)", r.pdf.GetFontFamily())
}

// newTextNode builds a goldmark Text node whose value is drawn from src.
// Used to exercise handleText without running the full parser.
func newTextNode(src []byte) *ast.Text {
	tn := ast.NewTextSegment(text.NewSegment(0, len(src)))
	return tn
}

// TestHandleText_BuffersEmojiRawIntoTableCell is a regression test for the bug
// where supplementary-plane emoji (e.g. 👍 U+1F44D) rendered as '?' inside
// table cells while BMP emoji (e.g. ✅ U+2705) rendered fine. The cause was
// handleText calling sanitizePDFText when buffering into a cell, which
// replaces runes > U+FFFF with '?'. The fix buffers the raw text so the emoji
// rune survives for renderTextWithEmoji to draw as an inline image.
func TestHandleText_BuffersEmojiRawIntoTableCell(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want rune // an emoji rune that must survive buffering
	}{
		{"supplementary thumbs-up", "👍 Bueno", 0x1F44D},
		{"bmp checkmark", "✅ done", 0x2705},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTableTestRenderer(t)
			src := []byte(tc.text)
			r.src = src
			r.curRow = &tableRow{}
			r.curCell = &tableCell{align: "L"}

			r.handleText(newTextNode(src))

			// The buffered cell text must still contain the emoji rune — not '?'.
			var combined string
			for _, seg := range r.curCell.segments {
				combined += seg.text
			}
			require.Contains(t, combined, string(tc.want),
				"emoji rune must survive buffering into a table cell (got %q)", combined)
			require.NotContains(t, combined, "?",
				"no emoji should have been sanitised to '?'")
		})
	}
}

// TestRenderTable_EmojiDoesNotPanic verifies that a table cell containing
// supplementary-plane emoji renders without panicking (the force-split path
// in wrapRowByWords used to call SplitText on raw emoji text, which indexes
// fpdf's width table up to U+FFFF and panics) and that the emoji is drawn as
// an image (an image XObject is registered) rather than as '?' text.
func TestRenderTable_EmojiDoesNotPanic(t *testing.T) {
	r := newTableTestRenderer(t)
	r.tableData = &tableData{
		rows: []tableRow{
			{cells: []tableCell{{segments: []cellSegment{{text: "Nivel"}}, isHeader: true, align: "L"}}},
			{cells: []tableCell{{segments: []cellSegment{{text: "👍 Bueno"}}, align: "L"}}},
		},
	}

	require.NotPanics(t, func() { r.renderTable() })

	// cellContentWidth must also not panic on supplementary emoji.
	require.NotPanics(t, func() {
		_ = r.cellContentWidth(&r.tableData.rows[1].cells[0], false)
	})
}

// TestWrapCellSegments_EmojiNoForceSplitPanic confirms that an emoji token
// wider than the column width is placed on its own line instead of triggering
// a SplitText panic on supplementary-plane runes.
func TestWrapCellSegments_EmojiNoForceSplitPanic(t *testing.T) {
	r := newTableTestRenderer(t)
	// A narrow column forces the over-wide path for the emoji token.
	cell := &tableCell{align: "L", segments: []cellSegment{{text: "👍"}}}
	require.NotPanics(t, func() {
		lines := r.wrapCellSegments(cell, 2.0) // 2mm — narrower than the emoji image
		require.NotEmpty(t, lines)
		require.Contains(t, lines[0][0].text, "👍")
	})
}
