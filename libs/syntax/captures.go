package syntax

import "slices"

// captures is the vocabulary of capture names: the ones current Neovim and
// Helix themes key on, cut down to what trazo's languages use.
var captures = []string{
	// code
	"comment", "keyword", "string", "string.escape", "string.special", "character",
	"number", "boolean", "constant", "constant.builtin", "type", "type.builtin",
	"function", "function.builtin", "function.method", "constructor", "property",
	"variable", "variable.builtin", "variable.parameter", "attribute", "label",
	"operator", "punctuation", "punctuation.bracket", "punctuation.delimiter",
	"punctuation.special", "none",
	// markup
	"markup.heading", "markup.strong", "markup.italic", "markup.raw",
	"markup.link.label", "markup.link.url", "markup.quote", "markup.list",
}

// Captures returns the names an engine may capture: a Span's Capture is one
// of them, but never "none", which an engine captures to draw a stretch as
// plain text whatever contains it.
func Captures() []string { return slices.Clone(captures) }
