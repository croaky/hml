package hml

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// --------------------------------------------------------------------
// Tokens
// --------------------------------------------------------------------

type tokenType int

const (
	tokOp tokenType = iota
	tokDot
	tokComma
	tokColon
	tokLBrace
	tokRBrace
	tokLParen
	tokRParen
	tokLBracket
	tokRBracket
	tokSplat
	tokString       // plain string literal
	tokInterpString // string with #{} segments
	tokNumber
	tokBool
	tokNil
	tokIdent
)

type token struct {
	typ tokenType
	str string // raw text for ops/ident/string
	num int64  // for tokNumber
	b   bool   // for tokBool
	seg []seg  // for tokInterpString
}

type segKind int

const (
	segStr    segKind = iota
	segInterp         // expression inside #{}
)

type seg struct {
	kind segKind
	text string   // for segStr
	expr *astNode // for segInterp, parsed once at tokenize time
}

// tokenize converts an expression string into tokens.
func tokenize(src string) ([]token, error) {
	var tokens []token
	i := 0
	for i < len(src) {
		ch := src[i]

		// whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}

		// two-char operators
		if i+1 < len(src) {
			two := src[i : i+2]
			switch two {
			case "==", "!=", "&&", "||", ">=", "<=":
				tokens = append(tokens, token{typ: tokOp, str: two})
				i += 2
				continue
			case "**":
				tokens = append(tokens, token{typ: tokSplat, str: "**"})
				i += 2
				continue
			}
		}

		// single-char
		switch ch {
		case '!':
			tokens = append(tokens, token{typ: tokOp, str: "!"})
			i++
			continue
		case '>', '<':
			tokens = append(tokens, token{typ: tokOp, str: string(ch)})
			i++
			continue
		case '.':
			tokens = append(tokens, token{typ: tokDot})
			i++
			continue
		case ',':
			tokens = append(tokens, token{typ: tokComma})
			i++
			continue
		case ':':
			tokens = append(tokens, token{typ: tokColon})
			i++
			continue
		case '{':
			tokens = append(tokens, token{typ: tokLBrace})
			i++
			continue
		case '}':
			tokens = append(tokens, token{typ: tokRBrace})
			i++
			continue
		case '(':
			tokens = append(tokens, token{typ: tokLParen})
			i++
			continue
		case ')':
			tokens = append(tokens, token{typ: tokRParen})
			i++
			continue
		case '[':
			tokens = append(tokens, token{typ: tokLBracket})
			i++
			continue
		case ']':
			tokens = append(tokens, token{typ: tokRBracket})
			i++
			continue
		}

		// double-quoted string
		if ch == '"' {
			tok, ni, err := tokenizeDoubleString(src, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = ni
			continue
		}

		// single-quoted string
		if ch == '\'' {
			tok, ni, err := tokenizeSingleString(src, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = ni
			continue
		}

		// number
		if ch >= '0' && ch <= '9' {
			start := i
			i++
			for i < len(src) && src[i] >= '0' && src[i] <= '9' {
				i++
			}
			n, _ := strconv.ParseInt(src[start:i], 10, 64)
			tokens = append(tokens, token{typ: tokNumber, num: n})
			continue
		}

		// negative number
		if ch == '-' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9' {
			start := i
			i++
			for i < len(src) && src[i] >= '0' && src[i] <= '9' {
				i++
			}
			n, _ := strconv.ParseInt(src[start:i], 10, 64)
			tokens = append(tokens, token{typ: tokNumber, num: n})
			continue
		}

		// identifier or keyword
		if isIdentStart(ch) {
			start := i
			i++
			for i < len(src) && isIdentChar(src[i]) {
				i++
			}
			word := src[start:i]
			switch word {
			case "true":
				tokens = append(tokens, token{typ: tokBool, b: true})
			case "false":
				tokens = append(tokens, token{typ: tokBool, b: false})
			case "nil":
				tokens = append(tokens, token{typ: tokNil})
			default:
				tokens = append(tokens, token{typ: tokIdent, str: word})
			}
			continue
		}

		return nil, fmt.Errorf("unexpected character %q in expression", ch)
	}
	return tokens, nil
}

func tokenizeDoubleString(src string, start int) (token, int, error) {
	i := start + 1 // skip "
	var segments []seg
	var buf strings.Builder

	for i < len(src) {
		ch := src[i]

		if ch == '\\' && i+1 < len(src) {
			esc := src[i+1]
			switch esc {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case '\\':
				buf.WriteByte('\\')
			case '"':
				buf.WriteByte('"')
			case '#':
				buf.WriteByte('#')
			case '\'':
				buf.WriteByte('\'')
			default:
				buf.WriteByte('\\')
				buf.WriteByte(esc)
			}
			i += 2
			continue
		}

		if ch == '#' && i+1 < len(src) && src[i+1] == '{' {
			if buf.Len() > 0 {
				segments = append(segments, seg{kind: segStr, text: buf.String()})
				buf.Reset()
			}
			depth := 0
			j := i + 1
			for j < len(src) {
				if src[j] == '{' {
					depth++
				} else if src[j] == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
				j++
			}
			exprSrc := src[i+2 : j]
			toks, err := tokenize(exprSrc)
			if err != nil {
				return token{}, 0, err
			}
			// Parse the interpolation once here (Parse time) rather than
			// re-parsing the tokens on every render.
			p := &parser{tokens: toks}
			expr, err := p.parseValue()
			if err != nil {
				return token{}, 0, err
			}
			segments = append(segments, seg{kind: segInterp, expr: expr})
			i = j + 1
			continue
		}

		if ch == '"' {
			if len(segments) == 0 {
				return token{typ: tokString, str: buf.String()}, i + 1, nil
			}
			if buf.Len() > 0 {
				segments = append(segments, seg{kind: segStr, text: buf.String()})
			}
			return token{typ: tokInterpString, seg: segments}, i + 1, nil
		}

		buf.WriteByte(ch)
		i++
	}
	return token{}, 0, fmt.Errorf("unterminated string starting at position %d", start)
}

func tokenizeSingleString(src string, start int) (token, int, error) {
	i := start + 1
	var buf strings.Builder
	for i < len(src) {
		ch := src[i]
		if ch == '\\' && i+1 < len(src) {
			esc := src[i+1]
			switch esc {
			case '\\':
				buf.WriteByte('\\')
			case '\'':
				buf.WriteByte('\'')
			default:
				buf.WriteByte('\\')
				buf.WriteByte(esc)
			}
			i += 2
			continue
		}
		if ch == '\'' {
			return token{typ: tokString, str: buf.String()}, i + 1, nil
		}
		buf.WriteByte(ch)
		i++
	}
	return token{}, 0, fmt.Errorf("unterminated single-quoted string starting at position %d", start)
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

// --------------------------------------------------------------------
// AST nodes
// --------------------------------------------------------------------

type nodeType int

const (
	astString nodeType = iota
	astInterpString
	astNumber
	astBool
	astNil
	astField
	astCmp
	astAnd
	astOr
	astNot
	astHash
	astArray
	astCall
)

type astNode struct {
	typ      nodeType
	str      string     // for astString
	segments []seg      // for astInterpString
	num      int64      // for astNumber
	b        bool       // for astBool
	parts    []string   // for astField, astCall
	op       string     // for astCmp
	left     *astNode   // for astCmp, astAnd, astOr
	right    *astNode   // for astCmp, astAnd, astOr
	operand  *astNode   // for astNot
	pairs    []kv       // for astHash
	elements []*astNode // for astArray
	args     []*astNode // for astCall
}

type kv struct {
	key string
	val *astNode
}

// --------------------------------------------------------------------
// Parser
// --------------------------------------------------------------------

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) atEnd() bool { return p.pos >= len(p.tokens) }
func (p *parser) peek() *token {
	if p.pos >= len(p.tokens) {
		return nil
	}
	return &p.tokens[p.pos]
}
func (p *parser) advance() token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *parser) parseExpr() (*astNode, error) {
	if call, ok, err := p.tryParseTopLevelCall(); ok || err != nil {
		return call, err
	}
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if !p.atEnd() {
		return nil, fmt.Errorf("unexpected token %q after expression", p.peek().str)
	}
	return node, nil
}

func (p *parser) tryParseTopLevelCall() (*astNode, bool, error) {
	start := p.pos
	if p.peek() == nil || p.peek().typ != tokIdent {
		return nil, false, nil
	}

	call, err := p.parseCallMaybe()
	if err != nil {
		p.pos = start
		return nil, false, nil
	}
	if call == nil {
		p.pos = start
		return nil, false, nil
	}
	if !p.atEnd() {
		p.pos = start
		return nil, false, nil
	}
	return call, true, nil
}

func (p *parser) parseCallMaybe() (*astNode, error) {
	if p.peek() == nil || p.peek().typ != tokIdent {
		return nil, nil
	}
	parts := []string{p.advance().str}
	for p.peek() != nil && p.peek().typ == tokDot {
		p.advance()
		t := p.peek()
		if t == nil || t.typ != tokIdent {
			return nil, fmt.Errorf("expected identifier after '.'")
		}
		parts = append(parts, p.advance().str)
	}

	if p.peek() != nil && p.peek().typ == tokLParen {
		p.advance()
		args, err := p.parseCallArgs(tokRParen)
		if err != nil {
			return nil, err
		}
		if p.peek() == nil || p.peek().typ != tokRParen {
			return nil, fmt.Errorf("expected ')' to close call")
		}
		p.advance()
		return &astNode{typ: astCall, parts: parts, args: args}, nil
	}

	if !isCallArgStartToken(p.peek()) {
		return nil, nil
	}
	args, err := p.parseCallArgs(tokenType(-1))
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, nil
	}
	return &astNode{typ: astCall, parts: parts, args: args}, nil
}

func (p *parser) parseCallArgs(stop tokenType) ([]*astNode, error) {
	args := []*astNode{}
	kwPairs := []kv{}

	for !p.atEnd() {
		if stop >= 0 && p.peek() != nil && p.peek().typ == stop {
			break
		}
		if looksLikeKwPairStart(p) {
			keyTok := p.advance()
			key := keyTok.str
			p.advance() // :
			val, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			kwPairs = append(kwPairs, kv{key: key, val: val})
		} else {
			arg, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		if p.peek() != nil && p.peek().typ == tokComma {
			p.advance()
			continue
		}
		break
	}
	if len(kwPairs) > 0 {
		args = append(args, &astNode{typ: astHash, pairs: kwPairs})
	}
	return args, nil
}

func looksLikeKwPairStart(p *parser) bool {
	if p.peek() == nil {
		return false
	}
	switch p.peek().typ {
	case tokIdent, tokString:
		if p.pos+1 < len(p.tokens) {
			return p.tokens[p.pos+1].typ == tokColon
		}
	}
	return false
}

func isCallArgStartToken(t *token) bool {
	if t == nil {
		return false
	}
	switch t.typ {
	case tokString, tokInterpString, tokNumber, tokBool, tokNil, tokIdent, tokLParen, tokLBracket, tokLBrace:
		return true
	default:
		return false
	}
}

func evalCall(node *astNode, ctx context) (any, error) {
	callee, err := evalField(node.parts, ctx)
	if err != nil {
		return nil, err
	}
	args := make([]any, 0, len(node.args))
	for _, argNode := range node.args {
		arg, err := evaluate(argNode, ctx)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	switch fn := callee.(type) {
	case func(...any) (any, error):
		return fn(args...)
	case func(...any) any:
		return fn(args...), nil
	default:
		return nil, fmt.Errorf("cannot call %T as function", callee)
	}
}

func (p *parser) parseValue() (*astNode, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (*astNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() != nil && p.peek().typ == tokOp && p.peek().str == "||" {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &astNode{typ: astOr, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (*astNode, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.peek() != nil && p.peek().typ == tokOp && p.peek().str == "&&" {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &astNode{typ: astAnd, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseNot() (*astNode, error) {
	if p.peek() != nil && p.peek().typ == tokOp && p.peek().str == "!" {
		p.advance()
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &astNode{typ: astNot, operand: operand}, nil
	}
	return p.parseCmp()
}

func (p *parser) parseCmp() (*astNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t != nil && t.typ == tokOp {
		switch t.str {
		case "==", "!=", ">", "<", ">=", "<=":
			op := p.advance().str
			right, err := p.parsePrimary()
			if err != nil {
				return nil, err
			}
			return &astNode{typ: astCmp, op: op, left: left, right: right}, nil
		}
	}
	return left, nil
}

func (p *parser) parsePrimary() (*astNode, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("unexpected end of expression")
	}

	switch t.typ {
	case tokString:
		p.advance()
		return &astNode{typ: astString, str: t.str}, nil
	case tokInterpString:
		p.advance()
		return &astNode{typ: astInterpString, segments: t.seg}, nil
	case tokNumber:
		p.advance()
		return &astNode{typ: astNumber, num: t.num}, nil
	case tokBool:
		p.advance()
		return &astNode{typ: astBool, b: t.b}, nil
	case tokNil:
		p.advance()
		return &astNode{typ: astNil}, nil
	case tokLParen:
		p.advance()
		node, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek() == nil || p.peek().typ != tokRParen {
			return nil, fmt.Errorf("expected ')' after grouped expression")
		}
		p.advance()
		return node, nil
	case tokLBracket:
		return p.parseArrayLiteral()
	case tokLBrace:
		return p.parseHashLiteral()
	case tokIdent:
		return p.parseFieldAccess()
	default:
		return nil, fmt.Errorf("unexpected token %q", t.str)
	}
}

func (p *parser) parseArrayLiteral() (*astNode, error) {
	p.advance() // [
	var elements []*astNode
	for p.peek() != nil && p.peek().typ != tokRBracket {
		el, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		elements = append(elements, el)
		if p.peek() != nil && p.peek().typ == tokComma {
			p.advance()
		}
	}
	if p.peek() == nil || p.peek().typ != tokRBracket {
		return nil, fmt.Errorf("expected ']' to close array literal")
	}
	p.advance()
	return &astNode{typ: astArray, elements: elements}, nil
}

func (p *parser) parseHashLiteral() (*astNode, error) {
	p.advance() // {
	var pairs []kv
	for p.peek() != nil && p.peek().typ != tokRBrace {
		t := p.peek()
		var key string
		switch t.typ {
		case tokIdent:
			key = p.advance().str
		case tokString:
			key = p.advance().str
		default:
			return nil, fmt.Errorf("expected key in hash literal, got %q", t.str)
		}
		if p.peek() == nil || p.peek().typ != tokColon {
			return nil, fmt.Errorf("expected ':' after key %q", key)
		}
		p.advance()
		val, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, kv{key: key, val: val})
		if p.peek() != nil && p.peek().typ == tokComma {
			p.advance()
		}
	}
	if p.peek() == nil || p.peek().typ != tokRBrace {
		return nil, fmt.Errorf("expected '}' to close hash literal")
	}
	p.advance()
	return &astNode{typ: astHash, pairs: pairs}, nil
}

func (p *parser) parseFieldAccess() (*astNode, error) {
	name := p.advance().str
	parts := []string{name}
	for p.peek() != nil && p.peek().typ == tokDot {
		p.advance()
		t := p.peek()
		if t == nil || t.typ != tokIdent {
			return nil, fmt.Errorf("expected identifier after '.'")
		}
		parts = append(parts, p.advance().str)
	}

	if p.peek() != nil && p.peek().typ == tokLParen {
		p.advance()
		args, err := p.parseCallArgs(tokRParen)
		if err != nil {
			return nil, err
		}
		if p.peek() == nil || p.peek().typ != tokRParen {
			return nil, fmt.Errorf("expected ')' to close call")
		}
		p.advance()
		return &astNode{typ: astCall, parts: parts, args: args}, nil
	}

	return &astNode{typ: astField, parts: parts}, nil
}

// --------------------------------------------------------------------
// Evaluator
// --------------------------------------------------------------------

// Context holds template locals as a layered lookup chain. Each partial or
// loop iteration overlays a small vars map on its parent rather than copying
// the parent's entries, so a top-level name resolves by reading through the
// chain (nearest overlay first, then parent, up to the root). This trades an
// O(parent width) copy per partial for an O(chain depth) walk, which is a win
// because partial chains are shallow while parent locals maps are wide.
// Field access within a value resolves dot-separated names against nested
// maps and structs.
type Context struct {
	vars   map[string]any
	parent *Context
}

// context aliases *Context so existing `ctx context` signatures stay put.
type context = *Context

// NewContext builds a root context from a locals map. A nil map is treated as
// empty.
func NewContext(vars map[string]any) *Context {
	if vars == nil {
		vars = map[string]any{}
	}
	return &Context{vars: vars}
}

// Child overlays vars on the receiver without copying the parent. A nil
// overlay is treated as empty. Child bindings shadow parent bindings.
func (c *Context) Child(vars map[string]any) *Context {
	if vars == nil {
		vars = map[string]any{}
	}
	return &Context{vars: vars, parent: c}
}

// lookup resolves a top-level name by walking the chain from the receiver to
// the root, returning the nearest binding.
func (c *Context) lookup(key string) (any, bool) {
	for c != nil {
		if v, ok := c.vars[key]; ok {
			return v, true
		}
		c = c.parent
	}
	return nil, false
}

// evaluate resolves an AST node against a context.
func evaluate(node *astNode, ctx context) (any, error) {
	switch node.typ {
	case astString:
		return node.str, nil
	case astInterpString:
		return evalInterpString(node.segments, ctx)
	case astNumber:
		return node.num, nil
	case astBool:
		return node.b, nil
	case astNil:
		return nil, nil
	case astField:
		return evalField(node.parts, ctx)
	case astCall:
		return evalCall(node, ctx)
	case astHash:
		result := make(map[string]any)
		for _, p := range node.pairs {
			v, err := evaluate(p.val, ctx)
			if err != nil {
				return nil, err
			}
			result[p.key] = v
		}
		return result, nil
	case astArray:
		result := make([]any, len(node.elements))
		for i, el := range node.elements {
			v, err := evaluate(el, ctx)
			if err != nil {
				return nil, err
			}
			result[i] = v
		}
		return result, nil
	case astCmp:
		return evalCmp(node, ctx)
	case astAnd:
		left, err := evaluate(node.left, ctx)
		if err != nil {
			return nil, err
		}
		if !truthy(left) {
			return left, nil
		}
		return evaluate(node.right, ctx)
	case astOr:
		left, err := evaluate(node.left, ctx)
		if err != nil {
			return nil, err
		}
		if truthy(left) {
			return left, nil
		}
		return evaluate(node.right, ctx)
	case astNot:
		val, err := evaluate(node.operand, ctx)
		if err != nil {
			return nil, err
		}
		return !truthy(val), nil
	default:
		return nil, fmt.Errorf("unknown AST node type: %d", node.typ)
	}
}

func evalCmp(node *astNode, ctx context) (any, error) {
	left, err := evaluate(node.left, ctx)
	if err != nil {
		return nil, err
	}
	right, err := evaluate(node.right, ctx)
	if err != nil {
		return nil, err
	}
	switch node.op {
	case "==":
		return equal(left, right), nil
	case "!=":
		return !equal(left, right), nil
	case ">", "<", ">=", "<=":
		ln, lok := toFloat(left)
		rn, rok := toFloat(right)
		if !lok || !rok {
			return nil, fmt.Errorf("cannot compare %v %s %v", left, node.op, right)
		}
		switch node.op {
		case ">":
			return ln > rn, nil
		case "<":
			return ln < rn, nil
		case ">=":
			return ln >= rn, nil
		case "<=":
			return ln <= rn, nil
		}
	}
	return nil, fmt.Errorf("unknown operator %q", node.op)
}

// equal compares two values. Values must be the same type to be equal (no
// cross-type coercion). Numeric types (int, int64, float64) are compared by
// numeric value.
func equal(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	an, aNum := toFloat(a)
	bn, bNum := toFloat(b)
	if aNum && bNum {
		return an == bn
	}
	switch av := a.(type) {
	case string:
		if bv, ok := b.(string); ok {
			return av == bv
		}
	case bool:
		if bv, ok := b.(bool); ok {
			return av == bv
		}
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

// evalField resolves a dot-separated field path against the context.
// Supports nested map[string]any access.
func evalField(parts []string, ctx context) (any, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty field access")
	}
	val, ok := ctx.lookup(parts[0])
	if !ok {
		return nil, fmt.Errorf("undefined variable %q", parts[0])
	}
	for _, part := range parts[1:] {
		if m, ok := val.(map[string]any); ok {
			val, ok = m[part]
			if !ok {
				return nil, fmt.Errorf("undefined field %q", part)
			}
		} else {
			// Fallback to reflection for Go structs (using json or db tag, or field name)
			rv := reflect.ValueOf(val)
			if rv.Kind() == reflect.Pointer {
				rv = rv.Elem()
			}
			if rv.Kind() != reflect.Struct {
				return nil, fmt.Errorf("cannot access %q on %T", part, val)
			}
			idx, ok := structFieldIndex(rv.Type(), part)
			if !ok {
				return nil, fmt.Errorf("undefined field %q on struct %T", part, val)
			}
			val = rv.Field(idx).Interface()
		}
	}
	return val, nil
}

// fieldIndexCache memoizes the template-name -> struct-field-index lookup per
// reflect.Type. evalField's reflection fallback otherwise rescans NumField and
// re-parses json/db struct tags on every field access, which dominates loops
// over []struct. The name set a template can request is small and fixed, so an
// unbounded cache per type is safe.
var fieldIndexCache sync.Map // map[reflect.Type]map[string]int

// structFieldIndex resolves a template field name to a struct field index,
// matching by case-insensitive field name or by json/db tag (first segment).
func structFieldIndex(rt reflect.Type, name string) (int, bool) {
	m, ok := fieldIndexCache.Load(rt)
	if !ok {
		m, _ = fieldIndexCache.LoadOrStore(rt, buildFieldIndex(rt))
	}
	fields := m.(map[string]int)
	// Exact match covers json/db tags; lower-case match covers the
	// original EqualFold field-name comparison.
	if idx, ok := fields[name]; ok {
		return idx, true
	}
	idx, ok := fields[strings.ToLower(name)]
	return idx, ok
}

// buildFieldIndex maps every name a template might use (lower-cased field
// name, json tag, db tag) to that field's index. Earlier fields win on
// collision, matching the original first-match-wins scan order.
// Unexported fields are left out: reflect.Value.Interface panics on one, so
// indexing it would turn a template typo into a dead handler. Out of the
// index, the name misses and evalField reports an undefined field instead.
func buildFieldIndex(rt reflect.Type) map[string]int {
	m := make(map[string]int)
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		if _, ok := m[strings.ToLower(field.Name)]; !ok {
			m[strings.ToLower(field.Name)] = i
		}
		if jsonTag, _, _ := strings.Cut(field.Tag.Get("json"), ","); jsonTag != "" {
			if _, ok := m[jsonTag]; !ok {
				m[jsonTag] = i
			}
		}
		if dbTag, _, _ := strings.Cut(field.Tag.Get("db"), ","); dbTag != "" {
			if _, ok := m[dbTag]; !ok {
				m[dbTag] = i
			}
		}
	}
	return m
}

// evalInterpString renders an interpolated string literal ("a #{b}") from
// segments whose expressions were parsed at tokenize time, stringifying each
// value with stringify so nil renders empty, matching text interpolation and
// the = output path.
func evalInterpString(segments []seg, ctx context) (string, error) {
	var buf strings.Builder
	for _, s := range segments {
		switch s.kind {
		case segStr:
			buf.WriteString(s.text)
		case segInterp:
			val, err := evaluate(s.expr, ctx)
			if err != nil {
				return "", err
			}
			buf.WriteString(stringify(val))
		}
	}
	return buf.String(), nil
}

// truthy follows these semantics: nil, false, and boxed nil values (such as nil
// pointers, slices, or maps) are falsy; everything else (including 0 and "") is
// truthy. Common scalar types are handled before falling back to reflection,
// which is the only way to detect a boxed nil.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string, int, int64, float64:
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func, reflect.Interface:
		if rv.IsNil() {
			return false
		}
	}
	return true
}

// toAttrVal converts a Go value to an HTML attribute value string.
// Booleans and nil need special handling in HTML attributes: true
// means "emit the attribute name with no value" (e.g. <input checked>),
// false/nil means "omit the attribute entirely". We encode these as
// NUL-prefixed sentinel strings ("\x00true", "\x00false", "\x00nil")
// that renderTag checks when building the attribute string. The NUL
// prefix makes accidental collision with real attribute values
// impossible.
func toAttrVal(v any) string {
	switch x := v.(type) {
	case nil:
		return "\x00nil"
	case bool:
		if x {
			return "\x00true"
		}
		return "\x00false"
	case string:
		return x
	case SafeString:
		return string(x)
	case SafeJS:
		return string(x)
	case SafeCSS:
		return string(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}
