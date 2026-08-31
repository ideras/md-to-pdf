package md2pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ideras/md-to-pdf/internal/fonts"
)

func TestNewFontRegistry_RejectsReservedAndDuplicateRoles(t *testing.T) {
	_, err := NewFontRegistry(
		FontRole{Name: "DEFAULT", Regular: fonts.Regular},
		FontRole{Name: "serif", Regular: fonts.Regular},
		FontRole{Name: " Serif ", Regular: fonts.Regular},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved")
	require.Contains(t, err.Error(), "duplicate")
}

func TestLoadFontRegistry_AggregatesFileErrors(t *testing.T) {
	config := filepath.Join(t.TempDir(), "fonts.toml")
	require.NoError(t, os.WriteFile(config, []byte(`
[[fonts]]
name = "serif"
regular = "/missing/regular.ttf"
bold = "/missing/bold.ttf"
`), 0o600))

	_, err := LoadFontRegistry(config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "regular")
	require.Contains(t, err.Error(), "bold")
	require.Contains(t, err.Error(), "/missing/regular.ttf")
	require.Contains(t, err.Error(), "/missing/bold.ttf")
}

func TestFontRegistry_CustomRoleAndSpansRender(t *testing.T) {
	regular := bytes.Clone(fonts.Regular)
	registry, err := NewFontRegistry(FontRole{Name: "Serif", Regular: regular})
	require.NoError(t, err)
	// The registry owns a copy: later caller mutation cannot affect conversion.
	regular[0] ^= 0xff

	markdown := []byte("<span color=\"red\" background=\"#def\" font=\"serif\">**custom** 😀</span> normal\n\n| H |\n|---|\n| <span color=\"blue\" font=\"serif\">value</span> `code` 😀 |\n")
	var output bytes.Buffer
	require.NoError(t, Convert(markdown, &output, WithFontRegistry(registry)))
	require.True(t, bytes.HasPrefix(output.Bytes(), []byte("%PDF-")))
}

func TestConvert_SpanUnknownFontAndNilRegistryAreSafe(t *testing.T) {
	for _, option := range []Option{nil, WithFontRegistry(nil)} {
		var output bytes.Buffer
		err := Convert([]byte(`<span font="does-not-exist" color="#f00">text</span>`), &output, option)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(output.String(), "%PDF-"))
	}
}
