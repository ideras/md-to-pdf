package md2pdf_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	md2pdf "github.com/ideras/md-to-pdf"
)

func TestConvert_TableStyle(t *testing.T) {
	var output bytes.Buffer
	err := md2pdf.Convert([]byte("| Fecha | Valor |\n| --- | ---: |\n| 2026-08-08 | L 18,000.00 |\n"), &output,
		md2pdf.WithTableStyle(md2pdf.TableStyle{FontSize: 8, Borderless: true}),
	)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(output.Bytes(), []byte("%PDF-")))
}
