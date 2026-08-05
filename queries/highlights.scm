; A later pattern wins, so the general ones come first and the shapes
; that mean something more specific override them below.

; Values
(identifier) @variable

(field_access
  field: (identifier) @variable.member)

(string) @string

(escape_sequence) @string.escape

(number) @number

(boolean) @boolean

(nil) @constant.builtin

[
  "("
  ")"
  "["
  "]"
  "{"
  "}"
] @punctuation.bracket

[
  ","
  ":"
  "."
] @punctuation.delimiter

[
  "!"
  "!="
  "&&"
  "**"
  "<"
  "<="
  "=="
  ">"
  ">="
  "||"
] @operator

"=" @punctuation.special

"#{" @punctuation.special

; Structure
(tag_name) @tag

(class) @tag.attribute

(id) @tag.attribute

(doctype) @keyword.directive

(filter_name) @keyword.directive

(comment) @comment

(comment_body) @comment

; Attributes
(attribute
  key: (identifier) @tag.attribute)

(attribute
  key: (string) @tag.attribute)

(keyword_argument
  key: (identifier) @variable.parameter)

(keyword_argument
  key: (string) @variable.parameter)

; Control
"-" @keyword

[
  "if"
  "else"
] @keyword.conditional

[
  "for"
  "in"
] @keyword.repeat

(loop
  index: (identifier) @variable.parameter)

(loop
  item: (identifier) @variable.parameter)

"render" @keyword.import

; Calls
;
; A name with an argument list is a transform or a helper, and which one
; depends on what the app registered. The shape is highlighted, so there
; is no list of names to keep current.
(call
  function: (identifier) @function.call)

(call
  function: (field_access
    field: (identifier) @function.call))

(bare_call
  function: (identifier) @function.call)

(bare_call
  function: (field_access
    field: (identifier) @function.call))
