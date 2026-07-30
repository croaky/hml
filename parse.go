package hml

import (
	"fmt"
	"regexp"
	"strings"
)

// joinContinuationLines joins lines with unclosed braces (multi-line attribute
// hashes), skipping filter blocks where trailing commas are JS/CSS.
func joinContinuationLines(lines []string) []string {
	var result []string
	i := 0
	inFilter := false
	filterIndent := 0
	for i < len(lines) {
		line := lines[i]
		stripped := strings.TrimLeft(line, " \t")

		if stripped == ":javascript" || stripped == ":css" {
			inFilter = true
			filterIndent = len(line) - len(stripped)
			result = append(result, line)
			i++
			continue
		}

		if inFilter {
			if stripped == "" {
				result = append(result, line)
				i++
				continue
			}
			indent := len(line) - len(stripped)
			if indent <= filterIndent {
				inFilter = false
			} else {
				result = append(result, line)
				i++
				continue
			}
		}

		for unclosedBraces(line) && i+1 < len(lines) {
			i++
			line = line + "\n" + lines[i]
		}
		result = append(result, line)
		i++
	}
	return result
}

func unclosedBraces(line string) bool {
	depth := 0
	for _, ch := range line {
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
		}
	}
	return depth > 0
}

func parseLines(lines []string, baseIndent, from, to int, path string) ([]node, error) {
	var nodes []node
	i := from
	for i < to {
		line := lines[i]
		stripped := strings.TrimLeft(line, " \t")
		indent := len(line) - len(stripped)

		if stripped == "" {
			i++
			continue
		}

		if indent != baseIndent {
			return nil, fmt.Errorf("%s:%d: expected indent %d, got %d", path, i+1, baseIndent, indent)
		}

		// find children
		childEnd := i + 1
		for childEnd < to {
			nextLine := lines[childEnd]
			nextStripped := strings.TrimLeft(nextLine, " \t")
			if nextStripped == "" {
				childEnd++
				continue
			}
			nextIndent := len(nextLine) - len(nextStripped)
			if nextIndent <= indent {
				break
			}
			childEnd++
		}

		n, err := parseLine(stripped, indent, lines, i+1, childEnd, path)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
		i = childEnd
	}
	return nodes, nil
}

func parseLine(stripped string, indent int, lines []string, childFrom, childTo int, path string) (node, error) {
	// Doctype
	if stripped == "!!!" {
		return node{kind: kindDoctype, indent: indent}, nil
	}

	// Comment
	if strings.HasPrefix(stripped, "-#") {
		return node{kind: kindComment, indent: indent}, nil
	}

	// Filter
	if stripped == ":javascript" || stripped == ":css" {
		name := stripped[1:]
		var filterLines []string
		for j := childFrom; j < childTo; j++ {
			filterLines = append(filterLines, lines[j])
		}
		return node{kind: kindFilter, filterName: name, filterLines: filterLines, indent: indent}, nil
	}

	// Control: - if / - elsif / - else / - for ... in
	if strings.HasPrefix(stripped, "- ") {
		control := strings.TrimSpace(stripped[2:])
		if strings.HasPrefix(control, "if ") {
			children, err := parseLines(lines, indent+2, childFrom, childTo, path)
			if err != nil {
				return node{}, err
			}
			return node{kind: kindIf, expr: control[3:], indent: indent, children: children}, nil
		}
		if strings.HasPrefix(control, "elsif ") {
			children, err := parseLines(lines, indent+2, childFrom, childTo, path)
			if err != nil {
				return node{}, err
			}
			return node{kind: kindElsif, expr: control[6:], indent: indent, children: children}, nil
		}
		if control == "else" {
			children, err := parseLines(lines, indent+2, childFrom, childTo, path)
			if err != nil {
				return node{}, err
			}
			return node{kind: kindElse, indent: indent, children: children}, nil
		}
		// for item in collection (optional index: for i, item in collection)
		if forMatch := parseFor(control); forMatch != nil {
			children, err := parseLines(lines, indent+2, childFrom, childTo, path)
			if err != nil {
				return node{}, err
			}
			return node{kind: kindFor, indexVar: forMatch[0], elemVar: forMatch[1], expr: forMatch[2], indent: indent, children: children}, nil
		}
		return node{}, fmt.Errorf("%s: unsupported control: %s", path, stripped)
	}

	// Render partial
	if strings.HasPrefix(stripped, "= render ") {
		return node{kind: kindRender, text: strings.TrimSpace(stripped[2:]), indent: indent}, nil
	}

	// A parenthesized call: = markdown(body), = commit_url(id, sha).
	// Which one it is depends on the app-registered transforms, which
	// this does not have, so compileNodes decides: a registered name is
	// a transform, anything else is a helper func injected as a local.
	//
	// Deciding here is what made the two positions disagree. An
	// attribute value has always resolved a parenthesized call through
	// the expression parser, so `href: url(id)` worked while `= url(id)`
	// was rejected as an unknown transform. One syntax, one meaning,
	// wherever it appears.
	if m := callRE.FindStringSubmatch(stripped); m != nil {
		return node{
			kind:     kindCall,
			callName: m[1],
			expr:     strings.TrimSpace(m[2]),
			text:     stripped[2:],
			indent:   indent,
		}, nil
	}

	// Raw output was removed: = honors SafeString for renderer-built
	// HTML, and the transforms render rich text from source.
	if strings.HasPrefix(stripped, "!= ") {
		return node{}, fmt.Errorf("%s: raw output (!=) is not supported; use = with a transform or SafeString", path)
	}

	// Escaped output
	if strings.HasPrefix(stripped, "= ") {
		return node{kind: kindOutput, text: stripped[2:], indent: indent}, nil
	}

	// Tag
	if stripped[0] == '%' || stripped[0] == '.' || (stripped[0] == '#' && (len(stripped) == 1 || stripped[1] != '{')) {
		return parseTag(stripped, indent, lines, childFrom, childTo, path)
	}

	// Static text
	return node{kind: kindText, text: stripped, indent: indent}, nil
}

// forRE matches `for item in collection` and the indexed form
// `for i, item in collection`. Group 1 is the optional index variable,
// group 2 the element variable, group 3 the collection expression.
var forRE = regexp.MustCompile(`\Afor (?:(\w+), )?(\w+) in (.+)\z`)

// callRE matches a whole-line call: a name, then a parenthesized
// argument list.
var callRE = regexp.MustCompile(`\A= ([a-z_][a-z0-9_]*)\((.*)\)\z`)

// A transform argument: one field-access path, nothing else.
var transformFieldRE = regexp.MustCompile(`\A[a-z_][a-zA-Z0-9_]*(\.[a-z_][a-zA-Z0-9_]*)*\z`)

func parseFor(control string) []string {
	m := forRE.FindStringSubmatch(control)
	if m == nil {
		return nil
	}
	return []string{m[1], m[2], m[3]}
}

func parseTag(stripped string, indent int, lines []string, childFrom, childTo int, path string) (node, error) {
	rest := stripped
	tagName := "div"
	var classes []string
	var id string

	// %tagname
	if rest[0] == '%' {
		if len(rest) < 2 || !isAlphaNum(rest[1]) {
			return node{}, fmt.Errorf("%s: invalid tag shorthand: %s", path, stripped)
		}
		end := 1
		for end < len(rest) && (isAlphaNum(rest[end]) || rest[end] == '-') {
			end++
		}
		tagName = rest[1:end]
		rest = rest[end:]
	}

	// .classes and #id
	for len(rest) > 0 && (rest[0] == '.' || rest[0] == '#') {
		switch rest[0] {
		case '.':
			if len(rest) < 2 {
				return node{}, fmt.Errorf("%s: invalid class shorthand: %s", path, stripped)
			}
			start := 1
			if rest[start] == '-' {
				start++
				if start >= len(rest) {
					return node{}, fmt.Errorf("%s: invalid class shorthand: %s", path, stripped)
				}
			}
			if !isAlpha(rest[start]) && rest[start] != '_' {
				return node{}, fmt.Errorf("%s: invalid class shorthand: %s", path, stripped)
			}
			end := start + 1
			for end < len(rest) && (isAlphaNum(rest[end]) || rest[end] == '-') {
				end++
			}
			classes = append(classes, rest[1:end])
			rest = rest[end:]
		case '#':
			if len(rest) < 2 || (!isAlpha(rest[1]) && rest[1] != '_') {
				return node{}, fmt.Errorf("%s: invalid id shorthand: %s", path, stripped)
			}
			end := 2
			for end < len(rest) && (isAlphaNum(rest[end]) || rest[end] == '-') {
				end++
			}
			id = rest[1:end]
			rest = rest[end:]
		}
	}

	// { attributes }
	var attrsStr string
	if len(rest) > 0 && rest[0] == '{' {
		depth := 0
		endPos := -1
		for j := 0; j < len(rest); j++ {
			if rest[j] == '{' {
				depth++
			} else if rest[j] == '}' {
				depth--
				if depth == 0 {
					endPos = j
					break
				}
			}
		}
		if endPos >= 0 {
			attrsStr = rest[1:endPos]
			rest = rest[endPos+1:]
		}
	}

	// Reject inline content
	rest = strings.TrimSpace(rest)
	if rest != "" {
		return node{}, fmt.Errorf("%s: inline content on tags is not allowed: %s", path, stripped)
	}

	children, err := parseLines(lines, indent+2, childFrom, childTo, path)
	if err != nil {
		return node{}, err
	}

	return node{
		kind:     kindTag,
		tag:      tagName,
		classes:  classes,
		id:       id,
		attrsStr: attrsStr,
		indent:   indent,
		children: children,
	}, nil
}

func isAlphaNum(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func isAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}
