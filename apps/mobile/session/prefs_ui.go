package session

// SetLanguage keeps lang, one of locales.Interface or "" for the system's, as the language of the app's interface, with the user's other choices; the app's root shows it.
func (s *Session) SetLanguage(lang string) {
	p := s.Prefs()
	if p.Language == lang {
		return
	}
	p.Language = lang
	s.SetPrefs(p)
}

// SendTo is the paired computer Settings shows as the one that processes new links: the one the user chose while it is paired, else the first paired; "" with none paired.
func (s *Session) SendTo() string {
	ds := s.Desktops()
	if len(ds) == 0 {
		return ""
	}
	want := s.Prefs().SendTo
	for _, d := range ds {
		if d.ID == want {
			return d.ID
		}
	}
	return ds[0].ID
}

// SetSendTo keeps computer id as the one new links go to.
func (s *Session) SetSendTo(id string) {
	p := s.Prefs()
	if p.SendTo == id {
		return
	}
	p.SendTo = id
	s.SetPrefs(p)
}
