package subscribe

import (
	"strings"

	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Ask is the dialog a link to subscribe to is pasted in, over the window while Open returns true: the field, then a spinner while the list is read or why it could not be. 订阅 and Return in the field run OnSubmit, which reads the list; the app closes the dialog once it subscribed. 取消, Escape and closing it run OnCancel, which also stops a reading.
type Ask struct {
	Open    func() bool
	Link    func() string
	OnLink  func(text string)
	Reading func() bool
	// Err says why the link could not be subscribed to, in the interface's language; "" while there is nothing to say.
	Err      func() string
	OnSubmit func()
	OnCancel func()
}

// Render implements ns.Node.
func (d Ask) Render(cx *ns.Scope) ns.Node {
	return ns.Show{When: d.Open, Then: func(cx *ns.Scope) ns.Node {
		colors := ui.ColorsOf(cx)
		field := ui.Input{TestID: "subscribe-link", Label: locales.SubscribeAddLink(cx), PlaceholderFunc: func() string { return locales.SubscribeAddPlaceholder(cx) },
			AutoFocus: true, NoAutoCorrect: true, AutoCapitalize: ns.AutoCapitalizeNone, KeyboardType: ns.KeyboardURL,
			Value: d.Link, OnChange: d.OnLink, OnSubmit: func(string) { d.OnSubmit() },
			Disabled: d.Reading, Style: ns.Style{Width: ns.Pct(100)}}
		status := ns.Dynamic(func(cx *ns.Scope) ns.Node {
			switch {
			case d.Reading():
				return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, AlignItems: ns.AlignCenter}, Children: []ns.Node{
					ui.Spinner{Size: 16, TestID: "subscribe-spinner"},
					ns.Text{Content: locales.SubscribeAddReading(cx), Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)},
				}}
			case d.Err() != "":
				return ns.Text{TestID: "subscribe-error", Role: ns.RoleAlert, ContentFunc: d.Err, Selectable: ns.SelectableOn, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Danger)}
			}
			return nil
		})
		return ui.Dialog{
			TestID:                 "subscribe-ask",
			NoCloseOnBackdropPress: true,
			Open:                   func() bool { return true },
			TitleFunc:              func() string { return locales.SubscribeAddButton(cx) },
			DescriptionFunc:        func() string { return locales.SubscribeAddDescription(cx) },
			Children:               []ns.Node{ns.View{Style: ns.Style{Gap: 10, AlignItems: ns.AlignStretch}, Children: []ns.Node{field, status}}},
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					d.OnCancel()
				}
			},
			Footer: []ns.Node{
				ui.Button{Text: locales.SubscribeAddCancel(cx), Variant: ui.ButtonSecondary, TestID: "subscribe-cancel", OnPress: d.OnCancel},
				ui.Button{Text: locales.SubscribeAddSubmit(cx), TestID: "subscribe-submit", OnPress: d.OnSubmit,
					Disabled: func() bool { return d.Reading() || strings.TrimSpace(d.Link()) == "" }},
			},
		}
	}}
}
