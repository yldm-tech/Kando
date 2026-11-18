package film

import (
	"slices"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// SideExplain is the side panel's FilmSide while it shows the explanation of a line (App.ExplainLine) in place of its tabs' content (design Main, 解释).
const SideExplain = "explain"

// Explainer is the explanation of a subtitle line the side panel shows (design Main 1062-1083): the line, the AI service's explanation in sections, the follow-up questions asked about it, and saving it to the notes.
type Explainer interface {
	// Explanation is the line explained, the zero Explanation while none is.
	Explanation() Explanation
	// CloseExplain goes back from the explanation to the subtitle list (‹ 字幕); ExpandExplain shows the sections the explanation keeps for 展开; ExplainSettings opens the AI service's settings, for an explanation no AI service could give.
	CloseExplain()
	ExpandExplain()
	ExplainSettings()
	// ExplainQuestion is the follow-up question field; AskExplain asks what it holds about the line and empties it.
	ExplainQuestion() *string
	AskExplain()
	// SaveExplained puts the line in the saved lines, which go into the daily review, with what the explanation says of it as its note.
	SaveExplained()
}

// Explanation is the line explained as the panel shows it: where it starts, its original and its translation; Busy while the AI service is asked, Failed when no explanation came, with Say saying why, NeedsAI when the AI service's settings are what helps; the sections, More when there are sections to expand; the questions asked; Note, the footnote that says which model wrote it and from what; and Saved once the line is in the saved lines.
type Explanation struct {
	Open                  bool
	At                    time.Duration
	Original, Translation string
	Busy, Failed, NeedsAI bool
	Say                   func(*ns.Scope) string
	Sections              []ExplainSection
	More                  bool
	Asked                 []ExplainQA
	Note                  func(*ns.Scope) string
	Saved                 bool
}

// ExplainSection is a section of an explanation: which it is, one of the Explain kinds, and what it says.
type ExplainSection struct {
	Kind, Body string
}

// The kinds of an explanation's sections, which the panel heads in the language of the interface.
const (
	ExplainMeaning     = "meaning"
	ExplainBreakdown   = "breakdown"
	ExplainTranslation = "translation"
	ExplainTerms       = "terms"
	ExplainTone        = "tone"
	ExplainRelated     = "related"
)

// ExplainQA is a follow-up question and its answer; Busy while it is asked, Failed when no answer came, and Answer writes the answer, or why there is none, in the language the interface shows under the scope it is given.
type ExplainQA struct {
	Question     string
	Busy, Failed bool
	Answer       func(*ns.Scope) string
}

// explainHeading is the heading of a section of kind, in the language the interface shows under cx.
func explainHeading(cx *ns.Scope, kind string) string {
	switch kind {
	case ExplainBreakdown:
		return locales.ExplainBreakdown(cx)
	case ExplainTranslation:
		return locales.ExplainTranslation(cx)
	case ExplainTerms:
		return locales.ExplainTerms(cx)
	case ExplainTone:
		return locales.ExplainTone(cx)
	case ExplainRelated:
		return locales.ExplainRelated(cx)
	}
	return locales.ExplainMeaning(cx)
}

// explainTab is the side panel showing the line explained in place of its tabs' content (design Main 1062-1083): ‹ 字幕 back to the list and the line's start, the line, then the sections of the explanation, 展开 for the rest, the questions asked and the footnote, which scroll; at the bottom the follow-up field, which asks at Return, and 存这句到笔记.
func (s *screen) explainTab(cx *ns.Scope) ns.Node {
	a := s.a
	e := live.Func(a, a.Explanation)
	muted := ns.RGBA(0xFFFFFF, 0.6)
	back := func() string { return locales.ExplainBack(cx) }
	head := ns.View{Style: ns.Style{Padding: ns.Edges{Right: 14, Bottom: 10, Left: 14}, Gap: 6, Borders: ns.Edges{Bottom: 0.5}, BorderColor: panelLine}, Children: []ns.Node{
		ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 8}, Children: []ns.Node{
			base.Button{TestID: "film-explain-back", LabelFunc: back, OnPress: live.On(a, func(ns.PressEvent) { a.CloseExplain() }),
				Style: ns.Style{Direction: ns.Row, Gap: 4, Height: ns.Pt(24), Padding: ns.Symmetric(0, 8), AlignItems: ns.AlignCenter, CornerRadius: 8,
					Hover: &ns.Style{Background: panelLine}, FocusVisible: ui.FocusRing(cx, -2)},
				Children: []ns.Node{ns.Text{ContentFunc: func() string { return "‹ " + locales.FilmSubtitles(cx) }, PointerEvents: ns.PointerEventsNone, Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass}}}},
			ns.View{Style: ns.Style{FlexGrow: 1}},
			ns.Text{TestID: "film-explain-at", ContentFunc: func() string { return cueClockTenths(e().At) }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: muted, FontVariant: ns.FontTabularNums}},
		}},
		ns.Text{TestID: "film-explain-original", ContentFunc: func() string { return e().Original }, Selectable: ns.SelectableOn, Role: ns.RoleHeader,
			Style: ns.Style{FontSize: 15, LineHeight: 21, FontWeight: ns.FontWeightBold, Color: onGlass}},
		ns.Show{When: func() bool { return e().Translation != "" }, Then: func(*ns.Scope) ns.Node {
			return ns.Text{TestID: "film-explain-translation", ContentFunc: func() string { return e().Translation }, Selectable: ns.SelectableOn,
				Style: ns.Style{FontSize: 13, LineHeight: 18, Color: onGlass, Opacity: ns.Alpha(0.8)}}
		}},
	}}
	sections := ns.NewMemoFunc(cx, live.Func(a, func() []ExplainSection { return e().Sections }), slices.Equal[[]ExplainSection]).Get
	body := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		list := sections()
		nodes := make([]ns.Node, 0, len(list))
		for _, sec := range list {
			nodes = append(nodes, ns.View{TestID: "film-explain-section", Style: ns.Style{Gap: 4}, Children: []ns.Node{
				ns.Text{ContentFunc: func() string { return explainHeading(cx, sec.Kind) }, Role: ns.RoleHeader, Style: ns.Style{FontSize: 11, LineHeight: 14, FontWeight: ns.FontWeightBold, Color: muted}},
				ns.Text{Content: sec.Body, Selectable: ns.SelectableOn, Style: ns.Style{FontSize: 13, LineHeight: 20, Color: onGlass}},
			}})
		}
		return ns.View{Style: ns.Style{Gap: 12}, Children: nodes}
	})
	status := ns.Show{When: func() bool { x := e(); return x.Busy || x.Failed }, Then: func(cx *ns.Scope) ns.Node {
		return ns.View{Style: ns.Style{Gap: 8, AlignItems: ns.AlignStart}, Children: []ns.Node{
			ns.Text{TestID: "film-explain-status", ContentFunc: func() string {
				if x := e(); x.Say != nil {
					return x.Say(cx)
				}
				return ""
			}, LiveRegion: ns.LiveRegionPolite, Style: ns.Style{FontSize: 13, LineHeight: 19, Color: muted}},
			ns.Show{When: func() bool { return e().NeedsAI }, Then: func(cx *ns.Scope) ns.Node {
				return explainPill(cx, "film-explain-settings", func() string { return locales.ExplainSettings(cx) }, live.Do(a, a.ExplainSettings))
			}},
		}}
	}}
	more := ns.Show{When: func() bool { return e().More }, Then: func(cx *ns.Scope) ns.Node {
		return explainPill(cx, "film-explain-more", func() string { return locales.ExplainMore(cx) }, live.Do(a, a.ExpandExplain))
	}}
	asked := ns.NewMemoFunc(cx, live.Func(a, func() int { return len(e().Asked) }), nil).Get
	qa := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		n := asked()
		nodes := make([]ns.Node, 0, n)
		for i := range n {
			item := func() ExplainQA {
				if list := e().Asked; i < len(list) {
					return list[i]
				}
				return ExplainQA{}
			}
			nodes = append(nodes, ns.View{TestID: "film-explain-qa", Style: ns.Style{Gap: 4, Padding: ns.Symmetric(10, 12), CornerRadius: 12, Background: panelCard}, Children: []ns.Node{
				ns.Text{ContentFunc: func() string { return item().Question }, Selectable: ns.SelectableOn, Style: ns.Style{FontSize: 12, LineHeight: 17, FontWeight: ns.FontWeightBold, Color: onGlass}},
				ns.Text{ContentFunc: func() string {
					if x := item(); x.Answer != nil {
						return x.Answer(cx)
					}
					return ""
				}, Selectable: ns.SelectableOn, LiveRegion: ns.LiveRegionPolite, Style: ns.Style{FontSize: 12, LineHeight: 18, Color: onGlass},
					StyleFunc: func() ns.Style {
						if x := item(); x.Busy || x.Failed {
							return ns.Style{Opacity: ns.Alpha(0.6)}
						}
						return ns.Style{Opacity: ns.Alpha(0.85)}
					}},
			}})
		}
		return ns.View{Style: ns.Style{Gap: 8}, Children: nodes}
	})
	note := ns.Show{When: func() bool { x := e(); return x.Note != nil && !x.Busy && !x.Failed }, Then: func(cx *ns.Scope) ns.Node {
		return ns.Text{TestID: "film-explain-note", ContentFunc: func() string { return e().Note(cx) }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: ns.RGBA(0xFFFFFF, 0.5)}}
	}}
	scroll := ns.Scroll{TestID: "film-explain-scroll", Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)},
		Children: []ns.Node{ns.View{Style: ns.Style{Padding: ns.Symmetric(12, 14), Gap: 12}, Children: []ns.Node{status, body, more, qa, note}}}}
	return ns.View{TestID: "film-explain", Role: ns.RoleGroup, LabelFunc: func() string { return locales.ExplainCard(cx) },
		Style:    ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)},
		Children: []ns.Node{head, scroll, s.explainFoot(cx, e)}}
}

// explainFoot is the bottom of the explanation: the follow-up field, which asks what it holds at Return, and 存这句到笔记, which says 已在笔记里 once the line is saved.
func (s *screen) explainFoot(cx *ns.Scope, e func() Explanation) ns.Node {
	a := s.a
	question := live.Func(a, func() string { return *a.ExplainQuestion() })
	// TODO(nsgo#145): give core.Input (base.Input, ui.Input) LabelFunc and AccessibilityDescriptionFunc twins, so the field's accessible name follows the interface language as it changes; it is read once here, and a Dynamic would take the focus and the typed text with it https://github.com/nsgo-dev/nsgo/issues/145
	input := base.Input{
		TestID:          "film-explain-ask",
		Label:           locales.ExplainAskLabel(cx),
		Value:           question,
		PlaceholderFunc: func() string { return locales.ExplainAskPlaceholder(cx, cx.KeyLabel("enter")) },
		OnChange:        live.On(a, func(text string) { *a.ExplainQuestion() = text }),
		SubmitBehavior:  ns.SubmitBehaviorSubmit,
		OnSubmit:        live.On(a, func(string) { a.AskExplain() }),
		Style: ns.Style{Height: ns.Pt(30), CornerRadius: 10, Background: ns.RGBA(0xFFFFFF, 0.1), FontSize: 12,
			Padding: ns.Symmetric(0, 10), Color: onGlass, PlaceholderColor: ns.RGBA(0xFFFFFF, 0.45), CaretColor: onGlass},
	}
	saved := live.Func(a, func() bool { return e().Saved })
	label := func() string {
		if saved() {
			return locales.ExplainSaved(cx)
		}
		return locales.ExplainSave(cx)
	}
	save := base.Button{TestID: "film-explain-save", LabelFunc: label, Selected: saved, OnPress: live.On(a, func(ns.PressEvent) { a.SaveExplained() }),
		Tooltip: tip(cx, label),
		Style:   ns.Style{Height: ns.Pt(30), CornerRadius: 10, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter, FocusVisible: ui.FocusRing(cx, 2)},
		StyleFunc: func() ns.Style {
			if saved() {
				return ns.Style{Background: ns.RGBA(0xFFFFFF, 0.14)}
			}
			fill := ui.ColorsOf(cx).AccentFill()
			return ns.Style{Background: fill, Hover: &ns.Style{Background: fill.Faded(0.85)}}
		},
		Children: []ns.Node{ns.Text{ContentFunc: label, MaxLines: 1, PointerEvents: ns.PointerEventsNone, Style: ns.Style{FontSize: 12, LineHeight: 16, FontWeight: ns.FontWeightSemibold, Color: onGlass}}}}
	return ns.View{Style: ns.Style{Padding: ns.Edges{Top: 10, Right: 14, Bottom: 12, Left: 14}, Gap: 8, Borders: ns.Edges{Top: 0.5}, BorderColor: panelLine},
		Children: []ns.Node{input, save}}
}

// explainPill is a small button of the explanation that says what it does, as 展开 and the AI service's settings.
func explainPill(cx *ns.Scope, id string, label func() string, press func()) ns.Node {
	return base.Button{TestID: id, LabelFunc: label, OnPress: func(ns.PressEvent) { press() },
		Style: ns.Style{AlignSelf: ns.AlignStart, Height: ns.Pt(26), Padding: ns.Symmetric(0, 12), AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter, CornerRadius: 13,
			Background: ns.RGBA(0xFFFFFF, 0.14), Hover: &ns.Style{Background: ns.RGBA(0xFFFFFF, 0.22)}, FocusVisible: ui.FocusRing(cx, -2)},
		Children: []ns.Node{ns.Text{ContentFunc: label, MaxLines: 1, PointerEvents: ns.PointerEventsNone, Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass}}}}
}
