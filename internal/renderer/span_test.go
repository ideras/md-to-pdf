package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseColor(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  [3]int
		ok    bool
	}{
		{"#AbC", [3]int{170, 187, 204}, true},
		{"#336699", [3]int{51, 102, 153}, true},
		{"orange", [3]int{255, 165, 0}, true},
		{"GREY", [3]int{128, 128, 128}, true},
		{"#12", [3]int{}, false},
		{"#ggg", [3]int{}, false},
	} {
		r, g, b, ok := parseColor(tc.input)
		require.Equal(t, tc.ok, ok, tc.input)
		if ok {
			require.Equal(t, tc.want, [3]int{r, g, b}, tc.input)
		}
	}
}

func TestParseSpanTag(t *testing.T) {
	attrs, closing := parseSpanTag(` <span FONT='serif' unknown=x background="#fff" color=red> `)
	require.False(t, closing)
	require.Equal(t, "serif", attrs.font)
	require.Equal(t, "#fff", attrs.background)
	require.Equal(t, "red", attrs.color)

	_, closing = parseSpanTag(`</span>`)
	require.True(t, closing)
	selfClosing, closing := parseSpanTag(`<span color="red"/>`)
	require.False(t, closing)
	require.True(t, selfClosing.selfClosing)

	require.NotPanics(t, func() { _, _ = parseSpanTag(`<span color="unterminated>`) })
}

func TestTableStyleSurvivesWrappingAndDoesNotCoalesce(t *testing.T) {
	r := newTableTestRenderer(t)
	red := inlineStyle{textColor: [3]int{255, 0, 0}, textColorSet: true, fontFamily: "default"}
	blue := inlineStyle{textColor: [3]int{0, 0, 255}, textColorSet: true, fontFamily: "default"}
	cell := &tableCell{}
	cell.appendStyledText("red words ", red)
	cell.appendStyledText("blue words", blue)
	lines := r.wrapCellSegments(cell, 20)
	require.Greater(t, len(lines), 1)
	var colors [][3]int
	for _, line := range lines {
		for _, segment := range line {
			if segment.color != nil {
				colors = append(colors, *segment.color)
			}
		}
	}
	require.Contains(t, colors, [3]int{255, 0, 0})
	require.Contains(t, colors, [3]int{0, 0, 255})
}
