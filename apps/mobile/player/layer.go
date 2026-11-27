package player

import (
	"time"

	"kando/apps/mobile/session"

	"github.com/nsgo-dev/nsgo/core"
)

// Layer is the film the app plays: the player over the whole app, or, minimized, a bar over the tabs (MiniBar) while the film plays on, from which a tap opens the player again where it was. The player stays one and the same while it is minimized, so the film plays on without a break and keeps its loop, its sleep timer and the word looked up; it goes once another film plays, or once its film is no longer on the phone. Going on to the next film of a course or playlist (next.go) keeps the sleep timer running in the next film's player.
type Layer struct {
	s *session.Session
	// key is the film playing, "" for none; full is set while the player shows over the app, unset while it is minimized.
	key  *core.Signal[string]
	full *core.Signal[bool]
	// cur is the player's state while it is built, nil before.
	cur *core.Signal[*play]
	// held is set while the film playing was opened held where it was left (PlayHeld): its player shows the recap of the part watched until it is played (newRecapper).
	held bool
	// carried is what the player gives the next film's as it goes on to it (goOn), which that player takes as it is built (carry).
	carried carried
	// OpenStyle opens Settings › Subtitle Style, as the playback settings' row does; nil leaves the row out.
	OpenStyle func()
	// feed is the films of the list the film playing was opened from, in the list's order, which a vertical swipe goes on through (neighbor), nil for none; asked is the one SetFeed gave for the film opened next.
	feed, asked func() []string
}

// NewLayer is the app's player, with nothing playing yet.
func NewLayer(cx *core.Scope, s *session.Session) *Layer {
	l := &Layer{s: s, key: core.NewSignal(cx, ""), full: core.NewSignal(cx, false), cur: core.NewSignal[*play](cx, nil)}
	// A film deleted from the phone stops playing.
	cx.Effect(func() {
		k := l.key.Get()
		if k == "" || !s.Loaded() {
			return
		}
		if _, ok := s.Film(k); !ok {
			l.key.Set("")
			l.full.Set(false)
		}
	})
	return l
}

// Play shows the player of the film kept under key over the app: the one minimized when it plays that film, else a new one, which starts where the film was left.
func (l *Layer) Play(key string) { l.open(key, false) }

// PlayHeld shows the player of the film kept under key over the app held where the film was left, with the recap of the part watched, as from the film's page; the one minimized when it plays that film shows as it is.
func (l *Layer) PlayHeld(key string) { l.open(key, true) }

// open shows the player of the film kept under key over the app, a new one opened held (held) unless it is the one minimized. A computer's film matched to a film of a source plays as that film, which plays either video and holds where both were left (session.PlayKey). It takes the list SetFeed gave for it; a film opened from elsewhere, as its page or a widget, has none.
func (l *Layer) open(key string, held bool) {
	key = l.s.PlayKey(key)
	if l.key.GetUntracked() != key {
		l.held = held
		l.feed = l.asked
		l.key.Set(key)
	} else if l.asked != nil {
		l.feed = l.asked
	}
	l.asked = nil
	l.full.Set(true)
}

// SetFeed gives the player the films of the list the film about to be opened is played from, in the list's order as the list shows them, read when a swipe asks for the next or the one before: the Library's rows as filtered, searched and scoped, Continue Watching, a work's films or a folder of a NAS.
func (l *Layer) SetFeed(keys func() []string) { l.asked = keys }

// neighbor is the film a swipe from the film under key goes on to, after it (step 1) or before it (step -1) in the list it was opened from, skipping those that cannot play now; "" for none, at either end or without a list.
func (l *Layer) neighbor(key string, step int) string {
	if l.feed == nil {
		return ""
	}
	keys := l.feed()
	at := -1
	for i, k := range keys {
		if l.s.PlayKey(k) == key {
			at = i
			break
		}
	}
	if at < 0 {
		return ""
	}
	for i := at + step; i >= 0 && i < len(keys); i += step {
		if k := l.s.PlayKey(keys[i]); k != key && l.playable(k) {
			return k
		}
	}
	return ""
}

// playable reports whether the film under key can play now, as a row of the Library opens it in the player: its video on the phone, a film of a NAS, one kept to stream whose computer or storage answers, or a computer's film matched to one.
func (l *Layer) playable(key string) bool {
	k, ok := l.s.Film(key)
	return ok && (k.Done() && k.HasVideo() || k.FromSource() || (k.Streamed && l.s.StreamLink(key) != "")) || l.s.Matched(key)
}

// PlayAt shows the player of the film kept under key over the app, playing from at.
func (l *Layer) PlayAt(key string, at time.Duration) {
	key = l.s.PlayKey(key)
	l.s.SavePosition(key, at)
	if p := l.cur.GetUntracked(); p != nil && p.key == key {
		p.seek(at)
	}
	l.Play(key)
}

// Minimize leaves the player for the tabs, the film playing on in the mini player.
func (l *Layer) Minimize() {
	if l.key.GetUntracked() != "" {
		l.full.Set(false)
	}
}

// Full reports whether the player shows over the app. Read it in a binding.
func (l *Layer) Full() bool { return l.key.Get() != "" && l.full.Get() }

// Mini reports whether the mini player shows over the tabs: not while the film plays in picture in picture, whose window stands in for it. Read it in a binding.
func (l *Layer) Mini() bool {
	p := l.cur.Get()
	return l.key.Get() != "" && !l.full.Get() && p != nil && !p.pip.Get().Active
}

// Room is how much of a tab page's foot the mini player covers, 0 while it does not show, for look.ProvideMini: its bar and the gap under it, over a tab bar (overTabBar) or beside a tablet's sidebar. Read it in a binding.
func (l *Layer) Room(overTabBar bool) core.Points {
	switch {
	case !l.Mini():
		return 0
	case overTabBar:
		return miniHeight + miniGap
	}
	return miniHeight + miniSideGap
}

// StatusBar is the status bar while the player shows over the app, light over its dark, hidden while a phone on its side hides the controls; nothing while it does not show, so the pages' own settings hold. Read it in a binding.
func (l *Layer) StatusBar() core.StatusBarSetting {
	p := l.cur.Get()
	if p == nil || !l.Full() || p.status == nil {
		return core.StatusBarSetting{}
	}
	return p.status()
}

// Node is the player, to put over the app's tabs, last of the root page's children.
func (l *Layer) Node(cx *core.Scope) core.Node {
	keys := func() []string {
		if k := l.key.Get(); k != "" {
			return []string{k}
		}
		return nil
	}
	return core.Each(keys, func(k string) string { return k }, func(cx *core.Scope, k func() string, _ func() int) core.Node {
		return view(cx, l, k())
	})
}
