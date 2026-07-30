package hml

import (
	"fmt"
	"html"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true,
	"embed": true, "hr": true, "img": true, "input": true,
	"link": true, "meta": true, "source": true, "track": true,
	"wbr": true,
}

var preserveElements = map[string]bool{
	"textarea": true, "pre": true,
}

func renderNodes(nodes []node, buf *strings.Builder, ctx context, partialFn PartialFunc, path string) error {
	i := 0
	for i < len(nodes) {
		n := nodes[i]
		switch n.kind {
		case kindDoctype:
			buf.WriteString("<!DOCTYPE html>\n")
		case kindComment:
			// skip
		case kindText:
			s, err := evalInterp(n.textSegs, ctx, true)
			if err != nil {
				return fmt.Errorf("%s: text interpolation: %w", path, err)
			}
			buf.WriteString(s)
			buf.WriteByte('\n')
		case kindOutput:
			val, err := evaluate(n.exprAST, ctx)
			if err != nil {
				return fmt.Errorf("%s: output eval %q: %w", path, n.text, err)
			}
			if s, ok := val.(SafeString); ok {
				buf.WriteString(string(s))
			} else {
				buf.WriteString(escapeHTML(stringify(val)))
			}
			buf.WriteByte('\n')
		case kindTransform:
			val, err := evaluate(n.exprAST, ctx)
			if err != nil {
				return fmt.Errorf("%s: transform eval %q: %w", path, n.expr, err)
			}
			buf.WriteString(n.transform(stringify(val)))
			buf.WriteByte('\n')
		case kindRender:
			s, err := renderPartialCall(n, ctx, partialFn, path)
			if err != nil {
				return err
			}
			buf.WriteString(s)
		case kindTag:
			if err := renderTag(n, buf, ctx, partialFn, path); err != nil {
				return err
			}
		case kindFilter:
			if err := renderFilter(n, buf, ctx, path); err != nil {
				return err
			}
		case kindIf:
			// collect if/elsif/else chain
			chain := []node{n}
			for i+1 < len(nodes) && (nodes[i+1].kind == kindElsif || nodes[i+1].kind == kindElse) {
				i++
				chain = append(chain, nodes[i])
			}
			if err := renderConditional(chain, buf, ctx, partialFn, path); err != nil {
				return err
			}
		case kindFor:
			if err := renderFor(n, buf, ctx, partialFn, path); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: unsupported node kind: %d", path, n.kind)
		}
		i++
	}
	return nil
}

func renderTag(n node, buf *strings.Builder, ctx context, partialFn PartialFunc, path string) error {
	attrs, err := buildAttrs(n, ctx, path)
	if err != nil {
		return err
	}

	var attrStr strings.Builder
	// Sentinel values from toAttrVal: \x00true → boolean attribute,
	// \x00false/\x00nil → omit entirely.
	for _, a := range attrs {
		switch a.val {
		case "\x00true":
			attrStr.WriteByte(' ')
			attrStr.WriteString(a.key)
		case "\x00false", "\x00nil":
			// omit
		default:
			val, err := policyAttr(a)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			attrStr.WriteByte(' ')
			attrStr.WriteString(a.key)
			attrStr.WriteString("=\"")
			attrStr.WriteString(escapeHTML(val))
			attrStr.WriteByte('"')
		}
	}

	tag := n.tag
	as := attrStr.String()

	if voidElements[tag] {
		buf.WriteByte('<')
		buf.WriteString(tag)
		buf.WriteString(as)
		buf.WriteString(">\n")
		return nil
	}

	if len(n.children) > 0 {
		if preserveElements[tag] || loneTextChild(n.children) {
			var inner strings.Builder
			if err := renderNodes(n.children, &inner, ctx, partialFn, path); err != nil {
				return err
			}
			text := trimRenderedNewline(inner.String())
			// A run of text can turn out to be several lines: a
			// transform emits the HTML it was given, and markdown of
			// two paragraphs is two blocks. Hanging those off the
			// opening tag buys nothing -- the collapsed space they
			// would have is between blocks, where nothing sees it --
			// and costs the shape that makes the output readable. A
			// preserve element keeps its lines regardless; there the
			// whitespace is the content.
			if !preserveElements[tag] && strings.Contains(text, "\n") {
				buf.WriteByte('<')
				buf.WriteString(tag)
				buf.WriteString(as)
				buf.WriteString(">\n")
				buf.WriteString(inner.String())
				buf.WriteString("</")
				buf.WriteString(tag)
				buf.WriteString(">\n")
				return nil
			}
			buf.WriteByte('<')
			buf.WriteString(tag)
			buf.WriteString(as)
			buf.WriteByte('>')
			buf.WriteString(text)
			buf.WriteString("</")
			buf.WriteString(tag)
			buf.WriteString(">\n")
		} else {
			buf.WriteByte('<')
			buf.WriteString(tag)
			buf.WriteString(as)
			buf.WriteString(">\n")
			if err := renderNodes(n.children, buf, ctx, partialFn, path); err != nil {
				return err
			}
			buf.WriteString("</")
			buf.WriteString(tag)
			buf.WriteString(">\n")
		}
	} else {
		buf.WriteByte('<')
		buf.WriteString(tag)
		buf.WriteString(as)
		buf.WriteString("></")
		buf.WriteString(tag)
		buf.WriteString(">\n")
	}
	return nil
}

// trimRenderedNewline drops the single trailing newline the renderer
// itself wrote after the last child. Inside a preserve element the
// rest of the whitespace is content: a diff patch or a terminal
// transcript begins its lines with the spaces that align them, and a
// blanket trim would silently reflow the text the element exists to
// preserve.
func trimRenderedNewline(s string) string {
	return strings.TrimSuffix(s, "\n")
}

// loneTextChild reports whether children are a single run of text, so
// the tag can hold it on one line: <a href="/">home</a>.
//
// Otherwise a tag's content starts on the line below, and the newline
// before the closing tag is whitespace inside the element. HTML
// collapses it to a space, which is usually nothing to look at and
// occasionally the whole problem: inside an anchor it is a space the
// underline runs through, past the end of the word.
//
// One child only. Two lines of text are two lines the author separated,
// and joining them would close a gap that is in the source on purpose.
// Elements, partials, and conditionals are left alone: they bring their
// own lines, and a tag holding them reads as the block it is.
//
// The newline between siblings stays either way, so words in a row are
// still words in a row -- what goes is the space inside the element,
// which nothing was spacing.
func loneTextChild(children []node) bool {
	if len(children) != 1 {
		return false
	}
	switch children[0].kind {
	case kindText, kindOutput, kindTransform:
		return true
	default:
		return false
	}
}

func buildAttrs(n node, ctx context, path string) ([]attrVal, error) {
	var result []attrVal
	shorthandClasses := n.classes
	var attrClasses []string

	if n.attrs != nil {
		pairs, err := evalAttrs(n.attrs, ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: attribute eval: %w", path, err)
		}
		for _, p := range pairs {
			if p.key == "class" {
				// Omit nil/false/empty class values so conditional
				// classes don't leave sentinels or stray spaces.
				if p.val == "\x00true" || p.val == "\x00false" || p.val == "\x00nil" || p.val == "" {
					continue
				}
				attrClasses = append(attrClasses, p.val)
			} else {
				result = append(result, p)
			}
		}
	}

	allClasses := append(shorthandClasses, attrClasses...)
	if len(allClasses) > 0 {
		result = append(result, attrVal{key: "class", val: strings.Join(allClasses, " "), authored: true})
	}

	if n.id != "" {
		result = append(result, attrVal{key: "id", val: n.id, authored: true})
	}

	// Sort alphabetically by key for stable output
	sortAttrs(result)

	return result, nil
}

// urlAttrs are the attributes a browser fetches or navigates to. Their
// values are URLs, so a scheme in them is code, not text.
var urlAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true,
	"poster": true, "cite": true, "background": true, "ping": true,
	"xlink:href": true,
}

// urlSchemes is the allowlist for absolute URLs. Everything else —
// javascript:, data:, vbscript:, and any scheme nobody has thought of —
// is rejected.
var urlSchemes = map[string]bool{
	"http": true, "https": true, "mailto": true, "tel": true,
}

// unsafeURL replaces a rejected URL. It is a sentinel, not a link
// target: the name matches html/template so the two engines fail the
// same way and the string is greppable in rendered output.
const unsafeURL = "#ZgotmplZ"

// policyAttr constrains a value by the context its attribute puts it in.
// hml needs no HTML state machine for this the way html/template does:
// the renderer holds the attribute name and the value together, so it
// can decide per attribute.
//
// URL attributes take only relative URLs and an allowlisted scheme. on*
// and style attributes are code, so they take code the template author
// wrote — that is application source — or a value the handler marked
// SafeJS or SafeCSS. A plain dynamic value in either is an error rather
// than an escape, because there is no escaping that makes untrusted data
// safe as code.
//
// Where the author wrote it does not enter into it. A string literal is
// authored wherever it is typed, and it stays so through a hash literal,
// a partial argument, and a splat, so markup factored into a partial
// means what it meant inline.
func policyAttr(a attrVal) (string, error) {
	name := strings.ToLower(a.key)
	switch {
	case urlAttrs[name]:
		if !safeURL(a.val) {
			return unsafeURL, nil
		}
	case strings.HasPrefix(name, "on"):
		if !a.authored && a.trust != trustJS {
			return "", fmt.Errorf("attribute %s: dynamic value in a JavaScript context requires hml.SafeJS", a.key)
		}
	case name == "style":
		if !a.authored && a.trust != trustCSS {
			return "", fmt.Errorf("attribute %s: dynamic value in a CSS context requires hml.SafeCSS", a.key)
		}
	}
	return a.val, nil
}

// safeURL reports whether s is a relative URL or carries an allowlisted
// scheme. Control characters and spaces are dropped before the scheme is
// read, because browsers drop them too: "java\tscript:" navigates.
func safeURL(s string) bool {
	s = strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		return r
	}, s)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ':' {
			// A scheme must be a non-empty run of scheme
			// characters; "foo/bar:baz" never reaches here.
			return i > 0 && urlSchemes[strings.ToLower(s[:i])]
		}
		if !isSchemeChar(c) {
			// No scheme: a path, query, fragment, or
			// interpolated value. Relative URLs are safe.
			return true
		}
	}
	return true
}

func isSchemeChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'
}

func renderConditional(chain []node, buf *strings.Builder, ctx context, partialFn PartialFunc, path string) error {
	for _, n := range chain {
		switch n.kind {
		case kindIf, kindElsif:
			val, err := evaluate(n.exprAST, ctx)
			if err != nil {
				return fmt.Errorf("%s: condition eval %q: %w", path, n.expr, err)
			}
			if truthy(val) {
				return renderNodes(n.children, buf, ctx, partialFn, path)
			}
		case kindElse:
			return renderNodes(n.children, buf, ctx, partialFn, path)
		}
	}
	return nil
}

func renderFor(n node, buf *strings.Builder, ctx context, partialFn PartialFunc, path string) error {
	val, err := evaluate(n.exprAST, ctx)
	if err != nil {
		return fmt.Errorf("%s: for collection eval %q: %w", path, n.expr, err)
	}
	collection, ok := toAnySlice(val)
	if !ok {
		return fmt.Errorf("%s: for requires a slice, got %s", path, typeName(val))
	}
	// Overlay a single child on the parent context and rebind the loop
	// variables each iteration. The parent is read through, not copied, so a
	// loop costs one small map regardless of how wide the parent locals are.
	// Loop bodies never mutate the context (nested loops overlay their own;
	// partials get their own child), so reusing one overlay is safe.
	overlay := map[string]any{}
	childCtx := ctx.Child(overlay)
	for i, item := range collection {
		overlay[n.elemVar] = item
		if n.indexVar != "" {
			overlay[n.indexVar] = int64(i)
		}
		if err := renderNodes(n.children, buf, childCtx, partialFn, path); err != nil {
			return err
		}
	}
	return nil
}

func renderFilter(n node, buf *strings.Builder, ctx context, path string) error {
	switch n.filterName {
	case "javascript":
		buf.WriteString("<script>\n")
	case "css":
		buf.WriteString("<style>\n")
	}
	for _, segs := range n.filterSegs {
		s, err := evalInterp(segs, ctx, false)
		if err != nil {
			return fmt.Errorf("%s: filter interpolation: %w", path, err)
		}
		buf.WriteString(s)
		buf.WriteByte('\n')
	}
	switch n.filterName {
	case "javascript":
		buf.WriteString("</script>\n")
	case "css":
		buf.WriteString("</style>\n")
	}
	return nil
}

func stripFilterIndent(line string, baseIndent int) string {
	expected := baseIndent + 2
	if len(line) >= expected {
		prefix := line[:expected]
		if strings.TrimSpace(prefix) == "" {
			return line[expected:]
		}
	}
	return strings.TrimLeft(line, " \t")
}

// renderCallRE / renderVarRE split a `render` call into name + args. They run
// once per node at Parse time (compileRender), never during Render.
var renderCallRE = regexp.MustCompile(`\Arender\s+"([^"]+)"(?:,\s*(.*))?\z`)
var renderVarRE = regexp.MustCompile(`\Arender\s+(\w[\w.]*)(?:,\s*(.*))?\z`)

// renderPartialCall resolves a precompiled render node (see compileRender):
// the name comes from renderNameSegs (literal, possibly interpolated) or
// renderNameExpr (variable), and args from the compiled renderArgs. The args
// become a child context layered on the caller's, so the partial inherits the
// caller's locals without copying them.
func renderPartialCall(n node, ctx context, partialFn PartialFunc, path string) (string, error) {
	if partialFn == nil {
		return "", fmt.Errorf("%s: no partialFn provided for: %s", path, n.text)
	}

	var name string
	if n.renderNameSegs != nil {
		s, err := evalInterp(n.renderNameSegs, ctx, false)
		if err != nil {
			return "", err
		}
		name = s
	} else {
		val, err := evaluate(n.renderNameExpr, ctx)
		if err != nil {
			return "", err
		}
		name = stringify(val)
	}

	overlay := map[string]any{}
	if n.renderArgs != nil {
		var err error
		overlay, err = evalLocals(n.renderArgs, ctx)
		if err != nil {
			return "", fmt.Errorf("%s: render args eval: %w", path, err)
		}
	}

	return partialFn(name, ctx.Child(overlay))
}

// stringify converts a value to a string: nil becomes "" (not "<nil>").
func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case authored:
		return string(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case SafeString:
		return string(x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func sortAttrs(result []attrVal) {
	switch len(result) {
	case 0, 1:
		// already sorted
	case 2:
		if result[0].key > result[1].key {
			result[0], result[1] = result[1], result[0]
		}
	case 3:
		if result[0].key > result[1].key {
			result[0], result[1] = result[1], result[0]
		}
		if result[1].key > result[2].key {
			result[1], result[2] = result[2], result[1]
		}
		if result[0].key > result[1].key {
			result[0], result[1] = result[1], result[0]
		}
	default:
		sort.Slice(result, func(i, j int) bool {
			return result[i].key < result[j].key
		})
	}
}

// toAnySlice converts any slice type ([]string, []int, []map[string]any,
// etc.) to []any using reflection. Returns false if v is not a slice.
func toAnySlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	switch s := v.(type) {
	case []any:
		return s, true
	case []map[string]any:
		out := make([]any, len(s))
		for i := range s {
			out[i] = s[i]
		}
		return out, true
	case []string:
		out := make([]any, len(s))
		for i := range s {
			out[i] = s[i]
		}
		return out, true
	case []int64:
		out := make([]any, len(s))
		for i := range s {
			out[i] = s[i]
		}
		return out, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, false
	}
	result := make([]any, rv.Len())
	for i := range result {
		result[i] = rv.Index(i).Interface()
	}
	return result, true
}

// escapeHTML escapes &, <, >, " for HTML output.
func escapeHTML(s string) string {
	return html.EscapeString(s)
}
