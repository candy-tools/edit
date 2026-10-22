; Go, after the tree-sitter-go query gotreesitter bundles (see ../NOTICE).
; Where two patterns capture the same node, the later one wins, so general
; patterns come first.

(identifier) @variable

(type_identifier) @type

(field_identifier) @property

(label_name) @label

; Functions and methods, where they are declared and where they are called.

(function_declaration
  name: (identifier) @function)

(method_declaration
  name: (field_identifier) @function.method)

(method_elem
  name: (field_identifier) @function.method)

(call_expression
  function: (identifier) @function)

(call_expression
  function: (selector_expression
    field: (field_identifier) @function.method))

; Predeclared identifiers.

((call_expression
  function: (identifier) @function.builtin)
  (#any-of? @function.builtin
    "append" "cap" "clear" "close" "complex" "copy" "delete" "imag" "len"
    "make" "max" "min" "new" "panic" "print" "println" "real" "recover"))

((type_identifier) @type.builtin
  (#any-of? @type.builtin
    "any" "bool" "byte" "comparable" "complex64" "complex128" "error"
    "float32" "float64" "int" "int8" "int16" "int32" "int64" "rune"
    "string" "uint" "uint8" "uint16" "uint32" "uint64" "uintptr"))

[
  (true)
  (false)
] @boolean

[
  (nil)
  (iota)
] @constant.builtin

; Literals.

[
  (interpreted_string_literal)
  (raw_string_literal)
] @string

(rune_literal) @character

(escape_sequence) @string.escape

[
  (int_literal)
  (float_literal)
  (imaginary_literal)
] @number

(comment) @comment

; Keywords, operators and punctuation.

[
  "break"
  "case"
  "chan"
  "const"
  "continue"
  "default"
  "defer"
  "else"
  "fallthrough"
  "for"
  "func"
  "go"
  "goto"
  "if"
  "import"
  "interface"
  "map"
  "package"
  "range"
  "return"
  "select"
  "struct"
  "switch"
  "type"
  "var"
] @keyword

[
  "--"
  "-"
  "-="
  ":="
  "!"
  "!="
  "..."
  "*"
  "*="
  "/"
  "/="
  "&"
  "&&"
  "&="
  "&^"
  "&^="
  "%"
  "%="
  "^"
  "^="
  "+"
  "++"
  "+="
  "<-"
  "<"
  "<<"
  "<<="
  "<="
  "="
  "=="
  ">"
  ">="
  ">>"
  ">>="
  "|"
  "|="
  "||"
  "~"
] @operator

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
  "."
  ":"
  ";"
] @punctuation.delimiter
