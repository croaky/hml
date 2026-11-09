# Remove computed partial names

Finding: `find_015VDhLF53V5pUU5Xu6JAWfe` (high, CWE-674). No depth
limit on partial rendering. A partial name from data can make a
partial render itself without a limit.

Outcome: remove the feature that causes it. Accept the risk that is
left, and record it below.

## Threat

- Attacker: an end user of an app that embeds hml. The attacker needs
  no login.
- Way in: a template computes a partial name from a local that the
  attacker controls. The template writes `= render name` or
  `= render "cards/#{kind}"`. The resolver returns a template that
  reaches the same line again.
- Consequence: the goroutine stack grows until the Go runtime stops
  the process with `stack overflow`. `recover` in `net/http` cannot
  stop this. One request stops every request in flight.
- Boundary: the partial name crosses from render data into the app
  resolver, and `renderPartialCall` (`render.go`) does not check it.

No user of hml writes a computed partial name today. Each user
renders partials by a literal name only. No partial in a user renders
itself.

## Options

1. Remove the computed name forms. Parse refuses `= render name` and
   a quoted name that holds `#{}`. This is the recommended option.
   - Speed: no cost. Render does less work, because a literal name
     needs no evaluation.
   - Size: the code gets smaller. Remove `renderVarRE`, the
     `renderNameExpr` field, the variable branch in
     `renderPartialCall`, and the computed branch in `walkRender`.
     The name becomes one `string` on the node, not `[]interpSeg`.
   - Attack surface: part (2) of the finding goes away. The resolver
     never gets a name from data.
   - `Renders` then lists every partial a template can reach. An app
     can find a cycle in that graph at startup, with no engine code.
2. Keep the computed forms and add a depth limit. `Context` gets a
   `depth int`. `renderPartialCall` adds one and returns an error
   above a fixed limit, for example 64.
   - Speed: one compare per partial call, and one int per `Context`.
   - Size: about ten lines and two tests. The computed forms stay,
     with their code and their documentation.
   - The resolver still gets a name from data. `doc.go` must tell
     the app to resolve it only against a fixed set of names.
3. Do both. This is the most isolated option. It also keeps the depth
   code for a recursive partial that no user has.

Recommend option 1. Add option 2 later only when a user needs a
recursive partial, such as a comment thread.

## Steps

1. Write the failing tests first. `Parse` refuses `= render name` and
   `= render "cards/#{kind}"`, and names the line. `= render "card"`
   and `= render "card", user: person` still render the same bytes.
2. In `compileRender` (`compile.go`), keep `renderCallRE` and change
   its name group to refuse `#`. Delete `renderVarRE`. Store the name
   as a `string` on the node.
3. In `renderPartialCall` (`render.go`), pass the stored name to the
   resolver. Delete the evaluation branches.
4. In `walkRender` (`api.go`), record the name. Delete
   `literalSegs`. Replace `TestRendersOmitsComputedNames` with the
   parse-error test from step 1.
5. Change the `partial` rule in `grammar.js` so its `name` field is a
   `string`, not an `_expr`. Run `tree-sitter generate --js-runtime
   native` and `tree-sitter test`.
6. Update `doc.go`. The `Subset grammar` section says a partial name
   is a literal string. Say why: a name from data lets data pick what
   renders, and a literal name makes the partial graph known at
   Parse.
7. Add a cycle check to the startup example in `README.md`. The
   example walks `Renders` from each template and stops on a name it
   already holds on the current path. This is app code, not engine
   code.
8. Run the `replace` check in `AGENTS.md` on each user before the tag.
   No user writes a computed name, so no deprecation path is needed.

## Risk that stays

- Attacker: an end user who controls nested data, such as a reply
  tree.
- Way in: an app writes a partial that renders itself by a literal
  name, and walks that data.
- Consequence: the same process stop as above.
- Boundary: the depth of the app data.

Accept this risk. No user has a recursive partial. The startup cycle
check in step 7 shows the app each cycle before it serves a request.
An app that adds a recursive partial must limit the depth of its data,
or ask for option 2.
