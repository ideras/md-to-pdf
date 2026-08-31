package renderer

import (
	"bytes"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/stretchr/testify/require"
)

func TestRenderCodeLine_PaintsBackgroundForEveryWrappedLine(t *testing.T) {
	r := newTableTestRenderer(t)
	r.pdf.SetCompression(false)
	r.pdf.SetFont(monoFontFamily, "", fontSize-2)

	tokens := []chroma.Token{{
		Type:  chroma.Text,
		Value: "go run ./cmd/md2pdf --font-config examples/fonts.local.toml examples/custom-fonts.md /tmp/custom-fonts.pdf",
	}}
	wrapped := r.wrapCodeTokens(tokens, r.width-8)
	require.Len(t, wrapped, 2, "the reported command should wrap onto two visual lines")
	for _, line := range wrapped {
		var text string
		for _, token := range line {
			text += token.Value
		}
		require.LessOrEqual(t, r.pdf.GetStringWidth(text), r.width-8)
	}

	yBefore := r.pdf.GetY()
	r.renderCodeLine(tokens)
	require.InDelta(t, 2*lineHeight, r.pdf.GetY()-yBefore, 0.01)

	var output bytes.Buffer
	require.NoError(t, r.pdf.Output(&output))
	require.Equal(t, 2, bytes.Count(output.Bytes(), []byte(" re f\n")),
		"each wrapped visual line must have a filled background rectangle")
}

func TestWrapCodeTokens_PreservesHighlightingAcrossWrap(t *testing.T) {
	r := newTableTestRenderer(t)
	r.pdf.SetFont(monoFontFamily, "", fontSize-2)

	tokens := []chroma.Token{
		{Type: chroma.Keyword, Value: "command "},
		{Type: chroma.LiteralString, Value: "a-very-long-argument"},
	}
	lines := r.wrapCodeTokens(tokens, r.pdf.GetStringWidth("command a-very"))
	require.Greater(t, len(lines), 1)

	var sawKeyword, sawString bool
	for _, line := range lines {
		for _, token := range line {
			sawKeyword = sawKeyword || token.Type == chroma.Keyword
			sawString = sawString || token.Type == chroma.LiteralString
		}
	}
	require.True(t, sawKeyword)
	require.True(t, sawString)
}
