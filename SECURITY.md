# Security

Tier: single-user local tool and rendering library.

`hml` is a Go HTML template engine that compiles `.hml` templates and
renders HTML with default escaping, attribute policies, and no raw
output (`!=`). It imports only the Go standard library and makes no
network requests or subprocess calls.

## Adversaries

1. Untrusted values rendered into templates (text, attributes, URLs,
   `:javascript` `#{}` values, and transform inputs) attempting HTML,
   attribute, URL-scheme, or script injection.
2. Crafted templates or deeply nested expressions causing panics or
   unbounded CPU and memory use during `Parse` or `Render`.
3. A compromised dependency in `tools.mod`.

## Accepted risks

Resolve scanner findings in these areas as `accepted_risk` instead of
adding defensive code:

- **`:javascript` filter**: `:javascript` is an intentional feature so
  server-rendered applications keep a small HTML attribute set instead
  of an HTMX-style attribute DSL (<https://dancroak.com/web/ajax>).
  `hml` separates dynamic `#{}` values from script source by emitting a
  value array and reading each slot from it, while callers supply
  per-request CSP nonces. Do not remove `:javascript` to satisfy
  scanner findings.
- **Trusted template source**: `.hml` template files are authored by
  application developers, not untrusted end users.
- **Caller-supplied `Safe*` values and transforms**: The host
  application owns the safety of `Safe*` values, registered `Transform`
  functions, and `Content-Security-Policy` headers.
