package films

import (
	"slices"
	"testing"
)

// TestSelectionClicks checks the selection as Finder's clicks make it: a click selects a film alone, a ⌘-click adds one or leaves it out and moves the anchor to it, and a ⇧-click adds the films from the anchor to the one clicked, either way, keeping those selected before.
func TestSelectionClicks(t *testing.T) {
	order := []string{"a", "b", "c", "d", "e", "f"}
	var s Selection
	s.Only("b")
	s.Toggle("e")
	if want := []string{"b", "e"}; !slices.Equal(s.Keys, want) || s.Anchor != "e" || s.Lead() != "e" {
		t.Fatalf("⌘-click: %+v, lead %q", s, s.Lead())
	}
	s.Extend(order, "c")
	if want := []string{"b", "e", "c", "d"}; !slices.Equal(s.Keys, want) || s.Lead() != "c" || s.Anchor != "e" {
		t.Fatalf("⇧-click up from the anchor: %v, lead %q", s.Keys, s.Lead())
	}
	s.Toggle("c")
	if s.Has("c") || s.Lead() != "d" {
		t.Fatalf("⌘-click on a selected film leaves it out and the lead goes to the one selected last: %v, lead %q", s.Keys, s.Lead())
	}
	s.Only("a")
	if !slices.Equal(s.Keys, []string{"a"}) {
		t.Fatalf("a click selects one film alone: %v", s.Keys)
	}
	var none Selection
	none.Extend(order, "d")
	if !slices.Equal(none.Keys, []string{"d"}) || none.Anchor != "d" {
		t.Fatalf("⇧-click with nothing selected selects the film alone: %+v", none)
	}
	none.Extend(order, "gone")
	if !slices.Equal(none.Keys, []string{"d"}) {
		t.Fatalf("⇧-click on a film not shown changes nothing: %v", none.Keys)
	}
}

// TestSelectionAllKeepAndOf checks ⌘A, the films the page stops showing leaving the selection, and the selected films read in the page's order.
func TestSelectionAllKeepAndOf(t *testing.T) {
	var s Selection
	s.All([]string{"a", "b", "c"})
	if !slices.Equal(s.Keys, []string{"a", "b", "c"}) || s.Anchor != "a" {
		t.Fatalf("⌘A: %+v", s)
	}
	if !s.Keep([]string{"c", "b", "x"}) || !slices.Equal(s.Keys, []string{"b", "c"}) || s.Anchor != "" {
		t.Fatalf("a film no longer shown leaves, with the anchor: %+v", s)
	}
	if s.Keep([]string{"b", "c"}) {
		t.Fatal("keeping every film selected changed the selection")
	}
	items := []Film{{Key: "c"}, {Key: "a"}, {Key: "b"}}
	if got := Keys(s.Of(items)); !slices.Equal(got, []string{"c", "b"}) {
		t.Fatalf("selected films in the page's order: %v", got)
	}
	if !s.Keep(nil) || s.Len() != 0 || s.Lead() != "" {
		t.Fatalf("no film shown empties it: %+v", s)
	}
	s.Toggle("a")
	s.Clear()
	if s.Len() != 0 || s.Anchor != "" {
		t.Fatalf("Escape: %+v", s)
	}
}
