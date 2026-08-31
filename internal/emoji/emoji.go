// Package emoji detects and rasterizes emoji sequences for PDF rendering.
package emoji

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"strconv"
	"strings"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"github.com/ideras/md-to-pdf/internal/fonts"
)

var emojiFace *font.Face

func init() {
	face, err := font.ParseTTF(bytes.NewReader(fonts.Emoji))
	if err != nil {
		panic(fmt.Sprintf("parse emoji font: %v", err))
	}

	// Match the largest bitmap strike to maximize extraction quality.
	sizes := face.Font.BitmapSizes()
	if len(sizes) > 0 {
		best := sizes[0]
		for _, s := range sizes[1:] {
			if s.YPpem > best.YPpem {
				best = s
			}
		}
		face.SetPpem(best.XPpem, best.YPpem)
	}
	emojiFace = face
}

// EmojiCodepoint converts a single emoji rune to its lowercase hex string
// (e.g. "1f4d8" for '📘').
func EmojiCodepoint(r rune) string {
	return strconv.FormatInt(int64(r), 16)
}

// EmojiSequenceKey returns the normalized hex stem for an emoji sequence.
// Variation selectors (U+FE0F, U+FE0E) are intentionally omitted from the
// name because they are presentation selectors, not semantic codepoints.
// ZWJ (U+200D) and skin-tone modifiers ARE included.
//
//	e.g. []rune{0x1F469, 0x200D, 0x1F393}  →  "1f469-200d-1f393"
//	     []rune{0x2600, 0xFE0F}             →  "2600"
func EmojiSequenceKey(runes []rune) string {
	parts := make([]string, 0, len(runes))
	for _, r := range runes {
		if r == 0xFE0F || r == 0xFE0E { // skip variation selectors
			continue
		}
		parts = append(parts, EmojiCodepoint(r))
	}
	return strings.Join(parts, "-")
}

// RenderEmojiPNG returns PNG bytes for a single emoji rune.
func RenderEmojiPNG(r rune) ([]byte, error) {
	return RenderEmojiPNGSeq([]rune{r})
}

// RenderEmojiPNGSeq renders an emoji sequence using HarfBuzz shaping and
// extracts bitmap glyphs from Noto Color Emoji.
func RenderEmojiPNGSeq(runes []rune) ([]byte, error) {
	if len(runes) == 0 {
		return nil, fmt.Errorf("empty emoji sequence")
	}

	var shaper shaping.HarfbuzzShaper
	input := shaping.Input{
		Text:     runes,
		RunStart: 0,
		RunEnd:   len(runes),
		Face:     emojiFace,
		Size:     fixed.I(128),
		Script:   language.Common,
	}
	output := shaper.Shape(input)
	if len(output.Glyphs) == 0 {
		return nil, fmt.Errorf("shaping produced no glyphs for %v", runes)
	}

	var images []image.Image
	for _, g := range output.Glyphs {
		data := emojiFace.GlyphData(g.GlyphID)
		bmp, ok := data.(font.GlyphBitmap)
		if !ok || bmp.Format != font.PNG {
			continue
		}
		img, err := png.Decode(bytes.NewReader(bmp.Data))
		if err != nil {
			continue
		}
		images = append(images, img)
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no bitmap glyphs for %v", runes)
	}
	if len(images) == 1 {
		var buf bytes.Buffer
		if err := png.Encode(&buf, images[0]); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	return tilePNGs(images)
}

// IsEmojiRune reports whether r falls within the standard Unicode emoji ranges.
func IsEmojiRune(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1F9FF) || // Misc Symbols & Pictographs, Emoticons, Transport …
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons block
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport & Map Symbols
		(r >= 0x2600 && r <= 0x26FF) || // Miscellaneous Symbols
		(r >= 0x2700 && r <= 0x27BF) || // Dingbats
		(r >= 0x1F1E6 && r <= 0x1F1FF) || // Regional Indicator (flags)
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental Symbols & Pictographs
		(r >= 0x1FA70 && r <= 0x1FAFF) // Symbols & Pictographs Extended-A
}

func tilePNGs(images []image.Image) ([]byte, error) {
	totalW := 0
	maxH := 0
	for _, img := range images {
		b := img.Bounds()
		totalW += b.Dx()
		if b.Dy() > maxH {
			maxH = b.Dy()
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, totalW, maxH))
	x := 0
	for _, img := range images {
		b := img.Bounds()
		draw.Draw(dst, image.Rect(x, 0, x+b.Dx(), b.Dy()), img, b.Min, draw.Over)
		x += b.Dx()
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
