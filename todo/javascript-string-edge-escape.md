# Escape a :javascript string value at the edge of literal text

Finding:

- `find_013DCxh7BLakJPVRUn2WF7H3` (low, CWE-79). A value in a
  JavaScript string can complete `</script` or `<!--` with the literal
  text before it, and end the script element.

Outcome: fix it. Do it in any order with
`todo/1-javascript-lexer-fail-closed.md`. The two plans touch
different functions.

## Threat

- Attacker: an end user whose string reaches render data. The attacker
  needs no login. The victim loads the page.
- Way in: the template author writes a literal `<` or `</` directly
  before a `#{}` in a string, such as `"<#{tag}>"` or
  `"#{name} <#{email}>"`. The attacker sends `/script`, or `script`
  after `</`.
- Consequence: the HTML tokenizer ends the script element at the
  `</script` that the literal and the value make together. The block
  does not run. The rest of its source becomes page text, and markup
  in later JavaScript strings becomes live elements. A value after the
  cut stays escaped, so the attacker cannot write a new tag directly.
  A `<!--` made the same way moves the tokenizer into the escaped
  script states.
- Boundary: a value crosses from render data into the HTML that the
  browser tokenizes.

Each case reproduces at `aea4929`:

- `x = "</#{v}>";` with `v` = `script` renders `x = "</script>";`.
- `x = "<#{v}>";` with `v` = `/script` gives the same result.
- `x = '<#{v}';` with `v` = `!--` renders `<!--`.
- A backtick literal with `v` = `SCRIPT` gives the same result.

No user view puts a `<` directly before a `#{}` today.

## Cause

A script element has two grammars. The HTML tokenizer decides where
the element ends, and JavaScript decides what the text means.
`writeJSStringContent` (`render.go`) escapes `<`, so no value can start
`</script` alone. It does not escape `/`, `!`, `-`, or a letter, and it
does not read the text before the value. The edge between the literal
text and the value is the gap.

Two values next to each other are safe in one case only: when the
first value is not empty, because `<` is always escaped. When the
first value is empty, the literal text before it still touches the
second value, as in `"<#{a}#{b}"`.

## Change

In `writeFilterValue`, for `placeJSString`, read the end of the output
buffer before the value. `strings.Builder.String` does not copy, so
the read costs no allocation. Look at the bytes after the last `<` in
the final 8 bytes. If they are a proper prefix of `</script` or
`<!--`, in any ASCII letter case, write the first rune of the value
as a `\u` escape. Write the rest as now.

- The JavaScript string keeps the same text, because `\u0073` is `s`.
- The check reads the buffer, not the template, so it covers an empty
  value between the literal and the next value.
- `placeJSValue` writes `"` before a string, and JSON starts with
  `"`, `{`, `[`, a digit, `-`, `t`, `f`, or `n`. A `-` after `<!` in
  code cannot make `<!--` alone, because a number holds one `-`. Test
  it anyway.

The other option is to escape `/`, `!`, and `-` everywhere, plus a
first rune that is a letter. It needs no buffer read. It makes every
URL in a script longer and harder to read, as `\/users\/`. Do not take
it.

## Cost

- Speed: one short byte scan for each `placeJSString` value at render.
  Run `go test -bench . -benchmem` before and after.
- Output: one escape, only after a `<` prefix. No user output changes
  today.

## Tests

Add to `jsescape_test.go`:

- No `</script` and no `<!--` in the output, in any letter case. Use
  each of `"`, `'`, and a backtick, with a literal `<`, `</`, `</scr`,
  `<!`, and `<!-` directly before `#{v}`.
- `"<#{a}#{b}"` with `a` = `""` and `b` = `/script`.
- The legitimate form renders the same text: `"<#{v}>"` with `v` =
  `b` renders `"<\u0062>"`. A test reads the JavaScript string value
  and checks that it equals `<b>`.
- A value with no `<` before it renders as now.

## Documentation

Update `doc.go` in the same commit. In the security model, say that a
value in a string cannot complete `</script` or `<!--` with the text
before it. Say why: the HTML tokenizer ends the element, not
JavaScript.

## Release

Tag, and bump every user in the same sitting. Then resolve:

```sh
claudesec resolve find_013DCxh7BLakJPVRUn2WF7H3
```
