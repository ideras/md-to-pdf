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
	text       string
	mono       bool
	color      *[3]int
	background *[3]int
	fontFamily string // empty means inherited/default
}

func (seg cellSegment) style() inlineStyle {
	color := [3]int{}
	set := seg.color != nil
	if set {
		color = *seg.color
	}
	return inlineStyle{textColor: color, textColorSet: set, background: cloneColor(seg.background), fontFamily: seg.fontFamily}
}

func cellSegmentFromStyle(text string, mono bool, style inlineStyle) cellSegment {
	var color *[3]int
	if style.textColorSet {
		value := style.textColor
		color = &value
	}
	return cellSegment{text: text, mono: mono, color: color, background: cloneColor(style.background), fontFamily: style.fontFamily}
}

func sameCellStyle(a, b cellSegment) bool {
	return a.mono == b.mono && sameInlineStyle(a.style(), b.style())
}

// appendText remains a convenient unstyled helper for table tests and callers.
func (c *tableCell) appendText(s string) { c.appendStyledText(s, inlineStyle{fontFamily: "default"}) }

func (c *tableCell) appendStyledText(s string, style inlineStyle) {
	if s == "" {
		return
	}
	seg := cellSegmentFromStyle(s, false, style)
	if n := len(c.segments); n > 0 && sameCellStyle(c.segments[n-1], seg) {
		c.segments[n-1].text += s
		return
	}
	c.segments = append(c.segments, seg)
}

// appendCode remains a convenient unstyled helper for table tests and callers.
func (c *tableCell) appendCode(s string) { c.appendStyledCode(s, inlineStyle{fontFamily: "default"}) }

func (c *tableCell) appendStyledCode(s string, style inlineStyle) {
	if s == "" {
		return
	}
	seg := cellSegmentFromStyle(s, true, style)
	if n := len(c.segments); n > 0 && sameCellStyle(c.segments[n-1], seg) {
		c.segments[n-1].text += s
		return
	}
	c.segments = append(c.segments, seg)
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

	r.pdf.SetFont(defFontFamily, "", r.tableBodyFontSize())
	for _, row := range r.tableData.rows {
		for i, cell := range row.cells {
			// Include cell padding (2 × cellPadX) plus slack so SplitText and
			// GetStringWidth rounding differences do not force unwanted wraps.
			// Slack scales with the table's actual body font size: using the
			// document default font size here balloons narrow numeric columns
			// when a dense report opts into a smaller table font.
			// Natural width is measured with the regular body font (code spans
			// with the mono font) to mirror the pre-segment behaviour.
			cw := r.cellContentWidth(&cell, false) + fontMM(r.tableBodyFontSize())*3.5
			if cw > naturalWidths[i] {
				naturalWidths[i] = cw
			}
			if cell.isHeader {
				// Include cell padding (2 × cellPadX) plus slack so short
				// headers (e.g. "Puntaje") stay on one line. Measured with
				// the bold font so the minimum protects the wider bold glyphs.
				hw := r.cellContentWidth(&cell, true) + fontMM(r.tableBodyFontSize())*4.0
				if hw > minWidths[i] {
					minWidths[i] = hw
				}
			}
		}
	}
	r.pdf.SetFont(defFontFamily, "", r.tableBodyFontSize())
	for i := range naturalWidths {
		if naturalWidths[i] < minWidths[i] {
			naturalWidths[i] = minWidths[i]
		}
	}

	availWidth := r.width - 4
	colWidths := fitTableColumnWidths(naturalWidths, minWidths, r.tableStyle.ColumnWeights, availWidth)
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
	if r.tableStyle.Borderless {
		// Borderless tables do not need room for visible cell outlines, so use
		// tighter padding to give dense reports more space for their content.
		cellPadX = fontMM(r.tableBodyFontSize()) * 0.7
		cellPadY = fontMM(r.tableBodyFontSize()) * 0.4
	}

	if !r.tableStyle.Borderless {
		// Ensure visible borders: set draw colour and line width for the whole table.
		r.pdf.SetDrawColor(120, 120, 120) // medium gray borders
		r.pdf.SetLineWidth(0.3)
	}

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
				r.pdf.SetFont(defFontFamily, "B", r.tableBodyFontSize())
			} else {
				r.pdf.SetFont(defFontFamily, "", r.tableBodyFontSize())
			}
			lineSegs := r.wrapCellSegments(&cell, colWidths[colIdx]-2*cellPadX)
			wrappedSegsByCol[colIdx] = lineSegs
			if len(lineSegs) > maxLines {
				maxLines = len(lineSegs)
			}
		}

		rowH := float64(maxLines)*r.tableLineHeight() + 2*cellPadY

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
				r.pdf.SetFont(defFontFamily, "B", r.tableBodyFontSize())
				r.pdf.SetFillColor(222, 235, 247) // subtle blue header background
				r.pdf.SetTextColor(44, 62, 80)    // dark slate text
			} else {
				r.pdf.SetFont(defFontFamily, "", r.tableBodyFontSize())
				// Alternate row background — skip for header (uses its own fill)
				if rowIdx%2 == 0 {
					r.pdf.SetFillColor(255, 255, 255)
				} else {
					r.pdf.SetFillColor(245, 247, 250)
				}
				r.pdf.SetTextColor(51, 51, 51) // dark gray body text
			}

			// Cell background, with an optional outline.
			drawMode := "FD"
			if r.tableStyle.Borderless {
				drawMode = "F"
			}
			r.pdf.Rect(x, rowY, colWidths[colIdx], rowH, drawMode)

			// Cell text (wrapped).
			textY := rowY + cellPadY
			innerW := colWidths[colIdx] - 2*cellPadX
			for _, line := range wrappedSegsByCol[colIdx] {
				r.drawCellSegmentsLine(x+cellPadX, textY, innerW, cell.align, cell.isHeader, line)
				textY += r.tableLineHeight()
			}

			x += colWidths[colIdx]
		}

		r.pdf.SetXY(xStart, rowY+rowH)

		// Draw a thicker separator line under the header row.
		if !r.tableStyle.Borderless && isHeaderRow && rowIdx+1 < len(r.tableData.rows) {
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

	// Spacing after table. Restore the enclosing inline span style rather than
	// leaking table body defaults into following content.
	r.pdf.SetFontStyle("")
	r.applyInlineStyle()
	r.pdf.Ln(4)
}

// fitTableColumnWidths distributes availWidth across the table's columns.
// Without any positive column weight it keeps the content-driven legacy
// behavior. With at least one positive weight, weighted columns share the
// width left after unweighted columns keep their natural content width, so a
// dominant column (e.g. a long client name) is not squeezed by surrounding
// short numeric columns whose content is already narrower than its heading.
func fitTableColumnWidths(naturalWidths, minWidths, columnWeights []float64, availWidth float64) []float64 {
	numCols := len(naturalWidths)
	if numCols == 0 {
		return nil
	}

	// Normalized minimums and content widths (natural at least its minimum).
	mins := make([]float64, numCols)
	naturals := make([]float64, numCols)
	for i := 0; i < numCols; i++ {
		m := 12.0
		if i < len(minWidths) && minWidths[i] > m {
			m = minWidths[i]
		}
		mins[i] = m
		naturals[i] = m
		if i < len(naturalWidths) && naturalWidths[i] > naturals[i] {
			naturals[i] = naturalWidths[i]
		}
	}

	weightOf := func(i int) float64 {
		if i < len(columnWeights) && columnWeights[i] > 0 {
			return columnWeights[i]
		}
		return 0
	}

	unweightedWidth := 0.0
	weightSum := 0.0
	weightedWidth := 0.0
	for i := 0; i < numCols; i++ {
		if weightOf(i) > 0 {
			weightSum += weightOf(i)
			continue
		}
		unweightedWidth += naturals[i]
	}
	weightedWidth = availWidth - unweightedWidth

	// Collect the columns that receive weighted distribution and the shortest
	// width they can honestly hold (headers on one line).
	widths := make([]float64, numCols)
	open := make([]int, 0, numCols)
	weightedMinSum := 0.0
	for i := 0; i < numCols; i++ {
		if weightOf(i) == 0 {
			widths[i] = naturals[i]
			continue
		}
		widths[i] = -1
		open = append(open, i)
		weightedMinSum += mins[i]
	}

	if weightSum <= 0 || availWidth <= 0 || weightedWidth < weightedMinSum {
		ok := weightSum > 0 && availWidth > 0 &&
			drainUnweightedHeadroom(naturals, mins, columnWeights, widths, availWidth)
		if !ok {
			return fitTableColumnWidthsByContent(naturals, mins, availWidth)
		}
		weightedWidth = weightedMinSum
	}

	// Assign weighted columns proportional to their weights. Columns whose
	// share would fall below their minimum are pinned at the minimum and the
	// remaining columns re-share what is left.
	fixedWeighted := 0.0
	for range numCols + 1 {
		if len(open) == 0 {
			break
		}
		flexBudget := weightedWidth - fixedWeighted
		flexWeight := 0.0
		for _, i := range open {
			flexWeight += weightOf(i)
		}
		if flexWeight <= 0 || flexBudget <= 0 {
			// Nothing left to share: pin the rest to their minimums and let the
			// final overflow correction restore consistency.
			for _, i := range open {
				widths[i] = mins[i]
			}
			break
		}
		shifted := false
		var stillOpen []int
		for _, i := range open {
			widths[i] = flexBudget * weightOf(i) / flexWeight
			if widths[i] < mins[i] {
				widths[i] = mins[i]
				fixedWeighted += mins[i]
				shifted = true
				continue
			}
			stillOpen = append(stillOpen, i)
		}
		open = stillOpen
		if !shifted {
			break
		}
	}

	totalWidth := 0.0
	for _, w := range widths {
		totalWidth += w
	}
	if totalWidth > availWidth {
		// Degenerate configuration (minimums alone overflow): fall back to the
		// content-driven distribution rather than exceeding the page width.
		return fitTableColumnWidthsByContent(naturals, mins, availWidth)
	}

	delta := availWidth
	for _, w := range widths {
		delta -= w
	}
	widths[numCols-1] += delta
	return widths
}

// drainUnweightedHeadroom reports whether the shortfall a weighted column
// needs can be recovered by shrinking the unweighted columns from their
// natural widths toward their minimums. On success it adjusts widths in place
// and the weighted pool becomes the weighted minimum sum.
func drainUnweightedHeadroom(naturals, mins, columnWeights, widths []float64, availWidth float64) bool {
	weightedMinSum := 0.0
	for i := range naturals {
		weight := 0.0
		if i < len(columnWeights) && columnWeights[i] > 0 {
			weight = columnWeights[i]
		}
		if weight > 0 {
			weightedMinSum += mins[i]
		}
	}
	pool := availWidth
	for i := range naturals {
		weight := 0.0
		if i < len(columnWeights) && columnWeights[i] > 0 {
			weight = columnWeights[i]
		}
		if weight == 0 {
			pool -= naturals[i]
		}
	}
	deficit := weightedMinSum - pool
	extraSum := 0.0
	for i := range naturals {
		weight := 0.0
		if i < len(columnWeights) && columnWeights[i] > 0 {
			weight = columnWeights[i]
		}
		if weight == 0 && naturals[i] > mins[i] {
			extraSum += naturals[i] - mins[i]
		}
	}
	if deficit <= 0 || extraSum < deficit {
		return false
	}
	for i := range naturals {
		weight := 0.0
		if i < len(columnWeights) && columnWeights[i] > 0 {
			weight = columnWeights[i]
		}
		if weight == 0 {
			extra := naturals[i] - mins[i]
			if extra > 0 {
				widths[i] = naturals[i] - deficit*(extra/extraSum)
			} else {
				widths[i] = naturals[i]
			}
		}
	}
	return true
}

// fitTableColumnWidthsByContent sizes columns purely by content: natural
// widths fill the page only when they fit, otherwise the surplus above the
// minimum widths is distributed proportionally.
func fitTableColumnWidthsByContent(naturalWidths, minWidths []float64, availWidth float64) []float64 {
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
	emojiW := (r.tableLineHeight() - 0.5) + 0.5
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

// setCellSegFont selects a built-in font. It remains for existing callers;
// styled table rendering uses setCellSegmentFont below.
func (r *renderer) setCellSegFont(mono, isHeader bool) {
	r.setCellSegmentFont(cellSegment{mono: mono}, isHeader)
}

func (r *renderer) setCellSegmentFont(seg cellSegment, isHeader bool) {
	if seg.mono {
		r.pdf.SetFont(monoFontFamily, "", r.tableMonoFontSize())
		return
	}
	family, ok := r.resolveFontFamily(seg.fontFamily)
	if !ok {
		family = defFontFamily
	}
	style := ""
	if isHeader {
		style = "B"
	}
	r.pdf.SetFont(family, style, r.tableBodyFontSize())
}

func (r *renderer) cellContentWidth(cell *tableCell, bold bool) float64 {
	var width float64
	for _, seg := range cell.segments {
		r.setCellSegmentFont(seg, bold)
		text := strings.ReplaceAll(seg.text, "\n", " ")
		if seg.mono {
			width += r.pdf.GetStringWidth(sanitizePDFText(text))
		} else {
			width += r.emojiTextWidth(text)
		}
	}
	return width
}

type cellToken struct {
	segment cellSegment
	isSpace bool
}

func tokenizeCellSegment(seg cellSegment) []cellToken {
	var tokens []cellToken
	for i := 0; i < len(seg.text); {
		j := i
		space := seg.text[i] == ' ' || seg.text[i] == '\t'
		for j < len(seg.text) && (seg.text[j] == ' ' || seg.text[j] == '\t') == space {
			j++
		}
		part := seg
		part.text = seg.text[i:j]
		tokens = append(tokens, cellToken{segment: part, isSpace: space})
		i = j
	}
	return tokens
}

func (r *renderer) cellTokenWidth(token cellToken, isHeader bool) float64 {
	r.setCellSegmentFont(token.segment, isHeader)
	if token.segment.mono {
		return r.pdf.GetStringWidth(sanitizePDFText(token.segment.text))
	}
	return r.emojiTextWidth(token.segment.text)
}

func (r *renderer) wrapCellSegments(cell *tableCell, maxWidth float64) [][]cellSegment {
	if len(cell.segments) == 0 {
		return [][]cellSegment{{}}
	}
	if maxWidth <= 0 {
		return [][]cellSegment{append([]cellSegment(nil), cell.segments...)}
	}
	var rows [][]cellSegment
	var current []cellSegment
	for _, segment := range cell.segments {
		for index, part := range strings.Split(segment.text, "\n") {
			if index > 0 {
				rows = append(rows, current)
				current = nil
			}
			if part != "" {
				segmentCopy := segment
				segmentCopy.text = part
				current = append(current, segmentCopy)
			}
		}
	}
	rows = append(rows, current)
	var lines [][]cellSegment
	for _, row := range rows {
		if len(row) == 0 {
			lines = append(lines, []cellSegment{{}})
			continue
		}
		lines = append(lines, r.wrapRowByWords(cell.isHeader, row, maxWidth)...)
	}
	if len(lines) == 0 {
		return [][]cellSegment{{}}
	}
	return lines
}

func (r *renderer) wrapRowByWords(isHeader bool, row []cellSegment, maxWidth float64) [][]cellSegment {
	var tokens []cellToken
	for _, segment := range row {
		tokens = append(tokens, tokenizeCellSegment(segment)...)
	}
	if len(tokens) == 0 {
		return [][]cellSegment{{}}
	}
	var lines [][]cellSegment
	var current []cellToken
	width := 0.0
	flush := func() {
		for len(current) > 0 && current[len(current)-1].isSpace {
			current = current[:len(current)-1]
		}
		var line []cellSegment
		for _, token := range current {
			segment := token.segment
			if n := len(line); n > 0 && sameCellStyle(line[n-1], segment) {
				line[n-1].text += segment.text
			} else {
				line = append(line, segment)
			}
		}
		if len(line) == 0 {
			line = []cellSegment{{}}
		}
		lines = append(lines, line)
		current = nil
		width = 0
	}
	for _, token := range tokens {
		if token.isSpace && len(current) == 0 {
			continue
		}
		tokenWidth := r.cellTokenWidth(token, isHeader)
		if !token.isSpace && tokenWidth > maxWidth {
			if len(current) > 0 {
				flush()
			}
			if !token.segment.mono && textContainsEmoji(token.segment.text) {
				lines = append(lines, []cellSegment{token.segment})
				continue
			}
			r.setCellSegmentFont(token.segment, isHeader)
			for _, part := range r.pdf.SplitText(token.segment.text, maxWidth) {
				segment := token.segment
				segment.text = part
				lines = append(lines, []cellSegment{segment})
			}
			continue
		}
		if width+tokenWidth > maxWidth && len(current) > 0 {
			flush()
			if token.isSpace {
				continue
			}
		}
		current = append(current, token)
		width += tokenWidth
	}
	if len(current) > 0 {
		flush()
	}
	if len(lines) == 0 {
		return [][]cellSegment{{}}
	}
	return lines
}

func (r *renderer) segDrawWidth(seg cellSegment, isHeader bool) float64 {
	r.setCellSegmentFont(seg, isHeader)
	if seg.mono {
		return r.pdf.GetStringWidth(sanitizePDFText(seg.text))
	}
	return r.emojiTextWidth(seg.text)
}

func (r *renderer) drawCellSegmentsLine(x, y, innerW float64, align string, isHeader bool, line []cellSegment) {
	ambientFamily, ambientStyle := r.pdf.GetFontFamily(), r.pdf.GetFontStyle()
	ambientSize, _ := r.pdf.GetFontSize()
	ambientR, ambientG, ambientB := r.pdf.GetTextColor()
	ambientFillR, ambientFillG, ambientFillB := r.pdf.GetFillColor()
	defer func() {
		r.pdf.SetFont(ambientFamily, ambientStyle, ambientSize)
		r.pdf.SetTextColor(ambientR, ambientG, ambientB)
		r.pdf.SetFillColor(ambientFillR, ambientFillG, ambientFillB)
	}()

	lineWidth := 0.0
	for _, seg := range line {
		lineWidth += r.segDrawWidth(seg, isHeader)
	}
	startX := x
	if align == "C" {
		startX = x + (innerW-lineWidth)/2
	} else if align == "R" {
		startX = x + innerW - lineWidth
	}
	if startX < x {
		startX = x
	}
	r.pdf.SetXY(startX, y)
	for _, seg := range line {
		width := r.segDrawWidth(seg, isHeader)
		if seg.color != nil {
			r.pdf.SetTextColor(seg.color[0], seg.color[1], seg.color[2])
		}
		if seg.background != nil {
			atX, atY := r.pdf.GetXY()
			r.pdf.SetFillColor(seg.background[0], seg.background[1], seg.background[2])
			r.pdf.Rect(atX, atY, width, r.tableLineHeight(), "F")
			r.pdf.SetXY(atX, atY)
		}
		if seg.mono {
			if text := sanitizePDFText(seg.text); text != "" {
				r.pdf.Write(r.tableLineHeight(), text)
			}
		} else {
			r.renderTextWithEmoji(seg.text)
		}
		// A segment's color/background must not become ambient table state.
		r.pdf.SetTextColor(ambientR, ambientG, ambientB)
		r.pdf.SetFillColor(ambientFillR, ambientFillG, ambientFillB)
	}
}
