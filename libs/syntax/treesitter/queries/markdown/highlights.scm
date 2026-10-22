; Markdown's block syntax, after the tree-sitter-markdown query gotreesitter
; bundles (see ../NOTICE). The inline syntax comes injected into every inline
; and table cell (see markdown_inline), and a fenced code block in one of
; edit's languages is colored in that language instead (see
; injections.scm).

[
  (atx_heading)
  (setext_heading)
] @markup.heading

(block_quote) @markup.quote

[
  (fenced_code_block)
  (indented_code_block)
] @markup.raw

[
  (list_marker_plus)
  (list_marker_minus)
  (list_marker_star)
  (list_marker_dot)
  (list_marker_parenthesis)
  (task_list_marker_checked)
  (task_list_marker_unchecked)
] @markup.list

(thematic_break) @punctuation.special

(link_reference_definition
  (link_label) @markup.link.label)

(link_reference_definition
  (link_destination) @markup.link.url)

(link_title) @string

(backslash_escape) @string.escape

"|" @punctuation.delimiter
