// Package md2pdf renders Markdown documents as PDF files.
package md2pdf

import (
	"fmt"
	"io"
	"os"

	"github.com/ideras/md-to-pdf/internal/renderer"
)

// Option configures a conversion. Options are applied in the order provided.
type Option func(*options)

type options struct {
	fonts *FontRegistry
}

// Convert renders Markdown source to PDF and writes it to w.
func Convert(src []byte, w io.Writer, opts ...Option) error {
	if w == nil {
		return fmt.Errorf("PDF output writer is nil")
	}

	cfg := options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return renderer.Render(src, w, renderer.Options{Fonts: cfg.fonts.rendererRoles()})
}

// ConvertFile reads a Markdown file and writes a PDF.
func ConvertFile(inputPath, outputPath string, opts ...Option) (err error) {
	src, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read markdown file: %w", err)
	}

	output, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create PDF file: %w", err)
	}
	defer func() {
		if closeErr := output.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close PDF file: %w", closeErr)
		}
	}()

	return Convert(src, output, opts...)
}
