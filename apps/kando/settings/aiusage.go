package settings

import (
	"strings"

	"kando/apps/kando/aiusage"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// aiUsage is the AI 服务 page's 本月用量, whichever service is chosen: the tokens each pass took this month, in the order of media.Passes, leaving out those that sent nothing, and the requests sent, all kept on this computer (App.AIUsage). A count with tokens Kando estimated, as the service said none, starts with ≈, which the note under the card explains. A month with nothing sent says so in one row.
func aiUsage(cx *core.Scope, a App) core.Node {
	// passes is the passes that sent something this month, joined, so the card is built again only when one starts to; the counts follow the month in bindings of their own.
	passes := core.NewMemo(cx, live.Func(a, func() string {
		month := a.AIUsage()
		var used []string
		for _, pass := range media.Passes {
			if c := month.ByPass[pass]; c.Requests > 0 || c.Tokens() > 0 {
				used = append(used, pass)
			}
		}
		return strings.Join(used, " ")
	}))
	card := core.Dynamic(func(cx *core.Scope) core.Node {
		used := strings.Fields(passes.Get())
		if len(used) == 0 {
			return form.Card(form.Row{Title: locales.AiusageNone(cx), TestID: "settings-aiusage-none"})
		}
		var rows []core.Node
		for _, pass := range used {
			tokens := live.Func(a, func() string { return aiUsageTokens(cx, a.AIUsage().ByPass[pass]) })
			rows = append(rows, form.Row{Title: aiUsagePass(cx, pass), TestID: "settings-aiusage-" + pass, Trailing: []core.Node{form.Value(tokens, 13)}}, form.Separator(true))
		}
		requests := live.Func(a, func() string { return locales.AiusageRequestsCount(cx, int(a.AIUsage().Total().Requests)) })
		rows = append(rows, form.Row{Title: locales.AiusageRequests(cx), TestID: "settings-aiusage-requests", Trailing: []core.Node{form.Value(requests, 13)}})
		return form.Card(rows...)
	})
	estimated := core.Show{When: live.Func(a, func() bool { return a.AIUsage().Total().Estimated > 0 }), Then: func(cx *core.Scope) core.Node {
		return form.Note(func() string { return locales.AiusageEstimatedNote(cx) }, 0)
	}}
	return core.Group([]core.Node{
		form.Label(locales.AiusageTitle(cx), 14),
		core.View{TestID: "settings-aiusage", Style: core.Style{FlexShrink: 0, AlignItems: core.AlignStretch}, Children: []core.Node{card}},
		estimated,
	})
}

// aiUsageTokens is the tokens of c as the page writes them, with ≈ before them when Kando estimated any.
func aiUsageTokens(cx *core.Scope, c aiusage.Count) string {
	if c.Estimated > 0 {
		return locales.AiusageTokensEstimated(cx, int(c.Tokens()))
	}
	return locales.AiusageTokens(cx, int(c.Tokens()))
}

// aiUsagePass is what the page calls pass, one of media.Passes.
func aiUsagePass(cx *core.Scope, pass string) string {
	switch pass {
	case media.PassTranslate:
		return locales.AiusagePassTranslate(cx)
	case media.PassProofread:
		return locales.AiusagePassProofread(cx)
	case media.PassReview:
		return locales.AiusagePassReview(cx)
	case media.PassSuggest:
		return locales.AiusagePassSuggest(cx)
	case media.PassPlugin:
		return locales.AiusagePassPlugin(cx)
	}
	return locales.AiusagePassChat(cx)
}
