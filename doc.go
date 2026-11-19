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
//
//   - = expr HTML-escapes output (&, <, >, ") unless the value is a
//     SafeString. There is no raw-output syntax: != is a parse error.
//     The only unescaped paths are SafeString values (produced by the
//     renderer itself: rendered partials, layout body, csrf tags) and
//     the transform builtins, which sanitize inside the engine.
//
//   - An attribute name holds only ASCII letters, digits, -, _, :, and
//     . — a quoted key on a tag is a Parse error otherwise, and a **
//     splat key a render error. A splat key is data, and a space, a
//     slash, a quote, =, or > in it would add an attribute the policy
//     never saw, or end the tag. With this rule, the name the policy
//     reads is the name the browser reads.
//
//   - Attribute values are constrained by the context the attribute
//     puts them in. A URL attribute (href, src, action, formaction,
//     poster, cite, background, ping, xlink:href, data, codebase,
//     classid, archive, longdesc, usemap, manifest, icon, profile)
//     takes a relative URL or one of http, https, mailto, tel. A value
//     from data with any other scheme renders as the sentinel
//     about:invalid#ZgotmplZ, so javascript:alert(1) never reaches the
//     browser. The about:invalid scheme makes the browser fetch nothing.
//     A bare #ZgotmplZ, as in html/template, resolves to the page
//     itself, so an img or an icon
//     link that held it loaded the page a second time. A string literal
//     with a refused scheme is an error: it can only ever render the
//     sentinel, so it is a mistake. Parse refuses one typed on a tag,
//     and render refuses one that arrives through a partial argument.
//
//     An on* attribute is a JavaScript context and a style attribute a
//     CSS context: each takes code the template author wrote, which is
//     application source, or a value the handler marked SafeJS or
//     SafeCSS. A plain dynamic value there is a render error, because
//     no escaping makes untrusted data safe as code.
//
//     A srcdoc attribute is a whole document in the page origin. The
//     browser decodes the value and parses it as HTML, so escaping does
//     not help. It takes a value the author wrote and nothing else.
//     Serve a document built from data as its own response, and point
//     src at it.
//
//     Code the author wrote means a string literal, wherever it was
//     typed. It stays authored through a hash literal, a partial
//     argument, and a ** splat, so markup factored into a partial means
//     what it meant inline. What ends authorship is the template's
//     assembling something: an interpolated string is data, however
//     literal its segments, because a value it did not write is now
//     part of it.
//
//   - The parser rejects anything outside the dumb subset — no method
//     calls on data, no arbitrary code, no eval. The only callables are
//     allowlisted helper funcs injected as locals (see Subset grammar).
//     The parser IS the linter.
//
//   - Templates cannot define variables, import modules, or execute
//     side effects. They only read pre-computed data from the handler.
//
//   - Partials nest at most 64 deep in one render. Past that, the
//     render call is an error that names the view and the partial. A
//     partial that renders itself, by a literal name or by a name from
//     data, would otherwise recurse until the Go runtime stops the
//     process, past any recover. The depth rides on the Context that
//     the renderer hands the resolver, so a resolver passes that
//     Context on and does not start a new one.
//
//   - A computed partial name (= render name, or #{} in a quoted name)
//     is data. The resolver looks it up in a fixed set of templates and
//     returns an error for any other name. It never builds a file path
//     from it.
//
//   - No #{} value goes into the source of a :javascript block. The
//     block starts with one statement that holds every value, in order,
//     as an element of an array, and each #{} becomes a read of its
//     element:
//
//     :javascript                  <script>
//     go("/users/#{id}", #{n});  var __hml_9f2c = ["7",42];
//     go(`/users/${__hml_9f2c[0]}`, __hml_9f2c[1]);
//     </script>
//
//     Parse rewrites a quoted string that holds a #{} as a template
//     literal, so the string stays one token and the value is a ${}
//     substitution. A backtick and a ${ in the string stay text.
//
//     Why: an escape keeps a value safe only in the place it was made
//     for. Parse finds that place with a small lexer, and a wrong
//     answer once made data in code render as string content and run.
//     With the array, a wrong answer moves only text the author or hml
//     wrote, so the script can be wrong but data never runs as code.
//     The array is JSON with <, >, &, U+2028, and U+2029 as \u
//     escapes, so no value can end the script element or start a
//     comment. The array is a var whose name holds the full SHA-256
//     hash of its values. So two blocks on one page keep their own
//     values, also in a function that runs later, even when one end
//     user chooses values in both blocks. Two arrays then share a name
//     only after a collision search of 2^128 steps. A hash that values
//     can steer, such as FNV-1a, lets one block take the name of
//     another, and its var then replaces the other block's array.
//
//     In a string, a value is its text, as in markup: 42 reads as "42"
//     and nil as "". In code, a string is a string; a number, a bool,
//     and nil are 42, true, and null; a SafeJS value is the expression
//     Go built, in ( ); and any other value is its JSON.
//
//     A #{} in a comment or a regular expression is a Parse error,
//     because a value has no meaning there. So is an octal escape such
//     as \1 in a string that holds a #{}, because a template literal
//     has none. Parse also refuses a line that ends inside a quoted
//     string, unless a backslash continues the string, and a line that
//     ends inside a regular expression. JavaScript allows neither, so
//     such a line shows that the lexer and the browser disagree, and
//     the error names the line before a page runs the wrong script.
//
//   - A #{} in a :css block takes a number or a SafeCSS value, and
//     anything else is a render error. CSS has no string escape that
//     holds in every place, so the handler builds the CSS.
//
//   - The body of a %script or %style tag takes only literal text and
//     comments, in any letter case of the tag, also inside %svg and
//     %math. Parse refuses any other child: a #{}, an = line, a call,
//     a partial, a condition, a loop, or a nested tag. The browser runs
//     that body as code, and the HTML escape stops markup, not code.
//     Put a value in :javascript or :css, which encode it. A script
//     element that needs an attribute such as nonce, and also a value,
//     takes the value in a data-* attribute, and the script reads it
//     from document.currentScript.dataset:
//
//     %script{ nonce: nonce, "data-id": id }
//     load(document.currentScript.dataset.id);
//
// What the renderer does NOT guarantee:
//
//   - No context awareness in body text. = output and #{}
//     interpolation in body text get uniform html.EscapeString. A
//     script or style element that a line of text types as markup,
//     such as <script>f("#{v}")</script>, is text to hml, and its
//     value gets that escape only. Write a %script tag or :javascript
//     instead.
//   - The :javascript lexer knows strings, template literals, comments
//     (//, /* */, <!--, and a --> at the start of a line), and regular
//     expressions, and nothing else. A slash after an operand, a
//     postfix ++ or --, or a property name such as a.in is a division.
//     A slash after an operator or a keyword such as return starts a
//     regular expression. A script that defeats that choice, and
//     balances its quotes again on the same line, renders a wrong
//     script that the browser refuses or runs wrong. Keep regular
//     expressions that hold a quote out of a block with a #{}.
//   - A tagged template, such as String.raw`a#{v}`, gets the value as
//     a substitution, not as text in its strings.
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
// Conditions (- if, - else if, !): the value must be a bool. Anything else
// is an error naming the view, the line, the condition, and the type —
// at Parse when the expression settles it (- if "x", - if title ||
// "Untitled"), at Render otherwise. There is no implicit truthiness to
// fall back on, because presence is not a property the engine can read
// off an untyped value: "" and 0 are present, a nil pointer is not, and
// which of those a field is depends on how the handler built it. Go
// knows the type, so the decision is made there — as a comparison the
// template states (- if s != "", - if n > 0, - if p == nil), or as a
// bool the handler computed (- if co.HasWebsite).
//
// See package viewcover to find views with a condition that no test
// renders.
//
// ! is held to the same rule, because it is a conditional written
// backwards: were it exempt, - if title would be an error and
// - if !title the same guess with the branches swapped.
//
// Truthiness (&&, ||): nil, the boolean false, and a boxed nil (a nil
// pointer, slice, map, chan, func, or interface) are falsy. Everything
// else is truthy — including the empty string "", the number 0, and an
// empty but non-nil slice. These two keep it because they are also the
// default-value idiom below; a conditional does not.
//
// Absence (== nil, != nil): a nil pointer, slice, map, chan, func, or
// interface is nil, the same values && and || read as falsy. Go's own ==
// would say otherwise for a typed nil inside an interface, which would
// make - if p == nil answer backwards for the pointer the doc above
// tells an author to test that way.
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
// JS/Ruby-style default-value idiom, so title || "Untitled" works in an
// output or an attribute.
//
// The two rules meet in a condition. hasX || hasY is fine: both operands
// are bools, so the operand returned is one. title || "Untitled" is a
// type error there, because the operand returned is a string.
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
// value, so a typo fails loudly instead of rendering blank. Because
// missing locals fail at render time, tests must render each view to
// check fields. See package viewcover.
//
// # Output shape
//
// Each node is written on its own line, so the output reads as the tree
// it came from. One exception, because whitespace inside an element is
// not always free: a tag whose only child is a run of text holds it on
// the tag's line, as <a href="/">home</a>. Left on its own line, the
// newline before the closing tag is whitespace inside the element, which
// HTML collapses to a space -- usually nothing to look at, but inside an
// anchor it is a space the underline runs through, past the end of the
// word.
//
// One child only. Two lines of text are two the author separated, and
// joining them would close a gap that is in the source on purpose.
// Elements, partials, and conditionals bring their own lines, and a tag
// holding them stays a block. The newline between siblings never goes,
// so words in a row are still words in a row.
//
// A run of text that renders as several lines -- markdown of two
// paragraphs, say -- stays a block. The space those lines would have
// collapsed to falls between blocks, where nothing sees it.
//
// A pre keeps everything: its whitespace is its content, which is why a
// diff hunk or a terminal transcript renders inside one.
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
//   - Write on* and style values as literals, in the template or in a
//     partial's arguments. When a value must be built from data, build
//     it in Go and mark it SafeJS or SafeCSS, or pass the data through
//     a data- attribute and read it from JavaScript. Interpolating data
//     into one in the template is the case the policy refuses.
//   - In a :javascript block, write a value as the data it is:
//     "#{name}" for text, f(#{count}) for a number, f(#{opts}) for a
//     map. Do not escape it in Go first, because the engine escapes it
//     again. Mark a value SafeJS only when Go built it as an
//     expression, for example with encoding/json/v2 and
//     jsontext.EscapeForHTML(true), which escapes <, >, and &.
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
//	- if expr / - else if / - else  conditionals
//	- for item in items             loops (optional index: for i, item)
//	= render "name", key: val       partials
//	:javascript / :css              filter blocks (#{} by place)
//	-#                              comments (omitted from output)
//	static text with #{field}       text interpolation (escaped)
//
// Banned: != raw output, method calls on data (including predicates like
// .nil?), hash access, ternaries, case/when, variable assignment, string
// interpolation with logic, content_for, yield, raw(), sanitize().
package hml
