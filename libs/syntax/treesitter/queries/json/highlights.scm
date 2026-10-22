; JSON, after the tree-sitter-json query gotreesitter bundles (see
; ../NOTICE). Where two patterns capture the same node, the later one wins,
; so keys, captured after strings, win over them.

(string) @string

(escape_sequence) @string.escape

(pair
  key: (string) @property)

(number) @number

[
  (true)
  (false)
] @boolean

(null) @constant.builtin

(comment) @comment

[
  "["
  "]"
  "{"
  "}"
] @punctuation.bracket

[
  ","
  ":"
] @punctuation.delimiter
