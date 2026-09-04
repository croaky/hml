# Agents guide

hml is an indentation-based subset of HTML that renders to HTML. See
`README.md` for why it exists and `doc.go` for the language and the
security model.

## Writing

Write every word in ASD-STE100 Simplified Technical English (STE):
Markdown docs, code comments, commit messages, and replies in an agent
conversation. See
<https://en.wikipedia.org/wiki/Simplified_Technical_English>.

STE is a controlled English for technical writing: one meaning per
word, one idea per sentence, and the actor named. It is not a house
style. It exists so a reader who is tired, or reading a second
language, or an agent matching on words, all read the same sentence the
same way.

- One idea per sentence. Keep an instruction to 20 words and a
  description to 25.
- Active voice, present tense, and the actor named: say what acts,
  rather than writing "the token is refused".
- One word, one meaning. Keep a term the same everywhere rather than
  varying it for tone.
- Use the simple verb, not a noun made from it: "run the formatter",
  not "perform execution of the formatter".
- Cut what carries nothing: "simply", "just", "note that", "in order
  to".
- Put a warning or a limit before the step it applies to.

Apply it to prose, not to code: an identifier, a command, and a quoted
error message stay as they are.

## Architecture

The engine is one package at the repo root. Source moves through four
stages, each its own file:

- `parse.go` — lines and indentation to a `[]node` tree. Knows nothing
  about expressions.
- `compile.go` — expression, attribute, and interpolation text to ASTs,
  once at `Parse` time rather than per render.
- `expr.go` — the expression language: tokens, precedence, calls, and
  value semantics.
- `render.go` — a compiled template plus a context to an HTML string,
  including the attribute policy and escaping.

`api.go` holds most of the exported surface: `Parse`, `Template` with
`Render`, `RenderContext`, `RenderContextTo`, `Names`, `Renders`, and
`HasCondition`, the `Transform`, `PartialFunc`, and `PartialWriter` func
types, and the `Safe*` types. `Context`, `NewContext`, and
`Context.Child` sit in `expr.go`, beside the evaluator that reads them.
`doc.go` is documentation only.

`viewcover/` and `cmd/viewcover/` are the render-coverage check: the
trace an app emits per resolved view, and the command that diffs a
traced test run against the views with a condition. They import the
engine; the engine does not import them. The package doc says why the
trace goes to stdout.

Render does no work Parse can do. An expression compiles to an AST at
Parse, a literal is boxed once there, and a tag whose attributes are
all literals has its attribute string written there
(`hoistStaticAttrs`). A render then writes strings into one buffer:
no builder per tag, per text node, or per partial. Measure a change
to `render.go` with `go test -bench . -benchmem` before and after.

## Grammar

Alongside the Go package, a tree-sitter grammar so editors can
highlight `.hml` rather than pretending it is HAML:

- `grammar.js` — the same language `doc.go` describes, in tree-sitter's
  DSL. Expression precedence mirrors `expr.go`.
- `src/scanner.c` — an external scanner emitting INDENT and DEDENT, and
  reading a filter or comment body as one opaque token.
- `queries/highlights.scm`, `queries/injections.scm` — the latter hands
  `:javascript` and `:css` bodies to those languages.
- `test/corpus/` — `tree-sitter test` cases.

`grammar.js` and `src/scanner.c` are the source. `src/parser.c`,
`src/grammar.json`, `src/node-types.json`, and `src/tree_sitter/` are
what the CLI writes from them, and are ignored rather than committed.
Most grammars commit that C because their users are compiling it
without a CLI to hand; both users here install `tree-sitter-cli` from
`~/laptop`, so a 15,000-line generated file in the tree would buy
nothing and go stale. nvim-treesitter generates at install time
(`generate = true`, `generate_from_json = false` in `vim/init.lua`),
which also builds against the ABI that nvim speaks.

After a `grammar.js` change:

```sh
tree-sitter generate --js-runtime native
tree-sitter test
```

`generate` first, always: `tree-sitter test` reads `src/grammar.json`
and fails without it.

Keep `grammar.js` to the DSL and nothing else. `--js-runtime native`
reads it with the QuickJS the CLI embeds, so a `grammar.js` that
required an npm package would not build. The `Checkfile` says why the
`grammar` job runs both commands and passes that flag.

Whether an example is hml at all is a different question, and the
CLI cannot answer it. `corpus_test.go` reads every
`test/corpus/*.txt` source and hands it to `Parse`. Without it a corpus
case can describe a language the engine rejects, and the grammar passes
its own tests saying so.

What the grammar highlights is the shape of a call, not a list of
names. Transforms and helpers are registered by the app, so a fixed
list would be wrong in every repo but one.

## Checks

The root `Checkfile` is the list, and CI runs it on every push. Run the
same things before committing, since a check that fails locally fails
there:

```sh
goimports -local "$(go list -m)" -w .
go vet ./...
go test -trimpath -buildvcs=false -race -cover ./...
git ls-files -z '*.go' | xargs -0 gopls check -severity=hint
dprint fmt
git ls-files -z '*.sh' 'scripts/*' | xargs -0 shellcheck
tree-sitter generate --js-runtime native && tree-sitter test
```

The local `goimports` and `dprint fmt` write; the `lint` and `fmt` jobs
only report, because a CI job that rewrites source has nowhere to put
it.

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

## Changes

Work happens on a cibot change. `cibot checkout` allocates one and
prints a worktree; `cibot edit` sets its title and description. Do the
edit before the code, not after. A change with neither is a blank row on
the dashboard and a blank `cibot show`, so nobody looking at either can
tell what it is or whether it overlaps what they are about to start. A
rough sentence beats an empty one, and the description gets rewritten
before the merge anyway.

After a push, read the checks with `git push && cibot show --wait`
rather than sleeping and then reading. The farmer holds the request open
and answers within a second of the last check, so a sleep is either time
spent waiting for an answer that already arrived or too short to reach
one. Too short is the worse half: a `cibot show` that lands before the
push is recorded reports the previous commit's checks, green, about the
wrong code. `--wait` follows the commit in the worktree it runs in,
exits nonzero when a check failed, and gives up after ten minutes
(`--timeout`).

## Commits

- Prefix with the stage the change acts on: `parse:`, `compile:`,
  `expr:`, `render:`, `api:`, `doc:`, `test:`, `todo:`, `ci:`,
  `grammar:` for the tree-sitter files. Not
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

There are three users now: blog joined when its pages became `.hml`.
That is the condition this section named as the end of the cheap
breaking change, so the next constraint needs a deprecation path rather
than an afternoon of bumping callers.
