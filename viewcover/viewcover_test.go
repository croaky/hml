package viewcover

import (
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/croaky/hml"

	"github.com/croaky/is"
)

// A view prints once however many times it renders, from however many
// goroutines, and an off tracer prints nothing.
func TestTracePrintsEachPathOnce(t *testing.T) {
	is := is.New(t)
	var out strings.Builder
	tr := &tracer{on: true, w: &out}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			tr.trace("ui/views/a.hml")
			tr.trace("ui/views/b.hml")
			tr.trace("")
		})
	}
	wg.Wait()

	got := strings.Split(strings.TrimSpace(out.String()), "\n")
	is.Eq(len(got), 2)
	rendered, err := ParseTrace(strings.NewReader(out.String()))
	is.NoErr(err)
	is.Eq(rendered, map[string]bool{"ui/views/a.hml": true, "ui/views/b.hml": true})
}

func TestTraceOffPrintsNothing(t *testing.T) {
	is := is.New(t)
	var out strings.Builder
	tr := &tracer{on: false, w: &out}
	tr.trace("ui/views/a.hml")
	is.Eq(out.String(), "")
}

// Conditional answers from the parsed tree, so a comment, a loop, and
// an output do not count, and a file that is not .hml is not read.
func TestConditional(t *testing.T) {
	is := is.New(t)
	fsys := fstest.MapFS{
		"views/a/plain.hml":   {Data: []byte("%p\n  Hello\n")},
		"views/a/cond.hml":    {Data: []byte("%p\n  - if data.has_rows\n    %span\n")},
		"views/a/chain.hml":   {Data: []byte("- if a\n  %p\n- else if n > 0\n  %p\n")},
		"views/a/loop.hml":    {Data: []byte("- for row in rows\n  = row.name\n")},
		"views/a/notes.md":    {Data: []byte("- if this is prose, not a view\n")},
		"views/a/comment.hml": {Data: []byte("-# if data.has_rows\n")},
		"views/a/rich.hml":    {Data: []byte("- if body != \"\"\n  = markdown(body)\n")},
		"other/cond.hml":      {Data: []byte("- if outside\n  %p\n")},
	}
	transforms := map[string]hml.Transform{"markdown": func(s string) string { return s }}

	got, err := Conditional(fsys, "views", transforms)
	is.NoErr(err)
	is.Eq(got, []string{
		"views/a/chain.hml",
		"views/a/cond.hml",
		"views/a/rich.hml",
	})
}

// A view that does not parse is an error, not a view left out of the
// list: left out, it would pass the check it most needs.
func TestConditionalRefusesUnparsableView(t *testing.T) {
	is := is.New(t)
	fsys := fstest.MapFS{
		"views/bad.hml": {Data: []byte("- if\n  %p\n")},
	}
	_, err := Conditional(fsys, "views", nil)
	is.HasErr(err)
}

// The parser reads `go test -v` output, so most of what it sees is not
// a trace line, and a subtest indents the ones that are.
func TestParseTrace(t *testing.T) {
	is := is.New(t)
	out := strings.Join([]string{
		"=== RUN   TestIndex",
		"hmltrace: views/a.hml",
		"    hmltrace: views/b.hml",
		"hmltrace: views/a.hml",
		"    handler_test.go:12: hmltrace: not a trace line",
		"--- PASS: TestIndex (0.03s)",
		"ok  \tapp/www\t(cached)",
	}, "\n")

	got, err := ParseTrace(strings.NewReader(out))
	is.NoErr(err)
	is.Eq(got, map[string]bool{
		"views/a.hml": true,
		"views/b.hml": true,
	})
}

func TestReport(t *testing.T) {
	is := is.New(t)
	conditional := []string{"views/a.hml", "views/b.hml", "views/c.hml"}
	rendered := map[string]bool{"views/a.hml": true, "views/d.hml": true}

	missed, err := Report(conditional, rendered)
	is.NoErr(err)
	is.Eq(missed, []string{"views/b.hml", "views/c.hml"})
}

// An empty side means the walk or the trace broke. Reporting nothing
// would read as a pass.
func TestReportRefusesEmptyInput(t *testing.T) {
	is := is.New(t)

	_, err := Report(nil, map[string]bool{"views/a.hml": true})
	is.HasErr(err)

	_, err = Report([]string{"views/a.hml"}, nil)
	is.HasErr(err)
}
