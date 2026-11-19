package film

import (
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// SideTask is the side panel's FilmSide while it shows what a task pack's run gave (App.TaskResult) in place of its tabs' content, as Explain Line's explanation does.
const SideTask = "task"

// The ways a task's reply shows, as the task pack names them (docs/plugin-system.md E6): headed sections of one reply, a list of items, or a card an item.
const (
	TaskSections = "sections"
	TaskList     = "list"
	TaskCards    = "cards"
)

// Tasker is the run of a task pack the side panel shows: the user's AI service asked, by a pack the user installed and turned on, about the line playing, the word looked up or the film so far.
type Tasker interface {
	// TaskResult is the run shown, the zero TaskResult while none is.
	TaskResult() TaskResult
	// CloseTask goes back from the run to the subtitle list (‹ 字幕); TaskSettings opens the settings that let it run, for a run that could not: the AI service's, or the plugins' for a pack over its daily budget.
	CloseTask()
	TaskSettings()
	// TaskMenu is the task packs that can run now, an item each, as ⋯ lists them after its own items; none while no pack can run.
	TaskMenu(cx *ns.Scope) []nsdesktop.MenuItem
}

// TaskResult is a task's run as the panel shows it: Title the pack's name and Subject what it ran on (the line, the word, or how far the film was watched), in the language the interface shows under the scope they are given; Busy while the AI service is asked, Failed when no reply came, with Say saying why and Settings naming the button that fixes it, nil when none does; the reply's items, each a value a field in the order of Labels, the fields' headings, shown as Render says; and Note, the footnote that says which model wrote it and what is left of the pack's budget.
type TaskResult struct {
	Open         bool
	Title        func(*ns.Scope) string
	Subject      func(*ns.Scope) string
	Busy, Failed bool
	Say          func(*ns.Scope) string
	Settings     func(*ns.Scope) string
	Render       string
	Labels       func(*ns.Scope) []string
	Items        [][]string
	Note         func(*ns.Scope) string
}

// taskTab is the side panel showing a task's run in place of its tabs' content: ‹ 字幕 back to the list and the pack's name, what it ran on, then its reply, as sections under their headings, a list or cards, and the footnote, which scroll; or what it is doing or why it gave nothing, with the button of the settings that fix it.
func (s *screen) taskTab(cx *ns.Scope) ns.Node {
	a := s.a
	r := live.Func(a, a.TaskResult)
	muted := ns.RGBA(0xFFFFFF, 0.6)
	text := func(f func(*ns.Scope) string) func() string {
		return func() string {
			if f == nil {
				return ""
			}
			return f(cx)
		}
	}
	head := ns.View{Style: ns.Style{Padding: ns.Edges{Right: 14, Bottom: 10, Left: 14}, Gap: 6, Borders: ns.Edges{Bottom: 0.5}, BorderColor: panelLine}, Children: []ns.Node{
		ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 8}, Children: []ns.Node{
			base.Button{TestID: "film-task-back", LabelFunc: func() string { return locales.ExplainBack(cx) }, OnPress: live.On(a, func(ns.PressEvent) { a.CloseTask() }),
				Style: ns.Style{Direction: ns.Row, Gap: 4, Height: ns.Pt(24), Padding: ns.Symmetric(0, 8), AlignItems: ns.AlignCenter, CornerRadius: 8,
					Hover: &ns.Style{Background: panelLine}, FocusVisible: ui.FocusRing(cx, -2)},
				Children: []ns.Node{ns.Text{ContentFunc: func() string { return "‹ " + locales.FilmSubtitles(cx) }, PointerEvents: ns.PointerEventsNone, Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass}}}},
			ns.View{Style: ns.Style{FlexGrow: 1}},
			ns.Text{TestID: "film-task-title", ContentFunc: func() string { return text(r().Title)() }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
				Style: ns.Style{FontSize: 11, LineHeight: 15, Color: muted, FlexShrink: 1}},
		}},
		ns.Text{TestID: "film-task-subject", ContentFunc: func() string { return text(r().Subject)() }, Selectable: ns.SelectableOn, Role: ns.RoleHeader,
			Style: ns.Style{FontSize: 15, LineHeight: 21, FontWeight: ns.FontWeightBold, Color: onGlass}},
	}}
	// key tells one reply from another, so the reply's nodes are built again only when it changes, not on every change of the app's state.
	key := ns.NewMemoFunc(cx, live.Func(a, func() string {
		x := r()
		var b strings.Builder
		b.WriteString(x.Render)
		for _, item := range x.Items {
			b.WriteString("\x01" + strings.Join(item, "\x00"))
		}
		return b.String()
	}), nil).Get
	label := func(i int) func() string {
		return func() string {
			if x := r(); x.Labels != nil {
				if labels := x.Labels(cx); i < len(labels) {
					return labels[i]
				}
			}
			return ""
		}
	}
	body := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		key()
		x := ns.Untracked(cx, r)
		var nodes []ns.Node
		switch x.Render {
		case TaskSections:
			if len(x.Items) == 0 {
				break
			}
			for i, v := range x.Items[0] {
				if v == "" {
					continue
				}
				nodes = append(nodes, ns.View{TestID: "film-task-section", Style: ns.Style{Gap: 4}, Children: []ns.Node{
					ns.Text{ContentFunc: label(i), Role: ns.RoleHeader, Style: ns.Style{FontSize: 11, LineHeight: 14, FontWeight: ns.FontWeightBold, Color: muted}},
					ns.Text{Content: v, Selectable: ns.SelectableOn, Style: ns.Style{FontSize: 13, LineHeight: 20, Color: onGlass}},
				}})
			}
		case TaskList:
			for _, item := range x.Items {
				var parts []string
				for _, v := range item {
					if v != "" {
						parts = append(parts, v)
					}
				}
				nodes = append(nodes, ns.View{TestID: "film-task-item", Style: ns.Style{Direction: ns.Row, Gap: 8}, Children: []ns.Node{
					ns.Text{Content: "•", Style: ns.Style{FontSize: 13, LineHeight: 20, Color: muted}},
					ns.Text{Content: strings.Join(parts, " · "), Selectable: ns.SelectableOn, Style: ns.Style{FontSize: 13, LineHeight: 20, Color: onGlass, FlexShrink: 1}},
				}})
			}
		default:
			for _, item := range x.Items {
				children := []ns.Node{ns.Text{Content: item[0], Selectable: ns.SelectableOn, Style: ns.Style{FontSize: 13, LineHeight: 18, FontWeight: ns.FontWeightBold, Color: onGlass}}}
				for i, v := range item[1:] {
					if v == "" {
						continue
					}
					children = append(children, ns.View{Style: ns.Style{Gap: 2}, Children: []ns.Node{
						ns.Text{ContentFunc: label(i + 1), Style: ns.Style{FontSize: 11, LineHeight: 14, Color: muted}},
						ns.Text{Content: v, Selectable: ns.SelectableOn, Style: ns.Style{FontSize: 12, LineHeight: 18, Color: onGlass, Opacity: ns.Alpha(0.85)}},
					}})
				}
				nodes = append(nodes, ns.View{TestID: "film-task-card", Style: ns.Style{Gap: 6, Padding: ns.Symmetric(10, 12), CornerRadius: 12, Background: panelCard}, Children: children})
			}
		}
		return ns.View{Style: ns.Style{Gap: 12}, Children: nodes}
	})
	status := ns.Show{When: func() bool { x := r(); return x.Busy || x.Failed }, Then: func(cx *ns.Scope) ns.Node {
		return ns.View{Style: ns.Style{Gap: 8, AlignItems: ns.AlignStart}, Children: []ns.Node{
			ns.Text{TestID: "film-task-status", ContentFunc: func() string { return text(r().Say)() }, LiveRegion: ns.LiveRegionPolite, Style: ns.Style{FontSize: 13, LineHeight: 19, Color: muted}},
			ns.Show{When: func() bool { return r().Settings != nil }, Then: func(cx *ns.Scope) ns.Node {
				return explainPill(cx, "film-task-settings", func() string { return text(r().Settings)() }, live.Do(a, a.TaskSettings))
			}},
		}}
	}}
	note := ns.Show{When: func() bool { x := r(); return x.Note != nil && !x.Busy && !x.Failed }, Then: func(cx *ns.Scope) ns.Node {
		return ns.Text{TestID: "film-task-note", ContentFunc: func() string { return text(r().Note)() }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: ns.RGBA(0xFFFFFF, 0.5)}}
	}}
	scroll := ns.Scroll{TestID: "film-task-scroll", Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)},
		Children: []ns.Node{ns.View{Style: ns.Style{Padding: ns.Symmetric(12, 14), Gap: 12}, Children: []ns.Node{status, body, note}}}}
	return ns.View{TestID: "film-task", Role: ns.RoleGroup, LabelFunc: func() string { return text(r().Title)() },
		Style:    ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)},
		Children: []ns.Node{head, scroll}}
}
