; YAML, after the tree-sitter-yaml query gotreesitter bundles (see
; ../NOTICE). Where two patterns capture the same node, the later one wins,
; so keys, captured last, win over strings.

[
  (string_scalar)
  (double_quote_scalar)
  (single_quote_scalar)
  (block_scalar)
] @string

(escape_sequence) @string.escape

[
  (integer_scalar)
  (float_scalar)
] @number

(boolean_scalar) @boolean

(null_scalar) @constant.builtin

(timestamp_scalar) @number

(comment) @comment

[
  (anchor)
  (alias)
] @label

(tag) @type

[
  (yaml_directive)
  (tag_directive)
  (reserved_directive)
] @attribute

; Keys, in block and flow mappings.

(block_mapping_pair
  key: (flow_node
    [
      (double_quote_scalar)
      (single_quote_scalar)
    ] @property))

(block_mapping_pair
  key: (flow_node
    (plain_scalar
      (string_scalar) @property)))

(flow_mapping
  (_
    key: (flow_node
      [
        (double_quote_scalar)
        (single_quote_scalar)
      ] @property)))

(flow_mapping
  (_
    key: (flow_node
      (plain_scalar
        (string_scalar) @property))))

[
  ","
  "-"
  ":"
  ">"
  "?"
  "|"
] @punctuation.delimiter

[
  "["
  "]"
  "{"
  "}"
] @punctuation.bracket

[
  "*"
  "&"
  "---"
  "..."
] @punctuation.special
