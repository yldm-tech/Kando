package session

import (
	"slices"

	"kando/apps/mobile/store"
)

// Visit keeps the video at url, titled title ("" while the page has not named it), among the last ones looked at in Browse, newest first (store.Visited), with the user's other choices; a visit that changes nothing writes nothing.
func (s *Session) Visit(url, title string) {
	p := s.Prefs()
	visits := store.Visited(p.Visited, store.Visit{URL: url, Title: title})
	if slices.Equal(visits, p.Visited) {
		return
	}
	p.Visited = visits
	s.SetPrefs(p)
}
