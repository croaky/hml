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
