; Shell, after the tree-sitter-bash query gotreesitter bundles (see
; ../NOTICE). Where two patterns capture the same node, the later one wins,
; so general patterns come first. An expansion — $x, ${x}, $(…), $((…)) —
; is string.special, the text it stands for being decided when it runs.

[
  (string)
  (raw_string)
  (ansi_c_string)
  (translated_string)
  (heredoc_body)
] @string

[
  (heredoc_start)
  (heredoc_end)
] @label

(variable_assignment
  name: (variable_name) @variable)

(for_statement
  variable: (variable_name) @variable)

(command_name) @function

((command_name
  (word) @function.builtin)
  (#any-of? @function.builtin
    "alias" "bg" "bind" "break" "builtin" "caller" "cd" "command" "compgen"
    "complete" "continue" "dirs" "disown" "echo" "enable" "eval" "exec"
    "exit" "false" "fc" "fg" "getopts" "hash" "help" "history" "jobs" "kill"
    "let" "logout" "popd" "printf" "pushd" "pwd" "read" "return" "set"
    "shift" "shopt" "source" "suspend" "test" "times" "trap" "true" "type"
    "ulimit" "umask" "unalias" "wait"))

(function_definition
  name: (word) @function)

((command
  argument: (word) @constant)
  (#match? @constant "^-"))

[
  (number)
  (file_descriptor)
] @number

(comment) @comment

(test_operator) @operator

[
  "case"
  "declare"
  "do"
  "done"
  "elif"
  "else"
  "esac"
  "export"
  "fi"
  "for"
  "function"
  "if"
  "in"
  "local"
  "readonly"
  "select"
  "then"
  "typeset"
  "unset"
  "unsetenv"
  "until"
  "while"
] @keyword

[
  "!"
  "&&"
  "&>"
  "&>>"
  "<"
  "<&"
  "<<"
  "<<-"
  "<<<"
  "="
  "=="
  "!="
  "=~"
  "+="
  ">"
  ">&"
  ">>"
  ">|"
  "|"
  "|&"
  "||"
] @operator

[
  "("
  ")"
  "(("
  "))"
  "["
  "]"
  "[["
  "]]"
  "{"
  "}"
] @punctuation.bracket

[
  ";"
  ";;"
  ";&"
  ";;&"
] @punctuation.delimiter

; Expansions, over the brackets and words in them.

[
  (simple_expansion)
  (expansion)
] @string.special

(expansion
  [
    "${"
    "}"
  ] @string.special)

(command_substitution
  [
    "$("
    ")"
    "`"
  ] @string.special)

(arithmetic_expansion
  [
    "$(("
    "))"
  ] @string.special)
