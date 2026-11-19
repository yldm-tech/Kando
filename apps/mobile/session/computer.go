package session

import (
	"context"
	"maps"

	"kando/packages/sync"
)

// DesktopSettings are a paired computer's settings as its last sync answered them (sync.Settings): the AI service it translates with, which the phone only shows, and whether its translation sends the lines before a batch, which the phone sets; and whether the computer takes the phone's changes, as a read-only phone's does not (sync.Hello.ReadOnly).
type DesktopSettings struct {
	sync.Settings
	ReadOnly bool
}

// DesktopSettings are computer id's settings as it answered them at the last sync that reached it; false until one did this run, or for a computer that does not share them.
func (s *Session) DesktopSettings(id string) (DesktopSettings, bool) {
	got, ok := s.learn.settings.Get()[id]
	return got, ok
}

// setDesktopSettings keeps computer id's settings as it answered them.
func (s *Session) setDesktopSettings(id string, got DesktopSettings) {
	m := maps.Clone(s.learn.settings.GetUntracked())
	if m == nil {
		m = map[string]DesktopSettings{}
	}
	m[id] = got
	s.learn.settings.Set(m)
}

// forgetDesktopSettings forgets computer id's settings, as it is unpaired.
func (s *Session) forgetDesktopSettings(id string) {
	if _, ok := s.learn.settings.GetUntracked()[id]; !ok {
		return
	}
	m := maps.Clone(s.learn.settings.GetUntracked())
	delete(m, id)
	s.learn.settings.Set(m)
}

// SetDesktopContext turns computer id's sending of the lines before a batch with it as it translates on or off, there at once: the computer must be in reach, and done, on the engine's goroutine, is told why it was not changed, nil once it was. Unlike the language translated into, which the phone keeps and every computer takes, it is the one computer's own setting, so a change it cannot take now is not kept for later.
func (s *Session) SetDesktopContext(id string, on bool, done func(error)) {
	c := s.Client(id)
	if c == nil {
		done(&sync.Error{Status: 401})
		return
	}
	s.learn.settingsPuts[id]++
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, learnSyncFor)
		defer cancel()
		got, err := c.PutSettings(ctx, sync.SettingsPut{Context: &on})
		s.cx.Post(func() {
			s.learn.settingsPuts[id]++
			if _, paired := s.Desktop(id); err == nil && paired {
				was, _ := s.DesktopSettings(id)
				s.setDesktopSettings(id, DesktopSettings{Settings: got, ReadOnly: was.ReadOnly})
			}
			done(err)
		})
	})
}
