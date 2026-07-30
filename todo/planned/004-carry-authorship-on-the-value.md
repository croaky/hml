# Carry authorship on the value, not the position

A template cannot pass an `on*` or `style` attribute to a partial. That
is most of a composition mechanism, refused by accident.

The attribute policy asks whether a value in a code context is code the
author wrote or data that arrived. It answers by looking at the AST node
at the attribute: a plain string literal is authored, anything else is
data. Inline, that is right. One step away, it is wrong:

```hml
= render "ui/icon_close", attrs: { onclick: "APP.closeDrawer()" }
```

`_icon_close.hml` splats `**attrs` onto its button. The handler is a
literal a person typed, but it reaches the tag as a map value, so the
renderer sees data and refuses. EDS does this 219 times across 141
files, and most of them are literals as plain as `style:
"vertical-align:middle;"`.

There is no template-side answer. A template cannot build a `SafeJS`,
and it should not be able to.

## Why the position cannot be the answer

Propagating authorship through the AST would mean following a value from
a hash literal, through a partial argument, into a `**` splat inside
another file. hml cannot see that path: a partial is resolved at render
time by an app-supplied `PartialFunc`, and a splat's keys are known only
once its map exists. Any static approximation stays wrong somewhere, and
a rule that changes meaning when markup is factored into a partial is
not a rule.

So authorship belongs on the value. It survives a hash literal, a
partial argument, a splat, and a context chain for free, because it
travels with the thing it describes.

## Proposed changes

Evaluating a string literal yields a value that remembers it was
written in a template. An unexported type, since an app never
constructs one:

```go
// authored is a string a template author wrote, as against one that
// arrived as data. It rides on the value so the fact survives a hash
// literal, a partial argument, and a ** splat.
type authored string
```

The attribute policy then accepts `authored` in a code context on the
same grounds it accepts an inline literal, and `SafeJS`/`SafeCSS`
remain what a handler uses to vouch for a string it built.

`attrVal` carries the answer, since it holds the value stringified and
the type is gone by the time `policyAttr` sees it. So `literal` stops
being an AST question -- `a.val.typ == astString` -- and becomes a
question about the value, asked in the splat branch too, which is the
branch that has no AST node to ask about. That one line is the fix.

Everywhere else `authored` must behave as its underlying string. That
is the five type switches `AGENTS.md` names -- `equal`, `toFloat`,
`truthy`, `stringify`, `toAttrVal` -- where `toFloat` keeps saying not
a number, as it does for a plain string. Add `valTrust`, which must
keep saying `trustNone`. `escapeHTML` takes a `string` and is reached
only after one of the five unwraps, so it is safe by their being
right. The risk of the change is missing one, so each gets a test
rather than a reading.

`equal` is the one to write first. A template compares a local against
a literal constantly, and if `equal` does not unwrap, the comparison is
false, the other branch renders, and nothing reports anything. The rest
fail where someone can see it.

`doc.go` changes in the same commit, because it states the rule being
replaced twice: the security model says an `on*` or `style` attribute
"takes a string literal written in the template," and Secure usage says
to keep those attributes literal. Both are the positional rule. Neither
mentions `authored`, which is unexported and behaves as a string, so
what a reader needs is the shorter rule, not a new type.

## Boundaries

These are the lines that keep the rule short enough to hold in mind.

**Interpolation yields data.** `"this.src='#{url}'"` is not authored,
even when `url` is. The moment a template assembles code around a value,
the author should say so in Go and mark it. This is what EDS just did
for 94 image fallbacks, and the result was better than what it replaced.

**An app helper receives plain strings.** `evalCall` hands arguments
to funcs the app injected, and those assert `args[0].(string)`; EDS has
several. Unwrap at that boundary. A helper's result is app-computed
anyway, so a helper that wants code trust returns `SafeJS` or
`SafeCSS` -- which is the existing rule, unchanged.

The unwrap has to reach inside maps and slices, not just the top-level
argument. EDS passes hashes to helpers that read the values in them,
and `react_select` does `label, _ := opts["label"].(string)` with the
assertion's result discarded and its fallback guarded on `nil`, which
an `authored` is not. A shallow unwrap leaves that label empty and says
nothing. No hml type should cross into app code, so the boundary
recurses.

**A Go-supplied local is data.** Including a map a handler built and a
template splats. The handler is the one who knows, and it has the types
to say so.

## What this buys

The rule becomes one sentence: a code-context attribute takes code the
template author wrote, or a value the handler marked. No clause about
where the author happened to write it.

EDS needs nothing further for the 219 literal sites, and the three
per-row values it already typed stay typed.

## Alternatives considered

**Literal-only `js()` and `css()` constructors in template space.**
Sound -- the parser could refuse anything but a plain literal argument,
the way a transform takes only a field access. Rejected because it adds
ceremony to the safe case and puts an escape hatch in the language;
`js()` would eventually be pointed at something that is not a literal,
and then the argument restriction is the only thing between us and the
hole the policy exists to close. Also 219 call sites.

**Trust string literals inside a hash literal, for code-context keys
only.** Fixes the `attrs:` idiom and nothing else: a partial argument
that is not a hash (`= render "x", handler: "..."`, then `onclick:
handler`) fails the same way. The same accident, smaller.

**Ship v0.2.0 with the policy narrowed to URL attributes.** Defers the
decision and leaves EDS unable to adopt the part it most needs, since
its dynamic `style` values are what broke first.

## Ordering

This lands before the release plan beside it. v0.2.0 should not
introduce a policy that has to be explained away.
