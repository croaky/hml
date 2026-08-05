package hml

import (
	"strings"
	"testing"

	"github.com/croaky/is"
)

// partialFor renders one named partial against the context it is handed,
// which is what an app's PartialFunc does. It is how these tests get a
// value across the boundary the old policy could not see through.
func partialFor(t *testing.T, name, src string) PartialFunc {
	t.Helper()
	inner := mustParse(t, src)
	return func(got string, ctx *Context) (string, error) {
		is := is.New(t)
		is.Eq(got, name)
		return inner.RenderContext(ctx, nil)
	}
}

// The change exists for this: a handler a person typed, passed to a
// partial in a hash and splatted onto the tag there. A consumer app
// does it in the hundreds. The value arrives as a map entry with no AST
// node behind it, so the old positional rule called it data and
// refused.
func TestAuthoredSurvivesPartialAndSplat(t *testing.T) {
	is := is.New(t)
	src := `= render "icon", attrs: { onclick: "APP.closeDrawer()" }` + "\n"
	got := mustRenderPartial(t, src, nil, partialFor(t, "icon", "%button{ **attrs }\n  x\n"))
	is.True(strings.Contains(got, `onclick="APP.closeDrawer()"`))
}

// The same one step less indirect: a literal passed as a render arg and
// named directly in the partial's attribute.
func TestAuthoredSurvivesPartialArgument(t *testing.T) {
	is := is.New(t)
	src := `= render "icon", handler: "APP.close()"` + "\n"
	got := mustRenderPartial(t, src, nil, partialFor(t, "icon", "%button{ onclick: handler }\n  x\n"))
	is.True(strings.Contains(got, `onclick="APP.close()"`))
}

func TestAuthoredStyleSurvivesSplat(t *testing.T) {
	is := is.New(t)
	src := `= render "cell", attrs: { style: "vertical-align:middle;" }` + "\n"
	got := mustRenderPartial(t, src, nil, partialFor(t, "cell", "%td{ **attrs }\n  x\n"))
	is.True(strings.Contains(got, `style="vertical-align:middle;"`))
}

// Loosening the policy for a literal must not loosen it for data. A
// handler-supplied map splatted onto a tag is data however it is spelled,
// and the error still names the type that would vouch for it.
func TestSplattedDataInCodeContextStillRejected(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, "%button{ **attrs }\n  x\n")

	_, err := tmpl.Render(map[string]any{
		"attrs": map[string]any{"onclick": "steal()"},
	}, nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "SafeJS"))

	_, err = tmpl.Render(map[string]any{
		"attrs": map[string]any{"style": "width:50%"},
	}, nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "SafeCSS"))

	// And the marked forms still pass, so the policy is one a handler
	// can actually satisfy through a splat.
	got, err := tmpl.Render(map[string]any{
		"attrs": map[string]any{"onclick": SafeJS("APP.close()")},
	}, nil)
	is.NoErr(err)
	is.True(strings.Contains(got, `onclick="APP.close()"`))
}

// Authorship is minted at a string literal, not at a template's
// assembling something around a value. Interpolation is where a template
// builds code out of data, which is the case the policy exists for.
func TestInterpolatedStringIsNotAuthored(t *testing.T) {
	is := is.New(t)
	src := `= render "icon", attrs: { onclick: "hide(#{id})" }` + "\n"
	tmpl := mustParse(t, src)
	_, err := tmpl.Render(map[string]any{"id": int64(1)},
		partialFor(t, "icon", "%button{ **attrs }\n  x\n"))
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "SafeJS"))
}

// An authored value is still a value: it escapes, and it does not become
// a licence to write markup from an attribute. Authorship answers the
// policy's question about code contexts and nothing else.
func TestAuthoredStillEscapes(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, `%p{ title: "a <c> & 'd'" }`+"\n  x\n", nil)
	is.True(strings.Contains(got, `title="a &lt;c&gt; &amp; &#39;d&#39;"`))
	is.True(!strings.Contains(got, "<c>"))
}

// hml's own type must not reach app code. A helper asserts
// args[0].(string), and a consumer app has several.
func TestHelperReceivesPlainString(t *testing.T) {
	is := is.New(t)
	var got any
	locals := map[string]any{
		"fn": func(args ...any) any {
			got = args[0]
			return ""
		},
	}
	mustRender(t, `= fn("x")`+"\n", locals)
	_, ok := got.(string)
	is.True(ok)
}

// The unwrap reaches inside a hash, because a helper handed one reads the
// values in it. react_select does this and discards the assertion's
// result, so a surface-only unwrap would render an empty label and report
// nothing.
func TestHelperReceivesPlainStringsInsideHash(t *testing.T) {
	is := is.New(t)
	var label, nested, elem any
	locals := map[string]any{
		"fn": func(args ...any) any {
			opts := args[0].(map[string]any)
			label = opts["label"]
			nested = opts["deep"].(map[string]any)["k"]
			elem = opts["list"].([]any)[0]
			return ""
		},
	}
	src := `= fn({ label: "Stage", deep: { k: "v" }, list: ["a"] })` + "\n"
	mustRender(t, src, locals)

	// As react_select reads it, discarded ok and all.
	s, _ := label.(string)
	is.Eq(s, "Stage")
	s, _ = nested.(string)
	is.Eq(s, "v")
	s, _ = elem.(string)
	is.Eq(s, "a")
}

// A helper's arguments that hold no literal are handed through as they
// are, so the boundary costs nothing on the common call.
func TestHelperArgsNotCopiedWithoutAuthored(t *testing.T) {
	is := is.New(t)
	supplied := map[string]any{"k": "v"}
	var received any
	locals := map[string]any{
		"data": supplied,
		"fn": func(args ...any) any {
			received = args[0]
			return ""
		},
	}
	mustRender(t, "= fn(data)\n", locals)
	m, ok := received.(map[string]any)
	is.True(ok)
	// Same map, not a copy of it.
	m["k"] = "mutated"
	is.Eq(supplied["k"], "mutated")
}

// Authorship is not part of a value's identity. Each of these is a type
// switch that has to say so; a value type handled in some of them and not
// others behaves as a plain string in some places and not others.

// equal comes first: a miss renders the other branch and reports nothing.
func TestAuthoredComparesAsString(t *testing.T) {
	is := is.New(t)
	src := "- if status == \"Pool\"\n  yes\n- else\n  no\n"
	is.Eq(mustRender(t, src, map[string]any{"status": "Pool"}), "yes\n")
	is.Eq(mustRender(t, src, map[string]any{"status": "Early"}), "no\n")

	// Either side, and against itself.
	is.True(equal(authored("a"), "a"))
	is.True(equal("a", authored("a")))
	is.True(equal(authored("a"), authored("a")))
	is.True(!equal(authored("a"), "b"))

	// Still no cross-type coercion: a literal is not a number.
	is.True(!equal(authored("42"), int64(42)))
}

func TestAuthoredValueSemantics(t *testing.T) {
	is := is.New(t)
	a := authored("x")

	// Not a number, as a plain string is not.
	_, ok := toFloat(a)
	is.True(!ok)

	// Truthy, as a plain string is, including when empty.
	is.True(truthy(a))
	is.True(truthy(authored("")))

	is.Eq(stringify(a), "x")
	is.Eq(toAttrVal(a), "x")

	// Authored is not a handler's assertion. It answers the policy's
	// question on its own, and must not answer this one.
	is.Eq(valTrust(a), trustNone)
}

// A literal compared against a number errors the way a plain string does,
// rather than comparing as one.
func TestAuthoredOrderingComparisonFails(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, "- if \"2\" > n\n  yes\n")
	_, err := tmpl.Render(map[string]any{"n": int64(1)}, nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "cannot compare"))
}

// A literal renders as its text wherever a value renders, so authorship
// is invisible in output.
func TestAuthoredRendersAsItsText(t *testing.T) {
	is := is.New(t)
	is.Eq(mustRender(t, "%p\n  = \"hi\"\n", nil), "<p>hi</p>\n")
	is.Eq(mustRender(t, "%p\n  #{\"hi\"}\n", nil), "<p>hi</p>\n")
}

// An error a template author reads names string. authored is hml's
// bookkeeping, and a message naming it sends the reader looking for a
// type their app cannot have.
func TestErrorsNameStringNotAuthored(t *testing.T) {
	is := is.New(t)

	for _, src := range []string{
		"%button{ **\"x\" }\n  y\n",
		"- for c in \"abc\"\n  %p\n    x\n",
	} {
		_, err := mustParse(t, src).Render(nil, nil)
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "got string"))
	}

	// And through a partial, where a local is an authored literal the
	// template then misuses.
	for _, inner := range []string{"= h.foo\n", "= h(\"a\")\n"} {
		src := `= render "p", h: "x"` + "\n"
		_, err := mustParse(t, src).Render(nil, partialFor(t, "p", inner))
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "string"))
		is.True(!strings.Contains(err.Error(), "authored"))
	}
}

// A loop variable bound to a literal element carries the authorship of
// the literal, since that is what a person wrote.
func TestAuthoredThroughLoopVariable(t *testing.T) {
	is := is.New(t)
	src := "- for h in [\"APP.a()\", \"APP.b()\"]\n  %button{ onclick: h }\n    x\n"
	got := mustRender(t, src, nil)
	is.True(strings.Contains(got, `onclick="APP.a()"`))
	is.True(strings.Contains(got, `onclick="APP.b()"`))
}
