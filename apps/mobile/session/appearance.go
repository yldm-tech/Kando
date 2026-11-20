package session

import "kando/apps/mobile/store"

// SetAppearance keeps a, store.AppearanceLight, store.AppearanceDark or "" for the system's, as the app's look, with the user's other choices; the app's root shows it (Appearance).
func (s *Session) SetAppearance(a string) {
	p := s.Prefs()
	if p.Appearance == a {
		return
	}
	p.Appearance = a
	s.SetPrefs(p)
}

// NextAppearance is the appearance after a in the cycle Settings steps through: light, dark, then the system's.
func NextAppearance(a string) string {
	switch a {
	case store.AppearanceLight:
		return store.AppearanceDark
	case store.AppearanceDark:
		return ""
	}
	return store.AppearanceLight
}
