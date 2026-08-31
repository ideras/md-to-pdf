package emoji

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── IsEmojiRune ──────────────────────────────────────────────────────────────

func TestIsEmojiRune_EmoticonRange(t *testing.T) {
	assert.True(t, IsEmojiRune('😀'), "U+1F600 should be emoji")
	assert.True(t, IsEmojiRune('📘'), "U+1F4D8 should be emoji")
	assert.True(t, IsEmojiRune('🧪'), "U+1F9EA should be emoji")
}

func TestIsEmojiRune_MiscSymbols(t *testing.T) {
	assert.True(t, IsEmojiRune('☀'), "U+2600 should be emoji")
	assert.True(t, IsEmojiRune('✈'), "U+2708 (Dingbats) should be emoji")
}

func TestIsEmojiRune_RegularASCII(t *testing.T) {
	assert.False(t, IsEmojiRune('A'), "ASCII letter is not emoji")
	assert.False(t, IsEmojiRune('5'), "ASCII digit is not emoji")
	assert.False(t, IsEmojiRune(' '), "space is not emoji")
}

func TestIsEmojiRune_LatinExtended(t *testing.T) {
	assert.False(t, IsEmojiRune('é'), "accented letter is not emoji")
	assert.False(t, IsEmojiRune('ñ'), "tilde-n is not emoji")
}

// ── EmojiCodepoint ───────────────────────────────────────────────────────────

func TestEmojiCodepoint_KnownEmoji(t *testing.T) {
	assert.Equal(t, "1f4d8", EmojiCodepoint('📘'), "📘 codepoint")
	assert.Equal(t, "1f600", EmojiCodepoint('😀'), "😀 codepoint")
	assert.Equal(t, "2600", EmojiCodepoint('☀'), "☀ codepoint")
}

func TestEmojiCodepoint_IsLowerHex(t *testing.T) {
	// Must be lowercase for stable sequence/codepoint normalization.
	cp := EmojiCodepoint('🐍') // U+1F40D
	assert.Equal(t, "1f40d", cp)
}
