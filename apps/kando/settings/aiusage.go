package settings

import (
	"strconv"
	"strings"

	"kando/apps/kando/aiusage"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
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
		aiUsageLimit(cx, a),
	})
}

// aiUsageLimit is the card of the monthly limit of tokens under 本月用量, for every service alike (App.AIUsageLimit): its switch, off by default, says while it is on how much of the limit the month took, or that it is reached, when no request goes until the month ends or it is raised; the number of tokens shows while it is on. It is a card of its own, not built again as the usage card is, so the field keeps the keyboard's focus.
func aiUsageLimit(cx *core.Scope, a App) core.Node {
	on := live.Func(a, func() bool { return a.AIUsageLimit() > 0 })
	status := live.Func(a, func() string {
		limit := a.AIUsageLimit()
		if limit <= 0 {
			return ""
		}
		used := a.AIUsage().Total().Tokens()
		if used >= limit {
			return locales.AiusageLimitReached(cx)
		}
		return locales.AiusageLimitUsed(cx, int(used*100/limit))
	})
	toggle := form.SwitchRow{Title: locales.AiusageLimit(cx), SubtitleFunc: status, On: on, TestID: "settings-aiusage-limit",
		OnChange: live.On(a, func(on bool) {
			if on {
				a.SetAIUsageLimit(aiusage.DefaultLimit)
			} else {
				a.SetAIUsageLimit(0)
			}
		})}
	tokens := ui.NumberInput{TestID: "settings-aiusage-limit-tokens", Label: locales.AiusageLimitTokens(cx), Min: aiusage.MinLimit, Max: aiusage.MaxLimit, Step: aiusage.LimitStep,
		Value:    live.Func(a, func() float64 { return float64(a.AIUsageLimit()) }),
		OnChange: live.On(a, func(v float64) { a.SetAIUsageLimit(int64(v)) }),
		Format:   aiUsageGrouped,
		Parse:    aiUsageUngrouped,
		Style:    core.Style{Width: core.Pt(130)}}
	field := core.Show{When: on, Then: func(cx *core.Scope) core.Node {
		return core.Group([]core.Node{
			form.Separator(true),
			form.Row{Title: locales.AiusageLimitTokens(cx), TestID: "settings-aiusage-limit-row", Trailing: []core.Node{tokens}},
		})
	}}
	card := form.Card(toggle, field)
	card.Style.Margin = core.Edges{Top: 8}
	return card
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

// aiUsageGrouped writes a number of tokens with a comma between each three digits, as the usage card writes them: 1,000,000.
func aiUsageGrouped(v float64) string {
	digits := strconv.FormatInt(int64(v), 10)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}

// aiUsageUngrouped reads a number of tokens typed with or without the commas, spaces or underscores between its digits.
func aiUsageUngrouped(text string) (float64, bool) {
	text = strings.NewReplacer(",", "", " ", "", "_", "", "，", "").Replace(strings.TrimSpace(text))
	v, err := strconv.ParseFloat(text, 64)
	return v, err == nil
}
