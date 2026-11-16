package film

import (
	"cmp"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// panelTabs are the side panel's tabs, by the FilmSide each one is; tabLabel names them.
var panelTabs = []string{"", "clips", "info", "style"}

// tabLabel is what a tab of the side panel is called in the language the interface shows under cx.
func tabLabel(cx *ns.Scope, id string) string {
	switch id {
	case "clips":
		return locales.FilmTabClips(cx)
	case "info":
		return locales.FilmTabInfo(cx)
	case "style":
		return locales.FilmTabStyle(cx)
	}
	return locales.FilmSubtitles(cx)
}

// The side panel's surfaces (prototype template 642-876).
var (
	panelCard    = ns.RGBA(0xFFFFFF, 0.07)
	panelCurrent = ns.RGBA(0xFFFFFF, 0.14)
	panelLine    = ns.RGBA(0xFFFFFF, 0.1)
	panelGlass   = glassSpec{fill: ns.RGBA(0x1C1C20, 0.6), blur: 34, ring: 0.2, radius: 22}
)

// panelMotionStyle moves the panel in from the right and fades it as it comes and goes.
var panelMotionStyle = ns.Transition{Duration: panelMotion, Easing: ns.EaseInOut, Properties: ns.TransitionOpacity | ns.TransitionTransform}

// panel is the glass side panel floating over the right of the picture: the subtitle list, the 片段 and the film's files under three tabs, or the subtitle style in their place. It slides in from the right as it comes and out as it goes, and is gone once it has.
func (s *screen) panel() ns.Node {
	a := s.a
	return ns.Presence{When: s.room, Then: func(cx *ns.Scope) ns.Node {
		// It is built off to the right and moves in on the next frame; it moves out again before it goes.
		in := ns.NewSignal(cx, false)
		cx.RequestAnimationFrame(func(time.Time) { in.Set(true) })
		cx.OnExit(func(done func()) {
			in.Set(false)
			cx.AfterFunc(panelMotion, done)
		})
		side := live.Memo(cx, a, a.FilmSide)
		body := ns.Show{
			When: live.Func(a, a.StyleView),
			Then: s.styleView,
			Else: func(cx *ns.Scope) ns.Node {
				return ns.Dynamic(func(cx *ns.Scope) ns.Node {
					switch side() {
					case "clips":
						return s.clipsTab(cx)
					case "info":
						return s.infoTab(cx)
					}
					return s.subsTab(cx)
				})
			},
		}
		return ns.View{
			TestID: "film-panel", Role: ns.RoleGroup, LabelFunc: func() string { return locales.FilmSidebar(cx) },
			Style: panelGlass.style().Merge(ns.Style{Position: ns.Absolute, Right: ns.Pt(16), Top: ns.Pt(62), Bottom: ns.Pt(18), Width: ns.Pt(panelWidth), Overflow: ns.Hidden,
				Shadows: append(panelGlass.style().Shadows, ns.BoxShadow{Offset: ns.Point{Y: 20}, Blur: 50, Color: ns.RGBA(0x000000, 0.35)})}),
			StyleFunc: func() ns.Style {
				if in.Get() {
					return ns.Style{Opacity: ns.Alpha(1), Transition: panelMotionStyle}
				}
				return ns.Style{Opacity: ns.Alpha(0), Transform: ns.Transform{TranslateX: panelRoom}, Transition: panelMotionStyle, PointerEvents: ns.PointerEventsNone}
			},
			Children: []ns.Node{
				ns.View{Style: ns.Style{Padding: ns.Edges{Top: 14, Right: 14, Bottom: 10, Left: 14}}, Children: []ns.Node{s.panelTabBar(cx)}},
				body,
			},
		}
	}}
}

// panelTabBar is the prototype's tab bar across the panel (template 718-724): a track of white at 10%, its tabs sharing its width, the chosen one lit with white at 22% and its label semibold.
func (s *screen) panelTabBar(cx *ns.Scope) ns.Node {
	a := s.a
	chosen := live.Func(a, func() string {
		if a.StyleView() {
			return "style"
		}
		return a.FilmSide()
	})
	tabs := make([]ns.Node, 0, len(panelTabs))
	for _, tab := range panelTabs {
		on := func() bool { return chosen() == tab }
		tabs = append(tabs, base.Tab{
			TestID: "film-tab-" + cmp.Or(tab, "subs"), Selected: on,
			OnPress: live.On(a, func(ns.PressEvent) {
				if tab == "style" {
					a.SetStyleView(true)
					return
				}
				a.SetStyleView(false)
				a.SetFilmSide(tab)
			}),
			Style: ns.Style{FlexGrow: 1, FlexBasis: ns.Pt(0), Height: ns.Pt(26), CornerRadius: 9, JustifyContent: ns.JustifyCenter, AlignItems: ns.AlignCenter,
				Hover: &ns.Style{Background: panelLine}, FocusVisible: ui.FocusRing(cx, -2)},
			StyleFunc: func() ns.Style {
				if on() {
					return ns.Style{Background: ns.RGBA(0xFFFFFF, 0.22)}
				}
				return ns.Style{}
			},
			Children: []ns.Node{ns.Text{ContentFunc: func() string { return tabLabel(cx, tab) }, MaxLines: 1,
				Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass},
				StyleFunc: func() ns.Style {
					if on() {
						return ns.Style{FontWeight: ns.FontWeightSemibold}
					}
					return ns.Style{FontWeight: ns.FontWeightNormal}
				}}},
		})
	}
	// TODO(nsgo#145): give base.Tabs' Label a LabelFunc twin, so the tab list's accessible name follows the interface language as it changes; it is read once here https://github.com/nsgo-dev/nsgo/issues/145
	return base.Tabs{TestID: "film-tabs", Label: locales.FilmSidebar(cx),
		Style:    ns.Style{Direction: ns.Row, Height: ns.Pt(32), Padding: ns.All(3), CornerRadius: 12, Background: ns.RGBA(0xFFFFFF, 0.1)},
		Children: tabs}
}

// subsTab lists the cues with the time each starts, the playing one lit and kept in view as it changes. A click plays from a cue, and a right click explains or copies it (cueMenu). The field above it searches both lines, and while the film is held the line it is held on shows under it, to select and copy from (heldLine).
func (s *screen) subsTab(cx *ns.Scope) ns.Node {
	a := s.a
	state := live.Memo(cx, a, func() subsState {
		subs, item := a.Subs(), s.item()
		switch {
		case subs.Key != item.Key || !subs.Ready:
			return subsLoading
		case subs.Err != "":
			return subsFailed
		case item.SubtitlePath == "":
			return subsNone
		case len(subs.Cues) == 0:
			return subsEmpty
		}
		return subsReady
	})
	cues := filmCues(cx, a)
	query := live.Memo(cx, a, func() string { return strings.ToLower(strings.TrimSpace(*a.Search())) })
	shown := ns.NewMemoFunc(cx, func() []int { return cueMatches(cues(), query()) }, slices.Equal[[]int]).Get
	// The chip counts the lines, or while a search is typed the lines that hold it of them all.
	chip := ns.Show{When: func() bool { return state() == subsReady }, Then: func(cx *ns.Scope) ns.Node {
		chipText := ns.Style{FontSize: 11, LineHeight: 14, FontWeight: ns.FontWeightSemibold, Color: onGlass}
		return ns.View{TestID: "film-subs-count", Style: ns.Style{Direction: ns.Row, Height: ns.Pt(24), Padding: ns.Symmetric(0, 10), CornerRadius: 12, Gap: 3, AlignItems: ns.AlignCenter, AlignSelf: ns.AlignStart, Background: ns.RGBA(0xFFFFFF, 0.25)},
			Children: []ns.Node{
				ns.Show{When: func() bool { return query() == "" },
					Then: func(cx *ns.Scope) ns.Node {
						return ns.Group([]ns.Node{
							ns.Text{ContentFunc: func() string { return locales.FilmAll(cx) }, Style: chipText},
							ns.Text{ContentFunc: func() string { return fmt.Sprint(len(cues())) }, Style: chipText.Merge(ns.Style{Opacity: ns.Alpha(0.6)})},
						})
					},
					Else: func(cx *ns.Scope) ns.Node {
						return ns.Text{ContentFunc: func() string { return fmt.Sprintf("%d / %d", len(shown()), len(cues())) }, Style: chipText}
					}},
			}}
	}}
	body := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		switch state() {
		case subsLoading:
			return panelNote(func() string { return locales.FilmLoadingSubtitles(cx) }, nil)
		case subsFailed:
			return s.subsFailed(cx)
		case subsNone:
			return panelNote(func() string { return locales.FilmNoSubtitlesYet(cx) }, nil)
		case subsEmpty:
			return panelNote(func() string { return locales.FilmSubtitlesEmpty(cx) }, nil)
		}
		return s.cueList(cx, cues, shown)
	})
	// Return in the search plays from the next line that holds it, Shift+Return from the one before.
	step := func(back bool) {
		if query() == "" {
			return
		}
		list, rows := cues(), shown()
		k := searchStep(list, rows, a.PlayHead()-a.Delay(), back)
		if k < 0 {
			return
		}
		a.Seek(list[rows[k]].Start + a.Delay())
		a.Flash(locales.FilmMatchOf(cx, k+1, len(rows)))
	}
	return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{
		ns.View{Style: ns.Style{Padding: ns.Edges{Right: 14, Bottom: 8, Left: 14}, Gap: 8}, Children: []ns.Node{s.searchField(cx, step), chip, s.heldLine(cx, cues, func() bool { return state() == subsReady })}},
		ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0), Padding: ns.Edges{Right: 8, Bottom: 12, Left: 8}}, Children: []ns.Node{body}},
	}}
}

// heldLine is the line the picture is held on, while the film is held on one, in a card whose text the user can select, a word of it or the whole line, and copy, as the list's rows, which play their line at a click, cannot be. Its lines are the row's: those the mode shows, and the one it hides blurred until the pointer rests on the card. Explain Line under them has the AI service explain the line. It goes as the film plays on, so it never moves under the pointer. ready reports whether the film's lines are read.
func (s *screen) heldLine(cx *ns.Scope, cues func() []core.Segment, ready func() bool) ns.Node {
	a := s.a
	index := ns.NewMemo(cx, func() int {
		a.Track()
		if p := a.Play(); p == nil || p.Playing() || !ready() {
			return -1
		}
		return cueIndex(cues(), s.playhead()-a.Delay())
	}).Get
	return ns.Show{When: func() bool { return index() >= 0 }, Then: func(cx *ns.Scope) ns.Node {
		lines := live.Func(a, func() cueLines {
			list, i := cues(), index()
			if i < 0 || i >= len(list) {
				return cueLines{}
			}
			shown, hidden := listLines(a, list[i].Text)
			return cueLines{shown: shown, hidden: hidden}
		})
		nth := func(i int) func() string {
			return func() string {
				if l := lines().shown; i < len(l) {
					return l[i]
				}
				return ""
			}
		}
		hovered := ns.NewSignal(cx, false)
		explain := func() string { return locales.FilmExplainLine(cx) }
		return ns.View{TestID: "film-held-line", Style: ns.Style{Padding: ns.Symmetric(10, 12), Gap: 8, CornerRadius: 12, Background: panelCard},
			OnMouseEnter: func(ns.MouseEvent) { hovered.Set(true) }, OnMouseLeave: func(ns.MouseEvent) { hovered.Set(false) },
			Children: []ns.Node{
				ns.View{TestID: "film-held-text", Selectable: true, Style: ns.Style{Gap: 2}, Children: []ns.Node{
					ns.Text{ContentFunc: nth(0), Style: ns.Style{FontSize: 14, LineHeight: 19, Color: onGlass}},
					ns.Show{When: func() bool { return nth(1)() != "" }, Then: func(*ns.Scope) ns.Node {
						return ns.Text{ContentFunc: nth(1), Style: cueSecondStyle}
					}},
					ns.Show{When: func() bool { return lines().hidden != "" }, Then: func(*ns.Scope) ns.Node {
						return ns.Text{ContentFunc: func() string { return lines().hidden }, Style: cueSecondStyle,
							StyleFunc: func() ns.Style {
								if hovered.Get() {
									return ns.Style{Transition: cuePeekTransition}
								}
								return ns.Style{Opacity: ns.Alpha(0.45), Filters: []ns.Filter{ns.Blur(5)}, Transition: cuePeekTransition}
							}}
					}},
				}},
				base.Button{TestID: "film-held-explain", LabelFunc: explain, OnPress: live.On(a, func(ns.PressEvent) { a.ExplainLine(index()) }),
					Style: ns.Style{AlignSelf: ns.AlignStart, Height: ns.Pt(24), Padding: ns.Symmetric(0, 10), CornerRadius: 12, JustifyContent: ns.JustifyCenter, Background: ns.RGBA(0xFFFFFF, 0.14),
						Hover: &ns.Style{Background: ns.RGBA(0xFFFFFF, 0.22)}, FocusVisible: ui.FocusRing(cx, -2)},
					Children: []ns.Node{ns.Text{ContentFunc: explain, MaxLines: 1, Style: ns.Style{FontSize: 12, LineHeight: 16, FontWeight: ns.FontWeightSemibold, Color: onGlass}}}},
			}}
	}}
}

// cueMatches is the indexes of the cues that hold query, which is lower case, or all of them for none.
func cueMatches(cues []core.Segment, query string) []int {
	out := make([]int, 0, len(cues))
	for i, cue := range cues {
		if query == "" || strings.Contains(strings.ToLower(cue.Text), query) {
			out = append(out, i)
		}
	}
	return out
}

// searchRewind is how far into a match Shift+Return goes back past it to the match before, rather than to its start again, as Previous Line does with a line.
const searchRewind = time.Second

// searchStep is which of matches, indexes of cues in the order they start, Return plays from with the picture at at in the subtitles' time: the first starting after at, or the first of all after the last; with back the last starting more than searchRewind before at, or the last of all before the first. It is -1 without matches.
func searchStep(cues []core.Segment, matches []int, at time.Duration, back bool) int {
	if len(matches) == 0 {
		return -1
	}
	if back {
		for k := len(matches) - 1; k >= 0; k-- {
			if cues[matches[k]].Start < at-searchRewind {
				return k
			}
		}
		return len(matches) - 1
	}
	for k, i := range matches {
		if cues[i].Start > at {
			return k
		}
	}
	return 0
}

// subsState is what the subtitle tab shows: the list once the film's subtitles are read and hold lines, else why there is none.
type subsState int

const (
	subsLoading subsState = iota
	subsFailed
	subsNone
	subsEmpty
	subsReady
)

// searchField is the subtitle list's search field: the prototype's is 30 high, white at 10%, its text 12, a magnifying glass before it. ⌘F puts the keyboard in it (SearchFocus), and Escape empties it while it holds something. Return runs step, and Shift+Return step back; the field keeps the focus, so they can be pressed again.
func (s *screen) searchField(cx *ns.Scope, step func(back bool)) ns.Node {
	a := s.a
	field := &ns.Ref{}
	cx.Effect(func() {
		a.Track()
		if focus := a.SearchFocus(); *focus {
			*focus = false
			field.Focus()
		}
	})
	query := live.Func(a, func() string { return *a.Search() })
	clear := live.Do(a, func() { *a.Search() = "" })
	focused := ns.NewSignal(cx, false)
	cx.Cleanup(func() { *a.SearchFocused() = false })
	escape := ns.Show{When: func() bool { return focused.Get() && query() != "" }, Then: func(cx *ns.Scope) ns.Node {
		cx.OnKeyWith("escape", ns.KeyOptions{When: ns.KeyAlways}, clear)
		cx.OnKeyWith("shift+enter", ns.KeyOptions{When: ns.KeyAlways}, live.Do(a, func() { step(true) }))
		return nil
	}}
	// TODO(nsgo#145): give core.Input (base.Input, ui.Input) LabelFunc and AccessibilityDescriptionFunc twins, so the field's accessible name follows the interface language as it changes; it is read once here, and a Dynamic would take the focus and the typed text with it https://github.com/nsgo-dev/nsgo/issues/145
	// TODO(nsgo#144): give core.Input a search role, so assistive technology reads the field as a search field as MyGo's SearchField was https://github.com/nsgo-dev/nsgo/issues/144
	input := base.Input{
		Ref: field, TestID: "film-search",
		Label:           locales.FilmSearchLines(cx),
		Value:           query,
		PlaceholderFunc: func() string { return locales.FilmSearchLines(cx) + "  " + cx.KeyLabel("mod+f") },
		OnChange:        live.On(a, func(text string) { *a.Search() = text }),
		SubmitBehavior:  ns.SubmitBehaviorSubmit,
		OnSubmit:        live.On(a, func(string) { step(false) }),
		OnFocus:         func() { focused.Set(true); *a.SearchFocused() = true },
		OnBlur:          func() { focused.Set(false); *a.SearchFocused() = false },
		Style: ns.Style{FlexGrow: 1, MinWidth: ns.Pt(0), Height: ns.Pt(30), CornerRadius: 8, Background: ns.RGBA(0xFFFFFF, 0.1), FontSize: 12,
			Padding: ns.Edges{Left: 28, Right: 8}, Color: onGlass, PlaceholderColor: ns.RGBA(0xFFFFFF, 0.45), CaretColor: onGlass},
	}
	glass := ns.View{PointerEvents: ns.PointerEventsNone, Style: ns.Style{Position: ns.Absolute, Top: ns.Pt(8), Left: ns.Pt(9)},
		Children: []ns.Node{ns.Icon{SVG: iconSearch, Style: ns.Style{Width: ns.Pt(14), Height: ns.Pt(14), Color: ns.RGBA(0xFFFFFF, 0.6)}}}}
	return ns.View{Style: ns.Style{Direction: ns.Row}, Children: []ns.Node{
		ns.View{Style: ns.Style{FlexGrow: 1, MinWidth: ns.Pt(0)}, Children: []ns.Node{input, glass, escape}},
	}}
}

// cueList is the subtitle list, narrowed to shown, the indexes of the cues that hold the search, the playing one lit and scrolled into view as it changes.
func (s *screen) cueList(cx *ns.Scope, cues func() []core.Segment, shown func() []int) ns.Node {
	a := s.a
	current := ns.NewMemo(cx, func() int {
		a.Track()
		if a.Play() == nil {
			return -1
		}
		return cueIndex(cues(), s.playhead()-a.Delay())
	}).Get
	clockWidth := cueClockWidth(cx, a, cues)
	list := &ns.Ref{}
	// seen is the first and the last cue in view, by index, as the list reports its viewable rows, -1 before it has.
	seen := ns.NewSignal(cx, [2]int{-1, -1})
	inView := func(i int) bool { r := ns.Untracked(cx, seen.Get); return r[0] >= 0 && i >= r[0] && i <= r[1] }
	// anchor is the cue the list last scrolled to; away is set while the list was left scrolled off it, the user reading elsewhere in it, so it does not follow the film until the pill brings it back, the user scrolls the playing cue into view, or moves the picture.
	anchor := -1
	away := ns.NewSignal(cx, false)
	follow := func(cur int) {
		for row, i := range ns.Untracked(cx, shown) {
			if i == cur {
				list.ScrollToIndex(row, true)
				anchor = cur
				away.Set(false)
				return
			}
		}
	}
	jumps := live.Memo(cx, a, a.Jumps)
	// The list follows the playing cue as it changes, or as the user moves the picture (Jumps), not as it is scrolled by hand or narrowed by the search.
	followed, jumped := -2, ns.Untracked(cx, jumps)
	cx.Effect(func() {
		cur, j := current(), jumps()
		shown()
		moved := j != jumped
		jumped = j
		if cur == followed && !moved {
			return
		}
		followed = cur
		switch {
		case cur < 0:
		case moved || anchor < 0 || inView(anchor):
			follow(cur)
		default:
			away.Set(true)
		}
	})
	viewable := func(v ns.ViewableItems) {
		r := [2]int{-1, -1}
		if n := len(v.Viewable); n > 0 {
			first, ok1 := v.Viewable[0].Key.(int)
			last, ok2 := v.Viewable[n-1].Key.(int)
			if ok1 && ok2 {
				r = [2]int{first, last}
			}
		}
		seen.Set(r)
		// The user scrolled the playing cue back into view: the list follows it again from there.
		if cur := current(); away.Get() && cur >= 0 && inView(cur) {
			anchor = cur
			away.Set(false)
		}
	}
	pill := ns.Show{When: func() bool { return away.Get() && current() >= 0 }, Then: func(cx *ns.Scope) ns.Node {
		label := func() string { return locales.FilmBackToCurrentLine(cx) }
		return ns.View{PointerEvents: ns.PointerEventsBoxNone, Style: ns.Style{Position: ns.Absolute, Left: ns.Pt(0), Right: ns.Pt(0), Bottom: ns.Pt(10), AlignItems: ns.AlignCenter},
			Children: []ns.Node{base.Button{TestID: "film-cues-current", LabelFunc: label,
				OnPress: func(ns.PressEvent) { follow(current()) },
				Style: ns.Style{Height: ns.Pt(28), Padding: ns.Symmetric(0, 12), CornerRadius: 14, JustifyContent: ns.JustifyCenter, Background: ns.RGBA(0x3A3A40, 0.95),
					Shadows: []ns.BoxShadow{{Spread: 0.5, Color: ns.RGBA(0xFFFFFF, 0.2), Inset: true}, {Offset: ns.Point{Y: 6}, Blur: 18, Color: ns.RGBA(0x000000, 0.35)}},
					Hover:   &ns.Style{Background: ns.RGBA(0x4A4A52, 0.95)}, FocusVisible: ui.FocusRing(cx, -2)},
				Children: []ns.Node{ns.Text{ContentFunc: label, MaxLines: 1, Style: ns.Style{FontSize: 12, LineHeight: 16, FontWeight: ns.FontWeightSemibold, Color: onGlass}}},
			}}}
	}}
	return ns.Show{
		When: func() bool { return len(shown()) > 0 },
		Then: func(cx *ns.Scope) ns.Node {
			return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{s.cueRows(cx, list, cues, shown, current, clockWidth, viewable), pill}}
		},
		Else: func(cx *ns.Scope) ns.Node {
			return panelNote(func() string { return locales.FilmNoMatch(cx) }, nil)
		},
	}
}

// cueRows is the list of the cues of shown, the current one lit, which reports the cues in view to viewable.
func (s *screen) cueRows(cx *ns.Scope, list *ns.Ref, cues func() []core.Segment, shown func() []int, current func() int, clockWidth func() ns.Points, viewable func(ns.ViewableItems)) ns.Node {
	return ns.List{TestID: "film-cues", Ref: list, Style: ns.Style{FlexGrow: 1, MinHeight: ns.Pt(0)}, EstimatedSize: 56,
		OnViewableItemsChange: viewable,
		Count:                 func() int { return len(shown()) },
		Key: func(i int) any {
			if rows := shown(); i < len(rows) {
				return rows[i]
			}
			return i
		},
		Row: func(cx *ns.Scope, row ns.ListRow) ns.Node {
			cue := func() core.Segment {
				rows := shown()
				if i := row.Index(); i < len(rows) && rows[i] < len(cues()) {
					return cues()[rows[i]]
				}
				return core.Segment{}
			}
			on := func() bool {
				rows := shown()
				i := row.Index()
				return i < len(rows) && rows[i] == current()
			}
			index := func() int {
				if rows, i := shown(), row.Index(); i < len(rows) {
					return rows[i]
				}
				return -1
			}
			return s.cueRow(cx, cue, index, on, clockWidth)
		}}
}

// cueClockStyle is the style of a line's start in the subtitle list.
var cueClockStyle = ns.Style{FontSize: 11, LineHeight: 15, FontVariant: ns.FontTabularNums, Color: onGlass, Opacity: ns.Alpha(0.7), FlexShrink: 0, Margin: ns.Edges{Top: 2}}

// cueClockWidth is the width of the subtitle list's column of starts: the latest start the film can have, its length or the last line's start, as the column draws it, so every start fits on one line, an hour's as well; at least 38, the prototype's.
func cueClockWidth(cx *ns.Scope, a App, cues func() []core.Segment) func() ns.Points {
	return live.Memo(cx, a, func() ns.Points {
		latest := a.PlayLength()
		if list := cues(); len(list) > 0 {
			latest = max(latest, list[len(list)-1].Start)
		}
		m := cx.MeasureText(cueClockTenths(latest), ns.Font{Size: cueClockStyle.FontSize, Variant: cueClockStyle.FontVariant})
		return ns.Points(max(38, float32(math.Ceil(float64(m.Width)))))
	})
}

// cueRow is one cue of the list: its start, in a column clockWidth wide, and its lines as the picture shows them, in their order (listLines). The line the mode hides, as the translation in Original Only, shows under them blurred until the pointer rests on the row or the keyboard focus is in it, so the list reads as a transcript without giving away what the learner listens for. The row is named by both lines in their order, is selected while it plays, and has a bar at its start while its line loops. A click plays from its start, with Shift has the loop run through its line (LoopThrough), and a right click explains or copies it (cueMenu).
func (s *screen) cueRow(cx *ns.Scope, cue func() core.Segment, index func() int, current func() bool, clockWidth func() ns.Points) ns.Node {
	a := s.a
	looped := live.Func(a, func() bool {
		from, to, ok := a.LoopSpan()
		at := cue().Start
		return ok && at >= from && (to == 0 || at < to)
	})
	lines := live.Func(a, func() cueLines {
		shown, hidden := listLines(a, cue().Text)
		return cueLines{shown: shown, hidden: hidden}
	})
	nth := func(i int) func() string {
		return func() string {
			if l := lines().shown; i < len(l) {
				return l[i]
			}
			return ""
		}
	}
	main, second := nth(0), nth(1)
	both := live.Func(a, func() string {
		var out []string
		for _, l := range styledLines(bothLines{a}, cue().Text) {
			out = append(out, l.text)
		}
		return strings.Join(out, " ")
	})
	// peeking is set while the pointer rests on the row or the keyboard focus is in it.
	hovered, focused := ns.NewSignal(cx, false), ns.NewSignal(cx, false)
	peeking := func() bool { return hovered.Get() || focused.Get() }
	place := &ns.Ref{}
	return ns.View{Ref: place,
		OnMouseEnter: func(ns.MouseEvent) { hovered.Set(true) }, OnMouseLeave: func(ns.MouseEvent) { hovered.Set(false) },
		OnFocusIn: func() { focused.Set(true) }, OnFocusOut: func() { focused.Set(false) },
		Children: []ns.Node{base.Button{
			LabelFunc: func() string { return cueClock(cue().Start) + " " + both() },
			Selected:  current,
			OnPress: live.On(a, func(e ns.PressEvent) {
				if e.Modifiers.Shift {
					a.LoopThrough(index())
					return
				}
				a.Seek(cue().Start + a.Delay())
			}),
			Style: ns.Style{Direction: ns.Row, Gap: 10, Padding: ns.Symmetric(8, 10), CornerRadius: 10, AlignItems: ns.AlignStart,
				Hover: &ns.Style{Background: panelLine}, FocusVisible: ui.FocusRing(cx, -2)},
			StyleFunc: func() ns.Style {
				if current() {
					return ns.Style{Background: panelCurrent}
				}
				return ns.Style{}
			},
			Children: []ns.Node{
				ns.Show{When: looped, Then: func(*ns.Scope) ns.Node {
					return ns.View{TestID: "film-cue-looped", PointerEvents: ns.PointerEventsNone,
						Style: ns.Style{Position: ns.Absolute, Left: ns.Pt(3), Top: ns.Pt(9), Bottom: ns.Pt(9), Width: ns.Pt(3), CornerRadius: 1.5, Background: cueLoopBar}}
				}},
				ns.Text{ContentFunc: func() string { return cueClockTenths(cue().Start) }, MaxLines: 1, Style: cueClockStyle,
					StyleFunc: func() ns.Style { return ns.Style{Width: ns.Pt(clockWidth())} }},
				ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 2}, Children: []ns.Node{
					ns.Text{ContentFunc: main, Style: ns.Style{FontSize: 14, LineHeight: 19, Color: onGlass}, StyleFunc: func() ns.Style {
						if current() {
							return ns.Style{FontWeight: ns.FontWeightSemibold}
						}
						return ns.Style{FontWeight: ns.FontWeightNormal}
					}},
					ns.Show{When: func() bool { return second() != "" }, Then: func(*ns.Scope) ns.Node {
						return ns.Text{ContentFunc: second, Style: cueSecondStyle}
					}},
					ns.Show{When: func() bool { return lines().hidden != "" }, Then: func(*ns.Scope) ns.Node {
						return ns.Text{ContentFunc: func() string { return lines().hidden }, Role: ns.RoleNone, Style: cueSecondStyle,
							StyleFunc: func() ns.Style {
								if peeking() {
									return ns.Style{Transition: cuePeekTransition}
								}
								return ns.Style{Opacity: ns.Alpha(0.45), Filters: []ns.Filter{ns.Blur(5)}, Transition: cuePeekTransition}
							}}
					}},
				}},
			},
		}}, OnContextMenu: func(e ns.ContextMenuEvent) {
			nsdesktop.PopupMenu(cx, cueMenu(cx, cue(), live.Do(a, func() { a.ExplainLine(index()) })), e.PagePosition, nil)
		}}
}

// cueLines is a cue as a row of the subtitle list shows it (listLines).
type cueLines struct {
	shown  []string
	hidden string
}

// cueLoopBar marks the rows of the lines the player loops, by their start.
var cueLoopBar = ns.RGBA(0xFFD60A, 0.9)

// cueSecondStyle is the style of a row's lines under its first.
var cueSecondStyle = ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass, Opacity: ns.Alpha(0.7)}

// cuePeekTransition clears a row's hidden line, and blurs it again, gently.
var cuePeekTransition = ns.Transition{Duration: 150 * time.Millisecond, Easing: ns.EaseOut, Properties: ns.TransitionOpacity}

// cueMenu is what a right click on a cue of the list offers: explaining it with the AI service (explain), then copying the cue as the list shows it, and, for a cue that has a translation, its original or its translation alone.
func cueMenu(cx *ns.Scope, cue core.Segment, explain func()) []nsdesktop.MenuItem {
	items := []nsdesktop.MenuItem{
		{Label: locales.FilmExplainLine(cx), OnSelect: explain},
		{Separator: true},
		{Label: locales.FilmCopyLine(cx), OnSelect: func() { cx.SetClipboard(LineText(cue.Text)) }},
	}
	translation, original := splitCue(cue.Text)
	if translation == "" || original == "" {
		return items
	}
	return append(items,
		nsdesktop.MenuItem{Label: locales.FilmCopyOriginal(cx), OnSelect: func() { cx.SetClipboard(original) }},
		nsdesktop.MenuItem{Label: locales.FilmCopyTranslation(cx), OnSelect: func() { cx.SetClipboard(translation) }},
	)
}

// clipsTab is the film cut into 片段 of equal time; a click plays from one, and the playing one is lit.
func (s *screen) clipsTab(cx *ns.Scope) ns.Node {
	a := s.a
	cues := filmCues(cx, a)
	length := live.Memo(cx, a, a.PlayLength)
	chapters := ns.NewMemoFunc(cx, func() []Chapter { return Chapters(cues(), length()) }, slices.Equal[[]Chapter]).Get
	state := live.Memo(cx, a, func() subsState {
		subs := a.Subs()
		switch {
		case !subs.Ready:
			return subsLoading
		case subs.Err != "":
			return subsFailed
		}
		return subsReady
	})
	body := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		switch {
		case state() == subsLoading:
			return panelNote(func() string { return locales.FilmLoadingSubtitles(cx) }, nil)
		case state() == subsFailed:
			return s.subsFailed(cx)
		}
		return ns.Show{When: func() bool { return len(chapters()) > 0 }, Then: func(cx *ns.Scope) ns.Node {
			return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0), Gap: 8}, Children: []ns.Node{
				ns.List{TestID: "film-clips", Style: ns.Style{FlexGrow: 1, MinHeight: ns.Pt(0)}, Gap: 8, EstimatedSize: 64,
					Count: func() int { return len(chapters()) },
					Key:   func(i int) any { return i },
					Row: func(cx *ns.Scope, row ns.ListRow) ns.Node {
						ch := func() Chapter {
							if list := chapters(); row.Index() < len(list) {
								return list[row.Index()]
							}
							return Chapter{}
						}
						return s.chapterRow(cx, row.Index, ch)
					}},
			}}
		}, Else: func(cx *ns.Scope) ns.Node {
			return panelNote(func() string { return locales.FilmNoClipsYet(cx) }, nil)
		}}
	})
	return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0), Padding: ns.Edges{Right: 14, Bottom: 14, Left: 14}}, Children: []ns.Node{body}}
}

// clipTiles are the colors of the 片段's tiles, the prototype's stand-ins for a frame of each.
var clipTiles = []ns.Color{ns.RGB(0x3E6A5B), ns.RGB(0x6A8EA1), ns.RGB(0x4B7A8C), ns.RGB(0x5D7F63), ns.RGB(0x7C8F6B), ns.RGB(0x2E5B6E)}

// chapterRow is a 片段 of the clips tab: a tile with a frame of it, once read, and its start; the span it covers, and the line it is named after. Its times are the cues', so it plays from its start moved by the subtitle delay, where the picture shows its first line, as a line of the subtitle list does, and is lit while the playhead, moved back by the delay, is in it.
func (s *screen) chapterRow(cx *ns.Scope, index func() int, ch func() Chapter) ns.Node {
	a := s.a
	active := func() bool {
		if a.Play() == nil {
			return false
		}
		at, c := s.playhead()-a.Delay(), ch()
		return at >= c.Start && (c.End <= c.Start || at < c.End)
	}
	frame := live.Func(a, func() ns.ImageSource {
		data, ok := a.ClipPoster(ch().Start)
		if !ok {
			return ns.ImageSource{}
		}
		return ns.ImageSource{Data: data}
	})
	hasFrame := live.Func(a, func() bool { _, ok := a.ClipPoster(ch().Start); return ok })
	thumb := ns.View{
		Style: ns.Style{Width: ns.Pt(84), AspectRatio: 16.0 / 9, CornerRadius: 7, FlexShrink: 0, Overflow: ns.Hidden,
			Shadows: []ns.BoxShadow{{Spread: 0.5, Color: ns.RGBA(0xFFFFFF, 0.15), Inset: true}}},
		StyleFunc: func() ns.Style { return ns.Style{Background: clipTiles[index()%len(clipTiles)]} },
		Children: []ns.Node{
			ns.Show{When: hasFrame, Then: func(*ns.Scope) ns.Node {
				return ns.View{Style: fillStyle, Children: []ns.Node{
					ns.Image{SourceFunc: frame, ResizeMode: ns.Cover, Style: fillStyle},
					ns.View{Style: fillStyle.Merge(ns.Style{Background: ns.RGBA(0x000000, 0.18)})},
				}}
			}},
			ns.Text{ContentFunc: func() string { return cueClock(ch().Start) }, Style: ns.Style{Position: ns.Absolute, Right: ns.Pt(4), Bottom: ns.Pt(3), FontSize: 10, LineHeight: 12, FontWeight: ns.FontWeightSemibold, FontVariant: ns.FontTabularNums, Color: onGlass}},
		},
	}
	return base.Button{
		LabelFunc: func() string {
			c := ch()
			if c.Title == "" {
				return locales.FilmClipSpan(cx, index()+1, cueClock(c.Start), cueClock(c.End))
			}
			return locales.FilmClip(cx, index()+1, cueClock(c.Start), cueClock(c.End), c.Title)
		},
		Selected: active,
		OnPress:  live.On(a, func(ns.PressEvent) { a.Seek(ch().Start + a.Delay()) }),
		Style: ns.Style{Direction: ns.Row, Gap: 12, Padding: ns.All(8), CornerRadius: 12, AlignItems: ns.AlignCenter,
			Hover: &ns.Style{Background: panelLine}, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() ns.Style {
			if active() {
				return ns.Style{Background: ns.RGBA(0xFFFFFF, 0.16)}
			}
			return ns.Style{}
		},
		Children: []ns.Node{
			thumb,
			ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 2}, Children: []ns.Node{
				ns.Text{ContentFunc: func() string {
					c := ch()
					return locales.FilmClipSpan(cx, index()+1, cueClock(c.Start), cueClock(c.End))
				}, Style: ns.Style{FontSize: 11, LineHeight: 14, FontWeight: ns.FontWeightSemibold, Color: onGlass, Opacity: ns.Alpha(0.7)}},
				ns.Text{ContentFunc: func() string { return ch().Title }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
					Style: ns.Style{FontSize: 13, LineHeight: 18, Color: onGlass},
					StyleFunc: func() ns.Style {
						if active() {
							return ns.Style{FontWeight: ns.FontWeightBold}
						}
						return ns.Style{FontWeight: ns.FontWeightMedium}
					}},
				ns.Show{When: func() bool { return ch().Detail != "" }, Then: func(*ns.Scope) ns.Node {
					return ns.Text{ContentFunc: func() string { return ch().Detail }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.6)}}
				}},
			}},
		},
	}
}

// infoState is what the info tab lists of the film, which changes only as the film's folder is read or its job moves on.
type infoState struct {
	title, stage, language, spoken, folder, source, record string
	files                                                  string
	// speech is whether the film can be told the language spoken in it: a done film with a job (speechLanguage).
	speech bool
}

// infoTab shows where the film is: its folder, the files in it and where it came from, and for a done film the language spoken in it, which the user can correct.
func (s *screen) infoTab(cx *ns.Scope) ns.Node {
	a := s.a
	// The tab is built again when the film's folder has been read, its job moved on or the language changed; it holds no field that keeps the focus.
	files := ns.NewMemoFunc(cx, live.Func(a, func() []File {
		if info := a.Info(); len(info.Files) > 0 {
			return info.Files
		}
		return knownInfoFiles(s.item())
	}), slices.Equal[[]File]).Get
	state := live.Memo(cx, a, func() infoState {
		item := s.item()
		folder := a.Info().Folder
		if folder == "" && item.VideoPath != "" {
			folder = filepath.Dir(item.VideoPath)
		}
		record := a.LibraryPath()
		if record == "" {
			record = "~/Library/Application Support/Kando/library.json"
		}
		return infoState{title: title(item), stage: string(item.Stage), language: item.Language, spoken: item.Spoken, folder: folder, source: item.Source, record: record,
			speech: item.JobID != "" && item.Stage == core.StageComplete}
	})
	return ns.Dynamic(func(cx *ns.Scope) ns.Node {
		st := state()
		item := ns.Untracked(cx, s.item)
		groups := []ns.Node{panelGroup(ns.View{Style: ns.Style{Padding: ns.All(12), Gap: 8}, Children: []ns.Node{
			ns.Text{Content: shorten(st.title, 52), Style: ns.Style{FontSize: 15, LineHeight: 19, FontWeight: 650, Color: onGlass}},
			ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignCenter}, Children: []ns.Node{
				infoChip(s.infoStage(cx, item)),
				infoChip(languagesName(cx, st.spoken, st.language)),
			}},
		}})}
		if st.speech {
			groups = append(groups, panelHeading(locales.FilmSpokenLanguage(cx)), panelGroup(s.speechLanguage(cx, st.spoken)))
		}
		if st.folder != "" {
			groups = append(groups, panelHeading(locales.FilmFolder(cx)), panelGroup(ns.View{Style: ns.Style{Padding: ns.Symmetric(10, 12), Gap: 8}, Children: []ns.Node{
				ns.View{Selectable: true, Children: []ns.Node{ns.Text{Content: displayPath(st.folder), Style: monoStyle(11, 16, 0.9)}}},
				ns.View{Style: ns.Style{AlignSelf: ns.AlignStart}, Children: []ns.Node{
					ui.Button{TestID: "film-info-reveal", Text: locales.FilmShowInFinder(cx, desktop.Platform(cx)), Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
						OnPress: live.Do(a, func() { a.Reveal(s.item().VideoPath) })},
				}},
			}}))
		}
		if list := files(); len(list) > 0 {
			rows := make([]ns.Node, 0, len(list))
			for i, f := range filesInOrder(list) {
				rows = append(rows, fileRow(cx, f, i > 0, live.Do(a, func() {
					if st.folder != "" {
						a.Reveal(filepath.Join(st.folder, f.Name))
					}
				})))
			}
			groups = append(groups, panelHeading(locales.FilmFiles(cx)), panelGroup(rows...))
		}
		var source ns.Node
		if st.source == "" || strings.HasPrefix(st.source, "file://") {
			source = ns.Text{Content: locales.FilmLocalFile(cx), Style: ns.Style{FontSize: 12, LineHeight: 17, Color: onGlass, Opacity: ns.Alpha(0.9)}}
		} else {
			source = ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignStart}, Children: []ns.Node{
				ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0)}, Children: []ns.Node{ui.Link{Text: st.source, Href: st.source, Style: ns.Style{FontSize: 12, LineHeight: 17}}}},
				copyInfoButton(cx, st.source),
			}}
		}
		groups = append(groups, panelHeading(locales.FilmSource(cx)), panelGroup(ns.View{Style: ns.Style{Padding: ns.Symmetric(10, 12), Gap: 6}, Children: []ns.Node{
			source,
			ns.Text{Content: locales.FilmJobRecord(cx), Style: ns.Style{FontSize: 11, LineHeight: 14, Color: onGlass, Opacity: ns.Alpha(0.55)}},
			ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignStart}, Children: []ns.Node{
				ns.View{Selectable: true, Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0)}, Children: []ns.Node{ns.Text{Content: displayPath(st.record), Style: monoStyle(11, 15, 0.65)}}},
				ui.Button{TestID: "film-info-open-record", Text: locales.FilmOpen(cx), Variant: ui.ButtonGhost, Size: ui.SizeSmall, OnPress: live.Do(a, func() { a.Reveal(st.record) })},
				copyInfoButton(cx, st.record),
			}},
		}}))
		return ns.Scroll{TestID: "film-info", Style: ns.Style{FlexGrow: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{
			ns.View{Style: ns.Style{Padding: ns.Edges{Right: 14, Bottom: 16, Left: 14}, Gap: 12}, Children: groups},
		}}
	})
}

// monoStyle is a path's text in the info tab: Menlo, at size over lineHeight, in white at alpha.
func monoStyle(size, lineHeight ns.Points, alpha float32) ns.Style {
	return ns.Style{FontFamily: "Menlo, " + ns.FontMonospace, FontSize: size, LineHeight: lineHeight, Color: onGlass, Opacity: ns.Alpha(alpha)}
}

// copyInfoButton puts value on the clipboard.
func copyInfoButton(cx *ns.Scope, value string) ns.Node {
	return ui.Button{Text: locales.FilmCopy(cx), Variant: ui.ButtonGhost, Size: ui.SizeSmall, OnPress: func() { cx.SetClipboard(value) }}
}

// knownInfoFiles keeps the information page useful while the folder is still being read, or when a video was opened without a readable folder. It only uses paths already present in the film.
func knownInfoFiles(item Item) []File {
	seen := map[string]bool{}
	var out []File
	for _, path := range []string{item.VideoPath, item.SubtitlePath} {
		name := filepath.Base(path)
		if path == "" || name == "." || name == string(filepath.Separator) || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, File{Name: name, Size: -1})
	}
	return out
}

// infoChip is a small label of the film's, as its stage; nothing for "".
func infoChip(text string) ns.Node {
	if text == "" {
		return ns.Group{}
	}
	return ns.View{Style: ns.Style{CornerRadius: 8, Padding: ns.Symmetric(2, 7), Background: ns.RGBA(0xFFFFFF, 0.12)}, Children: []ns.Node{
		ns.Text{Content: text, MaxLines: 1, Style: ns.Style{FontSize: 10, LineHeight: 13, FontWeight: ns.FontWeightSemibold, Color: onGlass, Opacity: ns.Alpha(0.75)}},
	}}
}

func (s *screen) infoStage(cx *ns.Scope, item Item) string {
	if item.Stage != "" {
		return s.a.StageLabel(cx, item.Stage)
	}
	if item.VideoPath != "" {
		return locales.FilmLocalVideo(cx)
	}
	return locales.FilmUnprocessed(cx)
}

// languageName is what the info tab shows for the language a film's subtitles were translated into, the value its job saved, such as 简体中文 (subtitle.TargetLanguageName), or "" for none.
func languageName(cx *ns.Scope, saved string) string {
	if saved == "" {
		return ""
	}
	return subtitle.TargetLanguageName(cx, saved)
}

// languagesName is what the info tab shows for the languages of a film: the one heard in it, whisper's code spoken, and the one it was translated into, saved, such as 日语 → 简体中文; either alone when the other is not known.
func languagesName(cx *ns.Scope, spoken, saved string) string {
	switch {
	case spoken == "":
		return languageName(cx, saved)
	case saved == "":
		return subtitle.SpokenLanguageName(cx, spoken)
	}
	return locales.FilmInfoLanguages(cx, subtitle.SpokenLanguageName(cx, spoken), languageName(cx, saved))
}

// fileRow is a file of the film's folder: a tile with its kind, its name, its size, and a reveal action.
func fileRow(cx *ns.Scope, f File, divided bool, reveal func()) ns.Node {
	tag, color := fileTag(f.Name)
	about := fileSize(cx, f.Size)
	if kind := fileKind(f.Name); kind != "" {
		about = kindLabel(cx, kind) + " · " + about
	}
	st := ns.Style{Direction: ns.Row, Gap: 10, MinHeight: ns.Pt(50), Padding: ns.Symmetric(7, 12), AlignItems: ns.AlignCenter}
	if divided {
		st.Borders, st.BorderColor = ns.Edges{Top: 1}, panelLine
	}
	return ns.View{Style: st, Children: []ns.Node{
		ns.View{Style: ns.Style{Width: ns.Pt(28), Height: ns.Pt(28), CornerRadius: 7, FlexShrink: 0, Background: color, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter}, Children: []ns.Node{
			ns.Text{Content: tag, Style: ns.Style{FontSize: 9, LineHeight: 11, FontWeight: 800, LetterSpacing: 0.3, Color: onGlass}},
		}},
		ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 1}, Children: []ns.Node{
			ns.Text{Content: f.Name, Style: monoStyle(12, 16, 1)},
			ns.Text{Content: about, Style: ns.Style{FontSize: 11, LineHeight: 14, Color: onGlass, Opacity: ns.Alpha(0.65)}},
		}},
		ui.Button{TestID: "film-info-file-" + f.Name, Text: locales.FilmReveal(cx), Label: locales.FilmRevealFile(cx, f.Name), Variant: ui.ButtonGhost, Size: ui.SizeSmall, OnPress: reveal},
	}}
}

// The kinds of files the pipeline writes into a film's folder, as the prototype's 信息 tells them apart (script 3261-3266); kindLabel names them.
const (
	kindVideo     = "video"
	kindCover     = "cover"
	kindOriginal  = "original"
	kindBilingual = "bilingual"
)

// fileKind is what a file the pipeline writes into a film's folder is, one of the kinds above, or "" for another file.
func fileKind(name string) string {
	switch {
	case strings.HasSuffix(name, ".bilingual.srt"):
		return kindBilingual
	case name == "cover.jpg":
		return kindCover
	}
	switch strings.ToUpper(strings.TrimPrefix(fileExt(name), ".")) {
	case "SRT", "VTT", "ASS":
		return kindOriginal
	case "MP4", "MKV", "MOV", "WEBM", "M4V":
		return kindVideo
	}
	return ""
}

// kindLabel is what the info tab calls a kind of file in the language the interface shows under cx.
func kindLabel(cx *ns.Scope, kind string) string {
	switch kind {
	case kindVideo:
		return locales.FilmKindVideo(cx)
	case kindCover:
		return locales.FilmKindCover(cx)
	case kindOriginal:
		return locales.FilmKindOriginal(cx)
	case kindBilingual:
		return locales.FilmKindBilingual(cx)
	}
	return ""
}

// filesInOrder is files as the prototype lists a film's folder: the video, the cover, the subtitles, then the rest, the shorter name first within each, as video.srt before video.bilingual.srt.
func filesInOrder(files []File) []File {
	rank := func(f File) int {
		switch fileKind(f.Name) {
		case kindVideo:
			return 0
		case kindCover:
			return 1
		case kindOriginal, kindBilingual:
			return 2
		}
		return 3
	}
	out := slices.Clone(files)
	slices.SortStableFunc(out, func(a, b File) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(len(a.Name), len(b.Name)), strings.Compare(a.Name, b.Name))
	})
	return out
}

// fileTag is the kind a file's tile shows and its color, after the prototype's (script 3261-3266).
func fileTag(name string) (string, ns.Color) {
	ext := strings.ToUpper(strings.TrimPrefix(fileExt(name), "."))
	switch ext {
	case "SRT", "VTT", "ASS":
		return ext, ns.RGB(0x6B5A3A)
	case "JPG", "JPEG", "PNG", "WEBP":
		return "JPG", ns.RGB(0x4C6B4C)
	case "":
		return "—", ns.RGB(0x4A4A50)
	}
	if len(ext) > 4 {
		ext = ext[:4]
	}
	return ext, ns.RGB(0x3A5A8C)
}

func fileExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return name[i:]
	}
	return ""
}

func fileSize(cx *ns.Scope, n int64) string {
	if n < 0 {
		return locales.FilmSizeUnknown(cx)
	}
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return locales.FilmBytes(cx, int(n))
}

// panelHeading is a heading over a card of the panel.
func panelHeading(text string) ns.Node {
	return ns.Text{Content: text, Style: ns.Style{FontSize: 11, LineHeight: 14, FontWeight: ns.FontWeightBold, Color: onGlass, Opacity: ns.Alpha(0.6), Padding: ns.Symmetric(0, 4)}}
}

// panelGroup is a card of the panel, a lighter glass holding rows.
func panelGroup(children ...ns.Node) ns.Node {
	return ns.View{Style: ns.Style{CornerRadius: 14, Background: panelCard}, Children: children}
}

// subsFailed says why the film's subtitles did not load, the subtitle file gone or unreadable, in the danger color, over what writes them again: for a done film's job Process Entire Film Again, as its ⋯ offers. A video no job has processed has Start Processing in the title bar, and a job that failed Retry, so they get no button here. It is built under a region, which builds it again as the film or the language changes.
func (s *screen) subsFailed(cx *ns.Scope) ns.Node {
	a := s.a
	note := panelNote(live.Func(a, func() string { return a.ErrText(cx, a.Subs().Err) }), ui.ColorsOf(cx).Danger)
	item := s.item()
	if item.JobID == "" || item.Stage != core.StageComplete {
		return note
	}
	note.Style.Margin = ns.Edges{Top: 30, Right: 10, Bottom: 12, Left: 10}
	return ns.View{Style: ns.Style{AlignItems: ns.AlignCenter}, Children: []ns.Node{
		note,
		ui.Button{TestID: "film-subs-reprocess", Text: locales.FilmReprocess(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
			OnPress: live.Do(a, func() { a.RetryOpen(s.item().JobID) })},
	}}
}

// panelNote is a line of the panel saying why it lists nothing, in color, or white when nil.
func panelNote(text func() string, color func() ns.Color) ns.Text {
	return ns.Text{ContentFunc: text, Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass, Opacity: ns.Alpha(0.6), Margin: ns.Symmetric(30, 10), TextAlign: ns.TextAlignCenter},
		StyleFunc: func() ns.Style {
			if color != nil {
				return ns.Style{Color: color(), Opacity: ns.Alpha(1)}
			}
			return ns.Style{}
		}}
}

// Chapter is one of the 片段 the film is cut into.
type Chapter struct {
	Start, End    time.Duration
	Title, Detail string
}

// Chapters cuts the film, from the start to length, or to the last cue's end when that is later or length is not known, into up to 12 片段: evenly into 4 to 12 first (prototype script 3248-3259), and then every cut but the start moves to where a cue starts, so no 片段 starts or ends inside a line. A cut inside a line or a gap moves to the next cue's start, cuts that move to the same cue make one, and a cut with no cue starting after it goes. Their times are the cues', which the subtitle delay moves on the picture. Each is named after the cue it starts with, the first after the cue playing at the start or the first one: the translation, with the original as its detail.
func Chapters(cues []core.Segment, length time.Duration) []Chapter {
	if len(cues) == 0 {
		return nil
	}
	end := max(length, cues[len(cues)-1].End, cues[len(cues)-1].Start)
	if end <= 0 {
		return nil
	}
	n := int((end + 15*time.Second) / (30 * time.Second))
	n = min(max(n, 4), 12)
	starts := make([]time.Duration, 1, n)
	next := 0
	for k := 1; k < n; k++ {
		cut := end * time.Duration(k) / time.Duration(n)
		for next < len(cues) && cues[next].Start < cut {
			next++
		}
		if next == len(cues) {
			break
		}
		if at := cues[next].Start; at > starts[len(starts)-1] && at < end {
			starts = append(starts, at)
		}
	}
	out := make([]Chapter, 0, len(starts))
	for i, from := range starts {
		to := end
		if i+1 < len(starts) {
			to = starts[i+1]
		}
		out = append(out, chapterAt(cues, from, to))
	}
	return out
}

// chapterAt is the 片段 from from to to, named after the cue that started last by from, or the first one.
func chapterAt(cues []core.Segment, from, to time.Duration) Chapter {
	cue := cues[0]
	for _, c := range cues {
		if c.Start > from {
			break
		}
		cue = c
	}
	title, detail := splitCue(cue.Text)
	if title == "" {
		title, detail = detail, ""
	}
	return Chapter{Start: from, End: to, Title: shorten(title, 36), Detail: shorten(detail, 48)}
}
