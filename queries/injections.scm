; A filter block's body is JavaScript or CSS, so the language that owns
; it highlights it.
;
; #{} inside a filter body is hml, not JS or CSS, and it is not marked
; here: the body is one token, and the engine interpolates it without
; escaping or a trust type -- a gap doc.go states rather than hides.
((filter
  (filter_name) @_name
  (filter_body) @injection.content)
  (#eq? @_name ":javascript")
  (#set! injection.language "javascript"))

((filter
  (filter_name) @_name
  (filter_body) @injection.content)
  (#eq? @_name ":css")
  (#set! injection.language "css"))
