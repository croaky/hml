# Constrain attribute contexts

hml escapes attribute values with `html.EscapeString`, regardless of the
attribute. That prevents quote breakout. It does not stop a value like
`javascript:alert(1)` in `href`, and it does not know that `onclick`,
`onerror`, and `style` are not HTML text contexts.

The package documentation says this plainly today. The implementation
can do better than the documentation promises, because hml is not
parsing a byte stream. It already has the attribute name in hand when it
renders the value.

## Proposed changes

Add an attribute policy in `renderTag`, before the final HTML escape.

For URL-valued attributes -- `href`, `src`, `action`, `formaction`,
`poster`, `cite`, `background`, `ping`, and `xlink:href` -- reject
unsafe schemes. Relative URLs, root-relative URLs, query strings, and
fragments pass. Absolute URLs pass only for a small allowlist:
`http`, `https`, `mailto`, and `tel`. A rejected value renders as
`#ZgotmplZ`, matching `html/template`, and the tests document that name
as a sentinel, not a link target.

For JavaScript attributes -- any attribute whose lower-cased name begins
with `on` -- allow literal source strings, because those are application
code written in the template. Reject dynamic values unless they carry an
explicit trust type such as:

```go
type SafeJS string
```

For `style`, allow literal source strings and reject dynamic values
unless they carry `SafeCSS`.

For `:javascript` and `:css` filter blocks, allow literal blocks and
reject interpolation unless the interpolated value is `SafeJS` or
`SafeCSS`. If the parser cannot distinguish that cleanly without making
the renderer awkward, leave filter blocks for a follow-up and document
the omission in the commit.

## Compatibility

cibot should keep working: its `style` attributes are literal
`white-space: pre-wrap`, and it has no `on*` attributes.

EDS needs a compatibility pass before a tag can ship. It has many
literal `onclick` handlers, which should pass as authored source, and a
smaller set of dynamic `onerror` and `style` attributes. Those dynamic
values should be prebuilt in Go as `SafeJS` or `SafeCSS`, or moved to
data attributes plus ordinary JavaScript where that reads better.

## Tests

Cover at least these cases:

- `href: "javascript:alert(1)"` renders the unsafe sentinel.
- `href: "/change/CI-1"`, `href: "#x"`, and `href: "https://example.com"`
  pass.
- A dynamic `onclick` value errors unless it is `SafeJS`.
- A literal `onclick: "APP.close()"` passes.
- A dynamic `style` value errors unless it is `SafeCSS`.

Update `doc.go` so the security model names these attribute-specific
rules instead of saying hml has no context-aware escaping.
