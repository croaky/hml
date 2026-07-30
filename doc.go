// Package hml renders a small, structure-aware template language.
// It parses .hml source into an AST (indentation → tree) and renders
// that tree to well-formed HTML. .hml files are the source and the
// runtime input — no transpiler, no generated artifacts, no build step.
//
// # Why hml, not string templates
//
// hml is structure-aware. Indentation maps to HTML nesting, so the
// parser builds a tree, not a string. The renderer walks that tree
// and emits matched open/close tag pairs. Malformed HTML (unclosed
// tags, mismatched nesting, orphaned closing tags) is structurally
// impossible.
//
// This is the core argument from https://www.devever.net/~hl/stringtemplates:
// AST-based HTML generation prevents entire classes of bugs that
// string template systems create, because the template system cannot
// produce structurally invalid output. String template systems like
// Go's html/template, ERB, and Jinja concatenate strings with
// escaping. A forgotten closing tag, a misplaced {{end}}, or a
// conditional that opens a tag without closing it are all expressible.
// In hml, they are parse errors.
//
// # Security model
//
// What the renderer guarantees:
//
//   - Well-formed HTML. Every tag opened by the tree is closed by the
//     tree. You cannot produce <div><p></div></p>.
//   - = expr HTML-escapes output (&, <, >, ") unless the value is a
//     SafeString. There is no raw-output syntax: != is a parse error.
//     The only unescaped paths are SafeString values (produced by the
//     renderer itself: rendered partials, layout body, csrf tags) and
//     the transform builtins, which sanitize inside the engine.
//   - Attribute values are constrained by the context the attribute
//     puts them in. A URL attribute (href, src, action, formaction,
//     poster, cite, background, ping, xlink:href) takes a relative URL
//     or one of http, https, mailto, tel; anything else renders as the
//     sentinel #ZgotmplZ, as in html/template, so javascript:alert(1)
//     never reaches the browser. An on* attribute is a JavaScript
//     context and a style attribute a CSS context: each takes a string
//     literal written in the template, which is application source, or
//     a value the handler marked SafeJS or SafeCSS. A plain dynamic
//     value there is a render error, because no escaping makes
//     untrusted data safe as code.
//   - The parser rejects anything outside the dumb subset — no method
//     calls on data, no arbitrary code, no eval. The only callables are
//     allowlisted helper funcs injected as locals (see Subset grammar).
//     The parser IS the linter.
//   - Templates cannot define variables, import modules, or execute
//     side effects. They only read pre-computed data from the handler.
//
// What the renderer does NOT guarantee:
//
//   - No context awareness outside attribute values. = output and
//     #{} interpolation in body text get uniform html.EscapeString.
//     :javascript and :css filter blocks interpolate without escaping
//     and without a trust type; that is a known gap.
//   - Attribute rules key off the attribute name, not the value. The
//     renderer will not tell a well-formed http URL from one pointing
//     somewhere you did not intend, and it does not parse CSS or JS
//     that carries a trust type.
//   - SafeString, SafeJS, and SafeCSS are trust assertions, not
//     proofs. Wrap only renderer output, or content the producer
//     sanitized or built itself.
//
// # Transforms
//
// Rich text renders from source text through app-registered transforms
// (see Transform), invoked as = name(field). The engine ships zero
// built-ins; the app passes a name→Transform map to Parse. Each
// transform evaluates the field, sanitizes it, and the renderer emits
// the result unescaped, so a Transform must return safe HTML. A
// transform's argument must be exactly one field-access path — no
// literals, interpolation, or nesting — so template-side content
// assembly is impossible. Handlers pass source text (markdown, Slack
// mrkdwn, ts_headline output), never pre-built HTML. One project, for
// example, registers markdown, slack, and search_highlight in its own
// richtext package.
//
// # Calls
//
// name(args) is a call, and means the same thing in output position and
// in an attribute value. A registered transform name resolves to that
// transform, under the rules above. Any other name resolves to an
// allowlisted helper func injected as a local, takes as many arguments
// as it likes, and has its result escaped like any other = output.
//
// Resolving a helper needs the locals, which arrive per render, so an
// unknown name is a render error rather than a parse error. That is the
// one place the parser stops being the linter: a misspelled transform
// name reads as a helper the app did not inject.
//
// # Value semantics
//
// The engine evaluates expressions with a small, fixed set of coercion
// rules. They are documented here so the language has one specification
// rather than behavior discovered per template.
//
// Truthiness (- if, - elsif, &&, ||, !): nil, the boolean false, and a
// boxed nil (a nil pointer, slice, map, chan, func, or interface) are
// falsy. Everything else is truthy — including the empty string "", the
// number 0, and an empty but non-nil slice. This is why templates that
// want "non-empty string" must write - if s != "" rather than - if s.
//
// Equality (==, !=): values compare equal only within the same type; there
// is no cross-type coercion, with one exception — the numeric types int,
// int64, and float64 compare by numeric value, so an int64 column and an
// integer literal compare as expected. This numeric case exists to absorb
// the untyped ingest boundary (pgx yields int64, literals are int), not as
// a general coercion feature.
//
// Ordering (<, >, <=, >=): defined only for the numeric types above.
// Comparing a non-number is an error, not a coercion.
//
// && and || return an operand, not a bool: a && b yields a when a is
// falsy, else b; a || b yields a when a is truthy, else b. This is the
// JS/Ruby-style default-value idiom, so title || "Untitled" works. Only !
// always yields a bool.
//
// Stringification (= output, #{} interpolation): nil renders as the empty
// string; numbers and bools render in their Go form. Rendering never
// panics on a nil value.
//
// Field access (a.b.c): the first segment resolves through the context
// chain. Each later segment reads a map[string]any key; on any other
// value the engine falls back to reflection over a struct (pointers are
// dereferenced), matching an exported field by its json tag, its db tag,
// or its name case-insensitively. Unexported fields are invisible to a
// template. A missing key or field is a render error, not an empty
// value, so a typo fails loudly instead of rendering blank.
//
// # Secure usage
//
//   - Handlers own all data formatting. Templates receive pre-computed,
//     pre-formatted values and render markup from data.
//   - Use = (escaped output) everywhere. Render rich text through the
//     transform builtins. Reserve SafeString for renderer output and
//     producer-sanitized HTML; never wrap raw user input.
//   - Pre-build URLs in handlers, where you can validate them. The
//     renderer enforces the scheme allowlist but not the destination.
//   - Keep on* and style attributes literal. When a value must be
//     dynamic, build it in Go and mark it SafeJS or SafeCSS, or pass
//     the data through a data- attribute and read it from JavaScript.
//   - :javascript filter blocks pass through without escaping. Don't
//     interpolate user-controlled values into JS. Use data- attributes
//     on HTML elements instead, and read them from JS.
//
// # Subset grammar
//
// Allowed:
//
//	%tag, .class, #id, { key: value } attributes
//	= field                         escaped output (field access only;
//	                                SafeString values pass unescaped)
//	= name(field)                   app-registered rich-text transform
//	                                (render + sanitize app-side;
//	                                argument is one field access)
//	= helper(a, b)                  allowlisted helper call, in output or
//	                                in an attribute value
//	- if expr / - elsif / - else    conditionals
//	- for item in items             loops (optional index: for i, item)
//	= render "name", key: val       partials
//	= helper arg, key: val          allowlisted helper calls (Go funcs
//	                                injected as locals, e.g. do_react,
//	                                avatar_src, status_description)
//	:javascript / :css              filter blocks
//	-#                              comments (omitted from output)
//	static text with #{field}       text interpolation (escaped)
//
// Banned: != raw output, method calls on data (including predicates like
// .nil?), hash access, ternaries, case/when, variable assignment, string
// interpolation with logic, content_for, yield, raw(), sanitize().
package hml
