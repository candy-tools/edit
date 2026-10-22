; Make, after the tree-sitter-make query gotreesitter bundles (see
; ../NOTICE). Where two patterns capture the same node, the later one wins.
; A variable's expansion, $(X) or $@, is string.special, as in shell; the
; rest of a recipe line stays plain.

(comment) @comment

(rule
  (targets
    (word) @function))

(rule
  (targets) @_target
  (prerequisites
    (word) @function)
  (#eq? @_target ".PHONY"))

(rule
  (targets
    (word) @function.builtin)
  (#any-of? @function.builtin
    ".DEFAULT" ".DELETE_ON_ERROR" ".EXPORT_ALL_VARIABLES" ".IGNORE"
    ".INTERMEDIATE" ".LOW_RESOLUTION_TIME" ".NOTPARALLEL" ".ONESHELL"
    ".PHONY" ".POSIX" ".PRECIOUS" ".SECONDARY" ".SECONDEXPANSION" ".SILENT"
    ".SUFFIXES"))

(variable_assignment
  name: (word) @variable)

(shell_assignment
  name: (word) @variable)

(define_directive
  name: (word) @variable)

[
  "define"
  "else"
  "endef"
  "endif"
  "export"
  "ifdef"
  "ifeq"
  "ifndef"
  "ifneq"
  "include"
  "-include"
  "override"
  "private"
  "sinclude"
  "undefine"
  "unexport"
  "vpath"
] @keyword

[
  "="
  ":="
  "::="
  "?="
  "+="
  "!="
] @operator

(rule
  [
    ":"
    "::"
    "&:"
    "|"
  ] @operator)

[
  (variable_reference)
  (automatic_variable)
  (substitution_reference)
] @string.special

(function_call
  [
    "$"
    "("
    ")"
  ] @string.special)

(function_call
  [
    "abspath" "addprefix" "addsuffix" "and" "basename" "call" "dir" "error"
    "eval" "file" "filter" "filter-out" "findstring" "firstword" "flavor"
    "foreach" "if" "info" "intcmp" "join" "lastword" "let" "notdir" "or"
    "origin" "patsubst" "realpath" "sort" "strip" "subst" "suffix" "value"
    "warning" "wildcard" "word" "wordlist" "words"
  ] @function.builtin)

(shell_function
  [
    "$"
    "("
    ")"
  ] @string.special
  "shell" @function.builtin)

(recipe_line
  [
    "@"
    "-"
    "+"
  ] @punctuation.special)
