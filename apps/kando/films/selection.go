package films

import "slices"

// Selection is the done films selected on the library page, by key, so it stays with them as they are sorted again, and is the same in the posters and the list, as Finder's is across its views. Keys are in the order they were selected; Of reads them in the order the page shows them. Anchor is the film a ⇧-click or ⇧↑ and ⇧↓ select from, and lead the one the last click or key went to, which the list's keyboard moves on from (Lead). The app keeps it, so the menu bar and ⌘A act on it too, and keeps it to the films the page shows (Keep).
type Selection struct {
	Keys   []string
	Anchor string
	lead   string
}

// Has reports whether the film with key is selected.
func (s Selection) Has(key string) bool { return slices.Contains(s.Keys, key) }

// Len is how many films are selected.
func (s Selection) Len() int { return len(s.Keys) }

// Lead is the selected film the list's keyboard is on: the one the last click or key went to while it is selected, else the one selected last; "" when none is.
func (s Selection) Lead() string {
	switch {
	case len(s.Keys) == 0:
		return ""
	case s.Has(s.lead):
		return s.lead
	}
	return s.Keys[len(s.Keys)-1]
}

// Only selects the film with key alone, as a click does in the list.
func (s *Selection) Only(key string) {
	s.Keys, s.Anchor, s.lead = []string{key}, key, key
}

// Toggle selects the film with key, or leaves it out when it is selected, as a ⌘-click does; the next ⇧-click selects from it.
func (s *Selection) Toggle(key string) {
	if i := slices.Index(s.Keys, key); i >= 0 {
		s.Keys = slices.Delete(slices.Clone(s.Keys), i, i+1)
	} else {
		s.Keys = append(slices.Clone(s.Keys), key)
	}
	s.Anchor, s.lead = key, key
}

// Extend adds to the selection the films from the anchor to the one with key, in order, the keys of the films the page shows in the order it shows them, as a ⇧-click does. Without an anchor among them it selects that film alone.
func (s *Selection) Extend(order []string, key string) {
	from, to := slices.Index(order, s.Anchor), slices.Index(order, key)
	if to < 0 {
		return
	}
	if from < 0 {
		s.Only(key)
		return
	}
	keys := slices.Clone(s.Keys)
	for _, k := range order[min(from, to) : max(from, to)+1] {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	s.Keys, s.lead = keys, key
}

// All selects every film of order, the keys of the films the page shows, as ⌘A does.
func (s *Selection) All(order []string) {
	s.Keys = slices.Clone(order)
	if len(order) > 0 && !slices.Contains(order, s.Anchor) {
		s.Anchor, s.lead = order[0], order[0]
	}
}

// Clear selects nothing, as Escape does.
func (s *Selection) Clear() { *s = Selection{} }

// Keep leaves out of the selection the films that are not in order, the keys of the films the page shows, as when a filter or a search hides them, they went into 处理中, or they left the library, and reports whether it left any out. The anchor goes with its film.
func (s *Selection) Keep(order []string) bool {
	if len(s.Keys) == 0 {
		return false
	}
	kept := slices.DeleteFunc(slices.Clone(s.Keys), func(k string) bool { return !slices.Contains(order, k) })
	if len(kept) == len(s.Keys) {
		return false
	}
	s.Keys = kept
	if !slices.Contains(order, s.Anchor) {
		s.Anchor = ""
	}
	if len(kept) == 0 {
		*s = Selection{}
	}
	return true
}

// Of is the films of items that are selected, in the order of items.
func (s Selection) Of(items []Film) []Film {
	var out []Film
	for _, item := range items {
		if s.Has(item.Key) {
			out = append(out, item)
		}
	}
	return out
}

// Keys is the keys of items, in their order, as Extend, Step, All and Keep take them.
func Keys(items []Film) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Key
	}
	return out
}
