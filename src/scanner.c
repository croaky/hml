#include "tree_sitter/parser.h"

#include <stdlib.h>
#include <string.h>

// The scanner mirrors what parse.go does with indentation: a line's
// indent decides whose child it is, and a leading character counts one
// whether it is a space or a tab, as parse.go counts it. It emits
// zero-width INDENT and DEDENT tokens at the end of a line and leaves
// the newline itself to the grammar's extras, so several DEDENTs can
// land at one position -- a rescan from the same byte measures the same
// next line again.

enum TokenType {
  INDENT,
  DEDENT,
  FILTER_BODY,
  COMMENT_BODY,
  ERROR_SENTINEL,
};

#define MAX_DEPTH 64

typedef struct {
  uint8_t len;
  uint16_t stack[MAX_DEPTH];
} Scanner;

void *tree_sitter_hml_external_scanner_create(void) {
  Scanner *s = calloc(1, sizeof(Scanner));
  s->len = 1;
  s->stack[0] = 0;
  return s;
}

void tree_sitter_hml_external_scanner_destroy(void *payload) { free(payload); }

// The whole struct is the state, and it is 129 bytes at most, well
// under the buffer tree-sitter passes. So it goes out and comes back
// whole rather than a byte at a time: a length that disagrees with what
// was written is a buffer this scanner did not produce, and starting
// over at the top level is the only safe reading of it.
unsigned tree_sitter_hml_external_scanner_serialize(void *payload,
                                                    char *buffer) {
  Scanner *s = payload;
  unsigned size = sizeof(uint8_t) + s->len * sizeof(uint16_t);
  buffer[0] = (char)s->len;
  memcpy(buffer + sizeof(uint8_t), s->stack, s->len * sizeof(uint16_t));
  return size;
}

void tree_sitter_hml_external_scanner_deserialize(void *payload,
                                                  const char *buffer,
                                                  unsigned length) {
  Scanner *s = payload;
  s->len = 1;
  s->stack[0] = 0;
  if (length == 0) {
    return;
  }
  uint8_t len = (uint8_t)buffer[0];
  if (len == 0 || len > MAX_DEPTH ||
      length != sizeof(uint8_t) + len * sizeof(uint16_t)) {
    return;
  }
  memcpy(s->stack, buffer + sizeof(uint8_t), len * sizeof(uint16_t));
  s->len = len;
}

static void advance(TSLexer *lexer) { lexer->advance(lexer, false); }

static bool is_space(int32_t c) { return c == ' ' || c == '\t'; }

// A filter or comment body is every following line indented past the
// line that opened it. The engine hands those lines to JS or CSS, or
// drops them, so neither is parsed here.
static bool scan_raw_body(Scanner *s, TSLexer *lexer, const bool *valid) {
  uint16_t base = s->stack[s->len - 1];
  lexer->mark_end(lexer);

  bool any = false;
  for (;;) {
    while (is_space(lexer->lookahead) || lexer->lookahead == '\r') {
      advance(lexer);
    }
    if (lexer->lookahead != '\n') {
      break;
    }
    advance(lexer);

    uint16_t indent = 0;
    while (is_space(lexer->lookahead)) {
      indent++;
      advance(lexer);
    }
    if (lexer->lookahead == '\n' || lexer->lookahead == '\r') {
      continue; // blank line, only body if a deeper line follows
    }
    if (lexer->eof(lexer) || indent <= base) {
      break;
    }

    while (lexer->lookahead != '\n' && !lexer->eof(lexer)) {
      advance(lexer);
    }
    lexer->mark_end(lexer);
    any = true;
  }

  if (!any) {
    return false;
  }
  lexer->result_symbol = valid[FILTER_BODY] ? FILTER_BODY : COMMENT_BODY;
  return true;
}

bool tree_sitter_hml_external_scanner_scan(void *payload, TSLexer *lexer,
                                           const bool *valid_symbols) {
  Scanner *s = payload;

  if (valid_symbols[ERROR_SENTINEL]) {
    return false;
  }

  if (valid_symbols[FILTER_BODY] || valid_symbols[COMMENT_BODY]) {
    return scan_raw_body(s, lexer, valid_symbols);
  }

  if (!valid_symbols[INDENT] && !valid_symbols[DEDENT]) {
    return false;
  }

  // Zero width: the token ends where it started, so the newline and the
  // indentation stay for the extras and for the next scan to measure.
  lexer->mark_end(lexer);

  bool saw_newline = false;
  uint16_t indent = 0;
  for (;;) {
    if (lexer->lookahead == '\n') {
      saw_newline = true;
      indent = 0;
      advance(lexer);
    } else if (lexer->lookahead == '\r') {
      advance(lexer);
    } else if (is_space(lexer->lookahead)) {
      indent++;
      advance(lexer);
    } else {
      break;
    }
  }

  bool at_eof = lexer->eof(lexer);
  if (!saw_newline && !at_eof) {
    return false;
  }
  if (at_eof) {
    indent = 0;
  }

  uint16_t top = s->stack[s->len - 1];

  if (indent > top && valid_symbols[INDENT] && !at_eof && s->len < MAX_DEPTH) {
    s->stack[s->len++] = indent;
    lexer->result_symbol = INDENT;
    return true;
  }

  // s->len > 1 cannot fail while stack[0] is 0, since indent is
  // unsigned and nothing is less than it. It is here so that a future
  // nonzero base, or a deserialize this scanner did not write, cannot
  // read off the front of the stack.
  if (indent < top && valid_symbols[DEDENT] && s->len > 1) {
    s->len--;
    lexer->result_symbol = DEDENT;
    return true;
  }

  return false;
}
