# Move :javascript values out of the script source

This is a design for a breaking release. It names no open finding,
because the line-end rule in `jsLexer` and the `endsInMarkupPrefix`
check in `writeFilterValue` close them. It removes the cause that
those findings share.

## Need

Today a value goes into the script source. Its safety then depends on
two lexers that hml must keep in step with the browser:

- `jsLexer` (`compile.go`) decides whether a `#{}` is in code or in a
  string. A wrong answer runs data as code. Two findings came from
  wrong answers: `find_01UFpyJfkCzqw2djaUyttoTp` and
  `find_01KZUwrtH6tToJ26Tbv4wmf8`.
- The HTML tokenizer decides where the script element ends. A value
  next to literal text can end it early:
  `find_013DCxh7BLakJPVRUn2WF7H3`.

Parse now refuses a line that ends inside a quoted string or a
regular expression, so the first lexer fails closed. That rule cannot
make the lexer correct. An error that does not continue past a line
end is still open.

## Design

Data does not go into the script source:

1. A `#{}` in a `:javascript` block is allowed only in code. A `#{}`
   in a string or a template literal is a Parse error that names the
   line. The fix is to write `"/users/" + #{id}`.
2. Parse puts a fixed reference where each `#{}` was, such as
   `__hml[3]`. The reference is the same on every render.
3. Render writes the values once, as a JSON array in a
   `<script type="application/json">` element before the block. It
   uses `jsValueOptions`, so `<`, `>`, and `&` are escaped and no value
   can end that element.
4. A short fixed prologue in the block reads that element.

With this design, the bytes that the attacker controls are only in the
JSON element. A lexer error now gives broken output, not code
execution. A fixed reference inside a real string is text.

What it removes:

- `placeJSString` and the string branch of `writeJSStringContent`.
- The `endsInMarkupPrefix` check in `writeFilterValue`.
- The need for the lexer to be correct for security. The lexer stays
  only to give a clear Parse error.

What it adds:

- The script text is the same on every render, so an app can allow it
  with a CSP hash.

## Open questions

Answer each one before the code:

- How the prologue finds its element. `document.currentScript` is null
  in a module and in a script that a client loader runs as text. One
  user runs returned `:javascript` through such a loader. Test that
  loader before you choose. The other choice is a unique id for each
  block on each render, which makes the script text change again.
- Scope. A prologue in a `{ }` block changes the scope of a top-level
  `const`, `let`, or `function`. A `var` prologue adds a global. Find
  which top-level names the users declare.
- Speed. One JSON encode for each block, against one encode for each
  value now. Measure with `go test -bench . -benchmem`.

## Cost

- One user writes 143 `#{}` in `:javascript` blocks. About 136 of them
  are in a string, such as `"#{push_url}"`. Each one must change to
  `"..." + #{x}`.
- `AGENTS.md` says the next constraint needs a deprecation path, not
  an afternoon of bumping callers.

## Deprecation path

The new form `"..." + #{x}` renders the same under the current engine,
because a code value is already a quoted literal. So each user can
change first, with no break:

1. Add a way to list each `#{}` in a string place, such as a method on
   `Template` or a command beside `cmd/viewcover`. Each user runs it in
   a test and changes each site it reports.
2. When every user reports zero sites, release the break: refuse a
   `#{}` in a string, and write the JSON element.
