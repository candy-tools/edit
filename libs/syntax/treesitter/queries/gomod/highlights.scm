; go.mod, after the tree-sitter-go-mod query gotreesitter bundles (see
; ../NOTICE). Where two patterns capture the same node, the later one wins.

[
  "exclude"
  "go"
  "ignore"
  "module"
  "replace"
  "require"
  "retract"
  "tool"
  "toolchain"
] @keyword

"=>" @operator

[
  "("
  ")"
  "["
  "]"
] @punctuation.bracket

"," @punctuation.delimiter

[
  (module_path)
  (file_path)
] @string.special

[
  (version)
  (go_version)
  (toolchain_name)
] @number

[
  (interpreted_string_literal)
  (raw_string_literal)
] @string

(escape_sequence) @string.escape

(comment) @comment
