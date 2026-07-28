# hml

A small, structure-aware template language for Go. Indentation maps to
HTML nesting, so the parser builds a tree and the renderer emits matched
tags. Malformed HTML is not expressible.

`.hml` files are source and runtime input: no transpiler, no generated
code, no build step. The engine evaluates but never computes; formatting
happens in Go and arrives pre-formatted.

```go
tmpl, err := hml.Parse(src, "show.hml", transforms)
out, err := tmpl.Render(locals, partialFn)
```

See `doc.go` for the grammar, security model, and value semantics.

## Transforms

The engine ships zero built-ins. Rich text renders through app-registered
transforms, invoked as `= name(field)`, each of which must sanitize its
own output:

```go
transforms := map[string]hml.Transform{
	"markdown": func(s string) string {
		var buf bytes.Buffer
		if err := goldmark.Convert([]byte(s), &buf); err != nil {
			return ""
		}
		return mdPolicy.Sanitize(buf.String())
	},
}
```

Unregistered names are parse errors, so the parser stays the linter.
The engine itself is stdlib-only.

## Why hml, not templ

templ is a transpiler: `.templ` compiles to `.go`, so field access is Go
and the compiler checks it. hml has no build step, and the parser is the
linter.

templ is faster — no runtime parse, tree walk, or reflection. Behind a
database query the gap is microseconds against milliseconds, so it is
not a reason to switch.

templ's templates are Go. There is no independent language to give a
grammar or a spec to, and nothing to share as one syntax across apps.
hml is exactly that: one small, language-agnostic syntax.
