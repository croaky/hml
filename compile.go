package hml

import (
	"fmt"
	"maps"
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
// (expr == nil) or an expression to evaluate and stringify.
type interpSeg struct {
	lit  string
	expr *astNode
}

// compileNodes pre-parses every expression, attribute hash, and text
// interpolation in the tree into ASTs once, so Render never has to
// re-tokenize or re-parse. It mutates nodes in place and recurses into
// children. Expression syntax errors now surface here, at Parse time —
// the parser stays the linter.
func compileNodes(nodes []node, path string, transforms map[string]Transform) error {
	for i := range nodes {
		n := &nodes[i]
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
		case kindIf, kindElsif, kindFor:
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
		case kindRender:
			if err := compileRender(n); err != nil {
				return fmt.Errorf("%s: render %q: %w", path, n.text, err)
			}
		case kindFilter:
			segs := make([][]interpSeg, len(n.filterLines))
			for j, line := range n.filterLines {
				s, err := compileInterp(stripFilterIndent(line, n.indent))
				if err != nil {
					return fmt.Errorf("%s: filter interpolation: %w", path, err)
				}
				segs[j] = s
			}
			n.filterSegs = segs
		}
		if err := compileNodes(n.children, path, transforms); err != nil {
			return err
		}
	}
	return nil
}

// neverBool names the literal kind of a condition that cannot be a bool
// whatever the locals hold, and "" for one that might be. A condition
// is checked at render time too, because most of them read a field
// whose type only the handler knows; this catches the ones the AST
// already settles, at Parse, where the parser is the linter.
//
// The point is the branch nobody exercises. `- elsif title || "Untitled"`
// is wrong the moment it is written, but a render-time check reports it
// only when that branch is reached -- which, for an elsif, may be in
// production and not in a test.
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
		str := stringify(val)
		if escape {
			str = escapeHTML(str)
		}
		buf.WriteString(str)
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
func evalAttrs(attrs []attr, ctx context) ([]attrVal, error) {
	var result []attrVal
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
			for k, v := range m {
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
