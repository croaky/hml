package hml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/croaky/is"
)

// resolverFor resolves partials from a fixed map, as an app does, on
// the PartialWriter path.
func resolverFor(t *testing.T, templates map[string]*Template) PartialWriter {
	t.Helper()
	var partial PartialWriter
	partial = func(name string, ctx *Context, w *strings.Builder) error {
		tmpl, ok := templates[name]
		if !ok {
			return fmt.Errorf("unknown partial %q", name)
		}
		return tmpl.RenderContextTo(w, ctx, partial)
	}
	return partial
}

// A partial that renders itself recursed until the Go runtime stopped
// the process with a stack overflow, which no recover catches. The
// depth limit makes it an ordinary error that names the partial.
func TestSelfRenderingPartialIsAnError(t *testing.T) {
	is := is.New(t)
	templates := map[string]*Template{"loop": mustParse(t, "%p\n  = render \"loop\"\n")}

	var w strings.Builder
	err := templates["loop"].RenderContextTo(&w, NewContext(nil), resolverFor(t, templates))
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), `partial "loop"`))
	is.True(strings.Contains(err.Error(), "deeper than"))

	// The PartialFunc path carries the same limit.
	var partial PartialFunc
	partial = func(name string, ctx *Context) (string, error) {
		return templates[name].RenderContext(ctx, partial)
	}
	_, err = templates["loop"].RenderContext(NewContext(nil), partial)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "deeper than"))
}

// A partial name from data can pick the page that is rendering it.
func TestDataSelectedPartialRecursionIsAnError(t *testing.T) {
	is := is.New(t)
	templates := map[string]*Template{"page": mustParse(t, "= render name\n")}

	var w strings.Builder
	ctx := NewContext(map[string]any{"name": "page"})
	err := templates["page"].RenderContextTo(&w, ctx, resolverFor(t, templates))
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "deeper than"))
}

// Nesting up to the limit renders, and one more partial does not.
func TestPartialNestingUpToTheLimitRenders(t *testing.T) {
	is := is.New(t)
	chain := func(depth int) map[string]*Template {
		templates := map[string]*Template{}
		for i := range depth {
			src := fmt.Sprintf("= render \"p%d\"\n", i+1)
			if i == depth-1 {
				src = "leaf\n"
			}
			templates[fmt.Sprintf("p%d", i)] = mustParse(t, src)
		}
		return templates
	}
	page := mustParse(t, "%div\n  = render \"p0\"\n")

	var w strings.Builder
	err := page.RenderContextTo(&w, NewContext(nil), resolverFor(t, chain(maxPartialDepth)))
	is.NoErr(err)
	is.Eq(w.String(), "<div>\nleaf\n</div>\n")

	w.Reset()
	err = page.RenderContextTo(&w, NewContext(nil), resolverFor(t, chain(maxPartialDepth+1)))
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), fmt.Sprintf(`partial "p%d"`, maxPartialDepth)))
}

// A loop overlays a context of its own. It is not a partial, so it
// does not count toward the limit.
func TestLoopsDoNotCountTowardThePartialLimit(t *testing.T) {
	is := is.New(t)
	src := "- for a in xs\n"
	indent := "  "
	for range maxPartialDepth + 1 {
		src += indent + "- for b in xs\n"
		indent += "  "
	}
	src += indent + "= render \"leaf\"\n"
	templates := map[string]*Template{"leaf": mustParse(t, "x\n")}

	var w strings.Builder
	err := mustParse(t, src).RenderContextTo(&w, NewContext(map[string]any{"xs": []string{"1"}}), resolverFor(t, templates))
	is.NoErr(err)
	is.Eq(w.String(), "x\n")
}
