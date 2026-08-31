# Custom Font Registry Demo

Render this file with `examples/fonts.local.toml` to register the logical `serif` font role:

```sh
go run ./cmd/md2pdf --font-config examples/fonts.local.toml examples/custom-fonts.md /tmp/custom-fonts.pdf
```

## Font selection

Normal built-in text uses the `default` role.

<span font="serif">This run uses the configured `serif` role.</span>

<span font="serif">**Bold custom text**, *italic custom text*, and ***bold-italic custom text*** use configured variants (or fall back to regular when a variant is omitted).</span>

<span font="serif" color="#1565c0" background="#e3f2fd">Custom font, color, and background can be combined.</span>

## Code keeps its role

<span font="serif" color="teal">The surrounding text uses `serif`, while `fmt.Println("hello")` stays in the built-in monospace role.</span>

## Custom font inside a table

| Description | Rendered value |
| --- | --- |
| Custom body font | <span font="serif">A custom-font table segment</span> |
| Bold header-compatible segment | <span font="serif">**Bold custom value**</span> |
| Styled and colored | <span font="serif" color="maroon" background="#fdecea">Important custom text</span> |
| Emoji | <span font="serif">Font selection with emoji 😀 👍</span> |

## Fallback behavior

<span font="missing-role">This uses the default font because `missing-role` is not registered.</span>
