; Java, after the tree-sitter-java query gotreesitter bundles (see
; ../NOTICE). Where two patterns capture the same node, the later one wins,
; so general patterns come first.

(identifier) @variable

((identifier) @constant
  (#match? @constant "^_*[A-Z][A-Z0-9_]+$"))

(type_identifier) @type

[
  (boolean_type)
  (integral_type)
  (floating_point_type)
  (void_type)
] @type.builtin

(field_access
  field: (identifier) @property)

; Types, methods and annotations, where they are declared and where they
; are used.

[
  (class_declaration
    name: (identifier) @type)
  (interface_declaration
    name: (identifier) @type)
  (enum_declaration
    name: (identifier) @type)
  (record_declaration
    name: (identifier) @type)
  (annotation_type_declaration
    name: (identifier) @type)
]

(method_declaration
  name: (identifier) @function.method)

(method_invocation
  name: (identifier) @function.method)

(constructor_declaration
  name: (identifier) @constructor)

(object_creation_expression
  type: (type_identifier) @constructor)

(object_creation_expression
  type: (generic_type
    (type_identifier) @constructor))

(annotation
  name: (identifier) @attribute)

(marker_annotation
  name: (identifier) @attribute)

[
  (this)
  (super)
] @variable.builtin

; Literals.

[
  (hex_integer_literal)
  (decimal_integer_literal)
  (octal_integer_literal)
  (binary_integer_literal)
  (decimal_floating_point_literal)
  (hex_floating_point_literal)
] @number

(character_literal) @character

(string_literal) @string

(escape_sequence) @string.escape

[
  (true)
  (false)
] @boolean

(null_literal) @constant.builtin

[
  (line_comment)
  (block_comment)
] @comment

; Keywords, operators and punctuation.

[
  "abstract"
  "assert"
  "break"
  "case"
  "catch"
  "class"
  "continue"
  "default"
  "do"
  "else"
  "enum"
  "exports"
  "extends"
  "final"
  "finally"
  "for"
  "if"
  "implements"
  "import"
  "instanceof"
  "interface"
  "@interface"
  "module"
  "native"
  "new"
  "non-sealed"
  "open"
  "opens"
  "package"
  "permits"
  "private"
  "protected"
  "provides"
  "public"
  "record"
  "requires"
  "return"
  "sealed"
  "static"
  "strictfp"
  "switch"
  "synchronized"
  "throw"
  "throws"
  "to"
  "transient"
  "transitive"
  "try"
  "uses"
  "volatile"
  "when"
  "while"
  "with"
  "yield"
] @keyword

[
  "!"
  "!="
  "%"
  "%="
  "&"
  "&&"
  "&="
  "*"
  "*="
  "+"
  "++"
  "+="
  "-"
  "--"
  "-="
  "->"
  "/"
  "/="
  "::"
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
  ">>>"
  ">>>="
  "^"
  "^="
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

[
  (type_arguments
    [
      "<"
      ">"
    ] @punctuation.bracket)
  (type_parameters
    [
      "<"
      ">"
    ] @punctuation.bracket)
]

[
  (annotation
    "@" @attribute)
  (marker_annotation
    "@" @attribute)
]
