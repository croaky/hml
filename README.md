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

## One buffer per page

`Render` returns a string. A page that renders a partial per row pays a
buffer and a copy per row on that path. `RenderContextTo` writes into a
buffer the caller owns, and a `PartialWriter` that calls it on the same
buffer renders the whole page into one:

```go
var partial hml.PartialWriter
partial = func(name string, ctx *hml.Context, w *strings.Builder) error {
	return load(name).RenderContextTo(w, ctx, partial)
}
var w strings.Builder
err := page.RenderContextTo(&w, hml.NewContext(locals), partial)
```

`RenderContext` and `PartialFunc` stay, and render the same bytes.

## Checking locals

A parsed template reports what it reads, so an app can check its locals
once at startup rather than one page at a time in production:

```go
tmpl.Names()   // free top-level identifiers the template reads
tmpl.Renders() // partials it renders by literal name
```

`Names` answers for one file. A partial inherits its caller's locals, so
follow `Renders` to check a whole page.

## Render coverage

`Parse` checks a condition's syntax and nothing about its type. A
non-bool in an `- if` is a render error, so a view that no test renders
is a view that nothing type-checks. `HasCondition` names the views that
need such a test, and the `viewcover` package and command find the ones
that lack it.

The app calls `viewcover.Trace(path)` where it resolves a view. With
`HML_TRACE` set, that prints the path to stdout once per process. A
`-v` test run then carries one line per view it reached, and the
command diffs that against the views with a condition:

```sh
go get -tool github.com/croaky/hml/cmd/viewcover
HML_TRACE=1 go test -v ./... > run.txt
go tool viewcover -views ui/views -trace run.txt
```

Stdout rather than a file so the run stays cacheable: Go's test cache
replays what a binary printed and keys on the env vars it read, so an
unchanged package replays its trace without running.

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

A name the map does not hold is not an error here. It compiles to a
call on a helper func the app injects as a local, so a misspelled
transform name is a render error. The engine itself is stdlib-only.

## Editors

This repo is also a tree-sitter grammar: `grammar.js`, an external
scanner for indentation, and highlight and injection queries under
`queries/`. The parser itself is not committed; `tree-sitter generate`
writes it, and nvim-treesitter runs that at install time.

## GitHub repo is a mirror

Development happens on [cibot](https://dancroak.com/cmd/cibot/), a
self-hosted review and CI server, which holds in progress branches.
GitHub receives `main` and the tags so `go get` works.

## License

MIT
