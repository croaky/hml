# Agents guide

hml is an indentation-based subset of HTML that renders to HTML. See
`README.md` for why it exists and `doc.go` for the language and the
security model.

## Architecture

One package at the repo root, standard library only. Source moves
through four stages, each its own file:

- `parse.go` — lines and indentation to a `[]node` tree. Knows nothing
  about expressions.
- `compile.go` — expression, attribute, and interpolation text to ASTs,
  once at `Parse` time rather than per render.
- `expr.go` — the expression language: tokens, precedence, calls, and
  value semantics.
- `render.go` — a compiled template plus a context to an HTML string,
  including the attribute policy and escaping.

`api.go` is the exported surface: `Parse`, `Render`, `RenderContext`,
`Names`, `Renders`, and the `Safe*` types. `doc.go` is documentation
only.

## Checks

The root `Checkfile` is the list, and CI runs it on every push. Run the
same things before committing, since a check that fails locally fails
there:

```sh
goimports -local "$(go list -m)" -w .
go vet ./...
go test -race -cover ./...
git ls-files -z '*.go' | xargs -0 gopls check -severity=hint
```

The local `goimports` writes; the `lint` job only reports, because a CI
job that rewrites source has nowhere to put it.

Nothing outside the standard library is imported. Taking a dependency is
a design decision, not a step.

## Tests

Red/green TDD. Test files are named for their topic: `hml_test.go` for
the language, `lonetext_test.go` for output shape, `perf_bench_test.go`
for benchmarks, `assert_test.go` for helpers.

- A rejection needs two tests: that the value is refused, and that the
  legitimate form still renders. A policy that only rejects is one
  nobody can use.
- Value semantics live in five type switches — `equal`, `toFloat`,
  `truthy`, `stringify`, and `toAttrVal`. A new value type needs a case
  and a test in each, or it behaves like a plain string in some places
  and not others.

## Documentation

`doc.go` is the language reference, and it is why a reader trusts the
security model. A change to the grammar, the value semantics, the
attribute policy, or the shape of the output updates it in the same
commit. Say _why_ there; the code says what.

Plans live in `todo/planned`, numbered in rough order, and are reviewed
as ordinary changes. Delete a plan's text as it ships rather than
leaving a record of work already done.

## Commits

- Prefix with the stage the change acts on: `parse:`, `compile:`,
  `expr:`, `render:`, `api:`, `doc:`, `todo:`, `ci:`. Not `hml:` —
  every commit here is hml, so it says nothing.
- A change touching several stages takes the one whose behavior
  changed, not the one with the most lines.
- Imperative mood, lowercase except proper nouns. Hard-wrap at 72.
- Include _why_, not just _what_. See `git log` for examples.
- Sign your work with a `Co-Authored-By` trailer.

## Releases

cibot is origin and holds no tags. `scripts/tag vX.Y.Z` publishes one
annotated tag to the GitHub mirror, which is what a `go get` resolves.
