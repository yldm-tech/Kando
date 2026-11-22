package player

import (
	"slices"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/shortcuts"

	"github.com/nsgo-dev/nsgo/core"
)

// keys binds the player's shortcuts on a hardware keyboard (shortcuts): play or hold, the line before and after, and the card's actions on the line it shows. They rest while a sheet covers the player, and the letters while a note is typed. Each has its name in the app's language under Playback, which iPadOS lists while ⌘ is held and in its menu bar, while the player shows over the app.
func keys(cx *core.Scope, p *play, w *words, shown func() bool) {
	group := func() string { return locales.ShortcutsGroupPlayback(cx) }
	bind := func(key string, repeat bool, title func(*core.Scope) string, fn func()) {
		cx.OnKeyWith(key, core.KeyOptions{Repeat: repeat, Group: group,
			// Minimized, the player's view stays on screen in the mini player, so its bindings would fire; they act, and iPadOS lists them, only while it shows over the app.
			Title: func() string {
				if !shown() {
					return ""
				}
				return title(cx)
			}}, func() {
			if shown() {
				fn()
			}
		})
	}
	bind(shortcuts.PlayPause, false, locales.ShortcutsPlayPause, p.toggle)
	bind(shortcuts.PrevLine, true, locales.PlayerPrevLine, p.prev)
	bind(shortcuts.NextLine, true, locales.PlayerNextLine, p.next)
	bind(shortcuts.Replay, false, locales.ShortcutsReplay, p.replay)
	bind(shortcuts.Loop, false, locales.ShortcutsLoop, p.toggleLoop)
	bind(shortcuts.Save, false, locales.ShortcutsSave, p.toggleSave)
	bind(shortcuts.Explain, false, locales.ShortcutsExplain, func() { w.explainLine(p.shown.GetUntracked()) })
	bind(shortcuts.Subtitles, false, locales.ShortcutsNextMode, p.nextMode)
}

// nextMode shows the lines in the subtitle mode after the one chosen, in the order of the player's menu (Modes), and says which.
func (p *play) nextMode() {
	var m string
	_ = p.mode.Update(func(cur string) string {
		m = Modes[(slices.Index(Modes, cur)+1)%len(Modes)]
		return m
	})
	p.say("mode", modeName(p.cx, m))
}
