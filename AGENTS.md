# Agents guide

hml is an indentation-based subset of HTML that renders to HTML. See
`README.md` for why it exists and `doc.go` for the language and the
security model.

## Architecture

One package at the repo root. Source moves through four stages, each
its own file:

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
go test -trimpath -race -cover ./...
git ls-files -z '*.go' | xargs -0 gopls check -severity=hint
```

The local `goimports` writes; the `lint` job only reports, because a CI
job that rewrites source has nowhere to put it.

The engine imports nothing outside the standard library, and that is the
point of it: an app embeds hml to render pages, and a template language
that drags a graph in behind it is a worse trade than writing the parser.
Taking a dependency for `parse.go`, `compile.go`, `expr.go`, or
`render.go` is a design decision, not a step.

A test-only import is the stated exception, and there is one:
`github.com/croaky/is`. It stays out of a consumer's build entirely,
since module graph pruning does not load a dependency's test
requirements, so what it costs is a line in this `go.mod` rather than
anything an app links. What it buys is the assertions being the same
ones the other repos here run, and one fix rather than four: the copy
this replaced read the caller's source the obvious way, which returns a
path that will not open under the `-trimpath` the `test` job passes, so
every failure in CI said `assertion failed` and nothing else.

## Tests

Red/green TDD. Test files are named for their topic: `hml_test.go` for
the language, `lonetext_test.go` for output shape, `perf_bench_test.go`
for benchmarks.

Assertions are `github.com/croaky/is`, one `is := is.New(t)` per test.
Pick the helper that names what you assert, so a failure prints both
values: `Eq` and `NotEq` for two values, which is the default; `NoErr`
and `HasErr` for errors; `Nil` and `NotNil` for nil checks; `True` only
for a predicate with no want to name. `True(got == want)` compiles and
is still wrong. `Eq` takes `any`, so type the literal when the value
under test is not the literal's default type: `Eq(gotInt64, int64(3))`.

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

Plans live in `todo/planned` and are reviewed as ordinary changes. A
numeric prefix means land in this order; unnumbered siblings are
parallel and pickable anytime. Delete a plan's text as it ships rather
than leaving a record of work already done.

## Commits

- Prefix with the stage the change acts on: `parse:`, `compile:`,
  `expr:`, `render:`, `api:`, `doc:`, `test:`, `todo:`, `ci:`. Not
  `hml:` — every commit here is hml, so it says nothing.
- A change touching several stages takes the one whose behavior
  changed, not the one with the most lines.
- Imperative mood, lowercase except proper nouns. Hard-wrap at 72.
- Include _why_, not just _what_. See `git log` for examples.
- Sign your work with a `Co-Authored-By` trailer.

## Releases

cibot is origin and holds no tags. `scripts/tag vX.Y.Z` publishes one
annotated tag to the GitHub mirror, which is what a `go get` resolves.

No user should discover a break from a tag. `scripts/tag` refuses a
dirty tree and a `main` behind the farmer's, but it cannot know whether
the code is any good to the repos that will fetch it. So before tagging,
point each user at the working tree:

```sh
go mod edit -replace github.com/croaky/hml=/path/to/hml/worktree
go build ./... && go test ./...
```

The `replace` is a local experiment and must not be committed. Every
breaking release so far has been sized wrong until this was run.

Bump each user in the same sitting. Two versions of the engine in use at
once means the next change has to reason about both.

A pre-1.0 library with two known users can keep constraining what used
to pass, but the cheapness depends on knowing every user. If a third
appears, the next constraint needs a deprecation path rather than an
afternoon.
