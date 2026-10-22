; Markdown's injections: a fenced code block's code in the language its info
; string names, and the inline syntax into every inline and table cell.

(fenced_code_block
  (info_string
    (language) @injection.language)
  (code_fence_content) @injection.content)

((inline) @injection.content
  (#set! injection.language "markdown_inline"))

((pipe_table_cell) @injection.content
  (#set! injection.language "markdown_inline"))
