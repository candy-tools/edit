// Package picker is the state of a popup that picks one of a list of items
// by a query: the query, typed on one line with a cursor, the items, a
// cursor over them and a window that follows it. It doesn't choose the
// items: the caller fills the list as the query changes. It draws nothing:
// the caller draws the query's line and the window.
//
// It is app-agnostic and depends only on libs/textedit and the standard
// library; a depguard rule in .golangci.yaml keeps it from importing the
// app.
package picker

import (
	"slices"
	"strings"

	"github.com/candy-tools/trazo/libs/textedit"
)

// Picker is a query and a list of items to pick from. It is not safe for
// concurrent use.
type Picker[T any] struct {
	query     *textedit.Buffer // the query, one line, with its cursor and window
	width     int              // the query window's width, for a query made anew
	prefilled bool             // the query is the one Prefill put in, still selected
	items     []T
	cursor    int // index into items of the cursor's item
	top       int // index into items of the window's first item
	height    int // how many items the window shows, at least 1
}

// oneLine turns each line break into a space, so a query stays one line.
var oneLine = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

// New returns a Picker with an empty query and no items, its query window
// one cell wide and its list one item tall until SetSize.
func New[T any]() *Picker[T] {
	p := &Picker[T]{width: 1, height: 1}
	p.reset("")
	return p
}

// reset makes query the query, the cursor after it, not selected.
func (p *Picker[T]) reset(query string) {
	p.query = textedit.New(oneLine.Replace(query), 1)
	p.query.SetSize(p.width, 1)
	p.query.Bottom()
	p.prefilled = false
}

// SetSize makes the query's window width cells wide and the list's window
// height items tall; a size below 1 counts as 1.
func (p *Picker[T]) SetSize(width, height int) {
	p.width, p.height = max(width, 1), max(height, 1)
	p.query.SetSize(p.width, 1)
	p.scroll()
}

// Query returns the query.
func (p *Picker[T]) Query() string { return p.query.String() }

// Prefill puts query in as the query, selected, the cursor after it:
// typing replaces it, deleting clears it, and moving the cursor keeps it
// and ends the selection. An empty query is not selected.
func (p *Picker[T]) Prefill(query string) {
	p.reset(query)
	p.prefilled = query != ""
}

// Prefilled reports whether the query is still the selected one Prefill
// put in.
func (p *Picker[T]) Prefilled() bool { return p.prefilled }

// Type types text at the query's cursor, each "\r\n", "\r" and "\n" as a
// space, so the query stays one line; a prefilled query is replaced.
// Typing "" does nothing.
func (p *Picker[T]) Type(text string) {
	if text == "" {
		return
	}
	if p.prefilled {
		p.reset("")
	}
	p.query.Insert(oneLine.Replace(text))
}

// Left moves the query's cursor a cell left.
func (p *Picker[T]) Left() {
	p.prefilled = false
	p.query.Left()
}

// Right moves the query's cursor a cell right.
func (p *Picker[T]) Right() {
	p.prefilled = false
	p.query.Right()
}

// Start moves the query's cursor to the query's start.
func (p *Picker[T]) Start() {
	p.prefilled = false
	p.query.LineStart()
}

// End moves the query's cursor to the query's end.
func (p *Picker[T]) End() {
	p.prefilled = false
	p.query.LineEnd()
}

// DeleteLeft deletes the cell before the query's cursor; a prefilled query
// it clears.
func (p *Picker[T]) DeleteLeft() {
	if p.prefilled {
		p.reset("")
		return
	}
	p.query.DeleteLeft()
}

// DeleteRight deletes the cell under the query's cursor; a prefilled query
// it clears.
func (p *Picker[T]) DeleteRight() {
	if p.prefilled {
		p.reset("")
		return
	}
	p.query.DeleteRight()
}

// QueryLine returns the query as its window shows it, with its cursor's
// cell a run of its own.
func (p *Picker[T]) QueryLine() textedit.Line { return p.query.Window(nil)[0] }

// SetItems makes a copy of items the list, the cursor on the first item.
func (p *Picker[T]) SetItems(items []T) {
	p.items = slices.Clone(items)
	p.cursor, p.top = 0, 0
}

// Append adds items at the end of the list. The cursor and the window stay
// where they are: on the first item, when the list was empty.
func (p *Picker[T]) Append(items ...T) { p.items = append(p.items, items...) }

// Len returns how many items the list has.
func (p *Picker[T]) Len() int { return len(p.items) }

// Select puts the cursor on item i, clamped to the list; when the item is
// out of the window, the window scrolls to show it in its middle. On an
// empty list it does nothing.
func (p *Picker[T]) Select(i int) {
	if len(p.items) == 0 {
		return
	}
	p.cursor = min(max(i, 0), len(p.items)-1)
	if p.cursor < p.top || p.cursor >= p.top+p.height {
		p.top = p.cursor - p.height/2
	}
	p.scroll()
}

// Up moves the cursor an item up, stopping at the first.
func (p *Picker[T]) Up() { p.moveTo(p.cursor - 1) }

// Down moves the cursor an item down, stopping at the last.
func (p *Picker[T]) Down() { p.moveTo(p.cursor + 1) }

// PageUp moves the cursor a window's height up, stopping at the first item.
func (p *Picker[T]) PageUp() { p.moveTo(p.cursor - p.height) }

// PageDown moves the cursor a window's height down, stopping at the last
// item.
func (p *Picker[T]) PageDown() { p.moveTo(p.cursor + p.height) }

// moveTo puts the cursor on item i, clamped to the list, and moves the
// window as little as needed to show it. On an empty list it does nothing.
func (p *Picker[T]) moveTo(i int) {
	if len(p.items) == 0 {
		return
	}
	p.cursor = min(max(i, 0), len(p.items)-1)
	p.scroll()
}

// scroll moves the window as little as needed to show the cursor's item,
// then keeps it from showing empty rows below the last item while earlier
// items are out of view.
func (p *Picker[T]) scroll() {
	p.top = max(min(p.top, p.cursor), p.cursor-p.height+1)
	p.top = max(min(p.top, len(p.items)-p.height), 0)
}

// Current returns the item under the cursor, or false when the list is
// empty.
func (p *Picker[T]) Current() (T, bool) {
	if len(p.items) == 0 {
		var zero T
		return zero, false
	}
	return p.items[p.cursor], true
}

// Window returns the items in the window, top to bottom, and the cursor's
// index among them, -1 when the list is empty. It doesn't change the
// Picker, so it is safe to call from a Bubble Tea View.
func (p *Picker[T]) Window() (items []T, cursor int) {
	if len(p.items) == 0 {
		return nil, -1
	}
	end := min(p.top+p.height, len(p.items))
	return slices.Clone(p.items[p.top:end]), p.cursor - p.top
}
