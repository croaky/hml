package hml

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/croaky/is"
)

// Every tree-sitter corpus example claims to be hml, and no Checkfile
// job runs the tree-sitter CLI, so nothing else stops the grammar's
// examples from drifting into a language the engine does not have. This
// is the half that can run without the CLI: the engine parses each
// example. What the grammar makes of it is still verified by hand with
// `tree-sitter test`.
func TestCorpusParses(t *testing.T) {
	is := is.New(t)

	files, err := filepath.Glob("test/corpus/*.txt")
	is.NoErr(err)
	is.True(len(files) > 0)

	for _, f := range files {
		src, err := os.ReadFile(f)
		is.NoErr(err)
		examples, err := corpusExamples(string(src))
		is.NoErr(err)
		is.True(len(examples) > 0)
		for _, ex := range examples {
			if _, err := Parse(ex.src, f, nil); err != nil {
				t.Errorf("%s: %s: %v", f, ex.name, err)
			}
		}
	}
}

type corpusExample struct {
	name string
	src  string
}

// A rule is three or more of the character, and the CLI accepts any
// length, so matching a prefix would read a longer tree rule as source.
var (
	nameRule = regexp.MustCompile(`\A={3,}\z`)
	treeRule = regexp.MustCompile(`\A-{3,}\z`)
)

// corpusExamples reads the tree-sitter corpus format: a name fenced by
// two ==== rules, the source, ----, then the tree the grammar should
// produce. Only the source is of interest here, but an example missing
// either half is an error rather than a source that quietly swallows
// the tree, since an S-expression parses as hml text and would pass.
func corpusExamples(text string) ([]corpusExample, error) {
	var out []corpusExample
	lines := strings.Split(text, "\n")
	name := ""
	var body []string
	flush := func() error {
		if name == "" {
			return nil
		}
		i := slices.IndexFunc(body, treeRule.MatchString)
		if i < 0 {
			return fmt.Errorf("%s: no --- rule before the expected tree", name)
		}
		src := strings.Trim(strings.Join(body[:i], "\n"), "\n")
		if src == "" {
			return fmt.Errorf("%s: no source", name)
		}
		out = append(out, corpusExample{name: name, src: src + "\n"})
		return nil
	}
	for i := 0; i < len(lines); i++ {
		if nameRule.MatchString(lines[i]) && i+2 < len(lines) &&
			nameRule.MatchString(lines[i+2]) {
			if err := flush(); err != nil {
				return nil, err
			}
			name = lines[i+1]
			body = nil
			i += 2
			continue
		}
		body = append(body, lines[i])
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}
