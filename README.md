# hml

A small, structure-aware template language for Go. Indentation maps to
HTML nesting, so the parser builds a tree and the renderer emits matched
tags. Malformed HTML is not expressible.

`.hml` files are source and runtime input. There is no transpiler, generated
code, or build step. The engine evaluates but never computes; formatting happens
in Go and arrives pre-formatted.

```go
tmpl, err := hml.Parse(src, "show.hml", transforms)
out, err := tmpl.Render(locals, partialFn)
```

See `doc.go` for the grammar, security model, and value semantics.

## Checking locals

A parsed template reports what it reads, so an app can check its locals
once at startup rather than one page at a time in production:

```go
tmpl.Names()   // free top-level identifiers the template reads
tmpl.Renders() // partials it renders by literal name
```

`Names` answers for one file. A partial inherits its caller's locals, so
follow `Renders` to check a whole page.

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

## GitHub repo is a mirror

Development happens on [cibot](https://dancroak.com/cmd/cibot/), a
self-hosted review and CI server, which holds in progress branches.
GitHub receives `main` and the tags so `go get` works.

## License

MIT
