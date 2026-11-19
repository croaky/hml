package hml

import (
	"html"
	"strings"
	"testing"

	"github.com/croaky/is"
)

// writeEscapedHTML writes into the render buffer what escapeHTML
// returns. The escapes are the security boundary for text and
// attribute values, so each output must equal html.EscapeString.
func TestWriteEscapedHTMLMatchesEscapeString(t *testing.T) {
	is := is.New(t)
	for _, s := range []string{
		"",
		"plain",
		"&",
		`<script>alert("x")</script>`,
		`a & b < c > d " e ' f`,
		"&&<<>>\"\"''",
		"trailing <",
		"> leading",
		"naïve café & 日本 <b>",
		"line\nbreak & tab\t<",
		"&amp; already escaped",
		// Past escapeRunLimit, the html.EscapeString path runs.
		strings.Repeat(`a&b<c>d"e'f`, escapeRunLimit),
	} {
		var b strings.Builder
		writeEscapedHTML(&b, s)
		is.Eq(b.String(), html.EscapeString(s))
		is.Eq(b.String(), escapeHTML(s))
	}
}

// Output text in a tag, an interpolated value in text, and an attribute
// value from data each go through writeEscapedHTML.
func TestRenderEscapesEachPath(t *testing.T) {
	is := is.New(t)
	tmpl := mustParse(t, "%p{ title: v }\n  = v\n%p\n  a #{v} b\n")
	got, err := tmpl.Render(map[string]any{"v": `<"&'>`}, nil)
	is.NoErr(err)
	is.Eq(got, "<p title=\"&lt;&#34;&amp;&#39;&gt;\">&lt;&#34;&amp;&#39;&gt;</p>\n"+
		"<p>a &lt;&#34;&amp;&#39;&gt; b</p>\n")
}

// A script or style element runs its content as code, and the HTML
// escape does not stop code. So Parse refuses a value anywhere in the
// body of a %script or %style tag.
func TestParseRefusesAValueInAScriptOrStyleTagBody(t *testing.T) {
	is := is.New(t)
	for _, src := range []string{
		"%script\n  x = \"#{v}\";\n",
		"%script\n  = v\n",
		"%style\n  p { color: #{v} }\n",
		"%style\n  = v\n",
		"%script\n  = helper(v)\n",
		"%script\n  = markdown(v)\n",
		"%script\n  = render \"x\"\n",
		"%script\n  - if ok\n    f(\"a\");\n",
		"%script\n  - for x in xs\n    f(\"a\");\n",
		"%script\n  %b\n    a\n",
		"%SCRIPT\n  = v\n",
		"%Style\n  = v\n",
		"%svg\n  %script\n    = v\n",
		"%math\n  %style\n    a #{v}\n",
		"%div\n  - if ok\n    %script\n      = v\n",
	} {
		_, err := Parse(src, "test.hml", testTransforms)
		is.HasErr(err)
		is.True(strings.Contains(err.Error(), "test.hml:"))
		is.True(strings.Contains(err.Error(), "literal text"))
	}
}

func TestScriptOrStyleTagRendersLiteralTextAndAttributes(t *testing.T) {
	is := is.New(t)
	got := mustRender(t, "%script\n  -# a note\n  f(\"a\");\n  g(1);\n%style\n  p { color: red }\n%script{ src: url, defer: true }\n", map[string]any{"url": "/app.js"})
	is.Eq(got, "<script>\nf(\"a\");\ng(1);\n</script>\n<style>p { color: red }</style>\n<script defer src=\"/app.js\"></script>\n")
	// A data-* attribute carries a value to a script, which reads it
	// from document.currentScript.dataset.
	got = mustRender(t, "%script{ \"data-id\": id }\n  f(document.currentScript.dataset.id);\n", map[string]any{"id": "</script>"})
	is.Eq(got, "<script data-id=\"&lt;/script&gt;\">f(document.currentScript.dataset.id);</script>\n")
}
