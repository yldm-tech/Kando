package film

import (
	"cmp"
	"fmt"
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

// subsTab lists the cues with the time each starts, the playing one lit and kept in view as it changes. A click plays from a cue, and a right click copies it (cueMenu). The field above it searches both lines.
func (s *screen) subsTab(cx *ns.Scope) ns.Node {
	a := s.a
	colors := ui.ColorsOf(cx)
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
	count := live.Func(a, func() int { return len(a.Subs().Cues) })
	chip := ns.Show{When: func() bool { return state() == subsReady }, Then: func(cx *ns.Scope) ns.Node {
		return ns.View{TestID: "film-subs-count", Style: ns.Style{Direction: ns.Row, Height: ns.Pt(24), Padding: ns.Symmetric(0, 10), CornerRadius: 12, Gap: 3, AlignItems: ns.AlignCenter, AlignSelf: ns.AlignStart, Background: ns.RGBA(0xFFFFFF, 0.25)},
			Children: []ns.Node{
				ns.Text{ContentFunc: func() string { return locales.FilmAll(cx) }, Style: ns.Style{FontSize: 11, LineHeight: 14, FontWeight: ns.FontWeightSemibold, Color: onGlass}},
				ns.Text{ContentFunc: func() string { return fmt.Sprint(count()) }, Style: ns.Style{FontSize: 11, LineHeight: 14, FontWeight: ns.FontWeightSemibold, Color: onGlass, Opacity: ns.Alpha(0.6)}},
			}}
	}}
	body := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		switch state() {
		case subsLoading:
			return panelNote(func() string { return locales.FilmLoadingSubtitles(cx) }, nil)
		case subsFailed:
			return panelNote(live.Func(a, func() string { return a.ErrText(cx, a.Subs().Err) }), colors.Danger)
		case subsNone:
			return panelNote(func() string { return locales.FilmNoSubtitlesYet(cx) }, nil)
		case subsEmpty:
			return panelNote(func() string { return locales.FilmSubtitlesEmpty(cx) }, nil)
		}
		return s.cueList(cx)
	})
	return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{
		ns.View{Style: ns.Style{Padding: ns.Edges{Right: 14, Bottom: 8, Left: 14}, Gap: 8}, Children: []ns.Node{s.searchField(cx), chip}},
		ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0), Padding: ns.Edges{Right: 8, Bottom: 12, Left: 8}}, Children: []ns.Node{body}},
		ns.View{Style: ns.Style{Padding: ns.Edges{Top: 8, Right: 14, Bottom: 12, Left: 14}, Borders: ns.Edges{Top: 1}, BorderColor: panelLine}, Children: []ns.Node{
			ns.Text{ContentFunc: func() string { return locales.FilmClickToJump(cx) }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.55)}},
		}},
	}}
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

// searchField is the subtitle list's search field: the prototype's is 30 high, white at 10%, its text 12, a magnifying glass before it. ⌘F puts the keyboard in it (SearchFocus), and Escape empties it while it holds something.
func (s *screen) searchField(cx *ns.Scope) ns.Node {
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
	escape := ns.Show{When: func() bool { return focused.Get() && query() != "" }, Then: func(cx *ns.Scope) ns.Node {
		cx.OnKeyWith("escape", ns.KeyOptions{When: ns.KeyAlways}, clear)
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
		OnFocus:         func() { focused.Set(true) },
		OnBlur:          func() { focused.Set(false) },
		Style: ns.Style{FlexGrow: 1, MinWidth: ns.Pt(0), Height: ns.Pt(30), CornerRadius: 8, Background: ns.RGBA(0xFFFFFF, 0.1), FontSize: 12,
			Padding: ns.Edges{Left: 28, Right: 8}, Color: onGlass, PlaceholderColor: ns.RGBA(0xFFFFFF, 0.45), CaretColor: onGlass},
	}
	glass := ns.View{PointerEvents: ns.PointerEventsNone, Style: ns.Style{Position: ns.Absolute, Top: ns.Pt(8), Left: ns.Pt(9)},
		Children: []ns.Node{ns.Icon{SVG: iconSearch, Style: ns.Style{Width: ns.Pt(14), Height: ns.Pt(14), Color: ns.RGBA(0xFFFFFF, 0.6)}}}}
	return ns.View{Style: ns.Style{Direction: ns.Row}, Children: []ns.Node{
		ns.View{Style: ns.Style{FlexGrow: 1, MinWidth: ns.Pt(0)}, Children: []ns.Node{input, glass, escape}},
	}}
}

// cueList is the subtitle list, narrowed to the cues that hold the search, the playing one lit and scrolled into view as it changes.
func (s *screen) cueList(cx *ns.Scope) ns.Node {
	a := s.a
	cues := filmCues(cx, a)
	query := live.Memo(cx, a, func() string { return strings.ToLower(strings.TrimSpace(*a.Search())) })
	shown := ns.NewMemoFunc(cx, func() []int {
		q := query()
		out := make([]int, 0, len(cues()))
		for i, cue := range cues() {
			if q == "" || strings.Contains(strings.ToLower(cue.Text), q) {
				out = append(out, i)
			}
		}
		return out
	}, slices.Equal[[]int]).Get
	current := ns.NewMemo(cx, func() int {
		a.Track()
		if a.Play() == nil {
			return -1
		}
		return cueIndex(cues(), s.playhead()-a.Delay())
	}).Get
	list := &ns.Ref{}
	// The list follows the playing cue as it changes, not as it is scrolled by hand or narrowed by the search.
	followed := -2
	cx.Effect(func() {
		cur, rows := current(), shown()
		if cur == followed {
			return
		}
		followed = cur
		for row, i := range rows {
			if i == cur {
				list.ScrollToIndex(row, true)
				return
			}
		}
	})
	return ns.Show{
		When: func() bool { return len(shown()) > 0 },
		Then: func(cx *ns.Scope) ns.Node {
			return ns.List{TestID: "film-cues", Ref: list, Style: ns.Style{FlexGrow: 1, MinHeight: ns.Pt(0)}, EstimatedSize: 56,
				Count: func() int { return len(shown()) },
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
					return s.cueRow(cx, cue, on)
				}}
		},
		Else: func(cx *ns.Scope) ns.Node {
			return panelNote(func() string { return locales.FilmNoMatch(cx) }, nil)
		},
	}
}

// cueRow is one cue of the list: its start, the translation and the original under it. A click plays from its start, and a right click copies it (cueMenu).
func (s *screen) cueRow(cx *ns.Scope, cue func() core.Segment, current func() bool) ns.Node {
	a := s.a
	lines := func() (string, string) {
		translation, original := splitCue(cue().Text)
		if translation == "" {
			return original, ""
		}
		return translation, original
	}
	main := func() string { t, _ := lines(); return t }
	second := func() string { _, o := lines(); return o }
	place := &ns.Ref{}
	return ns.View{Ref: place, Children: []ns.Node{base.Button{
		LabelFunc: func() string { return cueClock(cue().Start) + " " + main() },
		OnPress:   live.On(a, func(ns.PressEvent) { a.Seek(cue().Start + a.Delay()) }),
		Style: ns.Style{Direction: ns.Row, Gap: 10, Padding: ns.Symmetric(8, 10), CornerRadius: 10, AlignItems: ns.AlignStart,
			Hover: &ns.Style{Background: panelLine}, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() ns.Style {
			if current() {
				return ns.Style{Background: panelCurrent}
			}
			return ns.Style{}
		},
		Children: []ns.Node{
			ns.Text{ContentFunc: func() string { return cueClockTenths(cue().Start) }, Style: ns.Style{FontSize: 11, LineHeight: 15, FontVariant: ns.FontTabularNums, Color: onGlass, Opacity: ns.Alpha(0.7), Width: ns.Pt(38), FlexShrink: 0, Margin: ns.Edges{Top: 2}}},
			ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 2}, Children: []ns.Node{
				ns.Text{ContentFunc: main, Style: ns.Style{FontSize: 14, LineHeight: 19, Color: onGlass}, StyleFunc: func() ns.Style {
					if current() {
						return ns.Style{FontWeight: ns.FontWeightSemibold}
					}
					return ns.Style{FontWeight: ns.FontWeightNormal}
				}},
				ns.Show{When: func() bool { return second() != "" }, Then: func(*ns.Scope) ns.Node {
					return ns.Text{ContentFunc: second, Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass, Opacity: ns.Alpha(0.7)}}
				}},
			}},
		},
	}}, OnContextMenu: func(e ns.ContextMenuEvent) {
		nsdesktop.PopupMenu(cx, cueMenu(cx, cue()), e.PagePosition, nil)
	}}
}

// cueMenu is what a right click on a cue of the list offers: copying the cue as the list shows it, and, for a cue that has a translation, its original or its translation alone.
func cueMenu(cx *ns.Scope, cue core.Segment) []nsdesktop.MenuItem {
	items := []nsdesktop.MenuItem{{Label: locales.FilmCopyLine(cx), OnSelect: func() { cx.SetClipboard(LineText(cue.Text)) }}}
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
	colors := ui.ColorsOf(cx)
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
			return panelNote(live.Func(a, func() string { return a.ErrText(cx, a.Subs().Err) }), colors.Danger)
		}
		return ns.Show{When: func() bool { return len(chapters()) > 0 }, Then: func(cx *ns.Scope) ns.Node {
			return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0), Gap: 8}, Children: []ns.Node{
				ns.View{Style: ns.Style{MinHeight: ns.Pt(30), JustifyContent: ns.JustifyCenter}, Children: []ns.Node{
					ns.Text{ContentFunc: func() string { return locales.FilmClipsNote(cx) }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.7)}},
				}},
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

// chapterRow is a 片段 of the clips tab: a tile with a frame of it, once read, and its start; the span it covers, and the line it is named after.
func (s *screen) chapterRow(cx *ns.Scope, index func() int, ch func() Chapter) ns.Node {
	a := s.a
	active := func() bool {
		if a.Play() == nil {
			return false
		}
		at, c := s.playhead(), ch()
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
		LabelFunc: func() string { return locales.FilmClip(cx, index()+1) },
		OnPress:   live.On(a, func(ns.PressEvent) { a.Seek(ch().Start) }),
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
	title, stage, language, folder, source, record string
	files                                          string
}

// infoTab shows where the film is: its folder, the files in it and where it came from.
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
		return infoState{title: title(item), stage: string(item.Stage), language: item.Language, folder: folder, source: item.Source, record: record}
	})
	return ns.Dynamic(func(cx *ns.Scope) ns.Node {
		st := state()
		item := ns.Untracked(cx, s.item)
		groups := []ns.Node{panelGroup(ns.View{Style: ns.Style{Padding: ns.All(12), Gap: 8}, Children: []ns.Node{
			ns.Text{Content: shorten(st.title, 52), Style: ns.Style{FontSize: 15, LineHeight: 19, FontWeight: 650, Color: onGlass}},
			ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignCenter}, Children: []ns.Node{
				infoChip(s.infoStage(cx, item)),
				infoChip(languageName(cx, st.language)),
			}},
		}})}
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
		ui.Button{TestID: "film-info-file-" + f.Name, Text: locales.FilmReveal(cx), Variant: ui.ButtonGhost, Size: ui.SizeSmall, OnPress: reveal},
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

// panelNote is a line of the panel saying why it lists nothing, in color, or white when nil.
func panelNote(text func() string, color func() ns.Color) ns.Node {
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

// Chapters cuts the film into 4 to 12 片段 of equal time, about 30 seconds each, from the start to length, or to the last cue's end when that is later or length is not known. Each is named after the cue playing as it starts, or the first one after it: the translation, with the original as its detail (prototype script 3248-3259).
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
	out := make([]Chapter, 0, n)
	for k := range n {
		from := end * time.Duration(k) / time.Duration(n)
		to := end * time.Duration(k+1) / time.Duration(n)
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
