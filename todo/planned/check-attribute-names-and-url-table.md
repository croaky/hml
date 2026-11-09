# Check attribute names and widen the URL table

Findings, in one change, because both change the attribute policy in
`policyAttr` (`render.go`) and the attribute output path:

- `find_01FLLoX9Em4goshK3HE5JFK1` (high, CWE-79). A key in a splat
  map goes to the output as a raw attribute name, so it bypasses the
  attribute policy.
- `find_011u4Ad92XuREjehNhh3a8od` (low, CWE-184). The policy table
  leaves out `data` on `object`, `srcdoc`, and other attributes that
  load a URL or parse markup.

Outcome: fix both. Accept the risk for `srcset` and `imagesrcset`, as
the last section says.

## Threat

- Attacker: an end user whose input reaches render data. The attacker
  needs no login. The victim views the page.
- Way in, splat key: a template writes `%tag{ **attrs }`, and the
  attacker controls a key of `attrs`. A key such as `x onclick` or
  `x><script>` passes through `evalAttrs` (`compile.go`) and
  `writeAttrsDynamic` (`render.go`) with no check. `policyAttr` checks
  the whole key, not the names that the browser reads from it.
- Way in, table: a template binds data to `data` on `%object` or to
  `srcdoc` on `%iframe`. `policyAttr` has no rule for either name and
  returns the value unchanged.
- Consequence: script runs in the app origin with the victim session.
  The script reads what the victim reads and acts as the victim.
- Boundary: the attribute name and value cross from render data into
  the HTML that the browser parses.

No user of hml splats a map whose keys come from end-user input
today. One user splats maps that Go code builds with literal keys. No
user binds `data`, `srcdoc`, or `srcset`. The engine is a public
library, so the next app can do either.

## Options for the name check

1. Allow a fixed set of characters. A name is not empty, and each byte
   is an ASCII letter, a digit, `-`, `_`, `:`, or `.`. This is the
   recommended option.
   - Speed: one byte loop for each splat key at render. No cost on the
     static path: `hoistStaticAttrs` runs it once at Parse.
   - Size: one function of about ten lines, two call sites, and two
     tests.
   - It refuses `@click` and other names that some JavaScript
     frameworks use. No user writes one. An app that needs one can
     ask, and the set can grow.
2. Refuse the characters that the HTML tokenizer stops a name at:
   whitespace, control bytes, `/`, `>`, `<`, `=`, `"`, `'`.
   - Same speed and size. It allows more names, but a reader must
     know the HTML tokenizer to verify it.
3. Drop a bad key, not refuse it. This hides a bug in the app. hml
   returns an error for every other policy failure, so do not drop.

## Steps

1. Write the failing tests first, one pair for each rule, as
   `AGENTS.md` asks:
   - A splat key `x onclick` with a script value is a render error. A
     splat key `data-id` still renders.
   - A splat key that holds `>` is a render error, on the valued path
     and on the boolean (`\x00true`) path.
   - An authored string key `"x y"` is a Parse error. `"aria-label"`
     still renders.
   - A data value with a refused scheme in `data` on `%object`
     renders `about:invalid#ZgotmplZ`. An `https` value renders.
   - A data value in `srcdoc` is a render error. An authored literal
     in `srcdoc` renders.
2. Add `validAttrName` beside `policyAttr` in `render.go`. Call it in
   `evalAttrs` for each splat key, and in `compileAttrs` for each
   string key. An identifier key already passes, because the
   tokenizer reads only identifier characters.
3. `policyAttr` already lowers the name. After the check, the name it
   reads is the name the browser reads. This closes part (2) of the
   splat finding with no more code.
4. Add `data`, `codebase`, `classid`, `archive`, `longdesc`, `usemap`,
   `manifest`, `icon`, and `profile` to `urlAttrs`. These are the
   legacy URL names that `html/template` also treats as URLs. Each is
   one map entry, and the lookup already runs.
5. Add a `srcdoc` case to `policyAttr`. It takes an authored value
   only. hml has no HTML trust type, and this change adds none. A
   handler that needs a `srcdoc` from data renders that document in
   a separate response and points `src` at it.
6. Update the security model in `doc.go`. List the URL names that the
   code holds. Say that an attribute name holds only the allowed
   characters, and why: a name from data must not add an attribute or
   end the tag. Say that `srcdoc` is a markup context and takes an
   authored value only.
7. Measure `go test -bench . -benchmem` before and after. Expect no
   change on the static path and one loop for each splat key.
8. Run the `replace` check in `AGENTS.md` on each user before the tag.

## Risk that stays: srcset and imagesrcset

- Attacker: an end user whose input reaches `srcset` or `imagesrcset`.
- Consequence: the browser loads an image that the attacker picks. A
  browser does not run script from an image candidate URL, and a
  `data:` image is inert.
- Boundary: the image loader of the browser.

Accept this risk. An `https` image URL from data is already allowed,
and it has the same effect. A check for each candidate URL adds a
parser for a comma list, and it does not stop an attack that the
current rule permits.
