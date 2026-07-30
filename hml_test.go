package hml

import (
	"fmt"
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
	tu.OK(got == "<p>\nhello\n</p>\n")
}

func TestStaticText(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%p\n  hello world\n", nil)
	tu.OK(got == "<p>\nhello world\n</p>\n")
}

func TestTag(t *testing.T) {
	tu := newAssert(t)
	got := mustRender(t, "%div\n  %p\n    text\n", nil)
	tu.OK(got == "<div>\n<p>\ntext\n</p>\n</div>\n")
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

// Parenthesized call syntax (= name(field)) is reserved for the
// transform builtins; see TestTransformRejectsUnknownName. Bare calls
// (= fn "arg", key: val) remain for the do_react/react_select shims.
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
        %img{ src: image_url, width: "32", height: "32", style: "border-radius:#{image_radius};display:block;" }
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
		"url":            "https://example.com/co/1",
		"image_url":      "https://img.example.com/logo.png",
		"image_radius":   "4px",
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

// Transform rendering behavior (markdown/slack/search_highlight output)
// is tested app-side, where the sanitizing transforms
// live. The engine tests below cover only parse-time validation: the
// transform name against the registered set, and the single-field-access
// argument restriction.
func TestTransformRejectsUnknownName(t *testing.T) {
	tu := newAssert(t)
	_, err := Parse("= foobar(row.x)\n", "test.hml", testTransforms)
	tu.OK(err != nil)
	tu.OK(strings.Contains(err.Error(), "unknown transform"))
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
