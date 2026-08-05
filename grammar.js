/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

// Precedence mirrors the Go expression parser in expr.go: parseOr calls
// parseAnd calls parseNot calls parseCmp. So ! binds looser than a
// comparison, and `!a == b` is !(a == b), not (!a) == b.
const PREC = {
  or: 1,
  and: 2,
  not: 3,
  cmp: 4,
};

/**
 * @param {RuleOrLiteral} rule
 */
function commaSep1(rule) {
  return seq(rule, repeat(seq(",", rule)));
}

// A shorthand is written twice, once anchored to what precedes it, so
// the pattern lives here rather than in both tokens.
const CLASS = /\.-?[a-zA-Z_][a-zA-Z0-9_-]*/;
const ID = /#[a-zA-Z_][a-zA-Z0-9_-]*/;

module.exports = grammar({
  name: "hml",

  // Newlines are extras, not tokens. The external scanner runs before
  // extras at every position, so it still sees the line breaks it needs
  // to emit INDENT and DEDENT; everything after that is whitespace the
  // grammar has no use for.
  extras: (_) => [/[ \t\r\n]/],

  externals: ($) => [
    $._indent,
    $._dedent,
    $.filter_body,
    $.comment_body,
    // Tree-sitter sets this in error recovery. The scanner reads it and
    // gives up rather than inventing indentation inside a broken tree.
    $._error_sentinel,
  ],

  word: ($) => $.identifier,

  rules: {
    source_file: ($) => repeat($._item),

    _item: ($) =>
      choice(
        $.doctype,
        $.comment,
        $.filter,
        $.conditional,
        $.loop,
        $.output,
        $.element,
        $.text,
      ),

    _block: ($) => seq($._indent, repeat1($._item), $._dedent),

    doctype: (_) => "!!!",

    // A comment's indented lines are dropped by the engine, so they are
    // one opaque token here rather than items nobody renders.
    comment: ($) =>
      seq(token(seq("-#", /[^\n]*/)), optional($.comment_body)),

    filter: ($) => seq($.filter_name, optional($.filter_body)),

    filter_name: (_) => token(choice(":javascript", ":css")),

    // Elements
    //
    // The shorthands after the first one are immediate tokens: `%p` on
    // one line and `.note` on the next are two elements, and only the
    // absence of whitespace between them makes `%p.note` one.
    element: ($) =>
      seq(
        choice($.tag_name, $.class, $.id),
        repeat($._shorthand),
        optional($.attributes),
        optional($._block),
      ),

    _shorthand: ($) =>
      choice(alias($._class_immediate, $.class), alias($._id_immediate, $.id)),

    tag_name: (_) => token(/%[a-zA-Z0-9_][a-zA-Z0-9_-]*/),
    class: (_) => token(CLASS),
    id: (_) => token(ID),
    _class_immediate: (_) => token.immediate(CLASS),
    _id_immediate: (_) => token.immediate(ID),

    attributes: ($) =>
      seq(
        token.immediate("{"),
        optional(seq(commaSep1($._attr_entry), optional(","))),
        "}",
      ),

    _attr_entry: ($) => choice($.attribute, $.splat),

    attribute: ($) =>
      seq(
        field("key", choice($.identifier, $.string)),
        ":",
        field("value", $._expr),
      ),

    splat: ($) => seq("**", $._expr),

    // Output
    output: ($) => seq("=", choice($.partial, $._expr, $.bare_call)),

    partial: ($) =>
      seq(
        "render",
        field("name", $._expr),
        optional(seq(",", commaSep1($._attr_entry))),
      ),

    // `= helper arg, key: val`: a call whose arguments are not
    // parenthesized. Only whole-line output takes this form.
    bare_call: ($) =>
      prec.right(seq(field("function", $._callee), $._arguments)),

    // Control
    conditional: ($) =>
      prec.right(
        seq($.if_clause, repeat($.else_if_clause), optional($.else_clause)),
      ),

    if_clause: ($) =>
      seq("-", "if", field("condition", $._expr), optional($._block)),

    else_if_clause: ($) =>
      seq("-", "else", "if", field("condition", $._expr), optional($._block)),

    else_clause: ($) => seq("-", "else", optional($._block)),

    loop: ($) =>
      seq(
        "-",
        "for",
        optional(seq(field("index", $.identifier), ",")),
        field("item", $.identifier),
        "in",
        field("collection", $._expr),
        optional($._block),
      ),

    // Text
    //
    // A line the parser does not recognize is text. The token has
    // negative precedence so every other line form wins at the same
    // position, and stops at `#` so an interpolation can start there.
    text: ($) =>
      prec.right(
        seq(
          choice(alias($._text_start, $.text_chunk), $.interpolation),
          repeat(
            choice(
              alias($._text_continuation, $.text_chunk),
              alias($._interpolation_immediate, $.interpolation),
            ),
          ),
        ),
      ),

    _text_start: (_) => token(prec(-1, /[^ \t\r\n#][^\n#]*/)),
    _text_continuation: (_) => token.immediate(/[^\n#]+|#/),

    interpolation: ($) => seq("#{", $._expr, "}"),
    _interpolation_immediate: ($) =>
      seq(token.immediate("#{"), $._expr, "}"),

    // Expressions
    _expr: ($) =>
      choice(
        $.binary_expression,
        $.unary_expression,
        $.parenthesized_expression,
        $.call,
        $.field_access,
        $.identifier,
        $.string,
        $.number,
        $.boolean,
        $.nil,
        $.array,
        $.hash,
      ),

    parenthesized_expression: ($) => seq("(", $._expr, ")"),

    binary_expression: ($) =>
      choice(
        prec.left(PREC.or, seq($._expr, "||", $._expr)),
        prec.left(PREC.and, seq($._expr, "&&", $._expr)),
        prec.left(
          PREC.cmp,
          seq($._expr, choice("==", "!=", "<", "<=", ">", ">="), $._expr),
        ),
      ),

    unary_expression: ($) => prec.right(PREC.not, seq("!", $._expr)),

    // A call names something the app injected: a registered transform or
    // an allowlisted helper. Which one it is depends on the app, so the
    // shape is what gets highlighted, not a list of names.
    call: ($) =>
      seq(
        field("function", $._callee),
        token.immediate("("),
        optional($._arguments),
        ")",
      ),

    _callee: ($) => choice($.identifier, $.field_access),

    _arguments: ($) => commaSep1(choice($.keyword_argument, $._expr)),

    keyword_argument: ($) =>
      seq(
        field("key", choice($.identifier, $.string)),
        ":",
        field("value", $._expr),
      ),

    // The dot is immediate and outranks the class shorthand, which the
    // same characters would otherwise match: `= post.author` is one
    // field access, not an output followed by an element.
    field_access: ($) =>
      prec.left(
        seq(
          field("object", choice($.identifier, $.field_access)),
          token.immediate(prec(2, ".")),
          field("field", alias($._identifier_immediate, $.identifier)),
        ),
      ),

    _identifier_immediate: (_) => token.immediate(/[a-zA-Z_][a-zA-Z0-9_]*/),

    array: ($) =>
      seq("[", optional(seq(commaSep1($._expr), optional(","))), "]"),

    hash: ($) =>
      seq(
        "{",
        optional(seq(commaSep1($.keyword_argument), optional(","))),
        "}",
      ),

    identifier: (_) => /[a-zA-Z_][a-zA-Z0-9_]*/,

    number: (_) => token(/-?[0-9]+/),

    boolean: (_) => choice("true", "false"),

    nil: (_) => "nil",

    // A double-quoted string interpolates; a single-quoted one does not.
    string: ($) =>
      choice(
        seq(
          '"',
          repeat(
            choice(
              alias($._double_string_chunk, $.string_content),
              $.escape_sequence,
              $.interpolation,
            ),
          ),
          '"',
        ),
        seq(
          "'",
          repeat(
            choice(
              alias($._single_string_chunk, $.string_content),
              $.escape_sequence,
            ),
          ),
          "'",
        ),
      ),

    _double_string_chunk: (_) => token.immediate(/[^"\\#]+|#/),
    _single_string_chunk: (_) => token.immediate(/[^'\\]+/),
    escape_sequence: (_) => token.immediate(/\\./),
  },
});
