# Make the :javascript lexer fail closed

Findings, in one change, because both are errors in the `jsLexer` Go
type (`compile.go`) and both need the same line-end check:

- `find_01UFpyJfkCzqw2djaUyttoTp` (medium, CWE-79). The lexer does not
  know the HTML-like comments `<!--` and line-start `-->`. A quote in
  such a comment starts a false string.
- `find_01KZUwrtH6tToJ26Tbv4wmf8` (medium, CWE-79). The lexer reads a
  division after postfix `++` or `--`, or after a keyword used as a
  property name (`a.in / b`), as the start of a regular expression.

Outcome: fix both. This plan also closes the case that `doc.go` names
as a limit: a regular expression that holds a quote after `)`.

## Threat

- Attacker: an end user whose string reaches render data. The attacker
  needs no login. The victim loads the page.
- Way in: the template author writes a block that looks harmless, such
  as:

  ```
  :javascript
    var ratio = done++ / total;
    var url = "/users/" + #{id};
  ```

  The lexer reads `/ total;` as a regular expression. The regular
  expression ends at the `/` in `"/users/"`, so the lexer then reads
  the closing `"` as an opening quote. `place` records `placeJSString`
  for a `#{}` that the browser reads as code.
- Consequence: `writeFilterValue` (`render.go`) writes a
  `placeJSString` value with no quotes. `writeJSStringContent` keeps
  letters, `.`, `(`, `)`, `;`, and `=`, so a value such as
  `alert(document.domain)` runs in the app origin with the victim
  session.
- Boundary: a value crosses from render data into script source.

Each traced example reproduces at `aea4929` through `Parse` and
`Render`:

- `<!-- don't remove` / `track(#{v});` renders `track(alert(...));`.
- A line-start `--> it's legacy` gives the same result.
- `done++ / total;`, `a.in / b;`, and `stats.new / total;`, each
  followed by `"/users/" + #{v};`, give the same result.
- `if (ok) /'/.test(s);` / `f(#{v});` gives the same result. `doc.go`
  says this case "can move a #{} out of a regular expression". The
  result is code execution, so the text understates it.

No user view has any of these shapes today. One user writes 143
`#{}` in `:javascript` blocks, so a later edit can add one.

## Cause

`place` trusts the lexer state, and nothing checks that state against
the browser. Each missing token rule gives the same result: the lexer
reads code as a string. The `endLine` function resets only
`jsLineComment`, so the false state continues to later lines until the
next quote of the same kind.

A rule for each token closes only the tokens that a finding names. The
next unknown token stays open. A check at each line end closes every
error that continues past the line, whatever token caused it.

## Change

1. Fail closed at the line end. `endLine` returns an error when the
   state is `jsDouble` or `jsSingle` and the line does not end in an
   odd number of backslashes. It also returns an error when the state
   is `jsRegex` or `jsRegexClass`. JavaScript allows neither, so a
   valid script never reaches either error. `compileNodes` wraps the
   error with the path and the line, as it does for `place`.
   - Keep `jsTemplate` and `jsBlockComment` across lines. JavaScript
     allows both, and `TestJavaScriptStateCrossesLines` covers the
     template literal.
   - The check reads the text of the line, so `endLine` takes the line
     after `stripFilterIndent`.
2. Read HTML-like comments as the browser does. `renderFilter` always
   writes a classic `<script>`, where both forms are comments:
   - `<!--` in code starts a line comment, anywhere on the line.
   - `-->` in code starts a line comment when only white space or block
     comments come before it on the line. Track this with a flag that
     `endLine` sets and the first code byte clears.
   - A `#{}` in either comment is then a Parse error through the
     existing `place` rule.
3. Read a division after postfix `++` or `--`. In `readCode`, read
   `++` and `--` as one token. When the code before it ends in an
   operand, the token is postfix, so call `operand`. Otherwise keep it
   as an operator, because `++/re/` is not valid either way.
4. Read a word after `.` or `?.` as a property name. Record that the
   last code byte before the word is `.`. `regexAllowed` then returns
   false for that word, even if `jsRegexAfterWord` holds it.

Do not refuse `<!--` and `-->` in a block. Refusal is simpler, but the
browser reads both forms as comments, and step 2 makes the lexer read
them the same way.

## Cost

- Speed: one state test for each filter line at Parse. No cost at
  render. Run `go test -bench . -benchmem` before and after.
- Features: a quoted string across lines with no backslash is now a
  Parse error. JavaScript does not allow it.
- Users: a copy of `jsLexer` with the step 1 check read all 257
  `:javascript` blocks in the users of hml. It refused 0 of them. Run
  the `replace` check in `AGENTS.md` on every user before the tag to
  confirm the full change.

## Tests

Each rejection needs a test that the legitimate form still renders.
Add them to `jsescape_test.go`.

- Refused at Parse, with the path and the line in the error:
  - `x = "a` on one line, `b";` on the next.
  - `x = /a` on one line, `b/;` on the next.
  - `if (ok) /'/.test(s);` followed by `f(#{v});`.
  - `<!-- #{v}` and `--> #{v}`.
- Rendered, with each value as a quoted literal:
  - `<!-- don't` followed by `track(#{v});`.
  - A line-start `--> it's` followed by `track(#{v});`.
  - `done++ / total;`, `done-- / total;`, `a.in / b;`, and
    `stats.new / total;`, each followed by `"/users/" + #{v};`.
  - `var s = "a\` followed by `b #{v}";`. A backslash continues the
    string.
  - `` `a `` followed by `` #{v}` ``. A template literal crosses
    lines.
  - `x = ++i / 2;` and `return /a/.test(s);`. A regular expression
    after `return` still opens one.

## Documentation

Update `doc.go` in the same commit:

- Security model: say that Parse refuses a line that ends inside a
  quoted string or a regular expression. Say why: such a line shows
  that the lexer and the browser do not agree.
- What the renderer does not guarantee: replace the regular expression
  limit with the limit that remains. An error that does not continue
  past the line end can still move a `#{}`. Say what happens then: the
  value runs as code. The `doc.go` text must not say that the value
  only moves out of a regular expression.
- Add `<!--` and `-->` to the list of what the lexer knows.

## Release

Run the `replace` check on every user. Then tag, and bump every user
in the same sitting. Then resolve both findings:

```sh
claudesec resolve find_01UFpyJfkCzqw2djaUyttoTp
claudesec resolve find_01KZUwrtH6tToJ26Tbv4wmf8
```

`todo/2-javascript-values-out-of-source.md` removes the cause. This
plan closes the findings without a break for any user.
