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

### Custom fonts and inline spans

Load custom TrueType fonts explicitly and supply the resulting registry to a conversion:

```go
fonts, err := md2pdf.LoadFontRegistry("fonts.toml")
if err != nil { /* handle invalid configuration */ }
err = md2pdf.ConvertFile("report.md", "report.pdf", md2pdf.WithFontRegistry(fonts))
```

```toml
[[fonts]]
name = "serif"
regular = "/path/Merriweather-Regular.ttf"
bold = "/path/Merriweather-Bold.ttf"
italic = "/path/Merriweather-Italic.ttf"
bold_italic = "/path/Merriweather-BoldItalic.ttf"
```

`regular` is required; missing variants fall back to it. The built-in `default`
and `mono` roles cannot be replaced. Markdown may use direct span attributes:
`<span color="#336699" background="yellow" font="serif">text</span>`.
Supported colors are `#rgb`, `#rrggbb`, and common named CSS colors. Flow-text
backgrounds are best-effort for a single line only; backgrounds that would wrap
are skipped rather than painted inaccurately.

Both entry points accept optional `md2pdf.Option` values.

### Document header

Add a title and PNG logo above the rendered Markdown. The logo is provided as
bytes so applications can embed it with `go:embed`:

```go
err := md2pdf.Convert(markdown, output,
    md2pdf.WithHeader(md2pdf.Header{
        Title:   "Lotificacion Deras",
        LogoPNG: logoPNG,
    }),
)
```

### Dense tables

Use a smaller table font and omit cell borders for compact reports. Header and
alternating-row backgrounds are retained for readability:

```go
err := md2pdf.Convert(markdown, output,
    md2pdf.WithTableStyle(md2pdf.TableStyle{
        FontSize:   8, // supported range: 5–11 pt
        Borderless: true,
    }),
)
```

## CLI

```sh
go run ./cmd/md2pdf [--font-config fonts.toml] input.md [output.pdf]
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
