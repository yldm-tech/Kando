package player

import (
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// What plays after a film, as its computer goes on from it (session.Next): a course's next lesson, a disc's next track, a playlist's next video. Once a film ends, a card over the picture counts down to the next one the phone keeps with its video, and plays it in the same player, which keeps the sleep timer running; minimized, the next one plays at once. A film streamed from the storage goes on to the next one there. The sleep timer's End of Film holds the film at its end instead, and so does a looped line, which plays again.

// nextAfter is how long the card counts down before the next film plays.
const nextAfter = 8 * time.Second

// autoNextKey is the setting 自动播放下一部 is kept under in the app's storage.
const autoNextKey = "player.autoNext"

// upNext is the card while it shows: the next film's key, or for one to stream its film's ID (stream), and when it plays.
type upNext struct {
	key    string
	stream string
	due    time.Time
}

// carried is what the player gives the next film's player as it goes on to it: the sleep timer, still running.
type carried struct {
	key   string
	sleep sleepMode
	until time.Time
}

// carry gives p the sleep timer the player before it ran, when it went on to p's film.
func (l *Layer) carry(p *play) {
	if c := l.carried; c.key == p.key {
		p.sleep.Set(c.sleep)
		p.sleepUntil = c.until
	}
	l.carried = carried{}
}

// goOn plays the film kept under key in place of p's, the player shown or minimized as it is, with p's sleep timer.
func (l *Layer) goOn(p *play, key string) {
	l.carried = carried{key: key, sleep: p.sleep.GetUntracked(), until: p.sleepUntil}
	l.held = false
	l.key.Set(key)
}

// upNextAtEnd is the film ended: the card shows for the next one the phone keeps with its video or sound, or that it streams after a film it streams, unless 自动播放下一部 is off; minimized, the next one plays at once.
func (p *play) upNextAtEnd(l *Layer) {
	if !p.autoNext.Get() {
		return
	}
	next, kept := p.s.Next(p.key)
	up := upNext{key: next}
	if !kept {
		f, _ := p.s.Film(p.key)
		rf, ok := p.s.RelayFilm(f.Desktop, f.Film.Next)
		if next == "" || !p.streamed() || !ok || rf.Media() == "" {
			return
		}
		up = upNext{stream: f.Film.Next}
	}
	if !l.full.GetUntracked() {
		p.playNext(l, up)
		return
	}
	up.due = p.cx.Now().Add(nextAfter)
	p.card.Set(up)
	p.nextStop = p.cx.AfterFunc(nextAfter, func() {
		if cur := p.card.GetUntracked(); cur == up {
			p.playNext(l, up)
		}
	})
}

// playNext plays the film of up now, as the card's 立即播放 does: one kept at once, one to stream once its lines are read from the storage.
func (p *play) playNext(l *Layer, up upNext) {
	p.closeNext()
	if up.stream == "" {
		l.goOn(p, up.key)
		return
	}
	f, _ := p.s.Film(p.key)
	rf, ok := p.s.RelayFilm(f.Desktop, up.stream)
	if !ok {
		return
	}
	p.s.Stream(f.Desktop, f.Library, rf.Film, func(key string) { l.goOn(p, key) }, func(error) {})
}

// closeNext puts the card away, as its 取消 does, and once the film plays or seeks again.
func (p *play) closeNext() {
	if p.nextStop != nil {
		p.nextStop()
		p.nextStop = nil
	}
	p.card.Set(upNext{})
}

// nextCard is the card over the foot of the picture while it shows: 接下来, the next film's title, how many seconds until it plays, and 取消 and 立即播放.
func nextCard(cx *core.Scope, l *Layer, p *play) core.Node {
	open := func() bool { return p.card.Get() != (upNext{}) }
	return core.Show{When: open, Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		title := func() string {
			up := p.card.Get()
			if up.key != "" {
				// A film of a NAS matched to a computer's film is named as the computer names it.
				if f, ok := p.s.LinesFilm(up.key); ok {
					return f.Film.Title
				}
			}
			if f, ok := p.s.Film(p.key); ok {
				if rf, ok := p.s.RelayFilm(f.Desktop, up.stream); ok {
					return rf.Film.Title
				}
			}
			return ""
		}
		// left is the whole seconds until the next film plays, counted down as each passes.
		left := core.NewSignal(cx, int(nextAfter/time.Second))
		var stop func() bool
		var count func()
		count = func() {
			rest := p.card.GetUntracked().due.Sub(cx.Now())
			left.Set(max(int((rest+time.Second-1)/time.Second), 1))
			if rest > time.Second {
				step := rest % time.Second
				if step == 0 {
					step = time.Second
				}
				stop = cx.AfterFunc(step, count)
			}
		}
		count()
		cx.Cleanup(func() {
			if stop != nil {
				stop()
			}
		})
		return core.View{TestID: "player-up-next", Role: core.RoleGroup, LabelFunc: func() string { return locales.PlayerNextUp(cx) },
			Style: core.Style{Position: core.Absolute, Left: core.Pt(12), Right: core.Pt(12), Bottom: core.Pt(12), Padding: core.All(12), Gap: 8, CornerRadius: 14, Background: core.RGBA(0x1C1C20, 0.92)},
			Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.PlayerNextUp(cx) }, Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Muted)},
				core.Text{TestID: "player-up-next-title", ContentFunc: title, MaxLines: 2, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
					core.Text{TestID: "player-up-next-in", ContentFunc: func() string { return locales.PlayerNextIn(cx, left.Get()) }, LiveRegion: core.LiveRegionPolite,
						Style: look.Caption.Merge(core.Style{FlexGrow: 1, FontVariant: core.FontTabularNums}), StyleFunc: ui.Colored(colors.Muted)},
					look.Pill(cx, "player-up-next-cancel", func() string { return locales.PlayerNextCancel(cx) }, p.closeNext),
					look.Pill(cx, "player-up-next-play", func() string { return locales.PlayerNextPlay(cx) }, func() { p.playNext(l, p.card.GetUntracked()) }),
				}},
			}}
	}}
}
