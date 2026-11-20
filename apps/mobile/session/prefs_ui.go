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

// SendTo is the paired computer that processes new links, as Settings shows it and the Add sheet starts on it: the one the user chose in Settings while it is paired; else the first that answers and takes links, else the first that may take them once it answers (Queue.Takes), else the first paired; "" with none paired.
func (s *Session) SendTo() string {
	ds := s.Desktops()
	want := s.Prefs().SendTo
	for _, d := range ds {
		if d.ID == want {
			return d.ID
		}
	}
	for _, d := range ds {
		if q := s.Queue(d.ID); q.Reach == Online && q.Takes() {
			return d.ID
		}
	}
	for _, d := range ds {
		if s.Queue(d.ID).Takes() {
			return d.ID
		}
	}
	if len(ds) > 0 {
		return ds[0].ID
	}
	return ""
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
