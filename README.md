# md-to-pdf

`md2pdf` is a Go library and command-line tool for rendering Markdown as PDF with embedded Unicode, monospace, and emoji fonts.

## Library

```go
import (
    "bytes"

    md2pdf "github.com/ideras/md-to-pdf"
)

var pdf bytes.Buffer
if err := md2pdf.Convert([]byte("# Report\n"), &pdf); err != nil {
    // handle error
}
```

For file-based conversion:

```go
err := md2pdf.ConvertFile("report.md", "report.pdf")
```

Both entry points accept optional `md2pdf.Option` values for future renderer configuration.

## CLI

```sh
go run ./cmd/md2pdf input.md [output.pdf]
```

If the output path is omitted, the CLI replaces the input extension with `.pdf`.

## Project layout

- `md2pdf.go` — public library API
- `cmd/md2pdf` — command-line application
- `internal/renderer` — Markdown and table rendering
- `internal/emoji` — emoji detection and rasterization
- `internal/fonts` — embedded font assets

## Tests

```sh
go test ./...
```
