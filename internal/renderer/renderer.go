// Package renderer implements Markdown-to-PDF rendering.
package renderer

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"codeberg.org/go-pdf/fpdf"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/ideras/md-to-pdf/internal/emoji"
	"github.com/ideras/md-to-pdf/internal/fonts"
)

const (
	marginLeft     = 20.0
	marginRight    = 20.0
	marginTop      = 20.0
	marginBottom   = 20.0
	fontSize       = 11.0
	defFontFamily  = "DejaVu"
	monoFontFamily = "JetBrainsMono"

	// Table cell font sizes. Table body text and inline code are rendered
	// one and two steps below the page body font respectively, so dense
	// tables stay compact and inline code reads as smaller fixed-width text.
	tableBodySize = fontSize - 2 // 9 pt — body text inside table cells
	tableMonoSize = fontSize - 3 // 8 pt — inline code (monospace) inside table cells
)

var lineHeight = fontMM(fontSize) * 1.6

func fontMM(pt float64) float64 {
	return pt * 25.4 / 72.0
}

// Render converts Markdown source to PDF and writes it to w.
func Render(src []byte, w io.Writer) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("render markdown panic: %v", rec)
		}
	}()

	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	reader := text.NewReader(src)
	doc := md.Parser().Parse(reader)

	pdf := fpdf.New("P", "mm", "A4", "")

	// Regular font for body text, with bold/italic variants for headings and emphasis
	pdf.AddUTF8FontFromBytes("DejaVu", "", fonts.Regular)
	pdf.AddUTF8FontFromBytes("DejaVu", "B", fonts.Bold)
	pdf.AddUTF8FontFromBytes("DejaVu", "I", fonts.Italic)
	pdf.AddUTF8FontFromBytes("DejaVu", "BI", fonts.BoldItalic)

	// Mono font for code blocks and spans
	pdf.AddUTF8FontFromBytes("JetBrainsMono", "", fonts.Mono)
	pdf.AddUTF8FontFromBytes("JetBrainsMono", "B", fonts.MonoBold)
	pdf.SetFont("DejaVu", "", fontSize)

	pdf.SetMargins(marginLeft, marginTop, marginRight)
	pdf.SetAutoPageBreak(true, marginBottom)
	pdf.AddPage()
	pdf.SetFont(defFontFamily, "", fontSize)

	r := &renderer{
		pdf:             pdf,
		src:             src,
		width:           210 - marginLeft - marginRight, // A4 width minus margins
		registeredEmoji: make(map[string]bool),
	}

	if err := ast.Walk(doc, r.walk); err != nil {
		return fmt.Errorf("render markdown: %w", err)
	}

	return pdf.Output(w)
}

// --- renderer ---

type renderer struct {
	pdf       *fpdf.Fpdf
	src       []byte
	width     float64
	listIdx   int
	listStack []listState

	// Table state
	inTable   bool
	tableData *tableData
	curRow    *tableRow
	curCell   *tableCell

	// Saved left margin — restored when the outermost list ends
	// so wrapped list text aligns after the bullet, not at the page edge.
	savedLeftMargin float64

	// registeredEmoji tracks emoji sequences already registered as PNG images
	// in this fpdf instance. Keyed by EmojiSequenceKey (e.g. "1f469-200d-1f393").
	registeredEmoji map[string]bool

	// savedFontSizes is a stack used by <sub>/<sup> handling to restore the
	// font size when the closing tag is encountered.
	savedFontSizes []float64

	// Blockquote rendering state.
	// blockquoteDepth counts nesting levels (0 = not inside a blockquote).
	// blockquoteStartY / blockquoteStartPage record where the current
	// blockquote began so we can draw the left accent bar at exit.
	blockquoteDepth     int
	blockquoteStartY    float64
	blockquoteStartPage int
}

type listState struct {
	ordered bool
	idx     int
}

func (r *renderer) walk(n ast.Node, entering bool) (ast.WalkStatus, error) {
	switch node := n.(type) {
	case *ast.Heading:
		r.handleHeading(node, entering)
	case *ast.Paragraph:
		r.handleParagraph(entering)
	case *ast.Text:
		if entering {
			r.handleText(node)
		}
	case *ast.Emphasis:
		r.handleEmphasis(node, entering)
	case *ast.List:
		r.handleList(node, entering)
	case *ast.ListItem:
		r.handleListItem(node, entering)
	case *ast.FencedCodeBlock:
		if entering {
			r.handleFencedCodeBlock(node)
			return ast.WalkSkipChildren, nil
		}
	case *ast.CodeBlock:
		if entering {
			r.handleCodeBlock(node)
			return ast.WalkSkipChildren, nil
		}
	case *ast.CodeSpan:
		if entering {
			r.handleCodeSpan(node)
			return ast.WalkSkipChildren, nil
		}
	case *ast.RawHTML:
		if entering {
			r.handleRawHTML(node)
			return ast.WalkSkipChildren, nil
		}
	case *ast.ThematicBreak:
		if entering {
			r.handleThematicBreak()
		}
	case *ast.TextBlock:
		// TextBlock wraps inline content in tight list items.
		// Only add a line break when there is a following sibling
		// (e.g. a nested List after the paragraph text). Without this
		// check the Ln stacks with ListItem+List Leave Lns and creates
		// excessive gaps — 17mm instead of the natural 7-12mm.
		if !entering && n.NextSibling() != nil {
			r.pdf.Ln(lineHeight)
		}
	case *ast.Blockquote:
		r.handleBlockquote(node, entering)
	case *ast.Link:
		// Render link text; skip the URL
	case *ast.Document:
		// No-op

	// --- Table nodes ---
	case *extast.Table:
		if entering {
			r.handleTableEnter(node)
		} else {
			r.handleTableExit()
		}
	case *extast.TableHeader:
		// TableHeader cells are direct children (no TableRow wrapper in goldmark AST)
		// so we manually manage row lifecycle here
		if entering {
			r.handleTableRowEnter()
		} else {
			r.handleTableRowExit()
		}
	case *extast.TableRow:
		if entering {
			r.handleTableRowEnter()
		} else {
			r.handleTableRowExit()
		}
	case *extast.TableCell:
		if entering {
			r.handleTableCellEnter(node)
		} else {
			r.handleTableCellExit()
		}

	default:
		// Unknown node type — skip
	}
	return ast.WalkContinue, nil
}

// --- block handlers ---

// headingContainsEmoji returns true if the heading's text contains any
// emoji rune.  Used to decide whether to reserve extra space for the emoji
// image in the page-break guard.
func headingContainsEmoji(n *ast.Heading, src []byte) bool {
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if text, ok := child.(*ast.Text); ok {
			for _, r := range string(text.Segment.Value(src)) {
				if emoji.IsEmojiRune(r) {
					return true
				}
			}
		}
	}
	return false
}

func (r *renderer) handleHeading(node *ast.Heading, entering bool) {
	headingSize := 16.0 - float64(node.Level)*2
	if headingSize < fontSize {
		headingSize = fontSize
	}
	if !entering {
		// Use a line advance proportional to the heading font size so that
		// large headings have proper breathing room and their emoji images
		// (which are also sized to the font) never overflow into the next line.
		advance := fontMM(headingSize) * 1.45
		if advance < lineHeight {
			advance = lineHeight
		}
		r.pdf.Ln(advance)
		return
	}
	// Guard against page break between emoji image and heading text:
	// emoji images are placed with absolute coords (not flow), so the
	// PDF auto-page-break only sees the text after them.  If a heading
	// with emoji is near the page bottom, start it fresh on the next page.
	need := fontMM(headingSize) * 2.5 // text line + spacing
	if headingContainsEmoji(node, r.src) {
		need += 4.5 // space for the emoji image
	}
	_, brkMargin := r.pdf.GetAutoPageBreak()
	_, pageH := r.pdf.GetPageSize()
	if r.pdf.GetY()+need > pageH-brkMargin {
		r.pdf.AddPage()
	}
	if y := r.pdf.GetY(); y > marginTop+0.5 {
		r.pdf.Ln(fontMM(headingSize) * 0.5)
	}
	r.pdf.SetFont(defFontFamily, "B", headingSize)
}

func (r *renderer) handleParagraph(entering bool) {
	if entering {
		// Inside a blockquote use italic to visually distinguish the quoted text.
		if r.blockquoteDepth > 0 {
			r.pdf.SetFont(defFontFamily, "I", fontSize)
		} else {
			r.pdf.SetFont(defFontFamily, "", fontSize)
		}
	} else {
		r.pdf.Ln(lineHeight)
	}
}

func (r *renderer) handleBlockquote(node *ast.Blockquote, entering bool) {
	const (
		bqIndent = 6.0 // mm indent per nesting level
		barWidth = 2.5 // mm — left accent bar thickness
	)

	if entering {
		// A little vertical breathing room before the block.
		r.pdf.Ln(2)
		r.blockquoteDepth++
		r.blockquoteStartY = r.pdf.GetY()
		r.blockquoteStartPage = r.pdf.PageNo()

		// Indent all text (including wrapped lines) by setting lMargin.
		newLeft := marginLeft + float64(r.blockquoteDepth)*bqIndent
		r.pdf.SetLeftMargin(newLeft)
		r.pdf.SetX(newLeft)
	} else {
		endY := r.pdf.GetY()

		// --- draw left accent bar ---
		// If the blockquote crossed a page break start the bar at the top
		// margin of the current page (not at the original Y on the old page).
		startY := r.blockquoteStartY
		if r.pdf.PageNo() != r.blockquoteStartPage {
			startY = marginTop
		}
		barX := marginLeft + float64(r.blockquoteDepth-1)*bqIndent + barWidth/2 + 0.5
		r.pdf.SetDrawColor(108, 117, 125) // slate-gray accent
		r.pdf.SetLineWidth(barWidth)
		r.pdf.Line(barX, startY, barX, endY)
		r.pdf.SetDrawColor(0, 0, 0)
		r.pdf.SetLineWidth(0.2)

		// --- restore margins ---
		r.blockquoteDepth--
		if r.blockquoteDepth > 0 {
			r.pdf.SetLeftMargin(marginLeft + float64(r.blockquoteDepth)*bqIndent)
		} else {
			r.pdf.SetLeftMargin(marginLeft)
		}
		r.pdf.Ln(2)
	}
}

// --- inline handlers ---

func (r *renderer) handleText(node *ast.Text) {
	raw := string(node.Segment.Value(r.src))
	// Don't skip empty nodes: a hard line break (two trailing spaces in Markdown)
	// is represented as Text("") with HardLineBreak()==true.
	if raw == "" && !node.HardLineBreak() {
		return
	}
	// If inside a table cell, buffer the RAW text (not sanitised). Emoji
	// runes must survive buffering so the table renderer can draw them as
	// inline images later (drawCellSegmentsLine → renderTextWithEmoji).
	// Sanitising here would replace supplementary-plane emoji (e.g. 👍
	// U+1F44D) with '?', which is the bug where some emojis render as '?'
	// inside tables while BMP emojis (e.g. ✅ U+2705) render fine.
	// renderTextWithEmoji applies sanitizePDFText to plain-text portions only.
	if r.curCell != nil {
		if raw != "" {
			r.curCell.appendText(raw)
		}
		switch {
		case node.HardLineBreak():
			r.curCell.appendText("\n")
		case node.SoftLineBreak():
			r.curCell.appendText("\n")
		}
		return
	}
	// Render text with inline emoji images for emoji runes.
	if raw != "" {
		r.renderTextWithEmoji(raw)
	}
	switch {
	case node.HardLineBreak():
		// Two trailing spaces in Markdown → hard line break: advance to next line.
		r.pdf.Ln(lineHeight)
	case node.SoftLineBreak():
		// Soft line break: honor the source newline as a visible line break.
		r.pdf.Ln(lineHeight)
	}
}

// sanitizePDFText replaces runes unsupported by go-pdf/fpdf UTF-8 writer.
// fpdf internally uses a width table indexed by rune value up to U+FFFF,
// so supplementary-plane code points (e.g. emoji) can panic with
// "index out of range [128xxx] with length 65536".
// ZWJ (U+200D) and variation selectors (U+FE0F/FE0E) are also stripped here
// as a safety net — they are handled upstream by renderEmojiSequence and
// must never reach pdf.Write.
func sanitizePDFText(s string) string {
	if s == "" {
		return s
	}

	needsSanitize := false
	for _, ch := range s {
		if ch > 0xFFFF || ch == 0 || ch == 0x200D || ch == 0xFE0F || ch == 0xFE0E {
			needsSanitize = true
			break
		}
	}
	if !needsSanitize {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, ch := range s {
		switch {
		case ch == 0, ch == 0x200D, ch == 0xFE0F, ch == 0xFE0E:
			// skip NUL, ZWJ, variation selectors
		case ch > 0xFFFF:
			b.WriteRune('?')
		default:
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func (r *renderer) handleEmphasis(node *ast.Emphasis, entering bool) {
	if entering {
		style := "I"
		if node.Level == 2 {
			style = "B"
		}
		r.pdf.SetFontStyle(style)
	} else {
		// Restore the ambient base style for the current context:
		// italic inside a blockquote, regular everywhere else.
		if r.blockquoteDepth > 0 {
			r.pdf.SetFontStyle("I")
		} else {
			r.pdf.SetFontStyle("")
		}
	}
}

func (r *renderer) handleCodeSpan(node *ast.CodeSpan) {
	var buf bytes.Buffer
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			buf.Write(t.Segment.Value(r.src))
		}
	}
	text := sanitizePDFText(buf.String())

	// Inside a table cell, buffer the code text as a monospace segment
	// instead of writing it directly to the PDF. The table is rendered
	// later (in renderTable, from handleTableExit); writing now would
	// place the code at the current cursor Y — above the table — which
	// is the bug where inline code from table cells appears at the top
	// of the page instead of inside its cell. Buffering as a code
	// segment lets renderTable draw it in the monospace font.
	if r.curCell != nil {
		r.curCell.appendCode(text)
		return
	}

	// Render inline code in monospace at slightly smaller size
	r.pdf.SetFont(monoFontFamily, "", fontSize-1)
	r.pdf.Write(lineHeight, text)
	r.pdf.SetFont(defFontFamily, "", fontSize)
}

// --- list handlers ---

func (r *renderer) handleList(node *ast.List, entering bool) {
	if entering {
		// Save the original left margin when entering the outermost list
		if len(r.listStack) == 0 {
			r.savedLeftMargin, _, _, _ = r.pdf.GetMargins()
		}
		r.listStack = append(r.listStack, listState{
			ordered: node.IsOrdered(),
			idx:     node.Start, // use the list's start attribute (e.g. 2 for "2)")
		})
	} else {
		if len(r.listStack) > 0 {
			r.listStack = r.listStack[:len(r.listStack)-1]
		}
		// Only add trailing spacing for top-level lists.
		// Nested lists (parent is a ListItem) get their spacing
		// from the parent ListItem's exit Ln — otherwise Ln(2)
		// stacks with it and creates ~12mm instead of ~5mm.
		if _, isNested := node.Parent().(*ast.ListItem); !isNested {
			r.pdf.Ln(2)
		}
		// Restore original left margin when the outermost list ends
		if len(r.listStack) == 0 {
			r.pdf.SetLeftMargin(r.savedLeftMargin)
		}
	}
}

func (r *renderer) handleListItem(n ast.Node, entering bool) {
	if !entering {
		// Skip trailing Ln for the last item of a nested list.
		// The parent ListItem's trailing Ln already provides the
		// spacing — otherwise we get 5+5=10mm instead of 5mm.
		if n.NextSibling() == nil {
			parent := n.Parent()
			if parent != nil {
				if _, nested := parent.Parent().(*ast.ListItem); nested {
					return
				}
			}
		}
		r.pdf.Ln(lineHeight)
		return
	}
	if len(r.listStack) == 0 {
		return
	}
	state := &r.listStack[len(r.listStack)-1]

	indent := 5.0 * float64(len(r.listStack))
	r.pdf.SetX(marginLeft + indent)
	r.pdf.SetFont(defFontFamily, "", fontSize)

	prefix := "•"
	if state.ordered {
		prefix = fmt.Sprintf("%d.", state.idx)
		state.idx++
	}
	// Use the actual width of the bullet/number + 1mm gap
	bulletW := r.pdf.GetStringWidth(prefix)
	r.pdf.CellFormat(bulletW, lineHeight, prefix, "", 0, "L", false, 0, "")
	textStartX := r.pdf.GetX() + 1
	r.pdf.SetX(textStartX)
	// Set left margin so wrapped lines align with the text after
	// the bullet, not at the page edge.
	r.pdf.SetLeftMargin(textStartX)
}

// --- code block handlers ---

func (r *renderer) handleFencedCodeBlock(node *ast.FencedCodeBlock) {
	lang := string(node.Language(r.src))
	code := r.collectCodeText(node)
	if code == "" {
		return
	}
	if lang != "" {
		if r.renderHighlightedCode(lang, code) {
			return
		}
	}
	r.renderPlainCode(code)
}

func (r *renderer) handleCodeBlock(node *ast.CodeBlock) {
	code := r.collectCodeText(node)
	if code == "" {
		return
	}
	r.renderPlainCode(code)
}

func (r *renderer) collectCodeText(node ast.Node) string {
	lines := node.Lines()
	if lines.Len() == 0 {
		return ""
	}
	var buf bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(r.src))
	}
	return strings.TrimRight(buf.String(), "\n\r")
}

func (r *renderer) renderHighlightedCode(lang, code string) bool {
	lexer := lexers.Get(lang)
	if lexer == nil {
		return false
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		return false
	}
	tokens := it.Tokens()
	if len(tokens) == 0 {
		return false
	}

	r.pdf.Ln(4)
	r.pdf.SetFont(monoFontFamily, "", fontSize-2)
	r.pdf.SetFillColor(248, 249, 250)
	r.pdf.SetDrawColor(233, 236, 239)

	// Group tokens into lines by splitting on newlines.
	// A single chroma token may contain newlines embedded, and tokens
	// are typically one word/operator each (GAS lexer: "mul", "  ", "$s0", ...).
	// We split every token on "\n": the parts before go on the current line,
	// each newline starts the next line, and trailing empty strings after the
	// final newline are ignored (no spurious blank lines).
	var line []chroma.Token
	for _, token := range tokens {
		parts := strings.Split(token.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				// Newline separator: flush the current line
				r.renderCodeLine(line)
				line = nil
			}
			if part != "" || i < len(parts)-1 {
				// Non-empty part → add to current line
				// Empty part in middle (from "\n" at start) → skip
				if part != "" {
					line = append(line, chroma.Token{Type: token.Type, Value: part})
				}
			}
		}
	}
	if len(line) > 0 {
		r.renderCodeLine(line)
	}

	r.pdf.SetTextColor(0, 0, 0)
	r.pdf.SetFont(defFontFamily, "", fontSize)
	r.pdf.Ln(4)
	return true
}

func (r *renderer) renderCodeLine(tokens []chroma.Token) {
	// Check if we're near the page break boundary.
	// If so, add a new page BEFORE drawing the background — otherwise Rect()
	// paints the background on the old page while Write() auto-breaks to the
	// new one, leaving the text without its background.
	y := r.pdf.GetY()
	_, brkMargin := r.pdf.GetAutoPageBreak()
	_, pageH := r.pdf.GetPageSize()
	if y+lineHeight > pageH-brkMargin {
		r.pdf.AddPage()
		y = r.pdf.GetY()
	}

	x := marginLeft + 4

	// 1. Draw background rectangle for the full line width
	r.pdf.SetFillColor(248, 249, 250)
	r.pdf.Rect(x, y, r.width-8, lineHeight, "F")

	// 2. Write each coloured token on the same line using Write()
	r.pdf.SetX(x)
	for _, t := range tokens {
		rr, gg, bb := tokenColor(t.Type)
		r.pdf.SetTextColor(rr, gg, bb)
		r.pdf.Write(lineHeight, sanitizePDFText(t.Value))
	}

	// 3. Advance to the next line
	r.pdf.Ln(lineHeight)
}

func (r *renderer) renderPlainCode(code string) {
	if code == "" {
		return
	}

	r.pdf.Ln(4)
	r.pdf.SetFont(monoFontFamily, "", fontSize-2)
	r.pdf.SetFillColor(248, 249, 250)
	r.pdf.SetDrawColor(233, 236, 239)

	for _, line := range strings.Split(code, "\n") {
		line = sanitizePDFText(strings.TrimSuffix(line, "\r"))
		r.pdf.SetX(marginLeft + 4)
		r.pdf.CellFormat(r.width-8, lineHeight, line, "", 1, "L", true, 0, "")
	}

	r.pdf.SetFont(defFontFamily, "", fontSize)
	r.pdf.Ln(4)
}

// tokenColor maps chroma token types to PDF RGB colours.
func tokenColor(t chroma.TokenType) (int, int, int) {
	// Use category-based matching for broad coverage
	cat := t.Category()
	sub := t.SubCategory()
	switch {
	case cat == chroma.Keyword:
		return 0, 119, 170 // blue
	case cat == chroma.Comment:
		return 153, 153, 136 // gray-green
	case sub == chroma.LiteralString:
		return 196, 102, 0 // orange
	case sub == chroma.LiteralNumber:
		return 153, 0, 153 // magenta
	case cat == chroma.Name:
		// Sub-groups within Name
		switch {
		case t.InCategory(chroma.NameFunction):
			return 0, 128, 0 // green
		case t.InCategory(chroma.NameClass):
			return 0, 153, 153 // teal
		case t.InCategory(chroma.NameBuiltin):
			return 128, 0, 128 // purple
		default:
			return 0, 0, 0 // black
		}
	case cat == chroma.Operator || cat == chroma.Punctuation:
		return 0, 0, 0 // black
	default:
		return 0, 0, 0 // black default
	}
}

// --- inline HTML handling ---

// parseHTMLTag returns the lowercase tag name and whether it is a closing tag
// for a raw HTML fragment such as "<br>", "<br/>", "</sub>", "<sub>".
func parseHTMLTag(raw string) (name string, closing bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	s = strings.TrimSuffix(strings.TrimSpace(s), "/") // handle self-closing />
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "/") {
		closing = true
		s = strings.TrimSpace(s[1:])
	}
	parts := strings.Fields(s) // first word = tag name; rest = attributes
	if len(parts) == 0 {
		return "", false
	}
	return strings.ToLower(parts[0]), closing
}

// handleRawHTML processes inline HTML nodes for the small set of tags the PDF
// renderer understands: <br> and <sub>/<sup> (with matching closing tags).
// All other tags are silently ignored so they do not break the render.
func (r *renderer) handleRawHTML(node *ast.RawHTML) {
	var buf strings.Builder
	for i := 0; i < node.Segments.Len(); i++ {
		seg := node.Segments.At(i)
		buf.Write(seg.Value(r.src))
	}
	tagName, closing := parseHTMLTag(buf.String())

	switch tagName {
	case "br":
		// <br> inside a table cell → newline in the buffered segments
		// (wrapCellSegments splits on \n).  Outside a cell →
		// advance to the next line in the PDF.
		if r.curCell != nil {
			r.curCell.appendText("\n")
		} else {
			r.pdf.Ln(lineHeight)
		}

	case "sub", "sup":
		if !closing {
			// Push the current font size and shrink to 75%.
			ptSize, _ := r.pdf.GetFontSize()
			r.savedFontSizes = append(r.savedFontSizes, ptSize)
			if r.curCell == nil {
				r.pdf.SetFontSize(ptSize * 0.75)
			}
		} else {
			// Pop and restore the saved font size.
			if n := len(r.savedFontSizes); n > 0 {
				prev := r.savedFontSizes[n-1]
				r.savedFontSizes = r.savedFontSizes[:n-1]
				if r.curCell == nil {
					r.pdf.SetFontSize(prev)
				}
			}
		}
	}
}

// --- inline emoji rendering ---

// textSegment is either a run of plain text or an emoji sequence
// (single rune, ZWJ sequence, or modifier sequence).
type textSegment struct {
	text   string // non-empty when this is a plain-text run
	emojis []rune // non-empty when this is an emoji sequence
}

// splitTextSegments splits s into alternating plain-text and emoji segments.
// It correctly groups ZWJ sequences (e.g. 👩+ZWJ+🎓 → one segment),
// variation selectors (U+FE0F/FE0E), and skin-tone modifiers with their
// base emoji.  Orphan ZWJ / variation selectors are silently dropped.
func splitTextSegments(s string) []textSegment {
	runes := []rune(s)
	n := len(runes)
	var segs []textSegment
	var buf strings.Builder
	i := 0
	for i < n {
		r := runes[i]

		// Orphan modifiers that appear outside an emoji context are dropped.
		if r == 0x200D || r == 0xFE0F || r == 0xFE0E {
			i++
			continue
		}

		if !emoji.IsEmojiRune(r) {
			buf.WriteRune(r)
			i++
			continue
		}

		// Start of an emoji sequence — flush any pending text first.
		if buf.Len() > 0 {
			segs = append(segs, textSegment{text: buf.String()})
			buf.Reset()
		}

		// Collect the base emoji plus any modifiers / ZWJ continuations.
		seq := []rune{r}
		i++
		for i < n {
			next := runes[i]
			switch {
			case next == 0xFE0F || next == 0xFE0E:
				// Variation selector — include, advance.
				seq = append(seq, next)
				i++
			case next == 0x200D && i+1 < n && emoji.IsEmojiRune(runes[i+1]):
				// ZWJ followed by another emoji — include ZWJ + next emoji.
				seq = append(seq, next, runes[i+1])
				i += 2
			case next >= 0x1F3FB && next <= 0x1F3FF:
				// Skin-tone modifier — include.
				seq = append(seq, next)
				i++
			case next == 0x20E3:
				// Combining enclosing keycap — include.
				seq = append(seq, next)
				i++
			default:
				goto doneSeq
			}
		}
	doneSeq:
		segs = append(segs, textSegment{emojis: seq})
	}
	if buf.Len() > 0 {
		segs = append(segs, textSegment{text: buf.String()})
	}
	return segs
}

// renderTextWithEmoji writes s to the PDF, replacing each emoji sequence
// (including ZWJ sequences) with an inline rendered PNG image.
func (r *renderer) renderTextWithEmoji(s string) {
	for _, seg := range splitTextSegments(s) {
		if len(seg.emojis) > 0 {
			r.renderEmojiSequence(seg.emojis)
		} else {
			if t := sanitizePDFText(seg.text); t != "" {
				r.pdf.Write(lineHeight, t)
			}
		}
	}
}

// renderEmojiSequence places a rendered PNG for the given emoji sequence
// inline at the current cursor.  Falls back to the first emoji alone if
// the combined ZWJ render is unavailable, and to '?' if even that fails.
func (r *renderer) renderEmojiSequence(emojis []rune) {
	seqName := emoji.EmojiSequenceKey(emojis)
	imgName := "emoji-" + seqName

	if !r.registeredEmoji[seqName] {
		data, err := emoji.RenderEmojiPNGSeq(emojis)
		if err != nil && len(emojis) > 1 {
			// Combined render not available — fall back to the base emoji alone.
			data, err = emoji.RenderEmojiPNGSeq(emojis[:1])
		}
		if err != nil {
			r.pdf.Write(lineHeight, "?")
			return
		}
		r.pdf.RegisterImageOptionsReader(
			imgName,
			fpdf.ImageOptions{ImageType: "PNG"},
			bytes.NewReader(data),
		)
		r.registeredEmoji[seqName] = true
	}

	// Emoji size: fixed at lineHeight-0.5 (4.5 mm).
	// This guarantees the image always fits within the smallest possible line
	// advance (lineHeight = 5 mm for paragraph text) with a 0.5 mm gap at the
	// bottom — no overlap with the content below, in any heading level or body
	// context. Heading advances are proportionally larger (see handleHeading)
	// so the gap there is even more comfortable.
	emojiSize := lineHeight - 0.5 // 4.5 mm

	x := r.pdf.GetX()
	y := r.pdf.GetY()
	r.pdf.ImageOptions(
		imgName, x, y, emojiSize, emojiSize,
		false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "",
	)
	r.pdf.SetX(x + emojiSize + 0.5)
}

// --- thematic break ---

func (r *renderer) handleThematicBreak() {
	r.pdf.Ln(4)
	x := marginLeft
	y := r.pdf.GetY()
	r.pdf.SetDrawColor(189, 195, 199)
	r.pdf.Line(x, y, x+r.width, y)
	r.pdf.Ln(4)
}
