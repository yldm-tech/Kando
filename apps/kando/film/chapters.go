package film

import (
	"slices"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The 片段 of a link's film can be the chapters its site marks (core.Chapter), the timestamps of a YouTube video's description or a bilibili video's 视频看点, which its job kept beside the video (Info.Chapters), in place of the even cuts at its lines (Chapters). Whether a film is cut at them is the user's choice for that film (Info.ChapterUse), else the setting 导入平台章节 (App.ImportChapters).

// CutsAtSite reports whether the film whose folder info describes is cut at its site's chapters: it has them, and the user chose them for it, or chose nothing for it while importOn, the setting, is on.
func CutsAtSite(info Info, importOn bool) bool {
	if len(info.Chapters) < 2 {
		return false
	}
	return info.ChapterUse == core.ChaptersPlatform || (info.ChapterUse == "" && importOn)
}

// ChaptersFor is the 片段 the film is cut into: its site's chapters when it is cut at them (CutsAtSite), else the even cuts at its lines (Chapters). delay is the subtitle delay, which a site's chapter, in the video's time, is moved back by, as a 片段's times are the cues'.
func ChaptersFor(info Info, importOn bool, cues []core.Segment, length, delay time.Duration) []Chapter {
	if CutsAtSite(info, importOn) {
		return SiteChapters(info.Chapters, length, delay)
	}
	return Chapters(cues, length)
}

// SiteChapters is the 片段 of a site's chapters, each to the next one's start and the last to the video's end, length, or its own start when the length is not known; at the cues' time, delay before the video's. Each is named by its title's translation, with the title as the site wrote it as its detail, or by that title alone while it has no translation.
func SiteChapters(chapters []core.Chapter, length, delay time.Duration) []Chapter {
	out := make([]Chapter, 0, len(chapters))
	for i, c := range chapters {
		end := max(length, c.Start)
		if i+1 < len(chapters) {
			end = chapters[i+1].Start
		}
		title, detail := c.Translation, c.Title
		if title == "" {
			title, detail = c.Title, ""
		}
		out = append(out, Chapter{Start: c.Start - delay, End: end - delay, Title: shorten(title, 36), Detail: shorten(detail, 48)})
	}
	return out
}

// chapters is the 片段 the open film is cut into (ChaptersFor), which the clips tab lists, kept while they stay the same.
func (s *screen) chapters(cx *ns.Scope) func() []Chapter {
	a := s.a
	cues := filmCues(cx, a)
	return ns.NewMemoFunc(cx, func() []Chapter {
		info, importOn, length, delay := live.Func(a, a.Info)(), live.Func(a, a.ImportChapters)(), live.Func(a, a.PlayLength)(), live.Func(a, a.Delay)()
		return ChaptersFor(info, importOn, cues(), length, delay)
	}, slices.Equal[[]Chapter]).Get
}

// chapterSite is the site a film's chapters came from, as the catalog's select names it: youtube, bilibili, or other.
func chapterSite(source string) string {
	link, _ := core.ParseLink(source)
	switch link.Platform {
	case core.PlatformYouTube, core.PlatformBilibili:
		return string(link.Platform)
	}
	return "other"
}

// chaptersTranslated reports whether any of chapters has its title translated.
func chaptersTranslated(chapters []core.Chapter) bool {
	return slices.ContainsFunc(chapters, func(c core.Chapter) bool { return c.Translation != "" })
}

// chapterNote is the clips tab's note over its 片段 for a film whose site marks chapters: how many and where from, titles translated or not, while it is cut at them, else that it is cut evenly by time.
func chapterNote(cx *ns.Scope, info Info, importOn bool, source string) string {
	if !CutsAtSite(info, importOn) {
		return locales.FilmClipsEven(cx)
	}
	site := locales.FilmChapterSite(cx, chapterSite(source))
	if chaptersTranslated(info.Chapters) {
		return locales.FilmClipsChaptersTranslated(cx, len(info.Chapters), site)
	}
	return locales.FilmClipsChapters(cx, len(info.Chapters), site)
}

// chaptersHeader is the row over the clips tab's 片段 of a film whose site marks chapters: chapterNote, and a button that cuts it the other way, which the film keeps. A film without chapters has none.
func (s *screen) chaptersHeader(cx *ns.Scope) ns.Node {
	a := s.a
	has := live.Func(a, func() bool { return len(a.Info().Chapters) >= 2 })
	return ns.Show{When: has, Then: func(cx *ns.Scope) ns.Node {
		atSite := live.Func(a, func() bool { return CutsAtSite(a.Info(), a.ImportChapters()) })
		return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, AlignItems: ns.AlignCenter}, Children: []ns.Node{
			ns.Text{TestID: "film-clips-note", ContentFunc: live.Func(a, func() string { return chapterNote(cx, a.Info(), a.ImportChapters(), s.item().Source) }),
				Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.7)}},
			// The pill of the prototype's clips tab, which follows the cut the film is in without being built again, so it keeps the keyboard focus after a press.
			base.Button{TestID: "film-clips-cut",
				LabelFunc: func() string { return cutLabel(cx, atSite()) },
				OnPress: live.On(a, func(ns.PressEvent) {
					use := core.ChaptersPlatform
					if atSite() {
						use = core.ChaptersEven
					}
					a.SetFilmChapterUse(use)
				}),
				Style: ns.Style{FlexShrink: 0, Height: ns.Pt(22), Padding: ns.Symmetric(0, 9), CornerRadius: 11, Background: ns.RGBA(0xFFFFFF, 0.12), JustifyContent: ns.JustifyCenter,
					Hover: &ns.Style{Background: ns.RGBA(0xFFFFFF, 0.2)}, FocusVisible: ui.FocusRing(cx, 2)},
				Children: []ns.Node{ns.Text{ContentFunc: func() string { return cutLabel(cx, atSite()) }, Style: ns.Style{FontSize: 11, LineHeight: 14, Color: onGlass}}}},
		}}
	}}
}

// cutLabel is the button that cuts the film the other way: by time while it is cut at its site's chapters (atSite), else at them.
func cutLabel(cx *ns.Scope, atSite bool) string {
	if atSite {
		return locales.FilmClipsUseEven(cx)
	}
	return locales.FilmClipsUseChapters(cx)
}

// chapterInfo is what the info tab says of a film's count chapters: how many and where from, and that it is cut evenly by time instead, unless cut; "" for a film without chapters.
func chapterInfo(cx *ns.Scope, count int, cut bool, source string) string {
	if count < 2 {
		return ""
	}
	site := locales.FilmChapterSite(cx, chapterSite(source))
	if cut {
		return locales.FilmChaptersInfo(cx, count, site)
	}
	return locales.FilmChaptersInfoEven(cx, count, site)
}
