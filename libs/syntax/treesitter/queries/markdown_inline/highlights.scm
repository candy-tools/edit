; Markdown's inline syntax, injected into the block syntax's inlines and
; table cells; after the tree-sitter-markdown-inline query gotreesitter
; bundles (see ../NOTICE).

(emphasis) @markup.italic

(strong_emphasis) @markup.strong

(code_span) @markup.raw

[
  (link_text)
  (link_label)
  (image_description)
] @markup.link.label

[
  (link_destination)
  (uri_autolink)
  (email_autolink)
] @markup.link.url

(link_title) @string

[
  (backslash_escape)
  (entity_reference)
  (numeric_character_reference)
] @string.escape
