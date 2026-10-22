; Python, after the tree-sitter-python query gotreesitter bundles (see
; ../NOTICE). Where two patterns capture the same node, the later one wins,
; so general patterns come first.

(identifier) @variable

((identifier) @constructor
  (#match? @constructor "^[A-Z]"))

((identifier) @constant
  (#match? @constant "^_*[A-Z][A-Z0-9_]+$"))

(attribute
  attribute: (identifier) @property)

(type
  (identifier) @type)

; Functions, methods and classes, where they are defined and where they are
; called.

(function_definition
  name: (identifier) @function)

(class_definition
  name: (identifier) @type)

(class_definition
  superclasses: (argument_list
    (identifier) @type))

(call
  function: (identifier) @function)

(call
  function: (attribute
    attribute: (identifier) @function.method))

((call
  function: (identifier) @constructor)
  (#match? @constructor "^[A-Z]"))

; Built-ins.

((call
  function: (identifier) @function.builtin)
  (#any-of? @function.builtin
    "abs" "all" "any" "ascii" "bin" "bool" "breakpoint" "bytearray" "bytes"
    "callable" "chr" "classmethod" "compile" "complex" "delattr" "dict" "dir"
    "divmod" "enumerate" "eval" "exec" "filter" "float" "format" "frozenset"
    "getattr" "globals" "hasattr" "hash" "help" "hex" "id" "input" "int"
    "isinstance" "issubclass" "iter" "len" "list" "locals" "map" "max"
    "memoryview" "min" "next" "object" "oct" "open" "ord" "pow" "print"
    "property" "range" "repr" "reversed" "round" "set" "setattr" "slice"
    "sorted" "staticmethod" "str" "sum" "super" "tuple" "type" "vars" "zip"
    "__import__"))

((identifier) @variable.builtin
  (#any-of? @variable.builtin "self" "cls"))

; Literals.

[
  (true)
  (false)
] @boolean

(none) @constant.builtin

[
  (integer)
  (float)
] @number

(string) @string

[
  (escape_sequence)
  (escape_interpolation)
] @string.escape

(comment) @comment

; Keywords, operators and punctuation.

[
  "and"
  "as"
  "assert"
  "async"
  "await"
  "break"
  "case"
  "class"
  "continue"
  "def"
  "del"
  "elif"
  "else"
  "except"
  "finally"
  "for"
  "from"
  "global"
  "if"
  "import"
  "in"
  "is"
  "is not"
  "lambda"
  "match"
  "nonlocal"
  "not"
  "not in"
  "or"
  "pass"
  "raise"
  "return"
  "try"
  "type"
  "while"
  "with"
  "yield"
] @keyword

[
  "-"
  "-="
  "!="
  "*"
  "**"
  "**="
  "*="
  "/"
  "//"
  "//="
  "/="
  "&"
  "&="
  "%"
  "%="
  "^"
  "^="
  "+"
  "->"
  "+="
  "<"
  "<<"
  "<<="
  "<="
  "<>"
  "="
  ":="
  "=="
  ">"
  ">="
  ">>"
  ">>="
  "|"
  "|="
  "~"
  "@"
  "@="
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

(interpolation
  [
    "{"
    "}"
  ] @punctuation.special)

; Decorators, over the names and the @ in them.

(decorator) @attribute

(decorator
  "@" @attribute)

(decorator
  (identifier) @attribute)

(decorator
  (attribute
    (identifier) @attribute))

(decorator
  (call
    function: (identifier) @attribute))

(decorator
  (call
    function: (attribute
      (identifier) @attribute)))
