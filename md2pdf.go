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
	fonts      *FontRegistry
	tableStyle TableStyle
	header     Header
}

// TableStyle controls how Markdown tables are rendered. A zero value preserves
// the default table appearance.
type TableStyle struct {
	// FontSize sets the table body font size in points. Values outside the
	// supported range (5 through 11) use the default size.
	FontSize float64
	// Borderless omits cell outlines while retaining header and alternating-row
	// backgrounds for readable dense reports.
	Borderless bool
	// ColumnWeights optionally biases horizontal width distribution across the
	// table's columns. Columns with a positive weight share the page width left
	// after unweighted columns keep their natural content width, proportionally
	// to their weights (never below their minimum readable width). Columns with
	// a zero weight keep their natural width. Only relative ratios matter, and
	// fewer entries than columns behave as if the missing entries were zero. A
	// nil slice preserves the default content-driven layout.
	ColumnWeights []float64
}

// WithTableStyle applies table-specific rendering settings to a conversion.
func WithTableStyle(style TableStyle) Option {
	return func(cfg *options) { cfg.tableStyle = style }
}

// Header adds a title and optional PNG logo above the Markdown document.
type Header struct {
	Title   string
	LogoPNG []byte
}

// WithHeader adds a document header to a conversion.
func WithHeader(header Header) Option {
	return func(cfg *options) { cfg.header = header }
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

	return renderer.Render(src, w, renderer.Options{
		Fonts: cfg.fonts.rendererRoles(),
		TableStyle: renderer.TableStyle{
			FontSize:      cfg.tableStyle.FontSize,
			Borderless:    cfg.tableStyle.Borderless,
			ColumnWeights: cfg.tableStyle.ColumnWeights,
		},
		Header: renderer.Header{
			Title:   cfg.header.Title,
			LogoPNG: cfg.header.LogoPNG,
		},
	})
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
