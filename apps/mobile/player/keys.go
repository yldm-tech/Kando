package player

import (
	"slices"

	"kando/apps/mobile/shortcuts"

	"github.com/nsgo-dev/nsgo/core"
)

// keys binds the player's shortcuts on a hardware keyboard (shortcuts): play or hold, the line before and after, and the card's actions on the line it shows. They rest while a sheet covers the player, and the letters while a note is typed.
// TODO(nsgo#1077): iPadOS lists an app's shortcuts while ⌘ is held only from the UIKeyCommands of its responders, and NSGo's iOS keymap matches presses in pressesBegan:withEvent: (internal/platform/ios/ios.go:1695-1697) without giving UIKit any, so the overlay lists none of them; Settings › Keyboard Shortcuts lists them meanwhile, and its note leaves out the design's "hold ⌘ to see them on screen" until the keymap gives UIKit key commands with discoverability titles https://github.com/nsgo-dev/nsgo/issues/1077
func keys(cx *core.Scope, p *play, w *words, shown func() bool) {
	// Minimized, the player's view stays on screen in the mini player, so its bindings would fire; they act only while it shows over the app.
	on := func(fn func()) func() {
		return func() {
			if shown() {
				fn()
			}
		}
	}
	cx.OnKey(shortcuts.PlayPause, on(p.toggle))
	cx.OnKeyWith(shortcuts.PrevLine, core.KeyOptions{Repeat: true}, on(p.prev))
	cx.OnKeyWith(shortcuts.NextLine, core.KeyOptions{Repeat: true}, on(p.next))
	cx.OnKey(shortcuts.Replay, on(p.replay))
	cx.OnKey(shortcuts.Loop, on(p.toggleLoop))
	cx.OnKey(shortcuts.Save, on(p.toggleSave))
	cx.OnKey(shortcuts.Explain, on(func() { w.explainLine(p.shown.GetUntracked()) }))
	cx.OnKey(shortcuts.Subtitles, on(p.nextMode))
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
