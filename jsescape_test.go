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

// wantJS is the script that renderJS returns for a block whose values
// are vals, the elements of the array. Each $H in body is the name of
// the array.
func wantJS(vals, body string) string {
	name := string(jsValuesName(vals))
	return "var " + name + " = [" + vals + "];\n" + strings.ReplaceAll(body, "$H", name)
}

// hostile holds each character that can end a JavaScript string, a
// template literal, or the script element.
const hostile = "a\"b'c`d\\e</script>f${g}\nh\u2028i&<!--"

const hostileEscaped = `"a\u0022b\u0027c\u0060d\u005ce\u003c/script\u003ef\u0024{g}\u000ah\u2028i\u0026\u003c!--"`

func TestJavaScriptWritesEachValueInAnArrayAtTheStart(t *testing.T) {
	is := is.New(t)
	v := map[string]any{"v": hostile}
	for _, c := range []struct{ src, want string }{
		// A quoted string that holds a value becomes a template literal.
		{`x = "#{v}";`, "x = `${$H[0]}`;"},
		{`x = 'p #{v} q';`, "x = `p ${$H[0]} q`;"},
		{"x = `p #{v} q`;", "x = `p ${$H[0]} q`;"},
		{`f(#{v});`, "f($H[0]);"},
		{"x = `${#{v}}`;", "x = `${$H[0]}`;"},
		// An escaped quote does not end the string.
		{`x = "a\"#{v}";`, "x = `a\\\"${$H[0]}`;"},
	} {
		is.Eq(renderJS(t, c.src, v), wantJS(hostileEscaped, c.want))
	}
	// A block with no value has no array.
	is.Eq(renderJS(t, `f("a");`, nil), `f("a");`)
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
	for _, c := range []struct{ src, vals, want string }{
		{`f(#{n}, #{f}, #{b}, #{none});`, `42,1.5,true,null`, `f($H[0], $H[1], $H[2], $H[3]);`},
		{`f(#{code});`, `({"a": 1})`, `f($H[0]);`},
		{`f(#{m});`, `{"k":"\u003c/script\u003e"}`, `f($H[0]);`},
		{`f(#{s});`, `[1,"x"]`, `f($H[0]);`},
		// A slash after a value is a division, not a regex.
		{`x = #{n} / 2;`, `42`, `x = $H[0] / 2;`},
		// In a string, a value is its text: a number is "42", nil is "".
		{`x = "n=#{n}";`, `"42"`, "x = `n=${$H[0]}`;"},
		{`x = "#{none}";`, `""`, "x = `${$H[0]}`;"},
	} {
		is.Eq(renderJS(t, c.src, locals), wantJS(c.vals, c.want))
	}
}

func TestJavaScriptEscapesAJSONValueForTheScriptElement(t *testing.T) {
	is := is.New(t)
	locals := map[string]any{
		"m": map[string]any{"k": "</script>&\u2028\u2029"},
		"s": []string{"</script>&\u2028\u2029"},
	}
	const want = `"\u003c/script\u003e\u0026\u2028\u2029"`
	is.Eq(renderJS(t, `f(#{m});`, locals), wantJS(`{"k":`+want+`}`, `f($H[0]);`))
	is.Eq(renderJS(t, `f(#{s});`, locals), wantJS(`[`+want+`]`, `f($H[0]);`))
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
	for _, c := range []struct{ src, vals string }{
		// Map keys are in sorted order, so a page renders the same each time.
		{`f(#{keys});`, `{"B":3,"a":1,"b":2,"c":4}`},
		{`f(#{nilList});`, `null`},
		{`f(#{nilMap});`, `null`},
		// v1 wrote the escape \ufffd. v2 writes the character. Both
		// give the same JavaScript string.
		{`f(#{badUTF8});`, "[\"a\uFFFDb\"]"},
		{`f(#{dur});`, `[1500000000]`},
	} {
		is.Eq(renderJS(t, c.src, locals), wantJS(c.vals, `f($H[0]);`))
	}
}

func TestJavaScriptStateCrossesLines(t *testing.T) {
	is := is.New(t)
	got := renderJS(t, "const s = `a\n#{v}`;\nf(#{v});", map[string]any{"v": `"`})
	is.Eq(got, wantJS(`"\u0022","\u0022"`, "const s = `a\n${$H[0]}`;\nf($H[1]);"))
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
	is.Eq(renderJS(t, "// note\nf(#{v});", map[string]any{"v": int64(1)}), wantJS("1", "// note\nf($H[0]);"))
	// A slash after an operand is a division, and a value can follow it.
	is.Eq(renderJS(t, "x = a / #{v};", map[string]any{"v": int64(2)}), wantJS("2", "x = a / $H[0];"))
}

func TestJavaScriptRefusesALineThatEndsInsideAStringOrRegex(t *testing.T) {
	is := is.New(t)
	for _, src := range []string{
		"x = \"a\nb\";",
		"x = 'a\nb';",
		"x = \"a\\\\\nb\";",
		"x = /a\nb/;",
		"x = /[a\nb]/;",
		// The lexer reads the slash after ) as a division, so the
		// quote opens a string that the browser does not see.
		"if (ok) /'/.test(s);\nf(#{v});",
	} {
		_, err := Parse(":javascript\n  "+strings.ReplaceAll(src, "\n", "\n  ")+"\n", "test.hml", nil)
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "test.hml:2:"))
	}
}

func TestJavaScriptContinuesAStringOrTemplateAcrossLines(t *testing.T) {
	is := is.New(t)
	v := map[string]any{"v": "x"}
	is.Eq(renderJS(t, "var s = \"a\\\nb #{v}\";", v), wantJS(`"x"`, "var s = `a\\\nb ${$H[0]}`;"))
	is.Eq(renderJS(t, "var s = 'a\\\nb #{v}';", v), wantJS(`"x"`, "var s = `a\\\nb ${$H[0]}`;"))
	is.Eq(renderJS(t, "var s = `a\n#{v}`;", v), wantJS(`"x"`, "var s = `a\n${$H[0]}`;"))
	is.Eq(renderJS(t, "/* a\n' */ f(#{v});", v), wantJS(`"x"`, "/* a\n' */ f($H[0]);"))
}

func TestJavaScriptRewritesAQuotedStringThatHoldsAValue(t *testing.T) {
	is := is.New(t)
	v := map[string]any{"v": "x"}
	// A backtick and a ${ in the string stay text in the template literal.
	is.Eq(renderJS(t, "x = \"a`b${c}#{v}\";", v), wantJS(`"x"`, "x = `a\\`b\\${c}${$H[0]}`;"))
	// An escaped backtick stays one escape.
	is.Eq(renderJS(t, "x = \"a\\`#{v}\";", v), wantJS(`"x"`, "x = `a\\`${$H[0]}`;"))
	// The string stays one token, so a member access still applies to it.
	is.Eq(renderJS(t, `n = "a#{v}".length;`, v), wantJS(`"x"`, "n = `a${$H[0]}`.length;"))
	// Other strings on the line stay as they are.
	is.Eq(renderJS(t, `f("a", "#{v}", 'b');`, v), wantJS(`"x"`, "f(\"a\", `${$H[0]}`, 'b');"))
	// \0 alone is valid in a template literal.
	is.Eq(renderJS(t, `x = "\0#{v}";`, v), wantJS(`"x"`, "x = `\\0${$H[0]}`;"))
	// A template literal has no octal escape, so Parse refuses one in a
	// string that it must rewrite, and leaves one in any other string.
	for _, src := range []string{`x = "\1#{v}";`, `x = "\01#{v}";`, `x = '#{v}\7';`} {
		_, err := Parse(":javascript\n  "+src+"\n", "test.hml", nil)
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "octal"))
	}
	is.Eq(renderJS(t, `x = "\1";`, nil), `x = "\1";`)
}

func TestJavaScriptKeepsAValueOutOfTheSourceWhenTheLexerIsWrong(t *testing.T) {
	is := is.New(t)
	// The lexer reads each slash after ) as a division, so it reads the
	// text from the first quote to the second as a string. The browser
	// reads two regular expressions and a call in code. The value stays
	// in the array, and the call reads it by index. The script is then
	// wrong, but no value runs as code.
	got := renderJS(t, "if (a) /'/.test(s); f(#{v}); if (b) /'/.test(t);", map[string]any{"v": "alert(1)"})
	head, body, _ := strings.Cut(got, "\n")
	is.True(strings.Contains(head, `"alert(1)"`))
	is.True(!strings.Contains(body, "alert"))
}

func TestJavaScriptNamesTheArrayForItsValues(t *testing.T) {
	is := is.New(t)
	a := renderJS(t, "f(#{v});", map[string]any{"v": "a"})
	b := renderJS(t, "f(#{v});", map[string]any{"v": "b"})
	is.NotEq(a[:strings.Index(a, " = ")], b[:strings.Index(b, " = ")])
	is.Eq(renderJS(t, "f(#{v});", map[string]any{"v": "a"}), a)
}

// An end user whose data reaches two blocks can choose the values of
// both. A shared name then needs only a collision, not a second
// preimage. A collision of n bits costs about 2^(n/2) steps, so a
// name with 128 bits of the hash fell to a search of 2^64 steps. The
// name holds all 256 bits of the SHA-256 hash of `"a"`, so a
// collision costs 2^128 steps.
func TestJavaScriptNamesTheArrayWithTheFullSHA256Hash(t *testing.T) {
	is := is.New(t)
	is.Eq(string(jsValuesName(`"a"`)), "__hml_ac8d8342bbb2362d13f0a559a3621bb407011368895164b628a54f7fc33fc43c")
}

// An end user who controls a value in one block can pick it so that
// its array takes the name of another block's array. The second var
// then replaces the first, and the first block's later code reads the
// attacker's values. The two values here give the same 64-bit FNV-1a
// hash, the name hml once used. A Pollard rho search found them in
// 2^32 steps, so an attacker can find such a pair too.
func TestJavaScriptKeepsTwoBlocksApartWhenTheirValuesCollide(t *testing.T) {
	is := is.New(t)
	src := ":javascript\n  later(() => a(#{x}));\n:javascript\n  b(#{y});\n"
	out, err := mustParse(t, src).Render(map[string]any{"x": "594916704ce9bfe0", "y": "1deb96001e2b73d2"}, nil)
	is.NoErr(err)
	nameA := string(jsValuesName(`"594916704ce9bfe0"`))
	nameB := string(jsValuesName(`"1deb96001e2b73d2"`))
	is.NotEq(nameA, nameB)
	is.Eq(out, "<script>\nvar "+nameA+" = [\"594916704ce9bfe0\"];\nlater(() => a("+nameA+"[0]));\n</script>\n"+
		"<script>\nvar "+nameB+" = [\"1deb96001e2b73d2\"];\nb("+nameB+"[0]);\n</script>\n")
}

func TestJavaScriptReadsHTMLLikeCommentsAsComments(t *testing.T) {
	is := is.New(t)
	v := map[string]any{"v": "alert(1)"}
	for _, c := range []struct{ src, want string }{
		{"<!-- don't remove\ntrack(#{v});", "<!-- don't remove\ntrack($H[0]);"},
		{"a = 1; <!-- it's\ntrack(#{v});", "a = 1; <!-- it's\ntrack($H[0]);"},
		{"x = 1;\n--> it's legacy\ntrack(#{v});", "x = 1;\n--> it's legacy\ntrack($H[0]);"},
		{"/* a */ --> it's\ntrack(#{v});", "/* a */ --> it's\ntrack($H[0]);"},
		// After code on the line, --> is a decrement and a >.
		{"if (a--> 0) f(#{v});", "if (a--> 0) f($H[0]);"},
	} {
		is.Eq(renderJS(t, c.src, v), wantJS(`"alert(1)"`, c.want))
	}
	for _, src := range []string{"<!-- #{v}", "x = 1; <!-- #{v}", "--> #{v}"} {
		_, err := Parse(":javascript\n  "+src+"\n", "test.hml", nil)
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "in a comment"))
	}
}

func TestJavaScriptReadsASlashAfterAnOperandAsADivision(t *testing.T) {
	is := is.New(t)
	v := map[string]any{"v": "alert(1)"}
	for _, first := range []string{
		"var r = done++ / total;",
		"var r = done-- / total;",
		"var r = a[i]++ / total;",
		"var r = a.in / b;",
		"var r = stats.new / total;",
		"var r = a?.delete / b;",
		"var r = ++i / 2;",
	} {
		src := first + "\nvar url = \"/users/\" + #{v};"
		is.Eq(renderJS(t, src, v), wantJS(`"alert(1)"`, first+"\nvar url = \"/users/\" + $H[0];"))
	}
	// The same, on one line.
	is.Eq(renderJS(t, `r = n++ / 2; u = "/u/" + #{v};`, v), wantJS(`"alert(1)"`, `r = n++ / 2; u = "/u/" + $H[0];`))
	// A slash after a keyword still starts a regular expression.
	is.Eq(renderJS(t, "return /'/.test(s) && f(#{v});", v), wantJS(`"alert(1)"`, "return /'/.test(s) && f($H[0]);"))
	is.Eq(renderJS(t, "x = typeof /'/;\nf(#{v});", v), wantJS(`"alert(1)"`, "x = typeof /'/;\nf($H[0]);"))
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
