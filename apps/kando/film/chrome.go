package film

import (
	"math"
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// backGap is the room between the traffic lights and the back button. The prototype's 7 points left the back button about 2 points from the zoom button, so a click meant for 返回 often closed or zoomed the window; 40 points gives the two controls a clear hit-target gap.
const backGap ns.Points = 40

// topBar is the player's title bar over the picture, after the prototype's (template 556-579): the back button and the film's actions on the glass of the player's small buttons, and between them the film's title over where it stands. It is transparent, so the picture shows through it, and moves the window when dragged. It keeps clear of the traffic lights when it lies under them, a film filling the window; in the content column beside the sidebar it starts at the column's edge, the traffic lights being over the sidebar. It fades with the transport.
func (s *screen) topBar() ns.Node {
	a, cx := s.a, s.cx
	back := glyphButton(cx, svg(iconChevronLeft), func() string { return locales.FilmBackToLibrary(cx) }, nil, 34, 34, 10, 16, "film-back",
		live.Do(a, func() {
			a.SetOpenKey("")
			a.StopPlayback()
		}))
	back.Style = back.Style.Merge(capsuleGlass.style())
	heading := ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0)}, Children: []ns.Node{
		ns.Text{TestID: "film-title", ContentFunc: func() string { return title(s.item()) }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
			Style: ns.Style{FontSize: 14, LineHeight: 18, FontWeight: ui.Bold, Color: onGlass}},
		ns.Text{TestID: "film-status", ContentFunc: live.Func(a, func() string { return s.status(cx) }), MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
			Style: ns.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.8)}},
	}}
	return ns.View{
		TestID: "film-top", Role: ns.RoleGroup, LabelFunc: func() string { return title(s.item()) },
		Desktop: ns.DesktopViewProps{WindowDrag: ns.WindowDragOn},
		Style:   ns.Style{Position: ns.Absolute, Left: ns.Pt(0), Top: ns.Pt(0), Right: ns.Pt(0), Direction: ns.Row, Gap: 12, AlignItems: ns.AlignCenter},
		StyleFunc: func() ns.Style {
			st := s.faded()
			st.Padding = ns.Edges{Top: 14, Right: 16, Left: s.barStart()}
			return st
		},
		Children: []ns.Node{back, heading, s.actions(), s.panelButton()},
	}
}

// barStart is where the title bar's content starts: backGap past the room of the traffic lights when the bar lies under them, as for a film filling the window under a hidden title bar; else 20, the prototype's padding.
func (s *screen) barStart() ns.Points {
	s.a.Track()
	if bar := nsdesktop.TitleBar(s.cx); bar.Left > 0 && s.a.FillsWindow() {
		return bar.Left + backGap
	}
	return 20
}

// panelButton folds the side panel away or brings it back from the title bar's top right corner, where it is found without the transport, which hides while the film plays. It is named for what a click does. A film without a picture always shows its panel, so it has none.
func (s *screen) panelButton() ns.Node {
	a, cx := s.a, s.cx
	label := live.Func(a, func() string {
		if a.Panel() {
			return locales.FilmHideSidebar(cx)
		}
		return locales.FilmShowSidebar(cx)
	})
	return ns.Show{When: live.Func(a, func() bool { return a.PlayPath() != "" }), Then: func(cx *ns.Scope) ns.Node {
		btn := glyphButton(cx, svg(iconPanel), label, withKey(cx, label, "mod+shift+t"), 34, 34, 18, 15, "film-top-panel", live.Do(a, a.TogglePanel))
		btn.Style = btn.Style.Merge(capsuleGlass.style())
		return btn
	}}
}

// status is the line under the title: the library, then how much of the film's subtitles is translated, as the prototype says it ("字幕 94%"), or how far its job is while it has no subtitles to show.
func (s *screen) status(cx *ns.Scope) string {
	item := s.item()
	parts := []string{}
	if dir := s.a.LibraryDir(); dir != "" {
		parts = append(parts, sidebar.Title(dir))
	}
	if subs := s.a.Subs(); subs.Key == item.Key && subs.Ready && subs.Err == "" && len(subs.Cues) > 0 {
		parts = append(parts, locales.FilmStatusTranslated(cx, translatedPercent(subs.Cues)))
	} else if item.Stage != "" {
		parts = append(parts, s.a.StageLabel(cx, item.Stage))
	}
	return strings.Join(parts, " · ")
}

// translatedPercent is the share of cues that have a translation, rounded to a whole percent.
func translatedPercent(cues []core.Segment) int {
	n := 0
	for _, cue := range cues {
		if translation, _ := splitCue(cue.Text); translation != "" {
			n++
		}
	}
	return int(math.Round(float64(n) * 100 / float64(len(cues))))
}

// filmActions is which of the film's buttons the title bar shows, which change only as its job moves on or its files come.
type filmActions struct {
	// job is "cancel", "start", "failed" or "" for the job's own buttons.
	job string
	// reveal is the path 在访达中显示 shows, "" for none.
	reveal string
	upload bool
	more   bool
}

// actions are the film's buttons at the end of the title bar: the job's own (start, cancel, retry), then 在访达中显示, the upload and the ⋯ menu. The job buttons act on this film's job only and do not wait for whatever else runs: a job started or retried while another runs waits in the queue behind it.
func (s *screen) actions() ns.Node {
	a, cx := s.a, s.cx
	which := live.Memo(cx, a, func() filmActions {
		item := s.item()
		var f filmActions
		switch {
		case item.JobID != "" && item.Stage != "" && !item.Stage.Done():
			f.job = "cancel"
		case item.JobID == "":
			f.job = "start"
		case item.Stage == core.StageFailed:
			f.job = "failed"
		}
		f.reveal = item.SubtitlePath
		if f.reveal == "" {
			f.reveal = item.VideoPath
		}
		f.upload = a.FilmUpload().Show
		f.more = item.JobID != "" && item.Stage == core.StageComplete
		return f
	})
	// The buttons' names are read as they are built, as ui.Button takes a static one, so they are built again when the language or the set of buttons changes.
	jobs := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		row := ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, FlexShrink: 0, AlignItems: ns.AlignCenter}}
		switch which().job {
		case "cancel":
			row.Children = append(row.Children, ui.Button{TestID: "film-cancel", Text: locales.FilmCancel(cx), Variant: ui.ButtonSecondary, OnPress: live.Do(a, func() { a.Cancel(s.item().JobID) })})
		case "start":
			row.Children = append(row.Children, ui.Button{TestID: "film-start", Text: locales.FilmStartProcessing(cx), OnPress: live.Do(a, a.ProcessFilm)})
		case "failed":
			row.Children = append(row.Children,
				ui.Button{TestID: "film-remove", Text: locales.FilmRemove(cx), Variant: ui.ButtonSecondary, OnPress: live.Do(a, func() { a.Remove(s.item().JobID) })},
				ui.Button{TestID: "film-retry", Text: locales.FilmRetry(cx), OnPress: live.Do(a, func() { a.RetryOpen(s.item().JobID) })})
		}
		return row
	})
	revealLabel := func() string { return locales.FilmShowInFinder(cx, desktop.Platform(cx)) }
	uploadLabel := live.Func(a, func() string { return a.FilmUpload().Label })
	capsule := ns.View{
		Style: capsuleGlass.style().Merge(ns.Style{Direction: ns.Row, Height: ns.Pt(34), Padding: ns.Symmetric(0, 3), AlignItems: ns.AlignCenter, FlexShrink: 0}),
		Children: []ns.Node{
			ns.Show{When: func() bool { return which().reveal != "" }, Then: func(cx *ns.Scope) ns.Node {
				return glyphButton(cx, svg(iconFolder), revealLabel, nil, 34, 28, 18, 16, "film-reveal", live.Do(a, func() { a.Reveal(which().reveal) }))
			}},
			ns.Show{When: func() bool { return which().upload }, Then: func(cx *ns.Scope) ns.Node {
				return base.Button{TestID: "film-upload", LabelFunc: uploadLabel, Desktop: ns.DesktopViewProps{WindowDrag: ns.WindowDragOff},
					OnPress: live.On(a, func(ns.PressEvent) {
						*a.UploadNote() = true
						if a.FilmUpload().Enabled {
							a.BeginFilmUpload()
						}
					}),
					Style: ns.Style{Direction: ns.Row, Height: ns.Pt(28), Padding: ns.Symmetric(0, 10), AlignItems: ns.AlignCenter, CornerRadius: 10,
						Hover: &ns.Style{Background: ns.RGBA(0xFFFFFF, 0.12)}, FocusVisible: ui.FocusRing(cx, -2)},
					Children: []ns.Node{
						ns.Icon{SVG: iconUpload, Style: ns.Style{Width: ns.Pt(15), Height: ns.Pt(17), FlexShrink: 0, Margin: ns.Edges{Right: 6}, Color: onGlass}},
						ns.Text{ContentFunc: uploadLabel, MaxLines: 1, Style: ns.Style{FontSize: 13, LineHeight: 17, Color: onGlass}},
					}}
			}},
			ns.Show{When: func() bool { return which().more }, Then: func(cx *ns.Scope) ns.Node { return s.moreButton(cx) }},
		},
	}
	capsuleShown := func() bool { f := which(); return f.reveal != "" || f.upload || f.more }
	return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, FlexShrink: 0, AlignItems: ns.AlignCenter}, Children: []ns.Node{
		jobs,
		ns.Show{When: capsuleShown, Then: func(*ns.Scope) ns.Node { return capsule }},
	}}
}

// moreButton is the ⋯ of a finished film, whose system menu processes it again from the start, as the prototype's offers, or only translates it again, every line or those that failed, or reports a problem with its subtitles. A failed film has 重试 already.
func (s *screen) moreButton(cx *ns.Scope) ns.Node {
	a := s.a
	anchor := &ns.Ref{}
	label := func() string { return locales.FilmMoreActions(cx) }
	btn := glyphButton(cx, svg(iconMore), label, nil, 34, 28, 14, 4, "film-more", func() {
		item := s.item()
		items := []nsdesktop.MenuItem{
			{Label: locales.FilmFeedbackSubtitles(cx), OnSelect: live.Do(a, a.FeedbackSubtitles)},
			{Label: locales.FilmReprocess(cx), OnSelect: live.Do(a, func() { a.RetryOpen(item.JobID) })},
			{Label: locales.FilmTranslateAgain(cx), OnSelect: live.Do(a, a.TranslateFilmAgain)},
		}
		if item.Untranslated > 0 {
			items = append(items, nsdesktop.MenuItem{Label: locales.FilmTranslateMissing(cx), OnSelect: live.Do(a, a.TranslateFilmMissing)})
		}
		b, _ := anchor.Bounds()
		nsdesktop.PopupMenu(cx, items, ns.Point{X: b.Origin.X, Y: b.Origin.Y + b.Size.Height}, nil)
	})
	return ns.View{Ref: anchor, Children: []ns.Node{btn}}
}
