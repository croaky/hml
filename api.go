package hml

import "strings"

// Transform is an app-registered rich-text builtin available to templates
// as `= name(field)`. It takes the field value stringified and returns
// HTML the renderer emits unescaped, so a Transform must sanitize its
// output. The engine ships zero built-ins; apps register their own.
type Transform func(string) string

// PartialFunc resolves `= render "name", key: val` calls. It receives the
// partial name and a child Context: the render args layered on the caller's
// context, so the partial inherits the caller's locals without a copy. It
// renders the named partial (typically via Template.RenderContext) and
// returns HTML.
type PartialFunc func(name string, ctx *Context) (string, error)

// Template is a parsed hml template.
type Template struct {
	path  string
	nodes []node
}

// SafeString marks trusted HTML that should not be escaped when rendered with
// escaped output syntax (= expr), matching ViewHelper::SafeString behavior.
type SafeString string

// Parse parses hml source into a Template. transforms is the
// app-registered set of rich-text builtins (see Transform); an unknown
// `= name(field)` transform name is a Parse error, so the parser stays the
// linter. A nil map registers no transforms.
func Parse(source, path string, transforms map[string]Transform) (*Template, error) {
	rawLines := strings.Split(source, "\n")
	// Remove trailing empty line from final newline
	if len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	lines := joinContinuationLines(rawLines)
	nodes, err := parseLines(lines, 0, 0, len(lines), path)
	if err != nil {
		return nil, err
	}
	// Pre-parse conditionals, output, each collections, attribute
	// hashes, text interpolation, and render calls so Render evaluates
	// ASTs instead of re-tokenizing per render. Their syntax errors
	// surface here, at Parse time.
	if err := compileNodes(nodes, path, transforms); err != nil {
		return nil, err
	}
	return &Template{path: path, nodes: nodes}, nil
}

// Render executes the template with the given locals. It is a convenience
// wrapper that seeds a root Context; callers rendering partials thread a
// Context through PartialFunc via RenderContext instead.
func (t *Template) Render(locals map[string]any, partialFn PartialFunc) (string, error) {
	return t.RenderContext(NewContext(locals), partialFn)
}

// RenderContext executes the template against an existing Context. Partial
// rendering uses this to render a partial against the child context handed to
// PartialFunc, avoiding a per-partial copy of the caller's locals.
func (t *Template) RenderContext(ctx *Context, partialFn PartialFunc) (string, error) {
	var buf strings.Builder
	if err := renderNodes(t.nodes, &buf, ctx, partialFn, t.path); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type nodeKind int

const (
	kindDoctype nodeKind = iota
	kindComment
	kindText
	kindOutput // = expr
	kindTag
	kindFilter // :javascript, :css
	kindIf
	kindElsif
	kindElse
	kindFor
	kindRender    // = render "partial", key: val
	kindTransform // = markdown(field), = slack(field), = search_highlight(field)
)

type node struct {
	kind        nodeKind
	indent      int
	text        string   // for text, output, render expr
	expr        string   // if/elsif condition, for collection
	tag         string   // for tag
	classes     []string // for tag
	id          string   // for tag
	attrsStr    string   // raw attribute hash string for tag
	filterName  string   // for filter
	filterLines []string // for filter
	elemVar     string   // for loop element variable
	indexVar    string   // for loop index variable (optional; "" if absent)
	children    []node

	// Compiled at Parse time by compileNodes; consumed by Render.
	exprAST    *astNode      // if/elsif condition, for collection, output, transform
	transform  Transform     // resolved rich-text builtin (kindTransform)
	attrs      []attr        // tag attribute hash
	textSegs   []interpSeg   // static text interpolation
	filterSegs [][]interpSeg // filter lines, indent stripped, per line

	// Compiled render call (kindRender), replacing per-render regexp +
	// tokenize. Exactly one of renderNameSegs / renderNameExpr is set.
	renderNameSegs []interpSeg // literal partial name (may interpolate #{})
	renderNameExpr *astNode    // variable partial name
	renderArgs     []attr      // compiled locals (key: val and **splat)
}
