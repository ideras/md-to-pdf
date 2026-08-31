package renderer

import "strings"

type inlineStyle struct {
	textColor    [3]int
	textColorSet bool
	background   *[3]int // nil means no background
	fontFamily   string  // logical role name
}

type spanAttrs struct {
	color       string
	background  string
	font        string
	selfClosing bool
}

func cloneColor(color *[3]int) *[3]int {
	if color == nil {
		return nil
	}
	clone := *color
	return &clone
}

func sameInlineStyle(a, b inlineStyle) bool {
	if a.textColor != b.textColor || a.textColorSet != b.textColorSet || a.fontFamily != b.fontFamily || (a.background == nil) != (b.background == nil) {
		return false
	}
	return a.background == nil || *a.background == *b.background
}

// parseSpanTag parses the attributes of a span tag without relying on an HTML
// parser. It intentionally accepts malformed input as far as possible and
// never panics; unsupported attributes are ignored.
func parseSpanTag(raw string) (attrs spanAttrs, closing bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "/") {
		return attrs, true
	}
	s = strings.TrimSuffix(s, ">")
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "/") {
		attrs.selfClosing = true
		s = strings.TrimSpace(strings.TrimSuffix(s, "/"))
	}
	// Consume the tag name.
	i := 0
	for i < len(s) && !isHTMLSpace(s[i]) {
		i++
	}
	if !strings.EqualFold(s[:i], "span") {
		return attrs, false
	}
	for i < len(s) {
		for i < len(s) && isHTMLSpace(s[i]) {
			i++
		}
		start := i
		for i < len(s) && !isHTMLSpace(s[i]) && s[i] != '=' {
			i++
		}
		if start == i {
			i++
			continue
		}
		name := strings.ToLower(s[start:i])
		for i < len(s) && isHTMLSpace(s[i]) {
			i++
		}
		if i == len(s) || s[i] != '=' {
			continue
		}
		i++
		for i < len(s) && isHTMLSpace(s[i]) {
			i++
		}
		if i == len(s) {
			break
		}
		var value string
		if s[i] == '\'' || s[i] == '"' {
			quote := s[i]
			i++
			start = i
			for i < len(s) && s[i] != quote {
				i++
			}
			value = s[start:i]
			if i < len(s) {
				i++
			}
		} else {
			start = i
			for i < len(s) && !isHTMLSpace(s[i]) {
				i++
			}
			value = s[start:i]
		}
		switch name {
		case "color":
			attrs.color = value
		case "background":
			attrs.background = value
		case "font":
			attrs.font = value
		}
	}
	return attrs, false
}

func isHTMLSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' }

func parseColor(s string) (r, g, b int, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		if len(hex) == 3 {
			v0, ok0 := hexNibble(hex[0])
			v1, ok1 := hexNibble(hex[1])
			v2, ok2 := hexNibble(hex[2])
			if !ok0 || !ok1 || !ok2 {
				return 0, 0, 0, false
			}
			return v0 * 17, v1 * 17, v2 * 17, true
		}
		if len(hex) == 6 {
			vals := make([]int, 6)
			for i := range hex {
				var valid bool
				vals[i], valid = hexNibble(hex[i])
				if !valid {
					return 0, 0, 0, false
				}
			}
			return vals[0]*16 + vals[1], vals[2]*16 + vals[3], vals[4]*16 + vals[5], true
		}
		return 0, 0, 0, false
	}
	if color, found := namedColors[s]; found {
		return color[0], color[1], color[2], true
	}
	return 0, 0, 0, false
}

func hexNibble(b byte) (int, bool) {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0'), true
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10, true
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10, true
	default:
		return 0, false
	}
}

func (r *renderer) resolveFontFamily(name string) (family string, ok bool) {
	if name == "" {
		name = r.currentStyle.fontFamily
	}
	if name == "" {
		name = "default"
	}
	family, ok = r.fontFamilies[strings.ToLower(strings.TrimSpace(name))]
	if !ok && strings.EqualFold(name, "default") {
		return defFontFamily, true
	}
	if !ok && strings.EqualFold(name, "mono") {
		return monoFontFamily, true
	}
	return family, ok
}

func (r *renderer) applyInlineStyle() {
	family, ok := r.resolveFontFamily(r.currentStyle.fontFamily)
	if !ok {
		family = defFontFamily
	}
	style := r.pdf.GetFontStyle()
	size, _ := r.pdf.GetFontSize()
	r.pdf.SetFont(family, style, size)
	r.pdf.SetTextColor(r.currentStyle.textColor[0], r.currentStyle.textColor[1], r.currentStyle.textColor[2])
}

func (r *renderer) openSpan(attrs spanAttrs) {
	if attrs.selfClosing {
		return
	}
	r.savedSpanStyles = append(r.savedSpanStyles, inlineStyle{
		textColor: r.currentStyle.textColor, textColorSet: r.currentStyle.textColorSet, background: cloneColor(r.currentStyle.background), fontFamily: r.currentStyle.fontFamily,
	})
	if red, green, blue, ok := parseColor(attrs.color); ok {
		r.currentStyle.textColor = [3]int{red, green, blue}
		r.currentStyle.textColorSet = true
	}
	if red, green, blue, ok := parseColor(attrs.background); ok {
		r.currentStyle.background = &[3]int{red, green, blue}
	}
	if attrs.font != "" {
		if _, ok := r.resolveFontFamily(attrs.font); ok {
			r.currentStyle.fontFamily = strings.ToLower(strings.TrimSpace(attrs.font))
		} else {
			r.currentStyle.fontFamily = "default"
		}
	}
	if r.curCell == nil {
		r.applyInlineStyle()
	}
}

func (r *renderer) closeSpan() {
	if n := len(r.savedSpanStyles); n > 0 {
		r.currentStyle = r.savedSpanStyles[n-1]
		r.savedSpanStyles = r.savedSpanStyles[:n-1]
		if r.curCell == nil {
			r.applyInlineStyle()
		}
	}
}

var namedColors = map[string][3]int{
	"black": {0, 0, 0}, "silver": {192, 192, 192}, "gray": {128, 128, 128}, "grey": {128, 128, 128},
	"white": {255, 255, 255}, "maroon": {128, 0, 0}, "red": {255, 0, 0}, "purple": {128, 0, 128},
	"fuchsia": {255, 0, 255}, "magenta": {255, 0, 255}, "green": {0, 128, 0}, "lime": {0, 255, 0},
	"olive": {128, 128, 0}, "yellow": {255, 255, 0}, "navy": {0, 0, 128}, "blue": {0, 0, 255},
	"teal": {0, 128, 128}, "aqua": {0, 255, 255}, "cyan": {0, 255, 255}, "orange": {255, 165, 0},
}
