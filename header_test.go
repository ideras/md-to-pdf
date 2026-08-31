package md2pdf_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"

	md2pdf "github.com/ideras/md-to-pdf"
)

func TestConvert_HeaderWithPNGLogo(t *testing.T) {
	logo := image.NewRGBA(image.Rect(0, 0, 2, 2))
	logo.Set(0, 0, color.RGBA{R: 20, G: 75, B: 58, A: 255})
	var logoPNG bytes.Buffer
	require.NoError(t, png.Encode(&logoPNG, logo))

	var output bytes.Buffer
	err := md2pdf.Convert([]byte("# Reporte\n"), &output,
		md2pdf.WithHeader(md2pdf.Header{Title: "Lotificacion Deras", LogoPNG: logoPNG.Bytes()}),
	)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(output.Bytes(), []byte("%PDF-")))
}
