package hml

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// attr is a compiled tag-attribute entry. When splat is true it is a
// **expr that must evaluate to a map; otherwise it is a key: val pair.
type attr struct {
	splat bool
	key   string
	val   *astNode
}

// interpSeg is one compiled piece of interpolated text: a literal
// (expr == nil) or an expression to evaluate and stringify. In a filter
// block, place says how renderFilter writes the value.
type interpSeg struct {
	lit   string
	expr  *astNode
	place filterPlace
}

// filterPlace is where a #{} sits in a filter block. Parse decides it
// from the source around the #{}, so a render only looks it up.
type filterPlace uint8

const (
	// placeNone is text or an attribute, which have their own escape.
	placeNone filterPlace = iota
	// placeJSValue is JavaScript code, where a value is an expression.
	placeJSValue
	// placeJSString is inside a template literal, or a quoted string
	// that Parse rewrites as one.
	placeJSString
	// placeCSS is anywhere in a :css block.
	placeCSS
)

// compileNodes pre-parses every expression, attribute hash, and text
// interpolation in the tree into ASTs once, so Render never has to
// re-tokenize or re-parse. It mutates nodes in place and recurses into
// children. Expression syntax errors now surface here, at Parse time —
// the parser stays the linter.
func compileNodes(nodes []node, path string, transforms map[string]Transform) error {
	for i := range nodes {
		n := &nodes[i]
		if n.kind == kindIf {
			for j := i + 1; j < len(nodes) && (nodes[j].kind == kindElseIf || nodes[j].kind == kindElse); j++ {
				n.elseCount++
			}
		}
		switch n.kind {
		case kindText:
			segs, err := compileInterp(n.text)
			if err != nil {
				return fmt.Errorf("%s: text %q: %w", path, n.text, err)
			}
			n.textSegs = segs
		case kindOutput:
			ast, err := compileExpr(n.text)
			if err != nil {
				return fmt.Errorf("%s: output %q: %w", path, n.text, err)
			}
			n.exprAST = ast
		case kindCall:
			// A registered name is a transform: it takes one field,
			// sanitizes app-side, and its result is emitted unescaped, so
			// the narrow argument keeps template-side content assembly
			// impossible. Any other name is an ordinary call on a helper
			// func injected as a local, compiled from the line as written
			// and escaped like every other = output.
			fn, ok := transforms[n.callName]
			if !ok {
				ast, err := compileExpr(n.text)
				if err != nil {
					return fmt.Errorf("%s: call %q: %w", path, n.text, err)
				}
				n.kind = kindOutput
				n.exprAST = ast
				break
			}
			if !transformFieldRE.MatchString(n.expr) {
				return fmt.Errorf("%s: transform argument must be a single field access: = %s", path, n.text)
			}
			n.kind = kindTransform
			n.transform = fn
			ast, err := compileExpr(n.expr)
			if err != nil {
				return fmt.Errorf("%s: transform %q: %w", path, n.expr, err)
			}
			n.exprAST = ast
		case kindIf, kindElseIf, kindFor:
			ast, err := compileExpr(n.expr)
			if err != nil {
				return fmt.Errorf("%s: expression %q: %w", path, n.expr, err)
			}
			n.exprAST = ast
			if n.kind != kindFor {
				if lit := neverBool(ast); lit != "" {
					return fmt.Errorf("%s:%d: condition %q requires a bool, got a %s literal", path, n.line, n.expr, lit)
				}
			}
		case kindTag:
			if n.attrsStr != "" {
				attrs, err := compileAttrs(n.attrsStr)
				if err != nil {
					return fmt.Errorf("%s: attribute parse %q: %w", path, n.attrsStr, err)
				}
				n.attrs = attrs
			}
			if err := hoistStaticAttrs(n, path); err != nil {
				return err
			}
		case kindRender:
			if err := compileRender(n); err != nil {
				return fmt.Errorf("%s: render %q: %w", path, n.text, err)
			}
		case kindFilter:
			if err := compileFilter(n, path); err != nil {
				return err
			}
		}
		if err := compileNodes(n.children, path, transforms); err != nil {
			return err
		}
		// The layout reads the kind of each child, and compileNodes
		// turns a call child into an output or a transform above.
		if n.kind == kindTag {
			n.layout = layoutOf(n.tag, n.children)
		}
	}
	return nil
}

// compileFilter splits each line of a filter block into literal text
// and #{} values, and records the place of each value.
//
// A :javascript block takes two passes of jsLexer over the same text.
// The first finds the place of each value, refuses a place where no
// value can go, and notes each quoted string that holds a value. The
// second rewrites each such string as a template literal, so the value
// becomes a ${} substitution and the string stays one token. Both
// passes read the same text, so they count the same strings.
func compileFilter(n *node, path string) error {
	segs := make([][]interpSeg, len(n.filterLines))
	lines := make([]string, len(n.filterLines))
	var js jsLexer
	for j, line := range n.filterLines {
		lines[j] = stripFilterIndent(line, n.indent)
		s, err := compileInterp(lines[j])
		if err != nil {
			return fmt.Errorf("%s: filter interpolation: %w", path, err)
		}
		segs[j] = s
		for k := range s {
			switch {
			case n.filterName == "css":
				s[k].place = placeCSS
			case s[k].expr == nil:
				js.read(s[k].lit)
				js.out.Reset()
			default:
				place, err := js.place()
				if err != nil {
					return fmt.Errorf("%s:%d: :javascript interpolation %w", path, n.line+j+1, err)
				}
				s[k].place = place
				n.filterValues++
			}
		}
		if n.filterName != "css" {
			if err := js.endLine(lines[j]); err != nil {
				return fmt.Errorf("%s:%d: :javascript line %w", path, n.line+j+1, err)
			}
		}
	}
	n.filterSegs = segs
	if len(js.withValue) == 0 {
		return nil
	}
	conv := jsLexer{convert: js.withValue}
	for j, s := range segs {
		for k := range s {
			if s[k].expr != nil {
				_, _ = conv.place()
				continue
			}
			conv.read(s[k].lit)
			s[k].lit = conv.out.String()
			conv.out.Reset()
			if conv.err != nil {
				return fmt.Errorf("%s:%d: :javascript string %w", path, n.line+j+1, conv.err)
			}
		}
		_ = conv.endLine(lines[j])
	}
	return nil
}

// hoistStaticAttrs renders a tag's attributes once, at Parse, when
// nothing in them reads a local: no splat, and every value a literal.
// The result is what writeAttrs would write on every render, so the
// render writes it instead. A literal is authored, so the attribute
// policy answers the same at Parse as it would at render.
//
// It asks the policy about each string literal on every tag, not only
// a static one. A refused literal fails on every render, so it fails
// here, also on a tag in a branch that no test renders.
func hoistStaticAttrs(n *node, path string) error {
	static := true
	for _, a := range n.attrs {
		if a.splat {
			static = false
			continue
		}
		if !validAttrName(a.key) {
			return fmt.Errorf("%s: key %q is not an attribute name: use ASCII letters, digits, -, _, :, or .", path, a.key)
		}
		switch a.val.typ {
		case astString:
			if _, err := policyAttr(attrVal{key: a.key, val: toAttrVal(a.val.lit), authored: true}); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		case astNumber, astBool, astNil:
		default:
			static = false
		}
	}
	if !static {
		return nil
	}
	var buf strings.Builder
	if err := writeAttrsDynamic(*n, &buf, nil, path); err != nil {
		return err
	}
	n.staticAttrs = buf.String()
	n.attrsAreStatic = true
	return nil
}

// tagLayout is how renderTag writes a tag's content.
type tagLayout int

const (
	// layoutBlock puts the children on the lines below the opening tag.
	layoutBlock tagLayout = iota
	// layoutEmpty writes the closing tag right after the opening tag.
	layoutEmpty
	// layoutVoid writes no closing tag and no children.
	layoutVoid
	// layoutPreserve writes the children with their whitespace kept.
	layoutPreserve
	// layoutInline puts a lone text child on the line of the tag.
	layoutInline
)

// layoutOf gives the layout of a tag from its name and children. A void
// element ignores its children, and a preserve element keeps a lone
// text child on lines of its own.
func layoutOf(tag string, children []node) tagLayout {
	switch {
	case voidElements[tag]:
		return layoutVoid
	case len(children) == 0:
		return layoutEmpty
	case preserveElements[tag]:
		return layoutPreserve
	case loneTextChild(children):
		return layoutInline
	}
	return layoutBlock
}

// neverBool names the literal kind of a condition that cannot be a bool
// whatever the locals hold, and "" for one that might be. A condition
// is checked at render time too, because most of them read a field
// whose type only the handler knows; this catches the ones the AST
// already settles, at Parse, where the parser is the linter.
//
// The point is the branch nobody exercises. `- else if title ||
// "Untitled"` is wrong the moment it is written, but a render-time
// check reports it only when that branch is reached -- which, for an
// else if, may be in production and not in a test.
//
// && and || return an operand, so either side can be the value the
// condition sees and both are checked. ! and a comparison always yield a
// bool. A field or a call is unknown here by construction.
func neverBool(n *astNode) string {
	switch n.typ {
	case astString, astInterpString:
		return "string"
	case astNumber:
		return "number"
	case astNil:
		return "nil"
	case astHash:
		return "hash"
	case astArray:
		return "array"
	case astAnd, astOr:
		if lit := neverBool(n.left); lit != "" {
			return lit
		}
		return neverBool(n.right)
	}
	return ""
}

// compileExpr tokenizes and parses a single expression string into an
// AST. It is the parse half of the old evalString.
func compileExpr(src string) (*astNode, error) {
	tokens, err := tokenize(strings.TrimSpace(src))
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	return p.parseExpr()
}

// compileAttrs parses a tag's attribute hash into compiled pairs. It is
// the parse half of the old parseHash: values stay as ASTs, evaluated
// per render by evalAttrs.
func compileAttrs(src string) ([]attr, error) {
	cleaned := strings.ReplaceAll(src, "\n", " ")
	tokens, err := tokenize(cleaned)
	if err != nil {
		return nil, err
	}
	var result []attr
	p := &parser{tokens: tokens}
	for !p.atEnd() {
		// splat: **expr
		if p.peek().typ == tokSplat {
			p.advance()
			node, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			result = append(result, attr{splat: true, val: node})
			if p.peek() != nil && p.peek().typ == tokComma {
				p.advance()
			}
			continue
		}

		t := p.peek()
		var key string
		switch t.typ {
		case tokIdent:
			key = p.advance().str
		case tokString:
			key = p.advance().str
		default:
			return nil, fmt.Errorf("expected key, got %q", t.str)
		}

		if p.peek() == nil || p.peek().typ != tokColon {
			return nil, fmt.Errorf("expected ':' after key %q", key)
		}
		p.advance()

		node, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		result = append(result, attr{key: key, val: node})

		if p.peek() != nil && p.peek().typ == tokComma {
			p.advance()
		}
	}
	return result, nil
}

// compileRender parses a `render "name", key: val` call once at Parse time,
// so Render skips the regexp match and arg tokenization it used to do per
// call (renderPartialCall). Populates n.renderNameSegs or n.renderNameExpr
// plus n.renderArgs.
func compileRender(n *node) error {
	var argsStr string
	if m := renderCallRE.FindStringSubmatch(n.text); m != nil {
		segs, err := compileInterp(m[1])
		if err != nil {
			return err
		}
		n.renderNameSegs = segs
		argsStr = m[2]
	} else if m := renderVarRE.FindStringSubmatch(n.text); m != nil {
		ast, err := compileExpr(m[1])
		if err != nil {
			return err
		}
		n.renderNameExpr = ast
		argsStr = m[2]
	} else {
		return fmt.Errorf("unsupported render call")
	}
	if strings.TrimSpace(argsStr) != "" {
		args, err := compileAttrs(argsStr)
		if err != nil {
			return err
		}
		n.renderArgs = args
	}
	return nil
}

// jsLexer reads the literal source of a :javascript block, one segment
// at a time, so that place can say where each #{} sits. It knows the
// strings, template literals, comments, and regular expressions of
// JavaScript, and nothing else, which is all a place needs. The
// comments include <!-- and a line-start -->, because renderFilter
// writes a classic script, where the browser reads both as comments.
//
// A slash in code starts a regular expression or is a division. As in
// html/template, the token before the slash decides. A wrong choice
// can read a quote as the start of a string that the browser does not
// see. No value is in the script source (see renderFilter), so a wrong
// choice gives a wrong script, not data that runs as code. JavaScript
// ends no quoted string and no regular expression at a line end, so
// endLine refuses a line that ends in one. Such a line shows that the
// lexer and the browser disagree, and the error names the line.
type jsLexer struct {
	state jsState
	// braces counts the open { of each ${ in a template literal that
	// is still open, innermost last. A } at depth 0 ends the ${.
	braces []int
	// last is the last code byte that is not space, and word the
	// identifier that ends at it, for the regex-or-division choice.
	// sawSpace says a space follows last, so a new identifier starts
	// a new word. prop says a . comes before word, so word is a
	// property name and not a keyword.
	last     byte
	word     string
	sawSpace bool
	prop     bool
	// codeOnLine says the line holds code before the current byte. A
	// --> is a comment only when it does not.
	codeOnLine bool
	// strs counts the quoted strings opened so far. withValue holds
	// the number of each one that holds a #{}. A second pass sets
	// convert to the first pass's withValue, and read writes each such
	// string to out as a template literal. converting says the string
	// open now is one, and err records a string that cannot be one.
	strs       int
	withValue  map[int]bool
	convert    map[int]bool
	converting bool
	out        strings.Builder
	err        error
}

type jsState uint8

const (
	jsCode jsState = iota
	jsDouble
	jsSingle
	jsTemplate
	jsLineComment
	jsBlockComment
	jsRegex
	jsRegexClass
)

// jsRegexAfterWord holds the keywords after which a slash starts a
// regular expression, as after an operator.
var jsRegexAfterWord = map[string]bool{
	"return": true, "typeof": true, "case": true, "do": true,
	"else": true, "in": true, "instanceof": true, "new": true,
	"throw": true, "void": true, "delete": true, "yield": true,
	"await": true,
}

// read reads s and writes it to out, with each string in convert
// rewritten as a template literal.
func (l *jsLexer) read(s string) {
	for i := 0; i < len(s); i++ {
		start := i
		c := s[i]
		switch l.state {
		case jsCode:
			l.readCode(s, &i)
			if c == '"' || c == '\'' {
				l.strs++
				l.converting = l.convert[l.strs]
				if l.converting {
					l.out.WriteByte('`')
					continue
				}
			}
		case jsDouble, jsSingle:
			switch {
			case c == '\\':
				// A template literal has no octal escape: \1 to \9, or
				// \0 before a digit, is a SyntaxError there.
				if l.converting && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' &&
					(s[i+1] != '0' || i+2 < len(s) && s[i+2] >= '0' && s[i+2] <= '9') {
					l.err = fmt.Errorf("with a #{} holds an octal escape: write it as \\x or \\u")
				}
				i++
			case (c == '"' && l.state == jsDouble) || (c == '\'' && l.state == jsSingle):
				l.state = jsCode
				l.operand()
				if l.converting {
					l.converting = false
					l.out.WriteByte('`')
					continue
				}
			case l.converting && c == '`':
				l.out.WriteString("\\`")
				continue
			case l.converting && c == '$' && i+1 < len(s) && s[i+1] == '{':
				l.out.WriteString("\\$")
				continue
			}
		case jsTemplate:
			switch {
			case c == '\\':
				i++
			case c == '`':
				l.state = jsCode
				l.operand()
			case c == '$' && i+1 < len(s) && s[i+1] == '{':
				i++
				l.braces = append(l.braces, 0)
				l.state = jsCode
				l.last, l.word, l.sawSpace = '{', "", false
			}
		case jsLineComment:
		case jsBlockComment:
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				i++
				l.state = jsCode
			}
		case jsRegex:
			switch c {
			case '\\':
				i++
			case '[':
				l.state = jsRegexClass
			case '/':
				l.state = jsCode
				l.operand()
			}
		case jsRegexClass:
			switch c {
			case '\\':
				i++
			case ']':
				l.state = jsRegex
			}
		}
		l.out.WriteString(s[start:min(i+1, len(s))])
	}
}

// readCode reads the byte at s[*i] in code, and the byte after it when
// the two make one token.
func (l *jsLexer) readCode(s string, i *int) {
	c := s[*i]
	next := byte(0)
	if *i+1 < len(s) {
		next = s[*i+1]
	}
	codeOnLine := l.codeOnLine
	l.codeOnLine = true
	switch {
	case c == '"':
		l.state = jsDouble
	case c == '\'':
		l.state = jsSingle
	case c == '`':
		l.state = jsTemplate
	case c == '/' && next == '/':
		l.state = jsLineComment
	case c == '/' && next == '*':
		*i++
		l.state = jsBlockComment
		l.codeOnLine = codeOnLine
	case c == '<' && strings.HasPrefix(s[*i:], "<!--"):
		l.state = jsLineComment
	case c == '-' && !codeOnLine && strings.HasPrefix(s[*i:], "-->"):
		l.state = jsLineComment
	case (c == '+' || c == '-') && next == c:
		// ++ or --. After an operand it is postfix, and the code
		// ends in a value, so a slash after it is a division.
		*i++
		if l.endsInOperand() {
			l.operand()
		} else {
			l.last, l.word, l.sawSpace, l.prop = c, "", false, false
		}
	case c == '/' && l.regexAllowed():
		l.state = jsRegex
	case c == '}' && len(l.braces) > 0 && l.braces[len(l.braces)-1] == 0:
		l.braces = l.braces[:len(l.braces)-1]
		l.state = jsTemplate
	case c == ' ' || c == '\t' || c == '\r' || c == '\n':
		l.sawSpace = true
		l.codeOnLine = codeOnLine
	default:
		if len(l.braces) > 0 {
			switch c {
			case '{':
				l.braces[len(l.braces)-1]++
			case '}':
				l.braces[len(l.braces)-1]--
			}
		}
		if isIdentByte(c) {
			if !isIdentByte(l.last) || l.sawSpace {
				l.word = ""
				l.prop = l.last == '.'
			}
			l.word += string(c)
		} else {
			l.word = ""
		}
		l.last, l.sawSpace = c, false
	}
}

// operand records that the code before the next token ends in a value,
// so a slash there is a division.
func (l *jsLexer) operand() {
	l.last, l.word, l.sawSpace, l.prop = 'a', "", false, false
}

func (l *jsLexer) regexAllowed() bool {
	if l.last == 0 {
		return true
	}
	if isIdentByte(l.last) {
		return !l.prop && jsRegexAfterWord[l.word]
	}
	return strings.IndexByte("(,=:[!&|?{};+-*%<>~^", l.last) >= 0
}

// endsInOperand says the code before the current token ends in a value:
// an identifier that is not a keyword, a ), or a ].
func (l *jsLexer) endsInOperand() bool {
	if isIdentByte(l.last) {
		return l.prop || !jsRegexAfterWord[l.word]
	}
	return l.last == ')' || l.last == ']'
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// place says where a #{} at the current position sits, and refuses a
// position where a value has no meaning: a comment and a regular
// expression. The value counts as an operand for the next token. A
// value in a quoted string marks the string for a template literal.
func (l *jsLexer) place() (filterPlace, error) {
	switch l.state {
	case jsCode:
		l.operand()
		return placeJSValue, nil
	case jsDouble, jsSingle:
		if l.withValue == nil {
			l.withValue = map[int]bool{}
		}
		l.withValue[l.strs] = true
		return placeJSString, nil
	case jsTemplate:
		return placeJSString, nil
	case jsLineComment, jsBlockComment:
		return placeNone, fmt.Errorf("in a comment: move the value out of the comment")
	}
	return placeNone, fmt.Errorf("in a regular expression: build the pattern in Go, or pass the value as a string to new RegExp")
}

// endLine ends a line comment, and refuses a line that ends inside a
// quoted string or a regular expression. Each filter line is one
// source line. JavaScript continues a quoted string to the next line
// only after a backslash, and never continues a regular expression.
// So a line that ends in either shows that the lexer read the script
// in a way that the browser does not.
func (l *jsLexer) endLine(line string) error {
	l.codeOnLine = false
	switch l.state {
	case jsLineComment:
		l.state = jsCode
	case jsDouble, jsSingle:
		if n := len(line) - len(strings.TrimRight(line, `\`)); n%2 == 0 {
			return fmt.Errorf("ends inside a string: close the string on this line, or end the line with \\. A regular expression that holds a quote can cause this: build it with new RegExp")
		}
	case jsRegex, jsRegexClass:
		return fmt.Errorf("ends inside a regular expression: close it on this line. A division that the lexer reads as a regular expression can cause this: put a space or ( ) around the division")
	}
	return nil
}

// compileInterp splits interpolated text into literal and expression
// segments, parsing each #{} once. It is the parse half of the old
// interpolate.
func compileInterp(text string) ([]interpSeg, error) {
	var segs []interpSeg
	var lit strings.Builder
	i := 0
	for i < len(text) {
		if text[i] == '#' && i+1 < len(text) && text[i+1] == '{' {
			if lit.Len() > 0 {
				segs = append(segs, interpSeg{lit: lit.String()})
				lit.Reset()
			}
			depth := 0
			j := i + 1
			for j < len(text) {
				if text[j] == '{' {
					depth++
				} else if text[j] == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
				j++
			}
			ast, err := compileExpr(text[i+2 : j])
			if err != nil {
				return nil, err
			}
			segs = append(segs, interpSeg{expr: ast})
			i = j + 1
		} else {
			lit.WriteByte(text[i])
			i++
		}
	}
	if lit.Len() > 0 {
		segs = append(segs, interpSeg{lit: lit.String()})
	}
	return segs, nil
}

// evalInterp renders compiled interpolation segments, stringifying each value
// with stringify (nil renders empty) so text interpolation agrees with the =
// output path. stringify also skips reflection for common scalars.
func evalInterp(segs []interpSeg, ctx context, escape bool) (string, error) {
	// One segment needs no buffer: a literal is returned as is, and a
	// lone expression is stringified and escaped in place.
	if len(segs) == 1 {
		s := segs[0]
		if s.expr == nil {
			return s.lit, nil
		}
		val, err := evaluate(s.expr, ctx)
		if err != nil {
			return "", err
		}
		str := stringify(val)
		if escape {
			str = escapeHTML(str)
		}
		return str, nil
	}
	var buf strings.Builder
	for _, s := range segs {
		if s.expr == nil {
			buf.WriteString(s.lit)
			continue
		}
		val, err := evaluate(s.expr, ctx)
		if err != nil {
			return "", err
		}
		if escape {
			writeEscapedHTML(&buf, stringify(val))
		} else {
			buf.WriteString(stringify(val))
		}
	}
	return buf.String(), nil
}

// attrVal is one evaluated attribute, plus the provenance the attribute
// policy needs (see policyAttr): whether the value is source a template
// author wrote rather than data that arrived, and which trust type, if
// any, the value carried.
//
// authored is asked of the value, not of the AST node at the attribute.
// The two agree on an inline literal and part company one step away: a
// literal passed to a partial and splatted onto a tag there has no AST
// node here to ask about.
type attrVal struct {
	key      string
	val      string
	authored bool
	trust    trust
}

// trust records a SafeJS/SafeCSS assertion made by the handler.
type trust int

const (
	trustNone trust = iota
	trustJS
	trustCSS
)

// evalAttrs evaluates compiled attribute pairs into stringified
// key/value pairs. It matches the eval half of the old parseHash,
// including splat expansion and toAttrVal sentinel encoding.
func evalAttrs(result []attrVal, attrs []attr, ctx context) ([]attrVal, error) {
	for _, a := range attrs {
		val, err := evaluate(a.val, ctx)
		if err != nil {
			return nil, err
		}
		if a.splat {
			m, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("splat requires a map, got %s", typeName(val))
			}
			result = slices.Grow(result, len(m))
			for k, v := range m {
				if !validAttrName(k) {
					return nil, fmt.Errorf("splat key %q is not an attribute name: use ASCII letters, digits, -, _, :, or .", k)
				}
				result = append(result, attrVal{
					key:      k,
					val:      toAttrVal(v),
					authored: isAuthored(v),
					trust:    valTrust(v),
				})
			}
			continue
		}
		result = append(result, attrVal{
			key:      a.key,
			val:      toAttrVal(val),
			authored: isAuthored(val),
			trust:    valTrust(val),
		})
	}
	return result, nil
}

func valTrust(v any) trust {
	switch v.(type) {
	case SafeJS:
		return trustJS
	case SafeCSS:
		return trustCSS
	}
	return trustNone
}

// evalLocals evaluates compiled render args into a typed locals map,
// preserving Go types (string, int64, bool, nil, map, slice) rather than
// stringifying like evalAttrs. It is the eval half of the old parseLocals,
// including **splat expansion.
func evalLocals(attrs []attr, ctx context) (map[string]any, error) {
	result := make(map[string]any, len(attrs))
	for _, a := range attrs {
		val, err := evaluate(a.val, ctx)
		if err != nil {
			return nil, err
		}
		if a.splat {
			m, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("splat requires a map, got %s", typeName(val))
			}
			maps.Copy(result, m)
			continue
		}
		result[a.key] = val
	}
	return result, nil
}
