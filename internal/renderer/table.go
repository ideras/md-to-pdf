package renderer

import (
	"strings"

	extast "github.com/yuin/goldmark/extension/ast"

	"github.com/ideras/md-to-pdf/internal/emoji"
)

// --- table data structures ---

// cellSegment is a run of text inside a table cell. mono=true marks an
// inline code span, which is rendered with the monospace font at a slightly
// smaller size. Plain text segments are rendered with the cell's body font
// (bold for header cells) and may contain emoji, which render as inline images.
type cellSegment struct {
	text string
	mono bool
}

// appendText adds a plain-text fragment to the cell, coalescing with the
// previous segment when it is also plain text so that wrapping sees runs of
// the same font as a single unit.
func (c *tableCell) appendText(s string) {
	if s == "" {
		return
	}
	if n := len(c.segments); n > 0 && !c.segments[n-1].mono {
		c.segments[n-1].text += s
	} else {
		c.segments = append(c.segments, cellSegment{text: s})
	}
}

// appendCode adds a monospace (inline code) fragment to the cell. Code
// segments are never coalesced with plain text so the font boundary is exact.
func (c *tableCell) appendCode(s string) {
	if s == "" {
		return
	}
	c.segments = append(c.segments, cellSegment{text: s, mono: true})
}

type tableCell struct {
	segments []cellSegment
	align    string // "L", "C", "R"
	isHeader bool
}

type tableRow struct {
	cells []tableCell
}

type tableData struct {
	rows      []tableRow
	alignMode string // alignment for the full table
	colWidths []float64
}

// --- table handlers ---

func (r *renderer) handleTableEnter(node *extast.Table) {
	r.inTable = true
	r.tableData = &tableData{
		alignMode: "L",
	}
}

func (r *renderer) handleTableExit() {
	r.renderTable()
	r.inTable = false
	r.tableData = nil
}

func (r *renderer) handleTableRowEnter() {
	r.curRow = &tableRow{}
}

func (r *renderer) handleTableRowExit() {
	if r.curRow != nil {
		r.tableData.rows = append(r.tableData.rows, *r.curRow)
	}
	r.curRow = nil
}

func (r *renderer) handleTableCellEnter(node *extast.TableCell) {
	dir := "L"
	switch node.Alignment {
	case extast.AlignRight:
		dir = "R"
	case extast.AlignCenter:
		dir = "C"
	}
	r.curCell = &tableCell{
		align:    dir,
		isHeader: false, // we detect header via TableHead wrapper
	}
	// Check if this cell's parent is a TableHeader
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if _, ok := parent.(*extast.TableHeader); ok {
			r.curCell.isHeader = true
			break
		}
	}
}

func (r *renderer) handleTableCellExit() {
	if r.curCell != nil && r.curRow != nil {
		r.curRow.cells = append(r.curRow.cells, *r.curCell)
	}
	r.curCell = nil
}

func (r *renderer) renderTable() {
	if r.tableData == nil || len(r.tableData.rows) == 0 {
		return
	}

	// Determine column count from the row with the most cells
	numCols := 0
	for _, row := range r.tableData.rows {
		if len(row.cells) > numCols {
			numCols = len(row.cells)
		}
	}
	if numCols == 0 {
		return
	}

	// Calculate natural and minimum widths per column.
	// Natural width is content-driven; minimum width protects short columns
	// (e.g. "Puntaje") from collapsing when another column is very wide.
	naturalWidths := make([]float64, numCols)
	minWidths := make([]float64, numCols)
	for i := range minWidths {
		minWidths[i] = 12
	}

	r.pdf.SetFont(defFontFamily, "", tableBodySize)
	for _, row := range r.tableData.rows {
		for i, cell := range row.cells {
			// Include cell padding (2 × cellPadX) plus slack so SplitText and
			// GetStringWidth rounding differences do not force unwanted wraps.
			// Natural width is measured with the regular body font (code spans
			// with the mono font) to mirror the pre-segment behaviour.
			cw := r.cellContentWidth(&cell, false) + fontMM(fontSize)*3.5
			if cw > naturalWidths[i] {
				naturalWidths[i] = cw
			}
			if cell.isHeader {
				// Include cell padding (2 × cellPadX) plus slack so short
				// headers (e.g. "Puntaje") stay on one line. Measured with
				// the bold font so the minimum protects the wider bold glyphs.
				hw := r.cellContentWidth(&cell, true) + fontMM(fontSize)*4.0
				if hw > minWidths[i] {
					minWidths[i] = hw
				}
			}
		}
	}
	r.pdf.SetFont(defFontFamily, "", tableBodySize)
	for i := range naturalWidths {
		if naturalWidths[i] < minWidths[i] {
			naturalWidths[i] = minWidths[i]
		}
	}

	availWidth := r.width - 4
	colWidths := fitTableColumnWidths(naturalWidths, minWidths, availWidth)
	totalWidth := 0.0
	for _, w := range colWidths {
		totalWidth += w
	}

	// Spacing before table
	r.pdf.Ln(4)

	// Table is left-aligned regardless of width
	xStart := marginLeft + 2
	tableEndX := xStart + totalWidth
	cellPadX := fontMM(fontSize) * 1.0
	cellPadY := fontMM(fontSize) * 0.5

	// Ensure visible borders: set draw colour and line width for the whole table
	r.pdf.SetDrawColor(120, 120, 120) // medium gray borders
	r.pdf.SetLineWidth(0.3)

	for rowIdx, row := range r.tableData.rows {
		// Determine if this is a header row (first row with isHeader cells)
		isHeaderRow := len(row.cells) > 0 && row.cells[0].isHeader

		// Pre-wrap each cell to compute dynamic row height.
		wrappedSegsByCol := make([][][]cellSegment, numCols)
		maxLines := 1
		for colIdx := 0; colIdx < numCols; colIdx++ {
			var cell tableCell
			if colIdx < len(row.cells) {
				cell = row.cells[colIdx]
			}

			if cell.isHeader {
				r.pdf.SetFont(defFontFamily, "B", tableBodySize)
			} else {
				r.pdf.SetFont(defFontFamily, "", tableBodySize)
			}
			lineSegs := r.wrapCellSegments(&cell, colWidths[colIdx]-2*cellPadX)
			wrappedSegsByCol[colIdx] = lineSegs
			if len(lineSegs) > maxLines {
				maxLines = len(lineSegs)
			}
		}

		rowH := float64(maxLines)*lineHeight + 2*cellPadY

		// Add a page break before the row if needed.
		rowY := r.pdf.GetY()
		_, brkMargin := r.pdf.GetAutoPageBreak()
		_, pageH := r.pdf.GetPageSize()
		if rowY+rowH > pageH-brkMargin {
			r.pdf.AddPage()
			rowY = r.pdf.GetY()
		}

		x := xStart
		for colIdx := 0; colIdx < numCols; colIdx++ {
			var cell tableCell
			if colIdx < len(row.cells) {
				cell = row.cells[colIdx]
			}

			if cell.isHeader {
				r.pdf.SetFont(defFontFamily, "B", tableBodySize)
				r.pdf.SetFillColor(222, 235, 247) // subtle blue header background
				r.pdf.SetTextColor(44, 62, 80)    // dark slate text
			} else {
				r.pdf.SetFont(defFontFamily, "", tableBodySize)
				// Alternate row background — skip for header (uses its own fill)
				if rowIdx%2 == 0 {
					r.pdf.SetFillColor(255, 255, 255)
				} else {
					r.pdf.SetFillColor(245, 247, 250)
				}
				r.pdf.SetTextColor(51, 51, 51) // dark gray body text
			}

			// Cell background + border.
			r.pdf.Rect(x, rowY, colWidths[colIdx], rowH, "FD")

			// Cell text (wrapped).
			textY := rowY + cellPadY
			innerW := colWidths[colIdx] - 2*cellPadX
			for _, line := range wrappedSegsByCol[colIdx] {
				r.drawCellSegmentsLine(x+cellPadX, textY, innerW, cell.align, cell.isHeader, line)
				textY += lineHeight
			}

			x += colWidths[colIdx]
		}

		r.pdf.SetXY(xStart, rowY+rowH)

		// Draw a thicker separator line under the header row.
		if isHeaderRow && rowIdx+1 < len(r.tableData.rows) {
			r.pdf.SetDrawColor(80, 90, 100) // darker line under header
			r.pdf.SetLineWidth(0.6)
			r.pdf.Line(xStart, rowY+rowH-0.3, tableEndX, rowY+rowH-0.3)
			// Restore table border style for data rows.
			r.pdf.SetDrawColor(120, 120, 120)
			r.pdf.SetLineWidth(0.3)
		}
	}

	// Reset colours and font after table
	r.pdf.SetDrawColor(0, 0, 0)
	r.pdf.SetLineWidth(0.2)
	r.pdf.SetTextColor(0, 0, 0)

	// Spacing after table
	r.pdf.SetFont(defFontFamily, "", fontSize)
	r.pdf.Ln(4)
}

func fitTableColumnWidths(naturalWidths, minWidths []float64, availWidth float64) []float64 {
	numCols := len(naturalWidths)
	if numCols == 0 {
		return nil
	}

	widths := make([]float64, numCols)
	mins := make([]float64, numCols)
	sumNatural := 0.0
	sumMin := 0.0

	for i := 0; i < numCols; i++ {
		m := 12.0
		if i < len(minWidths) && minWidths[i] > m {
			m = minWidths[i]
		}
		mins[i] = m
		sumMin += m

		w := m
		if i < len(naturalWidths) && naturalWidths[i] > w {
			w = naturalWidths[i]
		}
		widths[i] = w
		sumNatural += w
	}

	if availWidth <= 0 {
		return widths
	}
	if sumNatural <= availWidth {
		return widths
	}

	// If even minimum widths do not fit, scale minima proportionally.
	if sumMin >= availWidth {
		out := make([]float64, numCols)
		if sumMin == 0 {
			each := availWidth / float64(numCols)
			for i := range out {
				out[i] = each
			}
		} else {
			for i := range out {
				out[i] = availWidth * (mins[i] / sumMin)
			}
		}
		delta := availWidth
		for _, w := range out {
			delta -= w
		}
		out[numCols-1] += delta
		return out
	}

	remaining := availWidth - sumMin
	extras := make([]float64, numCols)
	sumExtra := 0.0
	for i := 0; i < numCols; i++ {
		extra := widths[i] - mins[i]
		if extra < 0 {
			extra = 0
		}
		extras[i] = extra
		sumExtra += extra
	}

	out := make([]float64, numCols)
	if sumExtra == 0 {
		eachExtra := remaining / float64(numCols)
		for i := range out {
			out[i] = mins[i] + eachExtra
		}
	} else {
		for i := range out {
			out[i] = mins[i] + remaining*(extras[i]/sumExtra)
		}
	}

	delta := availWidth
	for _, w := range out {
		delta -= w
	}
	out[numCols-1] += delta
	return out
}

func (r *renderer) wrapTableCellText(text string, maxWidth float64) []string {
	text = sanitizePDFText(text)
	if text == "" {
		return []string{""}
	}
	if maxWidth <= 0 {
		return []string{text}
	}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(text, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			out = append(out, "")
			continue
		}
		lines := r.pdf.SplitText(part, maxWidth)
		if len(lines) == 0 {
			out = append(out, "")
			continue
		}
		out = append(out, lines...)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func (r *renderer) emojiTextWidth(text string) float64 {
	width := 0.0
	emojiW := (lineHeight - 0.5) + 0.5
	for _, seg := range splitTextSegments(text) {
		if len(seg.emojis) > 0 {
			width += emojiW
			continue
		}
		width += r.pdf.GetStringWidth(sanitizePDFText(seg.text))
	}
	return width
}

// textContainsEmoji reports whether s holds any emoji rune (including the
// start of a ZWJ / modifier sequence). It is used to guard fpdf calls that
// index a width table by rune up to U+FFFF (e.g. SplitText, GetStringWidth
// on raw text) so supplementary-plane emoji do not panic them.
func textContainsEmoji(s string) bool {
	for _, r := range s {
		if emoji.IsEmojiRune(r) {
			return true
		}
	}
	return false
}

// setCellSegFont selects the font for a cell segment: monospace for code,
// bold body for header cells, regular body otherwise. The caller must restore
// the ambient font when it needs to (the table render loop sets fonts per cell).
func (r *renderer) setCellSegFont(mono, isHeader bool) {
	if mono {
		r.pdf.SetFont(monoFontFamily, "", tableMonoSize)
	} else if isHeader {
		r.pdf.SetFont(defFontFamily, "B", tableBodySize)
	} else {
		r.pdf.SetFont(defFontFamily, "", tableBodySize)
	}
}

// cellContentWidth returns the unwrapped rendered width of a cell's segments,
// measuring each with its own font (mono for code, body/body-bold for text).
// The bold flag selects the body style for plain-text segments so the caller
// can measure the natural width (regular) and the header minimum (bold)
// independently, mirroring the pre-segment behaviour. Newlines are treated as
// spaces so GetStringWidth sees a single run.
func (r *renderer) cellContentWidth(cell *tableCell, bold bool) float64 {
	var w float64
	for _, seg := range cell.segments {
		if seg.mono {
			// Monospace code: measure with the mono font. Code spans never
			// contain emoji, so GetStringWidth(sanitize) is safe here.
			r.pdf.SetFont(monoFontFamily, "", tableMonoSize)
			s := strings.ReplaceAll(seg.text, "\n", " ")
			w += r.pdf.GetStringWidth(sanitizePDFText(s))
		} else {
			// Body text: measure with the body font and account for emoji
			// image widths (emojiTextWidth handles emoji via splitTextSegments,
			// so supplementary-plane runes do not panic fpdf's width table).
			if bold {
				r.pdf.SetFont(defFontFamily, "B", tableBodySize)
			} else {
				r.pdf.SetFont(defFontFamily, "", tableBodySize)
			}
			s := strings.ReplaceAll(seg.text, "\n", " ")
			w += r.emojiTextWidth(s)
		}
	}
	return w
}

// cellToken is a whitespace or non-whitespace run within a cell segment,
// carrying the segment's mono flag so each token is measured and drawn with
// the correct font.
type cellToken struct {
	text    string
	mono    bool
	isSpace bool
}

// tokenizeCellSegment splits a segment into alternating space / non-space
// tokens, preserving spaces so the wrapped text reflows identically to the
// source.
func tokenizeCellSegment(seg cellSegment) []cellToken {
	s := seg.text
	var toks []cellToken
	i := 0
	for i < len(s) {
		j := i
		sp := s[i] == ' ' || s[i] == '\t'
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') == sp {
			j++
		}
		toks = append(toks, cellToken{text: s[i:j], mono: seg.mono, isSpace: sp})
		i = j
	}
	return toks
}

// cellTokenWidth measures a token with its own font. Body text uses
// emojiTextWidth so emoji images contribute their real width.
func (r *renderer) cellTokenWidth(tk cellToken, isHeader bool) float64 {
	r.setCellSegFont(tk.mono, isHeader)
	if tk.mono {
		return r.pdf.GetStringWidth(sanitizePDFText(tk.text))
	}
	return r.emojiTextWidth(tk.text)
}

// wrapCellSegments wraps a cell's segments into lines of segments, respecting
// per-segment fonts and explicit newlines. Each returned line is a slice of
// segments ready to draw with drawCellSegmentsLine.
func (r *renderer) wrapCellSegments(cell *tableCell, maxWidth float64) [][]cellSegment {
	segs := cell.segments
	if len(segs) == 0 {
		return [][]cellSegment{{}}
	}
	if maxWidth <= 0 {
		return [][]cellSegment{append([]cellSegment(nil), segs...)}
	}

	// First split on explicit newlines (from <br> and source line breaks)
	// into rows; each row is then wrapped by words independently.
	var rows [][]cellSegment
	var cur []cellSegment
	for _, seg := range segs {
		parts := strings.Split(seg.text, "\n")
		for i, part := range parts {
			if i > 0 {
				rows = append(rows, cur)
				cur = nil
			}
			if part != "" {
				cur = append(cur, cellSegment{text: part, mono: seg.mono})
			}
		}
	}
	rows = append(rows, cur)

	var lines [][]cellSegment
	for _, row := range rows {
		if len(row) == 0 {
			lines = append(lines, []cellSegment{{}}) // blank line
			continue
		}
		lines = append(lines, r.wrapRowByWords(cell.isHeader, row, maxWidth)...)
	}
	if len(lines) == 0 {
		lines = [][]cellSegment{{}}
	}
	return lines
}

// wrapRowByWords greedily wraps one newline-free row of segments into lines,
// measuring each word with its own font. A single word wider than maxWidth is
// force-split with fpdf.SplitText using that word's font. Leading and trailing
// spaces on a wrapped line are dropped.
func (r *renderer) wrapRowByWords(isHeader bool, row []cellSegment, maxWidth float64) [][]cellSegment {
	var toks []cellToken
	for _, seg := range row {
		toks = append(toks, tokenizeCellSegment(seg)...)
	}
	if len(toks) == 0 {
		return [][]cellSegment{{}}
	}

	var lines [][]cellSegment
	var cur []int // indices into toks on the current line
	curW := 0.0

	flush := func() {
		// drop trailing space tokens
		for len(cur) > 0 && toks[cur[len(cur)-1]].isSpace {
			cur = cur[:len(cur)-1]
		}
		var line []cellSegment
		for _, idx := range cur {
			tk := toks[idx]
			if n := len(line); n > 0 && line[n-1].mono == tk.mono {
				line[n-1].text += tk.text
			} else {
				line = append(line, cellSegment{text: tk.text, mono: tk.mono})
			}
		}
		if line == nil {
			line = []cellSegment{{}}
		}
		lines = append(lines, line)
		cur = nil
		curW = 0.0
	}

	for i, tk := range toks {
		// No leading whitespace on a fresh line.
		if tk.isSpace && len(cur) == 0 {
			continue
		}
		tw := r.cellTokenWidth(tk, isHeader)
		// A single non-space word wider than the column: force-split it.
		// SplitText indexes fpdf's width table by rune up to U+FFFF, so it
		// panics on supplementary-plane emoji. Tokens that contain emoji are
		// placed on their own line intact instead of being force-split.
		if !tk.isSpace && tw > maxWidth {
			if len(cur) > 0 {
				flush()
			}
			if !tk.mono && textContainsEmoji(tk.text) {
				lines = append(lines, []cellSegment{{text: tk.text, mono: tk.mono}})
				continue
			}
			r.setCellSegFont(tk.mono, isHeader)
			for _, p := range r.pdf.SplitText(tk.text, maxWidth) {
				lines = append(lines, []cellSegment{{text: p, mono: tk.mono}})
			}
			continue
		}
		if curW+tw > maxWidth && len(cur) > 0 {
			flush()
			if tk.isSpace {
				continue
			}
		}
		cur = append(cur, i)
		curW += tw
	}
	if len(cur) > 0 {
		flush()
	}
	if len(lines) == 0 {
		lines = [][]cellSegment{{}}
	}
	return lines
}

// segDrawWidth returns the rendered width of one segment on a drawn line,
// accounting for emoji image widths in body text.
func (r *renderer) segDrawWidth(seg cellSegment, isHeader bool) float64 {
	r.setCellSegFont(seg.mono, isHeader)
	if seg.mono {
		return r.pdf.GetStringWidth(sanitizePDFText(seg.text))
	}
	return r.emojiTextWidth(seg.text)
}

// drawCellSegmentsLine draws one wrapped line of a table cell, switching to
// the monospace font for code segments and rendering emoji inline for body
// segments. The whole line is aligned within innerW according to align.
func (r *renderer) drawCellSegmentsLine(x, y, innerW float64, align string, isHeader bool, line []cellSegment) {
	lineW := 0.0
	for _, seg := range line {
		lineW += r.segDrawWidth(seg, isHeader)
	}
	startX := x
	switch align {
	case "C":
		startX = x + (innerW-lineW)/2
	case "R":
		startX = x + innerW - lineW
	}
	if startX < x {
		startX = x
	}

	r.pdf.SetXY(startX, y)
	for _, seg := range line {
		if seg.mono {
			r.pdf.SetFont(monoFontFamily, "", tableMonoSize)
			if t := sanitizePDFText(seg.text); t != "" {
				r.pdf.Write(lineHeight, t)
			}
		} else {
			r.setCellSegFont(false, isHeader)
			r.renderTextWithEmoji(seg.text)
		}
	}
}
