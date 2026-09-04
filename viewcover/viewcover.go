// Package viewcover finds the views that hold a condition and that no
// test renders.
//
// # Why
//
// Parse checks a condition's syntax and nothing about its type. The type
// is the handler's to know, so a non-bool in an `- if` is a render
// error. A view that no test renders is therefore a view that nothing
// type-checks. It drifts until a reader copies it as a pattern, or a
// handler renders it in front of a user. A check that every view parses
// proves the view is well-formed and nothing more.
//
// # How
//
// The app calls Trace with the view's path each time it resolves a view.
// When HML_TRACE is set, Trace prints the path to stdout, once per
// process, under the Prefix. A test run made with HML_TRACE=1 and -v
// therefore carries one line per view the run reached. The viewcover
// command reads that run, parses each .hml file under a directory, keeps
// the ones where Template.HasCondition is true, and reports the ones the
// trace does not name.
//
// Stdout rather than a file, because it makes the run cacheable. Go's
// test cache records what a test binary printed and replays it on a hit,
// and it keys on the environment variables the binary read. So an
// unchanged package replays its trace lines without running, and a run
// with HML_TRACE unset is a separate cache entry that neither traces nor
// evicts the traced results. A file the renders append to is a side
// effect the cache does not know about: a hit would produce no trace and
// the check would report every view as missed. -v is what makes testing
// show a passing test's stdout.
//
// A rendered view is a view something reached, not a view whose every
// branch was taken. An `- if` inside a `- for` over an empty slice
// counts as covered. Reaching the view is what the check buys.
//
// There is no allowlist. A view that no test renders needs a test, and a
// view that is unreachable during a refactor is better deleted: nothing
// renders it, so nothing checks it, and it will be wrong by the time it
// comes back.
package viewcover

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/croaky/hml"
)

// Env is the environment variable that turns Trace on. Any non-empty
// value does.
const Env = "HML_TRACE"

// Prefix marks a trace line in test output. ParseTrace picks out the
// lines that start with it, after any indentation testing added.
const Prefix = "hmltrace: "

// tracer is the state behind Trace: whether it is on, where it writes,
// and which paths it has written. A value rather than package globals
// so a test can build one that writes to a buffer.
type tracer struct {
	on   bool
	w    io.Writer
	mu   sync.Mutex
	seen sync.Map // map[string]struct{}
}

func (t *tracer) trace(path string) {
	if !t.on || path == "" {
		return
	}
	if _, dup := t.seen.LoadOrStore(path, struct{}{}); dup {
		return
	}
	// One line at a time: renders run concurrently, and testing
	// interleaves stdout with its own output.
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprintln(t.w, Prefix+path)
}

var (
	stdOnce sync.Once
	std     *tracer
)

// Trace records that the app resolved the view at path. It prints
// Prefix and the path to stdout when Env is set, and prints each path
// once per process, so the app can call it on every render. A caller
// with a template cache can call it on the miss path instead; the
// result is the same.
//
// The path is the one the viewcover command compares against: the same
// form its -views walk produces, such as "ui/views/companies/edit.hml"
// for -views ui/views.
func Trace(path string) {
	stdOnce.Do(func() {
		std = &tracer{on: os.Getenv(Env) != "", w: os.Stdout}
	})
	std.trace(path)
}

// Conditional returns the .hml files under root in fsys whose template
// holds a condition, as sorted slash paths that start with root. It
// parses each file with transforms, so a file that does not parse is an
// error here rather than a view silently left out.
func Conditional(fsys fs.FS, root string, transforms map[string]hml.Transform) ([]string, error) {
	var out []string
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".hml") {
			return nil
		}
		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		tmpl, err := hml.Parse(string(src), path, transforms)
		if err != nil {
			return err
		}
		if tmpl.HasCondition() {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(out)
	return out, nil
}

// ParseTrace returns the set of paths a traced test run printed. The
// run also carries everything else `go test -v` prints, and a subtest's
// output is indented, so it takes the lines whose trimmed form starts
// with Prefix.
func ParseTrace(r io.Reader) (map[string]bool, error) {
	out := map[string]bool{}
	s := bufio.NewScanner(r)
	// Test output lines are short, but one long line must not end the
	// scan and read as a pile of missed views.
	s.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		after, ok := strings.CutPrefix(line, Prefix)
		if !ok {
			continue
		}
		if after = strings.TrimSpace(after); after != "" {
			out[after] = true
		}
	}
	return out, s.Err()
}

// Report returns the conditional views that rendered does not name,
// sorted. It refuses an empty side: no conditional views means the walk
// found nothing, and no rendered views means the trace was not on, and
// reporting nothing in either case would read as a pass.
func Report(conditional []string, rendered map[string]bool) ([]string, error) {
	if len(conditional) == 0 {
		return nil, fmt.Errorf("no views with a condition")
	}
	if len(rendered) == 0 {
		return nil, fmt.Errorf("no views rendered; is %s set and the run -v?", Env)
	}
	var missed []string
	for _, v := range conditional {
		if !rendered[v] {
			missed = append(missed, v)
		}
	}
	slices.Sort(missed)
	return missed, nil
}
