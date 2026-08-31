// Package fonts provides the renderer's embedded font assets.
package fonts

import _ "embed"

//go:embed assets/DejaVuSans.ttf
var Regular []byte

//go:embed assets/DejaVuSans-Bold.ttf
var Bold []byte

//go:embed assets/DejaVuSans-Oblique.ttf
var Italic []byte

//go:embed assets/DejaVuSans-BoldOblique.ttf
var BoldItalic []byte

//go:embed assets/JetBrainsMono-Regular.ttf
var Mono []byte

//go:embed assets/JetBrainsMono-Bold.ttf
var MonoBold []byte

//go:embed assets/NotoColorEmoji.ttf
var Emoji []byte
