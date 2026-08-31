package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── parseHTMLTag ────────────────────────────────────────────────────────────

func TestParseHTMLTag_SelfClosingBr(t *testing.T) {
	name, closing := parseHTMLTag("<br/>")
	assert.Equal(t, "br", name)
	assert.False(t, closing)
}

func TestParseHTMLTag_OpenBr(t *testing.T) {
	name, closing := parseHTMLTag("<br>")
	assert.Equal(t, "br", name)
	assert.False(t, closing)
}

func TestParseHTMLTag_OpenSub(t *testing.T) {
	name, closing := parseHTMLTag("<sub>")
	assert.Equal(t, "sub", name)
	assert.False(t, closing)
}

func TestParseHTMLTag_CloseSub(t *testing.T) {
	name, closing := parseHTMLTag("</sub>")
	assert.Equal(t, "sub", name)
	assert.True(t, closing)
}

func TestParseHTMLTag_OpenSup(t *testing.T) {
	name, closing := parseHTMLTag("<sup>")
	assert.Equal(t, "sup", name)
	assert.False(t, closing)
}

func TestParseHTMLTag_CaseInsensitive(t *testing.T) {
	name, _ := parseHTMLTag("<BR>")
	assert.Equal(t, "br", name)
}

func TestParseHTMLTag_UnknownTag(t *testing.T) {
	name, closing := parseHTMLTag("<span class=\"x\">")
	assert.Equal(t, "span", name)
	assert.False(t, closing)
}

func TestParseHTMLTag_Empty(t *testing.T) {
	name, closing := parseHTMLTag("")
	assert.Equal(t, "", name)
	assert.False(t, closing)
}

func TestSplitTextSegments_PureText(t *testing.T) {
	segs := splitTextSegments("hello world")
	if assert.Len(t, segs, 1) {
		assert.Equal(t, "hello world", segs[0].text)
		assert.Nil(t, segs[0].emojis)
	}
}

func TestSplitTextSegments_PureEmoji(t *testing.T) {
	segs := splitTextSegments("📘")
	if assert.Len(t, segs, 1) {
		assert.Equal(t, []rune{'📘'}, segs[0].emojis)
		assert.Empty(t, segs[0].text)
	}
}

func TestSplitTextSegments_TextThenEmoji(t *testing.T) {
	segs := splitTextSegments("Nota: 📘")
	if assert.Len(t, segs, 2) {
		assert.Equal(t, "Nota: ", segs[0].text)
		assert.Equal(t, []rune{'📘'}, segs[1].emojis)
	}
}

func TestSplitTextSegments_EmojiThenText(t *testing.T) {
	segs := splitTextSegments("📘 intro")
	if assert.Len(t, segs, 2) {
		assert.Equal(t, []rune{'📘'}, segs[0].emojis)
		assert.Equal(t, " intro", segs[1].text)
	}
}

func TestSplitTextSegments_MixedMultiple(t *testing.T) {
	segs := splitTextSegments("A 😀 B 🧪 C")
	// Expected: ["A ", 😀, " B ", 🧪, " C"]
	if assert.Len(t, segs, 5) {
		assert.Equal(t, "A ", segs[0].text)
		assert.Equal(t, []rune{'😀'}, segs[1].emojis)
		assert.Equal(t, " B ", segs[2].text)
		assert.Equal(t, []rune{'🧪'}, segs[3].emojis)
		assert.Equal(t, " C", segs[4].text)
	}
}

func TestSplitTextSegments_EmptyString(t *testing.T) {
	segs := splitTextSegments("")
	assert.Empty(t, segs)
}

func TestSplitTextSegments_ConsecutiveEmoji(t *testing.T) {
	// Two plain emoji with no ZWJ — two separate segments.
	segs := splitTextSegments("📘🧪")
	if assert.Len(t, segs, 2) {
		assert.Equal(t, []rune{'📘'}, segs[0].emojis)
		assert.Equal(t, []rune{'🧪'}, segs[1].emojis)
	}
}

func TestSplitTextSegments_ZWJSequence(t *testing.T) {
	// 👩‍🎓 = U+1F469 + ZWJ + U+1F393 — must be a SINGLE segment.
	s := "👩‍🎓 Allison Lucero"
	segs := splitTextSegments(s)
	if assert.Len(t, segs, 2) {
		assert.Equal(t, []rune{0x1F469, 0x200D, 0x1F393}, segs[0].emojis,
			"ZWJ sequence must be a single segment, not split")
		assert.Equal(t, " Allison Lucero", segs[1].text)
	}
}

func TestSplitTextSegments_VariationSelector(t *testing.T) {
	// ☀️ = U+2600 + U+FE0F — variation selector included in the sequence.
	segs := splitTextSegments("☀️ sunny")
	if assert.Len(t, segs, 2) {
		assert.Equal(t, []rune{0x2600, 0xFE0F}, segs[0].emojis)
		assert.Equal(t, " sunny", segs[1].text)
	}
}

func TestSplitTextSegments_OrphanZWJ(t *testing.T) {
	// An orphan ZWJ between plain-text characters must be silently dropped.
	segs := splitTextSegments("hello\u200Dworld")
	if assert.Len(t, segs, 1) {
		assert.Equal(t, "helloworld", segs[0].text)
	}
}
