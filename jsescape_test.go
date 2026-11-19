package hml

import (
	"strings"
	"testing"
	"time"

	"github.com/croaky/is"
)

// renderJS renders body as one :javascript block and returns the
// script, without the script tags. It indents each line of body.
func renderJS(t *testing.T, body string, locals map[string]any) string {
	t.Helper()
	is := is.New(t)
	src := ":javascript\n  " + strings.ReplaceAll(body, "\n", "\n  ") + "\n"
	out, err := mustParse(t, src).Render(locals, nil)
	is.NoErr(err)
	out = strings.TrimPrefix(out, "<script>\n")
	return strings.TrimSuffix(out, "\n</script>\n")
}

// hostile holds each character that can end a JavaScript string, a
// template literal, or the script element.
const hostile = "a\"b'c`d\\e</script>f${g}\nh\u2028i&"

const hostileEscaped = `a\u0022b\u0027c\u0060d\u005ce\u003c/script\u003ef\u0024{g}\u000ah\u2028i\u0026`

func TestJavaScriptEscapesAStringByItsContext(t *testing.T) {
	is := is.New(t)
	v := map[string]any{"v": hostile}
	for _, c := range []struct{ src, want string }{
		{`x = "#{v}";`, `x = "` + hostileEscaped + `";`},
		{`x = 'p #{v} q';`, `x = 'p ` + hostileEscaped + ` q';`},
		{"x = `p #{v} q`;", "x = `p " + hostileEscaped + " q`;"},
		{`f(#{v});`, `f("` + hostileEscaped + `");`},
		{"x = `${#{v}}`;", "x = `${\"" + hostileEscaped + "\"}`;"},
		// An escaped quote does not end the string.
		{`x = "a\"#{v}";`, `x = "a\"` + hostileEscaped + `";`},
	} {
		is.Eq(renderJS(t, c.src, v), c.want)
	}
}

func TestJavaScriptWritesAValueOutsideAStringAsALiteral(t *testing.T) {
	is := is.New(t)
	locals := map[string]any{
		"n":    int64(42),
		"f":    1.5,
		"b":    true,
		"none": nil,
		"code": SafeJS(`{"a": 1}`),
		"m":    map[string]any{"k": "</script>"},
		"s":    []any{int64(1), "x"},
	}
	for _, c := range []struct{ src, want string }{
		{`f(#{n}, #{f}, #{b}, #{none});`, `f(42, 1.5, true, null);`},
		{`f(#{code});`, `f({"a": 1});`},
		{`f(#{m});`, `f({"k":"\u003c/script\u003e"});`},
		{`f(#{s});`, `f([1,"x"]);`},
		// A number after a value is a division, not a regex.
		{`x = #{n} / 2;`, `x = 42 / 2;`},
		// The value inside "..." is string content, so a number writes as text.
		{`x = "n=#{n}";`, `x = "n=42";`},
	} {
		is.Eq(renderJS(t, c.src, locals), c.want)
	}
}

func TestJavaScriptEscapesAJSONValueForTheScriptElement(t *testing.T) {
	is := is.New(t)
	locals := map[string]any{
		"m": map[string]any{"k": "</script>&\u2028\u2029"},
		"s": []string{"</script>&\u2028\u2029"},
	}
	const want = `"\u003c/script\u003e\u0026\u2028\u2029"`
	is.Eq(renderJS(t, `f(#{m});`, locals), `f({"k":`+want+`});`)
	is.Eq(renderJS(t, `f(#{s});`, locals), `f([`+want+`]);`)
}

func TestJavaScriptWritesAJSONValueAsEncodingJSONV1Did(t *testing.T) {
	is := is.New(t)
	locals := map[string]any{
		"keys":    map[string]int64{"b": 2, "a": 1, "B": 3, "c": 4},
		"nilList": []string(nil),
		"nilMap":  map[string]any(nil),
		"badUTF8": []string{"a\xffb"},
		"dur":     []time.Duration{1500 * time.Millisecond},
	}
	for _, c := range []struct{ src, want string }{
		// Map keys are in sorted order, so a page renders the same each time.
		{`f(#{keys});`, `f({"B":3,"a":1,"b":2,"c":4});`},
		{`f(#{nilList});`, `f(null);`},
		{`f(#{nilMap});`, `f(null);`},
		// v1 wrote the escape \ufffd. v2 writes the character. Both
		// give the same JavaScript string.
		{`f(#{badUTF8});`, "f([\"a\uFFFDb\"]);"},
		{`f(#{dur});`, `f([1500000000]);`},
	} {
		is.Eq(renderJS(t, c.src, locals), c.want)
	}
}

func TestJavaScriptStateCrossesLines(t *testing.T) {
	is := is.New(t)
	got := renderJS(t, "const s = `a\n#{v}`;\nf(#{v});", map[string]any{"v": `"`})
	is.Eq(got, "const s = `a\n\\u0022`;\nf(\"\\u0022\");")
}

func TestJavaScriptRefusesAValueItCannotPlace(t *testing.T) {
	is := is.New(t)
	for _, src := range []string{
		"// #{v}",
		"/* #{v} */",
		"/*\n  #{v} */",
		"x = /a#{v}/;",
		"x = /[#{v}]/;",
		"return /a#{v}/;",
	} {
		_, err := Parse(":javascript\n  "+strings.ReplaceAll(src, "\n", "\n  ")+"\n", "test.hml", nil)
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "test.hml"))
	}
	// A line comment ends at the end of its line.
	is.Eq(renderJS(t, "// note\nf(#{v});", map[string]any{"v": int64(1)}), "// note\nf(1);")
	// A slash after an operand is a division, and a value can follow it.
	is.Eq(renderJS(t, "x = a / #{v};", map[string]any{"v": int64(2)}), "x = a / 2;")
}

func TestCSSTakesOnlyANumberOrSafeCSS(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, ":css\n  .a { width: #{w}px; #{rule} }\n")
	got, err := tmpl.Render(map[string]any{"w": int64(3), "rule": SafeCSS("color: red;")}, nil)
	is.NoErr(err)
	is.Eq(got, "<style>\n.a { width: 3px; color: red; }\n</style>\n")
	_, err = tmpl.Render(map[string]any{"w": "3px} body{display:none", "rule": SafeCSS("")}, nil)
	is.HasErr(err)
}
