package hml

import (
	"slices"
	"strings"
)

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
	path    string
	nodes   []node
	names   []string
	renders []string
}

// SafeString marks trusted HTML that should not be escaped when rendered with
// escaped output syntax (= expr), matching ViewHelper::SafeString behavior.
type SafeString string

// SafeJS marks a string the handler asserts is JavaScript source safe to
// place in an on* event-handler attribute. Without it, a dynamic on*
// value is a render error: the renderer cannot tell data from code.
type SafeJS string

// SafeCSS marks a string the handler asserts is CSS declarations safe to
// place in a style attribute. Without it, a dynamic style value is a
// render error.
type SafeCSS string

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
	names, renders := collectNames(nodes)
	return &Template{path: path, nodes: nodes, names: names, renders: renders}, nil
}

// Names returns the free top-level identifiers this template reads,
// sorted and deduplicated, so a caller can check its locals before
// serving a request. Loop variables are bound, not free, and are
// excluded; the collection they iterate is included. Partials are not
// followed, because a PartialFunc resolves them app-side where hml
// cannot see: use Renders to walk that graph.
func (t *Template) Names() []string { return t.names }

// Renders returns the partial names this template renders with a
// literal `= render "name"`, sorted and deduplicated, so a caller can
// walk its own graph. A computed name is not resolvable here and is
// omitted.
func (t *Template) Renders() []string { return t.renders }

// collectNames walks the compiled tree once, at the end of Parse, for
// the free names it reads and the literal partials it renders. Both
// answers are fixed by the tree, so a caller asking per render should
// not pay for the walk.
func collectNames(nodes []node) (names, renders []string) {
	w := &nameWalk{
		names:   map[string]bool{},
		renders: map[string]bool{},
		bound:   map[string]int{},
	}
	w.walkNodes(nodes)
	return sortedKeys(w.names), sortedKeys(w.renders)
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// nameWalk accumulates the result of collectNames. bound counts the loop
// variables in scope by name, so a name shadowed by an enclosing loop is
// free again once that loop's body ends.
type nameWalk struct {
	names   map[string]bool
	renders map[string]bool
	bound   map[string]int
}

func (w *nameWalk) walkNodes(nodes []node) {
	for i := range nodes {
		n := &nodes[i]
		switch n.kind {
		case kindText:
			w.walkInterp(n.textSegs)
		case kindFilter:
			for _, segs := range n.filterSegs {
				w.walkInterp(segs)
			}
		case kindOutput, kindTransform, kindIf, kindElseIf:
			w.walkExpr(n.exprAST)
		case kindTag:
			for _, a := range n.attrs {
				// A **splat's value is read like any other; only a
				// key: pair has a key, and a key is not a read.
				w.walkExpr(a.val)
			}
		case kindRender:
			w.walkRender(n)
		case kindFor:
			// The collection is read in the enclosing scope; the loop
			// variables bind only over the body.
			w.walkExpr(n.exprAST)
			w.bind(n.elemVar)
			w.bind(n.indexVar)
			w.walkNodes(n.children)
			w.unbind(n.indexVar)
			w.unbind(n.elemVar)
			continue
		}
		w.walkNodes(n.children)
	}
}

// walkRender records a literal partial name and reads the argument
// values. The argument keys name locals the partial reads, not ones the
// caller must supply, so they are not names.
func (w *nameWalk) walkRender(n *node) {
	if name, ok := literalSegs(n.renderNameSegs); ok {
		w.renders[name] = true
	}
	w.walkInterp(n.renderNameSegs)
	w.walkExpr(n.renderNameExpr)
	for _, a := range n.renderArgs {
		w.walkExpr(a.val)
	}
}

// literalSegs returns the name of a partial whose every segment is a
// literal. An interpolated name is not resolvable at parse time.
func literalSegs(segs []interpSeg) (string, bool) {
	if len(segs) == 0 {
		return "", false
	}
	var b strings.Builder
	for _, s := range segs {
		if s.expr != nil {
			return "", false
		}
		b.WriteString(s.lit)
	}
	return b.String(), true
}

func (w *nameWalk) bind(name string) {
	if name != "" {
		w.bound[name]++
	}
}

func (w *nameWalk) unbind(name string) {
	if name == "" {
		return
	}
	if w.bound[name] <= 1 {
		delete(w.bound, name)
		return
	}
	w.bound[name]--
}

func (w *nameWalk) walkInterp(segs []interpSeg) {
	for _, s := range segs {
		w.walkExpr(s.expr)
	}
}

// walkExpr records the free top-level name of every field access and
// call in an expression, recursing through every node type that can
// hold another expression.
func (w *nameWalk) walkExpr(n *astNode) {
	if n == nil {
		return
	}
	switch n.typ {
	case astField, astCall:
		if len(n.parts) > 0 && w.bound[n.parts[0]] == 0 {
			w.names[n.parts[0]] = true
		}
		for _, arg := range n.args {
			w.walkExpr(arg)
		}
	case astInterpString:
		for _, s := range n.segments {
			w.walkExpr(s.expr)
		}
	case astHash:
		for _, p := range n.pairs {
			w.walkExpr(p.val)
		}
	case astArray:
		for _, el := range n.elements {
			w.walkExpr(el)
		}
	case astCmp, astAnd, astOr:
		w.walkExpr(n.left)
		w.walkExpr(n.right)
	case astNot:
		w.walkExpr(n.operand)
	}
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
	kindElseIf
	kindElse
	kindFor
	kindRender    // = render "partial", key: val
	kindCall      // = name(args), before compileNodes resolves it
	kindTransform // = markdown(field), = slack(field), = search_highlight(field)
)

type node struct {
	kind        nodeKind
	indent      int
	line        int      // 1-based source line, for error messages
	text        string   // for text, output, render expr
	expr        string   // if/else if condition, for collection
	callName    string   // for kindCall: the name before the parens
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
	exprAST    *astNode      // if/else if condition, for collection, output, transform
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
