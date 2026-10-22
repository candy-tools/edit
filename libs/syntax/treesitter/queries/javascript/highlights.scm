; JavaScript, after the tree-sitter-javascript query gotreesitter bundles
; (see ../NOTICE). Where two patterns capture the same node, the later one
; wins, so general patterns come first.

(identifier) @variable

[
  (property_identifier)
  (shorthand_property_identifier)
  (private_property_identifier)
] @property

((identifier) @constructor
  (#match? @constructor "^[A-Z]"))

((identifier) @constant
  (#match? @constant "^_*[A-Z][A-Z0-9_]+$"))

; Functions, methods and classes, where they are declared and where they
; are called.

(function_declaration
  name: (identifier) @function)

(function_expression
  name: (identifier) @function)

(generator_function_declaration
  name: (identifier) @function)

(variable_declarator
  name: (identifier) @function
  value: [
    (function_expression)
    (arrow_function)
  ])

(method_definition
  name: (property_identifier) @function.method)

(pair
  key: (property_identifier) @function.method
  value: [
    (function_expression)
    (arrow_function)
  ])

(call_expression
  function: (identifier) @function)

(call_expression
  function: (member_expression
    property: (property_identifier) @function.method))

(new_expression
  constructor: (identifier) @constructor)

(class_declaration
  name: (identifier) @type)

(class
  name: (identifier) @type)

(class_heritage
  (identifier) @type)

; Built-ins.

((identifier) @variable.builtin
  (#any-of? @variable.builtin
    "arguments" "Array" "Boolean" "console" "Date" "document" "Error"
    "exports" "globalThis" "JSON" "Map" "Math" "module" "Number" "Object"
    "process" "Promise" "Reflect" "RegExp" "require" "Set" "String"
    "Symbol" "window"))

[
  (this)
  (super)
] @variable.builtin

; Literals.

[
  (true)
  (false)
] @boolean

[
  (null)
  (undefined)
] @constant.builtin

(number) @number

[
  (string)
  (template_string)
] @string

(escape_sequence) @string.escape

(regex) @string.special

[
  (comment)
  (hash_bang_line)
] @comment

; Keywords, operators and punctuation.

[
  "as"
  "async"
  "await"
  "break"
  "case"
  "catch"
  "class"
  "const"
  "continue"
  "debugger"
  "default"
  "delete"
  "do"
  "else"
  "export"
  "extends"
  "finally"
  "for"
  "from"
  "function"
  "get"
  "if"
  "import"
  "in"
  "instanceof"
  "let"
  "new"
  "of"
  "return"
  "set"
  "static"
  "switch"
  "throw"
  "try"
  "typeof"
  "using"
  "var"
  "void"
  "while"
  "with"
  "yield"
] @keyword

[
  "--"
  "-"
  "-="
  "&&"
  "+"
  "++"
  "+="
  "&="
  "/="
  "**="
  "<<="
  "<"
  "<="
  "<<"
  "="
  "=="
  "==="
  "!="
  "!=="
  "=>"
  ">"
  ">="
  ">>"
  "||"
  "%"
  "%="
  "*"
  "**"
  ">>>"
  "&"
  "|"
  "^"
  "??"
  "*="
  ">>="
  ">>>="
  "^="
  "|="
  "&&="
  "||="
  "??="
  "..."
  "!"
  "~"
  "?."
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
  ";"
  "."
  ","
  ":"
] @punctuation.delimiter

(template_substitution
  [
    "${"
    "}"
  ] @punctuation.special)
