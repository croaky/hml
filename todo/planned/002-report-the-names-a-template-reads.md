# Report the names a template reads

An app cannot ask a template what it needs, so it finds out one page at
a time, in production.

A missing local is already a render error rather than a blank, which is
the right failure. But it is discovered only when someone loads the page
that has it, and a page nobody visited during a test run is a page whose
locals nobody checked. The information to do better is already in the
tree: every free identifier is `astField.parts[0]` or
`astCall.parts[0]`, resolved at parse time. Nothing exposes it.

## Proposed changes

```go
// Names returns the free top-level identifiers this template reads,
// sorted and deduplicated, so a caller can check its locals before
// serving a request.
func (t *Template) Names() []string
```

Walk the compiled tree once at the end of `Parse` and keep the result on
`Template`: the answer cannot change afterward, and a caller asking per
render should not pay for it.

The walk covers every place an expression can hide -- output nodes,
attribute values and `**` splats, text interpolation segments,
`- if`/`- elsif` conditions, `- for` collections, and partial argument
values. Loop variables bound by `- for item in items` are not free and
must not appear; `items` must. Names introduced by `= render "x", k: v`
belong to the partial, not the caller.

Partials are deliberately not followed. `Names` answers for one file,
because a partial is resolved by an app-supplied `PartialFunc` that hml
cannot see through.

But a caller checking a whole page needs the graph, since a partial
inherits its caller's locals through the context chain and so reads
names the caller must supply. Give it the edges rather than making it
reparse the source for them:

```go
// Renders returns the partial names this template renders with a
// literal `= render "name"`, so a caller can walk its own graph.
func (t *Template) Renders() []string
```

Literal names only. A computed partial name is not resolvable here, and
a caller that uses one owes itself the check by hand.

## What this does not do

It does not make a missing local a compile error. Nothing can, without
generating Go from the template, and `doc.go` opens by ruling that out:
".hml files are the source and the runtime input -- no transpiler, no
generated artifacts, no build step." That is the trade the library was
built to make, and it is not worth reopening for two users.

What `Names` buys instead is that the check can run once, early, over
every template at once, rather than per page per request. Where the
caller runs it is the caller's business -- cibot intends to assert at
init, so a missing local fails at startup and in any test that starts
the server. See `cibot:todo/planned/014-check-view-locals-at-startup.md`
for that side.
