package main

import (
	"kando/apps/mobile/session"
	"kando/apps/mobile/widgets"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
)

// widgetLinks hears the links of the app's own scheme (widgets.Scheme) the system opens the app with, as a tap on a widget does, and opens the last that says what a widget opens with open, once the session has read the store and the user has been through the introduction: a link the app launched with comes before the store is read, and waits for it. Any other link is left alone.
func widgetLinks(cx *core.Scope, s *session.Session, open func(widgets.Link)) {
	take := func(l widgets.Link) {
		if s.Prefs().Onboarded {
			open(l)
		}
	}
	var waiting *widgets.Link
	mobile.OnOpenLinks(cx, func(links []string) {
		var last *widgets.Link
		for _, link := range links {
			if l, ok := widgets.ReadLink(link); ok {
				last = &l
			}
		}
		switch {
		case last == nil:
		case s.Loaded():
			take(*last)
		default:
			waiting = last
		}
	})
	cx.Effect(func() {
		if !s.Loaded() || waiting == nil {
			return
		}
		l := *waiting
		waiting = nil
		cx.Post(func() { take(l) })
	})
}
