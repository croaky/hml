# Author a tree-sitter grammar for hml

Apps render from `.hml` in production. What's left is honest
highlighting: editors still have no grammar for the language, which is
why the files were named `.haml` in the first place.

## Grammar

In this repo, alongside the Go package:

- `grammar.js` plus `src/scanner.c`, an external scanner emitting
  INDENT/DEDENT that mirrors the Go parser's indent tracking. Cover the
  subset in `doc.go`: `%tag`/`.class`/`#id`, `{ key: value }` attrs,
  `= output`, `= transform(field)`, `- if/else if/else`, the single
  `- for [i, ]item in collection` loop, `= render`,
  `:javascript`/`:css` filters, `-#` comments, `#{}` interpolation.
- Highlight the transform-call shape generically, not a fixed keyword
  list: apps register their own transform names.
- `queries/highlights.scm` and `injections.scm`, delegating filter
  bodies and `#{}` to JS/CSS.

Server-rendered highlighting is not this repo's job; that is
`github.com/croaky/highlight`.

## Migrate ~/laptop

- Add `hml` filetype detection for `*.hml` and add `hml` to the
  `ts_parsers` list in `vim/init.lua`, pointing nvim-treesitter at
  `tree-sitter-hml`.
- Replace the `FileType haml` autocmd and `haml_partial_path` (builds
  `_<name>.haml` partial paths for `gf`) with `hml`/`.hml` equivalents.

## Migrate ~/blog

- Same treesitter/filetype registration; blog shares the config.
- Convert blog's two `html/template` pages (`ui/article.html`,
  `ui/index.html`) to `.hml`, to dogfood the engine on blog's own
  markup.

Blog would be a third user of the engine, which is the condition the
Releases section of `AGENTS.md` names as the end of the cheap breaking
change. Bump it to the current tag as part of the conversion rather
than pinning it to whatever is current when the work starts.

## Validation

- `tree-sitter generate` and `tree-sitter test`.
- `:InspectTree` on a representative view.
