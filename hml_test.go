package hml

import (
	"fmt"
	"slices"
	"strings"
	"testing"
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
	tu := newAssert(t)
	t.Helper()
	tmpl, err := Parse(src, "test.hml", testTransforms)
	tu.OK(err == nil)
	return tmpl
}

func mustRender(t *testing.T, src string, locals map[string]any) string {
	t.Helper()
	return mustRenderPartial(t, src, locals, nil)
}

func mustRenderPartial(t *testing.T, src string, locals map[string]any, partialFn PartialFunc) string {
	tu := newAssert(t)
	t.Helper()
	tmpl := mustParse(t, src)
	got, err := tmpl.Render(locals, partialFn)
	tu.OK(err == nil)
	return got
}

func TestDoctype(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "!!!\n", nil)
	tu.OK(got == "<!DOCTYPE html>\n")
}

func TestComment(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "-# this is ignored\n%p\n  hello\n", nil)
	tu.OK(got == "<p>hello</p>\n")
}

func TestStaticText(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%p\n  hello world\n", nil)
	tu.OK(got == "<p>hello world</p>\n")
}

// A tag holding a tag stays a block; the inner one holds its own text.
// See TestLoneTextChildRendersInline for the rule.
func TestTag(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%div\n  %p\n    text\n", nil)
	tu.OK(got == "<div>\n<p>text</p>\n</div>\n")
}

func TestTagClassAndID(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%div.foo.bar#baz\n  text\n", nil)
	tu.OK(strings.Contains(got, `class="foo bar"`))
	tu.OK(strings.Contains(got, `id="baz"`))
}

func TestImplicitDiv(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, ".wrapper\n  text\n", nil)
	tu.OK(strings.Contains(got, "<div"))
	tu.OK(strings.Contains(got, `class="wrapper"`))
}

func TestRejectsBareDotSelector(t *testing.T) {
	tu := newAssert(t)
	_, err := Parse(".\n", "test.hml", nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "invalid class shorthand"))
}

func TestRejectsBareHashSelector(t *testing.T) {
	tu := newAssert(t)
	_, err := Parse("#\n", "test.hml", nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "invalid id shorthand"))
}

func TestTagAttributes(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, `%a{ href: "/home", style: "color:red" }`+"\n  click\n", nil)
	tu.OK(strings.Contains(got, `href="/home"`))
	tu.OK(strings.Contains(got, `style="color:red"`))
}

func TestRejectsNilPredicate(t *testing.T) {
	tu := newAssert(t)
	// Expressions are compiled at Parse time, so the `?` is rejected when
	// the template is parsed, not when it is rendered.
	_, err := Parse("%option{ selected: data.selected_id.nil? }\n", "test.hml", nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "unexpected character"))
}

func TestParseCompilesExpressionsEagerly(t *testing.T) {
	tu := newAssert(t)
	// A syntactically invalid expression is rejected at Parse time,
	// before any Render call.
	_, err := Parse("= a +\n", "test.hml", nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "unexpected character"))
}

func TestTagAttributeInterpolation(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%a{ href: \"/items/#{id}\" }\n  link\n", map[string]any{"id": int64(42)})
	tu.OK(strings.Contains(got, `href="/items/42"`))
}

func TestBooleanAttribute(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%input{ type: \"checkbox\", checked: true }\n", nil)
	tu.OK(strings.Contains(got, " checked"))
	tu.OK(!strings.Contains(got, `checked="`))
}

func TestFalseAttributeOmitted(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%input{ type: \"text\", disabled: false }\n", nil)
	tu.OK(!strings.Contains(got, "disabled"))
}

func TestVoidElement(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%br\n", nil)
	tu.OK(got == "<br>\n")
}

func TestEmptyNonVoidTag(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%span\n", nil)
	tu.OK(got == "<span></span>\n")
}

func TestEscapedOutput(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "= name\n", map[string]any{"name": "<script>alert(1)</script>"})
	tu.OK(strings.Contains(got, "&lt;script&gt;"))
}

func TestRawOutputRejected(t *testing.T) {
	tu := newAssert(t)
	_, err := Parse("!= html\n", "test.hml", nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "raw output (!=) is not supported"))
}

func TestEscapedOutputSafeString(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "= meta\n", map[string]any{"meta": SafeString("<meta name=\"csrf-token\" content=\"x\" />")})
	tu.OK(got == "<meta name=\"csrf-token\" content=\"x\" />\n")
}

func TestIfTrue(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "- if show\n  visible\n", map[string]any{"show": true})
	tu.OK(got == "visible\n")
}

func TestIfFalse(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "- if show\n  visible\n", map[string]any{"show": false})
	tu.OK(got == "")
}

func TestIfElse(t *testing.T) {
	tu := newAssert(t)
	src := "- if show\n  yes\n- else\n  no\n"
	got := mustRender(t, src, map[string]any{"show": false})
	tu.OK(got == "no\n")
}

func TestIfElsif(t *testing.T) {
	tu := newAssert(t)
	src := "- if a\n  first\n- elsif b\n  second\n- else\n  third\n"
	got := mustRender(t, src, map[string]any{"a": false, "b": true})
	tu.OK(got == "second\n")
}

func TestForLoop(t *testing.T) {
	tu := newAssert(t)
	src := "- for item in items\n  = item.name\n"
	items := []any{
		map[string]any{"name": "Alice"},
		map[string]any{"name": "Bob"},
	}
	got := mustRender(t, src, map[string]any{"items": items})
	tu.OK(got == "Alice\nBob\n")
}

func TestForLoopWithIndex(t *testing.T) {
	tu := newAssert(t)
	src := "- for i, item in items\n  = i\n  = item.name\n"
	items := []any{
		map[string]any{"name": "Alice"},
		map[string]any{"name": "Bob"},
	}
	got := mustRender(t, src, map[string]any{"items": items})
	tu.OK(got == "0\nAlice\n1\nBob\n")
}

func TestEachLoopRejected(t *testing.T) {
	tu := newAssert(t)
	_, err := Parse("- items.each do |item|\n  = item.name\n", "test.hml", nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "unsupported control"))
}

func TestRenderPartial(t *testing.T) {
	tu := newAssert(t)
	src := "= render \"header\", title: \"Hello\"\n"
	partialFn := func(name string, ctx *Context) (string, error) {
		tu.OK(name == "header")
		title, _ := ctx.lookup("title")
		return "<h1>" + title.(string) + "</h1>\n", nil
	}
	got := mustRenderPartial(t, src, nil, partialFn)
	tu.OK(got == "<h1>Hello</h1>\n")
}

func TestPartialInheritsParentLocalsAndChildOverrides(t *testing.T) {
	tu := newAssert(t)
	inner := mustParse(t, "= greeting\n= name\n")
	src := "= render \"inner\", name: \"child\"\n"
	partialFn := func(name string, ctx *Context) (string, error) {
		return inner.RenderContext(ctx, nil)
	}
	// greeting is inherited from the parent through the context chain;
	// name is shadowed by the child render arg.
	got := mustRenderPartial(t, src, map[string]any{"greeting": "hi", "name": "parent"}, partialFn)
	tu.OK(got == "hi\nchild\n")
}

func TestJavascriptFilter(t *testing.T) {
	tu := newAssert(t)
	src := ":javascript\n  alert('hi');\n"
	got := mustRender(t, src, nil)
	tu.OK(got == "<script>\nalert('hi');\n</script>\n")
}

func TestCSSFilter(t *testing.T) {
	tu := newAssert(t)
	src := ":css\n  body { color: red; }\n"
	got := mustRender(t, src, nil)
	tu.OK(got == "<style>\nbody { color: red; }\n</style>\n")
}

func TestTextInterpolation(t *testing.T) {
	tu := newAssert(t)
	src := "%p\n  Hello #{name}!\n"
	got := mustRender(t, src, map[string]any{"name": "World"})
	tu.OK(strings.Contains(got, "Hello World!"))
}

func TestTextInterpolationEscapes(t *testing.T) {
	tu := newAssert(t)
	src := "%p\n  Hello #{name}!\n"
	got := mustRender(t, src, map[string]any{"name": "<b>"})
	tu.OK(strings.Contains(got, "Hello &lt;b&gt;!"))
}

func TestStringComparison(t *testing.T) {
	tu := newAssert(t)
	src := "- if status == \"active\"\n  yes\n"
	got := mustRender(t, src, map[string]any{"status": "active"})
	tu.OK(got == "yes\n")
}

func TestBooleanAnd(t *testing.T) {
	tu := newAssert(t)
	src := "- if a && b\n  both\n"
	got := mustRender(t, src, map[string]any{"a": true, "b": true})
	tu.OK(got == "both\n")
}

func TestBooleanNot(t *testing.T) {
	tu := newAssert(t)
	src := "- if !hidden\n  shown\n"
	got := mustRender(t, src, map[string]any{"hidden": false})
	tu.OK(got == "shown\n")
}

func TestNestedFieldAccess(t *testing.T) {
	tu := newAssert(t)
	src := "= data.user.name\n"
	locals := map[string]any{
		"data": map[string]any{
			"user": map[string]any{
				"name": "Alice",
			},
		},
	}
	got := mustRender(t, src, locals)
	tu.OK(got == "Alice\n")
}

func TestPreserveElement(t *testing.T) {
	tu := newAssert(t)
	src := "%textarea\n  hello\n"
	got := mustRender(t, src, nil)
	tu.OK(got == "<textarea>hello</textarea>\n")
}

func TestPreserveElementKeepsContentWhitespace(t *testing.T) {
	tu := newAssert(t)
	// A patch's leading spaces are the alignment a reader reads it by,
	// so a pre keeps them and drops only the newline the renderer added.
	src := "%pre\n  = patch\n"
	patch := SafeString("@@ -1,2 +1,2 @@\n context\n-old\n+new\n")
	got := mustRender(t, src, map[string]any{"patch": patch})
	tu.OK(got == "<pre>@@ -1,2 +1,2 @@\n context\n-old\n+new\n</pre>\n")
}

func TestMultiLineAttributes(t *testing.T) {
	tu := newAssert(t)
	src := "%a{ href: \"/home\",\n  class: \"link\" }\n  click\n"
	got := mustRender(t, src, nil)
	tu.OK(strings.Contains(got, `class="link"`))
	tu.OK(strings.Contains(got, `href="/home"`))
}

func TestAttributesSortAlphabetically(t *testing.T) {
	tu := newAssert(t)
	src := "%a{ style: \"x\", href: \"/\" }\n  text\n"
	got := mustRender(t, src, nil)
	hrefIdx := strings.Index(got, "href")
	styleIdx := strings.Index(got, "style")
	tu.OK(hrefIdx <= styleIdx)
}

func TestUndefinedVariable(t *testing.T) {
	tu := newAssert(t)
	tmpl := mustParse(t, "= missing\n")
	_, err := tmpl.Render(nil, nil)
	tu.OK(err != nil)
}

// fieldHolder stands in for any struct an app hands a template: a
// field the template may read and one it may not.
type fieldHolder struct {
	Name string
	vars map[string]any
}

func TestUnexportedFieldIsRenderError(t *testing.T) {
	tu := newAssert(t)
	tmpl := mustParse(t, "= data.vars\n")
	_, err := tmpl.Render(map[string]any{"data": fieldHolder{vars: map[string]any{}}}, nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "undefined field"))
}

func TestExportedFieldStillReadable(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "= data.name\n", map[string]any{"data": fieldHolder{Name: "Alice"}})
	tu.OK(got == "Alice\n")
}

func TestMailReminderTemplate(t *testing.T) {
	tu := newAssert(t)
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
	tu.OK(strings.Contains(got, "<h1>Acme</h1>"))
	tu.OK(strings.Contains(got, "<p>Follow up</p>"))
	tu.OK(strings.Contains(got, `href="https://example.com/done"`))
	tu.OK(strings.Contains(got, "Early status"))
}

func TestEqualityTypeSafety(t *testing.T) {
	tu :=
		// String "42" != integer 42 (no cross-type coercion)
		newAssert(t)

	src := "- if val == \"42\"\n  match\n"
	got := mustRender(t, src, map[string]any{"val": int64(42)})
	tu.OK(got == "")

	// Same types compare correctly
	got = mustRender(t, src, map[string]any{"val": "42"})
	tu.OK(got == "match\n")

	// nil == nil
	src = "- if val == nil\n  null\n"
	got = mustRender(t, src, map[string]any{"val": nil})
	tu.OK(got == "null\n")
}

func TestPartialLocalsPreserveTypes(t *testing.T) {
	tu := newAssert(t)
	src := "= render \"widget\", count: 5, active: true, label: \"hi\"\n"
	var received *Context
	partialFn := func(name string, ctx *Context) (string, error) {
		received = ctx
		return "", nil
	}
	mustRenderPartial(t, src, nil, partialFn)
	count, _ := received.lookup("count")
	vCount, okCount := count.(int64)
	tu.OK(okCount && vCount == 5)
	active, _ := received.lookup("active")
	vActive, okActive := active.(bool)
	tu.OK(okActive && vActive)
	label, _ := received.lookup("label")
	vLabel, okLabel := label.(string)
	tu.OK(okLabel && vLabel == "hi")
}

func TestNilOutputRendersEmpty(t *testing.T) {
	tu :=
		// = with nil should render empty string, not "<nil>"
		newAssert(t)

	got := mustRender(t, "= val\n", map[string]any{"val": nil})
	tu.OK(got == "\n")
}

func TestNilInterpolationRendersEmpty(t *testing.T) {
	tu := newAssert(t)
	// #{nil} renders empty, matching = output (not "<nil>").
	got := mustRender(t, "%p\n  Hello #{val}!\n", map[string]any{"val": nil})
	tu.OK(strings.Contains(got, "Hello !"))
	tu.OK(!strings.Contains(got, "nil"))
}

func TestNilInterpolationInStringLiteralRendersEmpty(t *testing.T) {
	tu := newAssert(t)
	// nil inside a "#{}" string expression renders empty, consistent with
	// text interpolation and = output.
	got := mustRender(t, "%a{ href: \"/co/#{val}\" }\n  x\n", map[string]any{"val": nil})
	tu.OK(strings.Contains(got, `href="/co/"`))
	tu.OK(!strings.Contains(got, "nil"))
}

func TestTypedNilPointerIsFalsy(t *testing.T) {
	tu := newAssert(t)
	src := "- if val\n  truthy\n- else\n  falsy\n"

	// nil pointer
	var p *string = nil
	tu.OK(mustRender(t, src, map[string]any{"val": p}) == "falsy\n")

	// nil slice
	var s []string = nil
	tu.OK(mustRender(t, src, map[string]any{"val": s}) == "falsy\n")

	// nil map
	var m map[string]any = nil
	tu.OK(mustRender(t, src, map[string]any{"val": m}) == "falsy\n")

	// nil func
	var f func() = nil
	tu.OK(mustRender(t, src, map[string]any{"val": f}) == "falsy\n")
}

func TestTypedSliceFor(t *testing.T) {
	tu :=
		// []string should work in for loops without pre-boxing to []any
		newAssert(t)

	src := "- for item in items\n  = item\n"
	got := mustRender(t, src, map[string]any{"items": []string{"a", "b"}})
	tu.OK(got == "a\nb\n")
}

func TestElseIfRejected(t *testing.T) {
	tu := newAssert(t)
	src := "- if a\n  yes\n- else if b\n  no\n"
	_, err := Parse(src, "test.hml", nil)
	tu.OK(err != nil)
}

func TestSplatBooleanAttributes(t *testing.T) {
	tu := newAssert(t)
	src := "%input{ **attrs }\n"
	locals := map[string]any{
		"attrs": map[string]any{
			"type":     "checkbox",
			"checked":  true,
			"disabled": false,
		},
	}
	got := mustRender(t, src, locals)
	tu.OK(strings.Contains(got, " checked"))
	tu.OK(!strings.Contains(got, `checked="`))
	tu.OK(!strings.Contains(got, "disabled"))
}

// Bare calls (= fn "arg", key: val) remain for the do_react/
// react_select shims. Parenthesized calls work too, and mean the same
// thing in output and attribute position; see TestCallResolvesHelper.
func TestBareFunctionCallWithKeywordArgsSupported(t *testing.T) {
	tu := newAssert(t)
	tmpl := mustParse(t, "= fn \"Widget\", id: \"abc\", active: true\n")
	got, err := tmpl.Render(map[string]any{
		"fn": func(args ...any) (any, error) {
			kind := fmt.Sprintf("%v", args[0])
			props := args[1].(map[string]any)
			return fmt.Sprintf("%s:%s:%v", kind, props["id"], props["active"]), nil
		},
	}, nil)
	tu.OK(err == nil)
	tu.OK(got == "Widget:abc:true\n")
}

func TestEntityHeaderTemplate(t *testing.T) {
	tu := newAssert(t)
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
	tu.OK(strings.Contains(got, "Acme Corp"))
	tu.OK(strings.Contains(got, "Jane Doe"))
	tu.OK(strings.Contains(got, "border-radius:4px"))
}

// #ZgotmplZ is a sentinel, not a link target: it marks a URL the
// renderer refused, and matches what html/template emits.
func TestURLAttributeRejectsUnsafeScheme(t *testing.T) {
	tu := newAssert(t)
	for _, url := range []string{
		"javascript:alert(1)",
		"JaVaScRiPt:alert(1)",
		"java\tscript:alert(1)",
		" javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"vbscript:msgbox",
	} {
		got := mustRender(t, "%a{ href: u }\n  x\n", map[string]any{"u": url})
		tu.OK(strings.Contains(got, `href="#ZgotmplZ"`))
		tu.OK(!strings.Contains(got, "alert"))
	}

	// Every URL attribute, not just href.
	got := mustRender(t, "%img{ src: u }\n", map[string]any{"u": "javascript:alert(1)"})
	tu.OK(strings.Contains(got, `src="#ZgotmplZ"`))
}

func TestURLAttributeAllowsRelativeAndAllowlistedSchemes(t *testing.T) {
	tu := newAssert(t)
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
		tu.OK(strings.Contains(got, `href="`+url+`"`))
	}
}

// An on* attribute is a JavaScript context. A literal is application
// source and passes; data is not code, so it needs SafeJS.
func TestJSAttributeRequiresLiteralOrSafeJS(t *testing.T) {
	tu := newAssert(t)

	got := mustRender(t, `%button{ onclick: "APP.close()" }`+"\n  x\n", nil)
	tu.OK(strings.Contains(got, `onclick="APP.close()"`))

	tmpl := mustParse(t, "%button{ onclick: handler }\n  x\n")
	_, err := tmpl.Render(map[string]any{"handler": "APP.close()"}, nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "SafeJS"))

	got, err = tmpl.Render(map[string]any{"handler": SafeJS("APP.close()")}, nil)
	tu.OK(err == nil)
	tu.OK(strings.Contains(got, `onclick="APP.close()"`))

	// Interpolation is not a literal, however much of it is authored.
	tmpl = mustParse(t, "%img{ onerror: \"hide(#{id})\" }\n")
	_, err = tmpl.Render(map[string]any{"id": int64(1)}, nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "SafeJS"))
}

func TestStyleAttributeRequiresLiteralOrSafeCSS(t *testing.T) {
	tu := newAssert(t)

	got := mustRender(t, `%p{ style: "white-space: pre-wrap" }`+"\n  x\n", nil)
	tu.OK(strings.Contains(got, `style="white-space: pre-wrap"`))

	tmpl := mustParse(t, "%p{ style: css }\n  x\n")
	_, err := tmpl.Render(map[string]any{"css": "width:50%"}, nil)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "SafeCSS"))

	got, err = tmpl.Render(map[string]any{"css": SafeCSS("width:50%")}, nil)
	tu.OK(err == nil)
	tu.OK(strings.Contains(got, `style="width:50%"`))
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
	tu := newAssert(t)
	tmpl, err := Parse("= foobar(row.x)\n", "test.hml", testTransforms)
	tu.OK(err == nil)
	_, err = tmpl.Render(map[string]any{"row": map[string]any{"x": "v"}}, nil)
	tu.OK(err != nil)
}

// One syntax, one meaning, wherever it appears: a parenthesized call on
// an injected helper reads the same in output and in an attribute. The
// two disagreed before -- output rejected the name as an unknown
// transform while the attribute path called it.
func TestCallResolvesHelper(t *testing.T) {
	tu := newAssert(t)
	locals := map[string]any{
		"c": map[string]any{"id": "CI-1", "sha": "abcdef"},
		"url": func(args ...any) (any, error) {
			if len(args) == 2 {
				return fmt.Sprintf("/change/%v/%v", args[0], args[1]), nil
			}
			return fmt.Sprintf("/change/%v", args[0]), nil
		},
	}
	tu.OK(mustRender(t, "= url(c.id)\n", locals) == "/change/CI-1\n")
	// More than one argument, which a transform cannot take.
	tu.OK(mustRender(t, "= url(c.id, c.sha)\n", locals) == "/change/CI-1/abcdef\n")
	got := mustRender(t, "%a{ href: url(c.id) }\n  x\n", locals)
	tu.OK(strings.Contains(got, `href="/change/CI-1"`))
}

// A registered name still means the transform, and a helper of the same
// name does not shadow it.
func TestTransformWinsOverLocal(t *testing.T) {
	tu := newAssert(t)
	transforms := map[string]Transform{
		"markdown": func(s string) string { return "<p>" + s + "</p>" },
	}
	tmpl, err := Parse("= markdown(row.x)\n", "test.hml", transforms)
	tu.OK(err == nil)
	got, err := tmpl.Render(map[string]any{
		"row":      map[string]any{"x": "hi"},
		"markdown": func(args ...any) (any, error) { return "local", nil },
	}, nil)
	tu.OK(err == nil)
	// The transform's output, emitted unescaped as transforms are.
	tu.OK(got == "<p>hi</p>\n")
}

func TestTransformRejectsNonFieldArguments(t *testing.T) {
	tu := newAssert(t)
	for _, src := range []string{
		`= markdown("literal")`,
		"= markdown(row.x, row.y)",
		"= markdown(markdown(row.x))",
		"= markdown(#{row.x})",
	} {
		_, err := Parse(src+"\n", "test.hml", testTransforms)
		tu.OK(err != nil)
		tu.OK(strings.Contains(err.Error(), "single field access"))
	}
}

func TestNilClassAttrOmitted(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%a.x{ class: active }\n", map[string]any{"active": nil})
	tu.OK(strings.Contains(got, `class="x"`))
	tu.OK(!strings.Contains(got, `class="x `))
}
func TestClassAttrMergesWhenPresent(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%a.x{ class: active }\n", map[string]any{"active": "active"})
	tu.OK(strings.Contains(got, `class="x active"`))
}

func TestFloat64IntegerFormatting(t *testing.T) {
	tu := newAssert(t)
	// float64 with no fractional part should render as a clean integer, not scientific notation
	got := mustRender(t, "%a{ href: \"/people/merge_form?id=#{id}\" }\n", map[string]any{"id": float64(1173828)})
	tu.OK(strings.Contains(got, `href="/people/merge_form?id=1173828"`))

	// float64 with a fractional part should still render with decimals/scientific notation as appropriate
	gotDecimal := mustRender(t, "= val\n", map[string]any{"val": float64(1173828.5)})
	tu.OK(gotDecimal == "1.1738285e+06\n" || gotDecimal == "1173828.5\n")
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
	tu := newAssert(t)
	src := "%a{ href: link, **attrs }\n" +
		"  text #{caption} more\n" +
		"- if flag\n" +
		"  = title\n" +
		"- elsif other\n" +
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
	tu.OK(slices.Equal(tmpl.Names(), want))
}

func TestNamesExcludesLoopVariables(t *testing.T) {
	tu := newAssert(t)
	src := "- for i, item in items\n" +
		"  = item.name\n" +
		"  = i\n" +
		"  = outer\n"
	tmpl := mustParse(t, src)
	tu.OK(slices.Equal(tmpl.Names(), []string{"items", "outer"}))
}

func TestNamesLoopVariableBindingIsScoped(t *testing.T) {
	tu := newAssert(t)
	// item is bound only inside the loop body; the same name read after
	// the loop is free.
	src := "- for item in items\n" +
		"  = item\n" +
		"= item\n"
	tmpl := mustParse(t, src)
	tu.OK(slices.Equal(tmpl.Names(), []string{"item", "items"}))
}

func TestNamesExcludesRenderArgKeys(t *testing.T) {
	tu := newAssert(t)
	// user is a name the partial reads, not one the caller supplies.
	tmpl := mustParse(t, "= render \"card\", user: person\n")
	tu.OK(slices.Equal(tmpl.Names(), []string{"person"}))
}

func TestNamesSortedAndDeduplicated(t *testing.T) {
	tu := newAssert(t)
	src := "= zebra\n= apple\n= zebra\n= apple.core\n"
	tmpl := mustParse(t, src)
	tu.OK(slices.Equal(tmpl.Names(), []string{"apple", "zebra"}))
}

func TestNamesIncludesHelperCallsAndNestedValues(t *testing.T) {
	tu := newAssert(t)
	src := "= helper(a, b.c)\n" +
		"%p{ data: { k: d }, list: [e] }\n" +
		"- if !f && g == \"x\"\n" +
		"  = \"lit #{h}\"\n"
	tmpl := mustParse(t, src)
	want := []string{"a", "b", "d", "e", "f", "g", "h", "helper"}
	tu.OK(slices.Equal(tmpl.Names(), want))
}

func TestRendersListsLiteralPartialNames(t *testing.T) {
	tu := newAssert(t)
	src := "= render \"b\"\n" +
		"= render \"a\", k: v\n" +
		"= render \"b\"\n"
	tmpl := mustParse(t, src)
	tu.OK(slices.Equal(tmpl.Renders(), []string{"a", "b"}))
	tu.OK(slices.Equal(tmpl.Names(), []string{"v"}))
}

func TestRendersOmitsComputedNames(t *testing.T) {
	tu := newAssert(t)
	src := "= render partial\n" +
		"= render \"cards/#{kind}\"\n" +
		"= render \"plain\"\n"
	tmpl := mustParse(t, src)
	tu.OK(slices.Equal(tmpl.Renders(), []string{"plain"}))
	tu.OK(slices.Equal(tmpl.Names(), []string{"kind", "partial"}))
}

func TestNamesDoesNotFollowPartials(t *testing.T) {
	tu := newAssert(t)
	tmpl := mustParse(t, "= render \"card\"\n")
	tu.OK(len(tmpl.Names()) == 0)
	tu.OK(slices.Equal(tmpl.Renders(), []string{"card"}))
}
