package hml

import (
	"errors"
	"strings"
	"testing"

	"github.com/croaky/is"
)

// A page that renders a partial per row used to pay a buffer, a copy,
// and the garbage of both, per row. RenderContextTo writes into the
// caller's buffer, and a PartialWriter that calls it on the same buffer
// renders the whole page into one.
func TestRenderContextToWritesIntoTheCallersBuffer(t *testing.T) {
	is := is.New(t)
	row := mustParse(t, "%li\n  = name\n")
	page := mustParse(t, "%ul\n  - for r in rows\n    = render \"row\", name: r\n")
	var partial PartialWriter
	partial = func(name string, ctx *Context, w *strings.Builder) error {
		is.Eq(name, "row")
		return row.RenderContextTo(w, ctx, partial)
	}

	var w strings.Builder
	w.WriteString("before\n")
	err := page.RenderContextTo(&w, NewContext(map[string]any{"rows": []string{"a", "b"}}), partial)

	is.NoErr(err)
	is.Eq(w.String(), "before\n<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n")
}

// Both paths render the same bytes, so an app can move to the buffer
// one partial at a time.
func TestRenderContextToMatchesRenderContext(t *testing.T) {
	is := is.New(t)
	inner := mustParse(t, "%span.tag{ title: label }\n  = label\n")
	src := "%p\n  - for l in labels\n    = render \"tag\", label: l\n  text\n"
	tmpl := mustParse(t, src)
	locals := map[string]any{"labels": []string{"x", "y"}}

	want, err := tmpl.RenderContext(NewContext(locals), func(name string, ctx *Context) (string, error) {
		return inner.RenderContext(ctx, nil)
	})
	is.NoErr(err)

	var w strings.Builder
	err = tmpl.RenderContextTo(&w, NewContext(locals), func(name string, ctx *Context, w *strings.Builder) error {
		return inner.RenderContextTo(w, ctx, nil)
	})
	is.NoErr(err)
	is.Eq(w.String(), want)
}

// A render call with no way to resolve it is an error, on either path.
func TestRenderContextToWithoutPartialWriterErrors(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, "= render \"missing\"\n")

	var w strings.Builder
	err := tmpl.RenderContextTo(&w, NewContext(nil), nil)

	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "no partialFn"))
}

// A partial's error reaches the caller with the buffer as it was when
// the partial failed, so nothing after it is written.
func TestRenderContextToStopsAtAPartialError(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, "%p\n  = render \"boom\"\n  after\n")

	var w strings.Builder
	err := tmpl.RenderContextTo(&w, NewContext(nil), func(name string, ctx *Context, w *strings.Builder) error {
		w.WriteString("partial\n")
		return errors.New("boom")
	})

	is.HasErr(err)
	is.Eq(w.String(), "<p>\npartial\n")
}
