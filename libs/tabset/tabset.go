// Package tabset is the state of a row of tabs: which are open, in what
// order, which one is active, and which of them a strip of a given width
// shows. It knows nothing about what a tab holds or how it is drawn: the
// caller gives each tab a key and a value, and measures the tabs for the
// strip.
//
// It is app-agnostic and depends only on the standard library; a depguard
// rule in .golangci.yaml keeps it from importing the app.
package tabset

import (
	"fmt"
	"slices"
)

// Tab is one open tab.
type Tab[K comparable, V any] struct {
	Key   K // tells tabs apart: at most one open tab has a given key
	Value V // what the tab holds; a pointer lets the caller change it in place
}

// Set is the state of a row of tabs: their order, the active tab and the
// window of tabs a strip shows. It is not safe for concurrent use.
type Set[K comparable, V any] struct {
	measure func(tabs []Tab[K, V]) []int // how many cells each tab takes in the strip
	tabs    []Tab[K, V]                  // the open tabs, left to right
	active  int                          // index into tabs of the active tab; -1 when none is open
	width   int                          // how many cells the strip has, at least 0
	first   int                          // index into tabs of the first tab in the window
	end     int                          // index into tabs one past the last tab in the window
}

// New returns an empty Set whose tabs measure sizes: given the open tabs,
// left to right, it returns how many cells each one takes in the strip, one
// width per tab. The strip is 0 cells wide until SetWidth is called.
func New[K comparable, V any](measure func(tabs []Tab[K, V]) []int) *Set[K, V] {
	return &Set[K, V]{measure: measure, active: -1}
}

// SetWidth makes the strip w cells wide (a width below 0 counts as 0) and
// moves the window to keep the active tab in view.
func (s *Set[K, V]) SetWidth(w int) {
	s.width = max(w, 0)
	s.fit()
}

// Open makes the tab with key active. If no open tab has that key, it first
// calls value once and opens a tab holding the result at the right end;
// value is not called when the tab is already open.
func (s *Set[K, V]) Open(key K, value func() V) {
	i := slices.IndexFunc(s.tabs, func(t Tab[K, V]) bool { return t.Key == key })
	if i < 0 {
		s.tabs = append(s.tabs, Tab[K, V]{Key: key, Value: value()})
		i = len(s.tabs) - 1
	}
	s.active = i
	s.fit()
}

// Close closes the active tab. The tab that was to its right becomes active
// or, if it was the rightmost, the one to its left. Closing the only tab
// leaves the set empty; on an empty set Close does nothing.
func (s *Set[K, V]) Close() {
	if s.active < 0 {
		return
	}
	s.tabs = slices.Delete(s.tabs, s.active, s.active+1)
	s.active = min(s.active, len(s.tabs)-1)
	s.fit()
}

// Next activates the tab to the right of the active one, wrapping around from
// the last tab to the first. On an empty set it does nothing.
func (s *Set[K, V]) Next() { s.activate(s.active + 1) }

// Prev activates the tab to the left of the active one, wrapping around from
// the first tab to the last. On an empty set it does nothing.
func (s *Set[K, V]) Prev() { s.activate(s.active - 1) }

// Len returns how many tabs are open.
func (s *Set[K, V]) Len() int { return len(s.tabs) }

// Active returns the active tab, or false when no tab is open.
func (s *Set[K, V]) Active() (Tab[K, V], bool) {
	if s.active < 0 {
		return Tab[K, V]{}, false
	}
	return s.tabs[s.active], true
}

// Tabs returns the open tabs, left to right, in a new slice, and the index of
// the active one (-1 when no tab is open).
func (s *Set[K, V]) Tabs() (tabs []Tab[K, V], active int) {
	return slices.Clone(s.tabs), s.active
}

// Window returns the tabs the strip shows, as the index range [first, end)
// into Tabs: (0, 0) when no tab is open. It does not change the set, so it is
// safe to call from a Bubble Tea View.
func (s *Set[K, V]) Window() (first, end int) { return s.first, s.end }

// activate makes tab i active, wrapping i around the ends of the row. On an
// empty set it does nothing.
func (s *Set[K, V]) activate(i int) {
	n := len(s.tabs)
	if n == 0 {
		return
	}
	s.active = (i%n + n) % n
	s.fit()
}

// fit measures the tabs, then moves the window only as far as needed to keep
// the active tab in it, and pulls it back left while the tabs after it leave
// room, so the strip never leaves cells unused at the right while tabs on the
// left are out of view.
func (s *Set[K, V]) fit() {
	widths := s.measure(slices.Clone(s.tabs))
	if len(widths) != len(s.tabs) {
		panic(fmt.Sprintf("tabset: measure returned %d widths for %d tabs", len(widths), len(s.tabs)))
	}
	if len(s.tabs) == 0 {
		s.first, s.end = 0, 0
		return
	}
	// fits reports whether the tabs from..to, both included, fit in the strip.
	fits := func(from, to int) bool {
		sum := 0
		for _, w := range widths[from : to+1] {
			sum += max(w, 0)
		}
		return sum <= s.width
	}
	s.first = min(s.first, s.active)
	for s.first < s.active && !fits(s.first, s.active) {
		s.first++
	}
	for s.first > 0 && fits(s.first-1, len(s.tabs)-1) {
		s.first--
	}
	s.end = s.active + 1
	for s.end < len(s.tabs) && fits(s.first, s.end) {
		s.end++
	}
}
