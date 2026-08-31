# Span Styling Demo

This paragraph has <span color="red">red text</span>, <span color="#336699">blue text</span>, and <span background="yellow">a highlighted run</span>.

## Nested spans

<span color="navy">Outer navy text with <span color="#f60" background="#fff0cc">orange highlighted text</span>, then navy again.</span>

The renderer also accepts case-insensitive attributes and names: <span COLOR="MAGENTA" BACKGROUND="#eee">styled text</span>.

## Markdown interaction

<span color="green">**Bold**, *italic*, and `inline code` inherit span color. Inline code remains monospace.</span>

<span font="does-not-exist" color="purple">An unknown font safely falls back to the default family.</span>

## Emoji

<span color="#00695c" background="#e0f2f1">Emoji still render in styled text: 😀 👍 👩‍🎓 ✅</span>

## Tables

| Feature | Example |
| --- | --- |
| Text color | <span color="blue">Blue table text</span> |
| Background | <span background="#ffe082">Highlighted table text</span> |
| Combined | <span color="white" background="#455a64">White text on slate</span> |
| Code and color | <span color="red">Use `go test ./...`</span> |
| Emoji | <span color="green">Ready 👍</span> |

## Background wrapping limitation

A span background is intentionally drawn only when its run fits on the current line. This very long highlighted run may wrap; if it does, its background is skipped instead of producing an inaccurate multi-line rectangle: <span background="#fff59d">Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</span>
