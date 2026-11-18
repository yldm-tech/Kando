package settings

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// ComputerPage is a paired computer's page: the AI service it translates with, which only the computer changes, as it keeps the service's key; whether its translation sends the lines before a batch with it, which the phone turns on and off there; and the way to unpair it. What the computer answered at the last sync this run is what it shows; before one reached it, it says so. unpaired runs once the user unpaired it, which takes the page away.
func ComputerPage(cx *core.Scope, s *session.Session, id string, unpaired func(id string), back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	d := func() store.Desktop { d, _ := s.Desktop(id); return d }
	got := func() (session.DesktopSettings, bool) { return s.DesktopSettings(id) }
	// pending is the context switch's position while the computer takes it.
	pending := core.NewSignalFunc[*bool](cx, nil, nil)
	contextOn := func() bool {
		if p := pending.Get(); p != nil {
			return *p
		}
		st, _ := got()
		return st.Context != nil && *st.Context
	}
	service := func() string {
		st, ok := got()
		switch {
		case !ok:
			return ""
		case st.Service == "":
			return locales.ComputerServiceNone(cx)
		case st.Model == "":
			return st.Service
		}
		return st.Service + " · " + st.Model
	}
	setContext := func(on bool) {
		if pending.GetUntracked() != nil {
			return
		}
		pending.Set(&on)
		s.SetDesktopContext(id, on, func(err error) {
			pending.Set(nil)
			if err != nil {
				cx.ShowAlert(core.AlertConfig{Title: locales.ComputerContextFailed(cx, d().Name), Message: locales.ComputerContextFailedMessage(cx),
					Actions: []core.AlertAction{{Label: locales.CommonOK(cx), Role: core.AlertCancel}}}, nil)
			}
		})
	}
	note := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Muted)}
	}
	ref := &core.Ref{}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return d().Name }, TestID: "computer-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					look.SectionTitle(cx, func() string { return locales.ComputerTranslation(cx) }),
					look.Group{Children: []core.Node{
						look.Row{TestID: "computer-service", Inset: 56,
							Leading: look.IconTile(cx, lucide.Sparkles, look.ToneAccent),
							Title:   func() string { return locales.ComputerService(cx) },
							Value:   service},
						look.Row{TestID: "computer-context", Index: func() int { return 1 }, Inset: 56, TitleLines: 2,
							Leading: look.IconTile(cx, lucide.TextQuote, look.ToneAccent),
							Title:   func() string { return locales.ComputerContext(cx) },
							Detail:  func() string { return locales.ComputerContextDetail(cx) },
							Trailing: ui.Switch{TestID: "computer-context-switch", Label: locales.ComputerContext(cx), Variant: ui.SwitchSystem,
								Checked: contextOn, OnChange: setContext,
								Disabled: func() bool {
									st, ok := got()
									return !ok || st.Context == nil || st.ReadOnly || pending.Get() != nil
								}}},
					}},
					note(func() string {
						st, ok := got()
						switch {
						case !ok:
							return locales.ComputerUnknown(cx)
						case st.ReadOnly:
							return locales.ComputerReadOnly(cx)
						}
						return locales.ComputerServiceNote(cx)
					}),
				}},
				look.Group{Children: []core.Node{
					look.Row{TestID: "computer-unpair", TitleTone: look.ToneDanger,
						Title:   func() string { return locales.ComputersUnpair(cx) },
						OnPress: func() { confirmUnpair(cx, s, id, d().Name, unpaired) }},
				}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}
