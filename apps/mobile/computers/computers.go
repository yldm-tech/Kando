// Package computers is what the Library tab says of a paired computer whose films do not show: why, with the way to try again or pair again.
package computers

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

// Problem is computer d's card while its last read failed (session.Shelf): why, and a row that reads it again or, for a computer that no longer takes the phone, pairs again. It takes no room otherwise.
func Problem(cx *core.Scope, s *session.Session, d func() store.Desktop, pair func()) core.Node {
	shelf := func() session.Shelf { return s.Shelf(d().ID) }
	problem := func() session.Problem { return session.ProblemOf(shelf().Err) }
	unpairable := func() bool { p := problem(); return p == session.Unpaired || p == session.Certificate }
	return core.Show{When: func() bool { return shelf().Failed() }, Then: func(cx *core.Scope) core.Node {
		return look.Group{TestID: "computers-desktop-" + d().ID, Children: []core.Node{
			look.Row{TestID: "computers-problem-" + d().ID, TitleLines: 3,
				Leading: look.IconTile(cx, func() *core.SVG {
					if problem() == session.Unreachable {
						return lucide.WifiOff()
					}
					return lucide.CircleAlert()
				}, look.ToneDanger),
				Title:  func() string { return desktopProblem(cx, problem(), d().Name) },
				Detail: func() string { return d().Addr.String() }},
			core.Show{When: unpairable,
				Then: func(cx *core.Scope) core.Node {
					return look.Row{TestID: "computers-pair-again-" + d().ID, Title: func() string { return locales.ComputersPairAgain(cx) }, TitleTone: look.ToneAccent,
						Index: func() int { return 1 }, OnPress: pair}
				},
				Else: func(cx *core.Scope) core.Node {
					return look.Row{TestID: "computers-retry-" + d().ID, Title: func() string { return locales.CommonRetry(cx) }, TitleTone: look.ToneAccent,
						Index: func() int { return 1 }, OnPress: func() { s.RefreshDesktop(d().ID) }}
				}},
		}}
	}}
}

// desktopProblem is why a computer's films do not show.
func desktopProblem(cx *core.Scope, p session.Problem, name string) string {
	switch p {
	case session.Unpaired:
		return locales.ComputersUnpaired(cx, name)
	case session.Certificate:
		return locales.ComputersCertificate(cx, name)
	}
	return locales.ComputersUnreachable(cx, name)
}
