package hml

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/croaky/is"
)

// testTransforms registers identity stand-ins for the transform names so
// parse-time name validation has names to check. The real sanitizing
// transforms live app-side; the engine only needs the
// registered set to accept `= name(field)` and reject unknown names.
var testTransforms = map[string]Transform{
	"markdown":         func(s string) string { return s },
	"slack":            func(s string) string { return s },
	"search_highlight": func(s string) string { return s },
}

func mustParse(t *testing.T, src string) *Template {
	is := is.New(t)
	t.Helper()
	tmpl, err := Parse(src, "test.hml", testTransforms)
	is.NoErr(err)
	return tmpl
}

func mustRender(t *testing.T, src string, locals map[string]any) string {
	t.Helper()
	return mustRenderPartial(t, src, locals, nil)
}

func mustRenderPartial(t *testing.T, src string, locals map[string]any, partialFn PartialFunc) string {
	is := is.New(t)
	t.Helper()
	tmpl := mustParse(t, src)
	got, err := tmpl.Render(locals, partialFn)
	is.NoErr(err)
	return got
}

func TestDoctype(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "!!!\n", nil)
	is.Eq(got, "<!DOCTYPE html>\n")
}

func TestComment(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "-# this is ignored\n%p\n  hello\n", nil)
	is.Eq(got, "<p>hello</p>\n")
}

func TestStaticText(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%p\n  hello world\n", nil)
	is.Eq(got, "<p>hello world</p>\n")
}

// A tag holding a tag stays a block; the inner one holds its own text.
// See TestLoneTextChildRendersInline for the rule.
func TestTag(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%div\n  %p\n    text\n", nil)
	is.Eq(got, "<div>\n<p>text</p>\n</div>\n")
}

func TestTagClassAndID(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%div.foo.bar#baz\n  text\n", nil)
	is.True(strings.Contains(got, `class="foo bar"`))
	is.True(strings.Contains(got, `id="baz"`))
}

func TestImplicitDiv(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, ".wrapper\n  text\n", nil)
	is.True(strings.Contains(got, "<div"))
	is.True(strings.Contains(got, `class="wrapper"`))
}

func TestRejectsBareDotSelector(t *testing.T) {
	is := is.New(t)
	_, err := Parse(".\n", "test.hml", nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "invalid class shorthand"))
}

func TestRejectsBareHashSelector(t *testing.T) {
	is := is.New(t)
	_, err := Parse("#\n", "test.hml", nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "invalid id shorthand"))
}

func TestTagAttributes(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, `%a{ href: "/home", style: "color:red" }`+"\n  click\n", nil)
	is.True(strings.Contains(got, `href="/home"`))
	is.True(strings.Contains(got, `style="color:red"`))
}

func TestRejectsNilPredicate(t *testing.T) {
	is := is.New(t)
	// Expressions are compiled at Parse time, so the `?` is rejected when
	// the template is parsed, not when it is rendered.
	_, err := Parse("%option{ selected: data.selected_id.nil? }\n", "test.hml", nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "unexpected character"))
}

func TestParseCompilesExpressionsEagerly(t *testing.T) {
	is := is.New(t)
	// A syntactically invalid expression is rejected at Parse time,
	// before any Render call.
	_, err := Parse("= a +\n", "test.hml", nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "unexpected character"))
}

func TestTagAttributeInterpolation(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%a{ href: \"/items/#{id}\" }\n  link\n", map[string]any{"id": int64(42)})
	is.True(strings.Contains(got, `href="/items/42"`))
}

func TestBooleanAttribute(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%input{ type: \"checkbox\", checked: true }\n", nil)
	is.True(strings.Contains(got, " checked"))
	is.True(!strings.Contains(got, `checked="`))
}

func TestFalseAttributeOmitted(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%input{ type: \"text\", disabled: false }\n", nil)
	is.True(!strings.Contains(got, "disabled"))
}

func TestVoidElement(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%br\n", nil)
	is.Eq(got, "<br>\n")
}

func TestEmptyNonVoidTag(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%span\n", nil)
	is.Eq(got, "<span></span>\n")
}

func TestEscapedOutput(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "= name\n", map[string]any{"name": "<script>alert(1)</script>"})
	is.True(strings.Contains(got, "&lt;script&gt;"))
}

func TestRawOutputRejected(t *testing.T) {
	is := is.New(t)
	_, err := Parse("!= html\n", "test.hml", nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "raw output (!=) is not supported"))
}

func TestEscapedOutputSafeString(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "= meta\n", map[string]any{"meta": SafeString("<meta name=\"csrf-token\" content=\"x\" />")})
	is.Eq(got, "<meta name=\"csrf-token\" content=\"x\" />\n")
}

func TestIfTrue(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "- if show\n  visible\n", map[string]any{"show": true})
	is.Eq(got, "visible\n")
}

func TestIfFalse(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "- if show\n  visible\n", map[string]any{"show": false})
	is.Eq(got, "")
}

func TestIfElse(t *testing.T) {
	is := is.New(t)
	src := "- if show\n  yes\n- else\n  no\n"
	got := mustRender(t, src, map[string]any{"show": false})
	is.Eq(got, "no\n")
}

func TestIfElseIf(t *testing.T) {
	is := is.New(t)
	src := "- if a\n  first\n- else if b\n  second\n- else\n  third\n"
	got := mustRender(t, src, map[string]any{"a": false, "b": true})
	is.Eq(got, "second\n")
}

func TestForLoop(t *testing.T) {
	is := is.New(t)
	src := "- for item in items\n  = item.name\n"
	items := []any{
		map[string]any{"name": "Alice"},
		map[string]any{"name": "Bob"},
	}
	got := mustRender(t, src, map[string]any{"items": items})
	is.Eq(got, "Alice\nBob\n")
}

func TestForLoopWithIndex(t *testing.T) {
	is := is.New(t)
	src := "- for i, item in items\n  = i\n  = item.name\n"
	items := []any{
		map[string]any{"name": "Alice"},
		map[string]any{"name": "Bob"},
	}
	got := mustRender(t, src, map[string]any{"items": items})
	is.Eq(got, "0\nAlice\n1\nBob\n")
}

func TestEachLoopRejected(t *testing.T) {
	is := is.New(t)
	_, err := Parse("- items.each do |item|\n  = item.name\n", "test.hml", nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "unsupported control"))
}

func TestRenderPartial(t *testing.T) {
	is := is.New(t)
	src := "= render \"header\", title: \"Hello\"\n"
	partialFn := func(name string, ctx *Context) (string, error) {
		is.Eq(name, "header")
		// stringify, as every read site in the renderer does: a
		// value in a context is an hml value, and a literal one
		// carries its authorship. Reaching past that with a type
		// assertion is reaching past the engine.
		title, _ := ctx.lookup("title")
		return "<h1>" + stringify(title) + "</h1>\n", nil
	}
	got := mustRenderPartial(t, src, nil, partialFn)
	is.Eq(got, "<h1>Hello</h1>\n")
}

func TestPartialInheritsParentLocalsAndChildOverrides(t *testing.T) {
	is := is.New(t)
	inner := mustParse(t, "= greeting\n= name\n")
	src := "= render \"inner\", name: \"child\"\n"
	partialFn := func(name string, ctx *Context) (string, error) {
		return inner.RenderContext(ctx, nil)
	}
	// greeting is inherited from the parent through the context chain;
	// name is shadowed by the child render arg.
	got := mustRenderPartial(t, src, map[string]any{"greeting": "hi", "name": "parent"}, partialFn)
	is.Eq(got, "hi\nchild\n")
}

func TestJavascriptFilter(t *testing.T) {
	is := is.New(t)
	src := ":javascript\n  alert('hi');\n"
	got := mustRender(t, src, nil)
	is.Eq(got, "<script>\nalert('hi');\n</script>\n")
}

func TestCSSFilter(t *testing.T) {
	is := is.New(t)
	src := ":css\n  body { color: red; }\n"
	got := mustRender(t, src, nil)
	is.Eq(got, "<style>\nbody { color: red; }\n</style>\n")
}

func TestTextInterpolation(t *testing.T) {
	is := is.New(t)
	src := "%p\n  Hello #{name}!\n"
	got := mustRender(t, src, map[string]any{"name": "World"})
	is.True(strings.Contains(got, "Hello World!"))
}

func TestTextInterpolationEscapes(t *testing.T) {
	is := is.New(t)
	src := "%p\n  Hello #{name}!\n"
	got := mustRender(t, src, map[string]any{"name": "<b>"})
	is.True(strings.Contains(got, "Hello &lt;b&gt;!"))
}

func TestStringComparison(t *testing.T) {
	is := is.New(t)
	src := "- if status == \"active\"\n  yes\n"
	got := mustRender(t, src, map[string]any{"status": "active"})
	is.Eq(got, "yes\n")
}

func TestBooleanAnd(t *testing.T) {
	is := is.New(t)
	src := "- if a && b\n  both\n"
	got := mustRender(t, src, map[string]any{"a": true, "b": true})
	is.Eq(got, "both\n")
}

func TestBooleanNot(t *testing.T) {
	is := is.New(t)
	src := "- if !hidden\n  shown\n"
	got := mustRender(t, src, map[string]any{"hidden": false})
	is.Eq(got, "shown\n")
}

func TestNestedFieldAccess(t *testing.T) {
	is := is.New(t)
	src := "= data.user.name\n"
	locals := map[string]any{
		"data": map[string]any{
			"user": map[string]any{
				"name": "Alice",
			},
		},
	}
	got := mustRender(t, src, locals)
	is.Eq(got, "Alice\n")
}

func TestPreserveElement(t *testing.T) {
	is := is.New(t)
	src := "%textarea\n  hello\n"
	got := mustRender(t, src, nil)
	is.Eq(got, "<textarea>hello</textarea>\n")
}

func TestPreserveElementKeepsContentWhitespace(t *testing.T) {
	is := is.New(t)
	// A patch's leading spaces are the alignment a reader reads it by,
	// so a pre keeps them and drops only the newline the renderer added.
	src := "%pre\n  = patch\n"
	patch := SafeString("@@ -1,2 +1,2 @@\n context\n-old\n+new\n")
	got := mustRender(t, src, map[string]any{"patch": patch})
	is.Eq(got, "<pre>@@ -1,2 +1,2 @@\n context\n-old\n+new\n</pre>\n")
}

func TestMultiLineAttributes(t *testing.T) {
	is := is.New(t)
	src := "%a{ href: \"/home\",\n  class: \"link\" }\n  click\n"
	got := mustRender(t, src, nil)
	is.True(strings.Contains(got, `class="link"`))
	is.True(strings.Contains(got, `href="/home"`))
}

func TestAttributesSortAlphabetically(t *testing.T) {
	is := is.New(t)
	src := "%a{ style: \"x\", href: \"/\" }\n  text\n"
	got := mustRender(t, src, nil)
	hrefIdx := strings.Index(got, "href")
	styleIdx := strings.Index(got, "style")
	is.True(hrefIdx <= styleIdx)
}

func TestUndefinedVariable(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, "= missing\n")
	_, err := tmpl.Render(nil, nil)
	is.HasErr(err)
}

// fieldHolder stands in for any struct an app hands a template: a
// field the template may read and one it may not.
type fieldHolder struct {
	Name string
	vars map[string]any
}

func TestUnexportedFieldIsRenderError(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, "= data.vars\n")
	_, err := tmpl.Render(map[string]any{"data": fieldHolder{vars: map[string]any{}}}, nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "undefined field"))
}

func TestExportedFieldStillReadable(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "= data.name\n", map[string]any{"data": fieldHolder{Name: "Alice"}})
	is.Eq(got, "Alice\n")
}

func TestMailReminderTemplate(t *testing.T) {
	is := is.New(t)
	src := `= header_html

%p
  = comments

- if has_thesis
  = thesis

%p
  %strong
    Actions
  %br
  Mark the reminder done by
  %a{ href: done_url }
    clicking here

- if status == "Early"
  %p
    This company is currently in Early status.
`
	locals := map[string]any{
		"header_html": SafeString("<h1>Acme</h1>"),
		"comments":    SafeString("<p>Follow up</p>"),
		"has_thesis":  true,
		"thesis":      SafeString("<p>Big picture</p>"),
		"done_url":    "https://example.com/done",
		"status":      "Early",
	}
	got := mustRender(t, src, locals)
	is.True(strings.Contains(got, "<h1>Acme</h1>"))
	is.True(strings.Contains(got, "<p>Follow up</p>"))
	is.True(strings.Contains(got, `href="https://example.com/done"`))
	is.True(strings.Contains(got, "Early status"))
}

func TestEqualityTypeSafety(t *testing.T) {
	// String "42" != integer 42 (no cross-type coercion)
	is := is.New(t)

	src := "- if val == \"42\"\n  match\n"
	got := mustRender(t, src, map[string]any{"val": int64(42)})
	is.Eq(got, "")

	// Same types compare correctly
	got = mustRender(t, src, map[string]any{"val": "42"})
	is.Eq(got, "match\n")

	// nil == nil
	src = "- if val == nil\n  null\n"
	got = mustRender(t, src, map[string]any{"val": nil})
	is.Eq(got, "null\n")
}

func TestPartialLocalsPreserveTypes(t *testing.T) {
	is := is.New(t)
	src := "= render \"widget\", count: 5, active: true, label: \"hi\"\n"
	var received *Context
	partialFn := func(name string, ctx *Context) (string, error) {
		received = ctx
		return "", nil
	}
	mustRenderPartial(t, src, nil, partialFn)
	count, _ := received.lookup("count")
	vCount, okCount := count.(int64)
	is.True(okCount && vCount == 5)
	active, _ := received.lookup("active")
	vActive, okActive := active.(bool)
	is.True(okActive && vActive)
	// A string literal keeps its authorship into the partial, which is
	// what lets the partial splat it onto an on* or style attribute. It
	// is not a plain string here, so it is read the way the renderer
	// reads one.
	label, _ := received.lookup("label")
	is.True(isAuthored(label))
	is.Eq(stringify(label), "hi")
}

func TestNilOutputRendersEmpty(t *testing.T) {
	// = with nil should render empty string, not "<nil>"
	is := is.New(t)

	got := mustRender(t, "= val\n", map[string]any{"val": nil})
	is.Eq(got, "\n")
}

func TestNilInterpolationRendersEmpty(t *testing.T) {
	is := is.New(t)
	// #{nil} renders empty, matching = output (not "<nil>").
	got := mustRender(t, "%p\n  Hello #{val}!\n", map[string]any{"val": nil})
	is.True(strings.Contains(got, "Hello !"))
	is.True(!strings.Contains(got, "nil"))
}

func TestNilInterpolationInStringLiteralRendersEmpty(t *testing.T) {
	is := is.New(t)
	// nil inside a "#{}" string expression renders empty, consistent with
	// text interpolation and = output.
	got := mustRender(t, "%a{ href: \"/co/#{val}\" }\n  x\n", map[string]any{"val": nil})
	is.True(strings.Contains(got, `href="/co/"`))
	is.True(!strings.Contains(got, "nil"))
}

// A condition takes a bool and nothing else. The message names the
// view, the condition, and the type, because the fix is in the handler
// that built the value and the reader has to find it.
func TestConditionRequiresBool(t *testing.T) {
	is := is.New(t)
	for _, val := range []any{"", "text", int64(0), int64(1), nil, []string{}} {
		tmpl := mustParse(t, "- if title\n  x\n")
		_, err := tmpl.Render(map[string]any{"title": val}, nil)
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "requires a bool"))
		is.True(strings.Contains(err.Error(), "test.hml"))
		is.True(strings.Contains(err.Error(), `"title"`))
	}

	// An else if is held to the same rule, and only when it is reached.
	tmpl := mustParse(t, "- if a\n  x\n- else if title\n  y\n")
	_, err := tmpl.Render(map[string]any{"a": true, "title": "t"}, nil)
	is.NoErr(err)
	_, err = tmpl.Render(map[string]any{"a": false, "title": "t"}, nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "requires a bool"))
}

// The forms that replace implicit truthiness still render: a comparison
// the template states, and a bool the handler computed.
func TestConditionAcceptsStatedComparisons(t *testing.T) {
	is := is.New(t)
	is.Eq(mustRender(t, "- if s != \"\"\n  x\n", map[string]any{"s": "a"}), "x\n")
	is.Eq(mustRender(t, "- if s != \"\"\n  x\n", map[string]any{"s": ""}), "")
	is.Eq(mustRender(t, "- if n > 0\n  x\n", map[string]any{"n": int64(1)}), "x\n")
	is.Eq(mustRender(t, "- if has_x\n  x\n", map[string]any{"has_x": true}), "x\n")
}

// && and || keep operand-return, so a condition built from bools is
// still one, and one built from a string is the error it should be.
func TestConditionShortCircuitStillReturnsOperand(t *testing.T) {
	is := is.New(t)
	src := "- if a || b\n  x\n"
	is.Eq(mustRender(t, src, map[string]any{"a": false, "b": true}), "x\n")
	is.Eq(mustRender(t, src, map[string]any{"a": false, "b": false}), "")
}

// A condition the AST already settles is refused at Parse, so the
// branch nobody exercises fails on the way in rather than the first
// time it is reached. The message carries the line, because a template
// has more than one condition in it.
func TestLiteralConditionRejectedAtParse(t *testing.T) {
	is := is.New(t)
	for _, src := range []string{
		"- if \"x\"\n  y\n",
		"- if 1\n  y\n",
		"- if nil\n  y\n",
		"- if title || \"Untitled\"\n  y\n",
		"- if a\n  y\n- else if \"x\"\n  z\n",
	} {
		_, err := Parse(src, "test.hml", nil)
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "requires a bool"))
	}

	_, err := Parse("%p\n%p\n- if \"x\"\n  y\n", "test.hml", nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "test.hml:3"))

	// A for collection is not a condition and takes a literal.
	is.Eq(mustRender(t, "- for x in [1, 2]\n  = x\n", nil), "1\n2\n")
}

// ! is a conditional written backwards, so it takes a bool and nothing
// else. Otherwise one character restores the guess.
func TestNotRequiresBool(t *testing.T) {
	is := is.New(t)
	is.Eq(mustRender(t, "- if !b\n  x\n", map[string]any{"b": false}), "x\n")
	is.Eq(mustRender(t, "- if !b\n  x\n", map[string]any{"b": true}), "")

	tmpl := mustParse(t, "- if !title\n  x\n")
	_, err := tmpl.Render(map[string]any{"title": ""}, nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "! requires a bool"))
}

// == nil is how a template asks about absence, and it has to answer for
// a typed nil in an interface -- which Go's own == reads as present.
func TestTypedNilPointerIsFalsy(t *testing.T) {
	is := is.New(t)
	src := "- if val == nil\n  falsy\n- else\n  truthy\n"

	// nil pointer
	var p *string = nil
	is.Eq(mustRender(t, src, map[string]any{"val": p}), "falsy\n")

	// nil slice
	var s []string = nil
	is.Eq(mustRender(t, src, map[string]any{"val": s}), "falsy\n")

	// nil map
	var m map[string]any = nil
	is.Eq(mustRender(t, src, map[string]any{"val": m}), "falsy\n")

	// nil func
	var f func() = nil
	is.Eq(mustRender(t, src, map[string]any{"val": f}), "falsy\n")

	// A non-nil pointer is present, which is the half a policy that
	// only rejects would not show.
	str := "x"
	is.Eq(mustRender(t, src, map[string]any{"val": &str}), "truthy\n")
}

func TestTypedSliceFor(t *testing.T) {
	// []string should work in for loops without pre-boxing to []any
	is := is.New(t)

	src := "- for item in items\n  = item\n"
	got := mustRender(t, src, map[string]any{"items": []string{"a", "b"}})
	is.Eq(got, "a\nb\n")
}

// - else if is one control, not an - else holding a nested - if: the
// chain walker only continues across siblings, so a parse that nested
// would put the branch out of its reach and the else would always win.
func TestElseIfIsOneControl(t *testing.T) {
	is := is.New(t)
	src := "- if a\n  first\n- else if b\n  second\n- else\n  third\n"
	is.Eq(mustRender(t, src, map[string]any{"a": true, "b": true}), "first\n")
	is.Eq(mustRender(t, src, map[string]any{"a": false, "b": true}), "second\n")
	is.Eq(mustRender(t, src, map[string]any{"a": false, "b": false}), "third\n")
}

func TestSplatBooleanAttributes(t *testing.T) {
	is := is.New(t)
	src := "%input{ **attrs }\n"
	locals := map[string]any{
		"attrs": map[string]any{
			"type":     "checkbox",
			"checked":  true,
			"disabled": false,
		},
	}
	got := mustRender(t, src, locals)
	is.True(strings.Contains(got, " checked"))
	is.True(!strings.Contains(got, `checked="`))
	is.True(!strings.Contains(got, "disabled"))
}

// Bare calls (= fn "arg", key: val) remain for the do_react/
// react_select shims. Parenthesized calls work too, and mean the same
// thing in output and attribute position; see TestCallResolvesHelper.
func TestBareFunctionCallWithKeywordArgsSupported(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, "= fn \"Widget\", id: \"abc\", active: true\n")
	got, err := tmpl.Render(map[string]any{
		"fn": func(args ...any) (any, error) {
			kind := fmt.Sprintf("%v", args[0])
			props := args[1].(map[string]any)
			return fmt.Sprintf("%s:%s:%v", kind, props["id"], props["active"]), nil
		},
	}, nil)
	is.NoErr(err)
	is.Eq(got, "Widget:abc:true\n")
}

func TestEntityHeaderTemplate(t *testing.T) {
	is := is.New(t)
	src := `%table{ style: "border-collapse:collapse;margin:0 0 16px 0;" }
  %tr
    %td{ style: "vertical-align:middle;padding:0 8px 0 0;" }
      %a{ href: url }
        %img{ src: image_url, width: "32", height: "32", style: avatar_style }
    %td{ style: "vertical-align:middle;" }
      %a{ href: url, style: "font-weight:bold;" }
        = name
  - if has_ceo
    %tr
      %td{ style: "vertical-align:middle;padding:8px 8px 0 0;" }
        %a{ href: ceo_url }
          %img{ src: ceo_avatar_url, width: "32", height: "32", style: "border-radius:50%;display:block;" }
      %td{ style: "vertical-align:middle;padding-top:8px;" }
        %a{ href: ceo_url, style: "font-weight:bold;" }
          = ceo_name
`
	locals := map[string]any{
		"url":       "https://example.com/co/1",
		"image_url": "https://img.example.com/logo.png",
		// A style built in Go, marked SafeCSS: the CSS context
		// takes no plain dynamic value.
		"avatar_style":   SafeCSS("border-radius:4px;display:block;"),
		"name":           "Acme Corp",
		"has_ceo":        true,
		"ceo_url":        "https://example.com/people/5",
		"ceo_avatar_url": "https://img.example.com/ceo.png",
		"ceo_name":       "Jane Doe",
	}
	got := mustRender(t, src, locals)
	is.True(strings.Contains(got, "Acme Corp"))
	is.True(strings.Contains(got, "Jane Doe"))
	is.True(strings.Contains(got, "border-radius:4px"))
}

// #ZgotmplZ is a sentinel, not a link target: it marks a URL the
// renderer refused, and matches what html/template emits.
func TestURLAttributeRejectsUnsafeScheme(t *testing.T) {
	is := is.New(t)
	for _, url := range []string{
		"javascript:alert(1)",
		"JaVaScRiPt:alert(1)",
		"java\tscript:alert(1)",
		" javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"vbscript:msgbox",
	} {
		got := mustRender(t, "%a{ href: u }\n  x\n", map[string]any{"u": url})
		is.True(strings.Contains(got, `href="#ZgotmplZ"`))
		is.True(!strings.Contains(got, "alert"))
	}

	// Every URL attribute, not just href.
	got := mustRender(t, "%img{ src: u }\n", map[string]any{"u": "javascript:alert(1)"})
	is.True(strings.Contains(got, `src="#ZgotmplZ"`))
}

func TestURLAttributeAllowsRelativeAndAllowlistedSchemes(t *testing.T) {
	is := is.New(t)
	for _, url := range []string{
		"/change/CI-1",
		"#x",
		"?q=1",
		"change/CI-1",
		"//example.com/logo.png",
		"https://example.com",
		"http://example.com",
		"mailto:a@example.com",
		"tel:+15555555555",
	} {
		got := mustRender(t, "%a{ href: u }\n  x\n", map[string]any{"u": url})
		is.True(strings.Contains(got, `href="`+url+`"`))
	}
}

// An on* attribute is a JavaScript context. A literal is application
// source and passes; data is not code, so it needs SafeJS.
func TestJSAttributeRequiresLiteralOrSafeJS(t *testing.T) {
	is := is.New(t)

	got := mustRender(t, `%button{ onclick: "APP.close()" }`+"\n  x\n", nil)
	is.True(strings.Contains(got, `onclick="APP.close()"`))

	tmpl := mustParse(t, "%button{ onclick: handler }\n  x\n")
	_, err := tmpl.Render(map[string]any{"handler": "APP.close()"}, nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "SafeJS"))

	got, err = tmpl.Render(map[string]any{"handler": SafeJS("APP.close()")}, nil)
	is.NoErr(err)
	is.True(strings.Contains(got, `onclick="APP.close()"`))

	// Interpolation is not a literal, however much of it is authored.
	tmpl = mustParse(t, "%img{ onerror: \"hide(#{id})\" }\n")
	_, err = tmpl.Render(map[string]any{"id": int64(1)}, nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "SafeJS"))
}

func TestStyleAttributeRequiresLiteralOrSafeCSS(t *testing.T) {
	is := is.New(t)

	got := mustRender(t, `%p{ style: "white-space: pre-wrap" }`+"\n  x\n", nil)
	is.True(strings.Contains(got, `style="white-space: pre-wrap"`))

	tmpl := mustParse(t, "%p{ style: css }\n  x\n")
	_, err := tmpl.Render(map[string]any{"css": "width:50%"}, nil)
	is.HasErr(err)
	is.True(strings.Contains(err.Error(), "SafeCSS"))

	got, err = tmpl.Render(map[string]any{"css": SafeCSS("width:50%")}, nil)
	is.NoErr(err)
	is.True(strings.Contains(got, `style="width:50%"`))
}

// Transform rendering behavior (markdown/slack/search_highlight output)
// is tested app-side, where the sanitizing transforms live. The engine
// tests below cover how a call resolves, and the single-field-access
// restriction on a transform's argument.

// A name that is not a registered transform is a helper func the app
// injected as a local, so it is resolved at render rather than
// rejected at parse. The cost is that a misspelled transform name is a
// render error instead of a parse error; the engine cannot tell the two
// apart without the locals, which arrive per render.
func TestCallOfUnknownNameFailsAtRender(t *testing.T) {
	is := is.New(t)
	tmpl, err := Parse("= foobar(row.x)\n", "test.hml", testTransforms)
	is.NoErr(err)
	_, err = tmpl.Render(map[string]any{"row": map[string]any{"x": "v"}}, nil)
	is.HasErr(err)
}

// One syntax, one meaning, wherever it appears: a parenthesized call on
// an injected helper reads the same in output and in an attribute. The
// two disagreed before -- output rejected the name as an unknown
// transform while the attribute path called it.
func TestCallResolvesHelper(t *testing.T) {
	is := is.New(t)
	locals := map[string]any{
		"c": map[string]any{"id": "CI-1", "sha": "abcdef"},
		"url": func(args ...any) (any, error) {
			if len(args) == 2 {
				return fmt.Sprintf("/change/%v/%v", args[0], args[1]), nil
			}
			return fmt.Sprintf("/change/%v", args[0]), nil
		},
	}
	is.Eq(mustRender(t, "= url(c.id)\n", locals), "/change/CI-1\n")
	// More than one argument, which a transform cannot take.
	is.Eq(mustRender(t, "= url(c.id, c.sha)\n", locals), "/change/CI-1/abcdef\n")
	got := mustRender(t, "%a{ href: url(c.id) }\n  x\n", locals)
	is.True(strings.Contains(got, `href="/change/CI-1"`))
}

// A registered name still means the transform, and a helper of the same
// name does not shadow it.
func TestTransformWinsOverLocal(t *testing.T) {
	is := is.New(t)
	transforms := map[string]Transform{
		"markdown": func(s string) string { return "<p>" + s + "</p>" },
	}
	tmpl, err := Parse("= markdown(row.x)\n", "test.hml", transforms)
	is.NoErr(err)
	got, err := tmpl.Render(map[string]any{
		"row":      map[string]any{"x": "hi"},
		"markdown": func(args ...any) (any, error) { return "local", nil },
	}, nil)
	is.NoErr(err)
	// The transform's output, emitted unescaped as transforms are.
	is.Eq(got, "<p>hi</p>\n")
}

func TestTransformRejectsNonFieldArguments(t *testing.T) {
	is := is.New(t)
	for _, src := range []string{
		`= markdown("literal")`,
		"= markdown(row.x, row.y)",
		"= markdown(markdown(row.x))",
		"= markdown(#{row.x})",
	} {
		_, err := Parse(src+"\n", "test.hml", testTransforms)
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "single field access"))
	}
}

func TestNilClassAttrOmitted(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%a.x{ class: active }\n", map[string]any{"active": nil})
	is.True(strings.Contains(got, `class="x"`))
	is.True(!strings.Contains(got, `class="x `))
}
func TestClassAttrMergesWhenPresent(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%a.x{ class: active }\n", map[string]any{"active": "active"})
	is.True(strings.Contains(got, `class="x active"`))
}

func TestFloat64IntegerFormatting(t *testing.T) {
	is := is.New(t)
	// float64 with no fractional part should render as a clean integer, not scientific notation
	got := mustRender(t, "%a{ href: \"/people/merge_form?id=#{id}\" }\n", map[string]any{"id": float64(1173828)})
	is.True(strings.Contains(got, `href="/people/merge_form?id=1173828"`))

	// float64 with a fractional part should still render with decimals/scientific notation as appropriate
	gotDecimal := mustRender(t, "= val\n", map[string]any{"val": float64(1173828.5)})
	is.True(gotDecimal == "1.1738285e+06\n" || gotDecimal == "1173828.5\n")
}

func BenchmarkRenderLoop(b *testing.B) {
	src := "%ul\n" +
		"  - for row in rows\n" +
		"    %li{ class: row.cls, \"data-id\": row.id }\n" +
		"      = row.name\n"
	tmpl, err := Parse(src, "bench.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	rows := make([]any, 100)
	for i := range rows {
		rows[i] = map[string]any{
			"name": fmt.Sprintf("Row %d", i),
			"cls":  "row",
			"id":   int64(i),
		}
	}
	locals := map[string]any{"rows": rows}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := tmpl.Render(locals, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestNamesCoversEveryExpressionSite(t *testing.T) {
	is := is.New(t)
	src := "%a{ href: link, **attrs }\n" +
		"  text #{caption} more\n" +
		"- if flag\n" +
		"  = title\n" +
		"- else if other\n" +
		"  = markdown(body)\n" +
		"- for row in rows\n" +
		"  = render \"card\", user: person\n" +
		":javascript\n" +
		"  var t = #{token};\n"
	tmpl := mustParse(t, src)
	want := []string{
		"attrs", "body", "caption", "flag", "link", "other",
		"person", "rows", "title", "token",
	}
	is.True(slices.Equal(tmpl.Names(), want))
}

func TestNamesExcludesLoopVariables(t *testing.T) {
	is := is.New(t)
	src := "- for i, item in items\n" +
		"  = item.name\n" +
		"  = i\n" +
		"  = outer\n"
	tmpl := mustParse(t, src)
	is.True(slices.Equal(tmpl.Names(), []string{"items", "outer"}))
}

func TestNamesLoopVariableBindingIsScoped(t *testing.T) {
	is := is.New(t)
	// item is bound only inside the loop body; the same name read after
	// the loop is free.
	src := "- for item in items\n" +
		"  = item\n" +
		"= item\n"
	tmpl := mustParse(t, src)
	is.True(slices.Equal(tmpl.Names(), []string{"item", "items"}))
}

func TestNamesExcludesRenderArgKeys(t *testing.T) {
	is := is.New(t)
	// user is a name the partial reads, not one the caller supplies.
	tmpl := mustParse(t, "= render \"card\", user: person\n")
	is.True(slices.Equal(tmpl.Names(), []string{"person"}))
}

func TestNamesSortedAndDeduplicated(t *testing.T) {
	is := is.New(t)
	src := "= zebra\n= apple\n= zebra\n= apple.core\n"
	tmpl := mustParse(t, src)
	is.True(slices.Equal(tmpl.Names(), []string{"apple", "zebra"}))
}

func TestNamesIncludesHelperCallsAndNestedValues(t *testing.T) {
	is := is.New(t)
	src := "= helper(a, b.c)\n" +
		"%p{ data: { k: d }, list: [e] }\n" +
		"- if !f && g == \"x\"\n" +
		"  = \"lit #{h}\"\n"
	tmpl := mustParse(t, src)
	want := []string{"a", "b", "d", "e", "f", "g", "h", "helper"}
	is.True(slices.Equal(tmpl.Names(), want))
}

func TestRendersListsLiteralPartialNames(t *testing.T) {
	is := is.New(t)
	src := "= render \"b\"\n" +
		"= render \"a\", k: v\n" +
		"= render \"b\"\n"
	tmpl := mustParse(t, src)
	is.True(slices.Equal(tmpl.Renders(), []string{"a", "b"}))
	is.True(slices.Equal(tmpl.Names(), []string{"v"}))
}

func TestRendersOmitsComputedNames(t *testing.T) {
	is := is.New(t)
	src := "= render partial\n" +
		"= render \"cards/#{kind}\"\n" +
		"= render \"plain\"\n"
	tmpl := mustParse(t, src)
	is.True(slices.Equal(tmpl.Renders(), []string{"plain"}))
	is.True(slices.Equal(tmpl.Names(), []string{"kind", "partial"}))
}

func TestNamesDoesNotFollowPartials(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, "= render \"card\"\n")
	is.Eq(len(tmpl.Names()), 0)
	is.True(slices.Equal(tmpl.Renders(), []string{"card"}))
}
