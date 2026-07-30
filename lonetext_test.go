package hml

import "testing"

// A tag holding one run of text holds it on the tag's line, and a tag
// holding anything else does not.
//
// The case that prompted this is an anchor: content on its own line
// leaves a newline before the closing tag, HTML collapses that to a
// space, and inside a link the underline runs through it past the end of
// the word. Nothing else about the layout wanted that space.
func TestLoneTextChildRendersInline(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		locals map[string]any
		want   string
	}{
		{
			name: "anchor with text",
			src:  "%a{ href: \"/\" }\n  cibot",
			want: "<a href=\"/\">cibot</a>\n",
		},
		{
			name:   "output expression",
			src:    "%title\n  = title",
			locals: map[string]any{"title": "sign in"},
			want:   "<title>sign in</title>\n",
		},
		{
			name:   "interpolated text",
			src:    "%b\n  / #{status}",
			locals: map[string]any{"status": "merged"},
			want:   "<b>/ merged</b>\n",
		},
		{
			// Two lines are two lines the author separated. Joining
			// them would close a gap that is in the source on purpose.
			name: "two text children stay on their own lines",
			src:  "%b\n  one\n  two",
			want: "<b>\none\ntwo\n</b>\n",
		},
		{
			// A tag holding a tag is a block, and reads as one.
			name: "element child stays on its own line",
			src:  "%b\n  %i\n    x",
			want: "<b>\n<i>x</i>\n</b>\n",
		},
		{
			// The newline between siblings is what spaces words in a
			// row, and it is untouched.
			name: "siblings keep the newline between them",
			src:  "%nav\n  %a{ href: \"/\" }\n    cibot\n  %a{ href: \"/merged\" }\n    merged",
			want: "<nav>\n<a href=\"/\">cibot</a>\n<a href=\"/merged\">merged</a>\n</nav>\n",
		},
		{
			// A pre's whitespace is its content, and the leading spaces
			// that align a diff hunk survive.
			name:   "pre keeps its own leading whitespace",
			src:    "%pre\n  = patch",
			locals: map[string]any{"patch": "  aligned"},
			want:   "<pre>  aligned</pre>\n",
		},
		{
			name: "empty tag is unchanged",
			src:  "%span",
			want: "<span></span>\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := Parse(tc.src, "t.hml", nil)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, err := tmpl.Render(tc.locals, nil)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// A transform emits the HTML it was handed, and markdown of two
// paragraphs is two blocks. A tag holding several lines stays a block:
// the space they would have collapsed to sits between blocks, where
// nothing sees it, so there is nothing to buy with the shape.
func TestMultiLineTransformStaysBlock(t *testing.T) {
	transforms := map[string]Transform{
		"markdown": func(s string) string { return "<p>one</p>\n<p>two</p>" },
	}
	tmpl, err := Parse("%div\n  = markdown(body)", "t.hml", transforms)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := tmpl.Render(map[string]any{"body": "x"}, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if want := "<div>\n<p>one</p>\n<p>two</p>\n</div>\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A conditional brings its own lines, so a tag holding one is a block
// even when the branch taken is a single line of text. The alternative
// is markup whose shape depends on data, which is worse to read than a
// newline nobody sees.
func TestConditionalChildIsNotInlined(t *testing.T) {
	tmpl, err := Parse("%b\n  - if yes\n    x", "t.hml", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := tmpl.Render(map[string]any{"yes": true}, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if want := "<b>\nx\n</b>\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
