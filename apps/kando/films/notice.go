package films

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// noticeLook is the message bar's state, which its parts follow: whether it shows, what kind it is, what it says, and its buttons.
type noticeLook struct {
	open          bool
	success       bool
	text, section string
	// film is the film the bar's 打开 opens: the film it names, while it is still in the library.
	film Film
	// list is the playlist the bar's Add Whole Playlist adds.
	list string
	// translate is whether the bar offers Translate (Notice.Translate).
	translate bool
	// library is the library the bar's 打开 <library> opens, and libraryName its name; undo and stop are whether it offers Undo and 取消 (Notice.Undo, Notice.Stop).
	library, libraryName string
	undo, stop           bool
}

// notice is the message bar under the top bar, as the prototype draws it: amber with a warning triangle for what went wrong, green with a check mark for what worked, 打开设置 when a setting fixes what it says, 打开 when it names a film in the library, Add Whole Playlist when the video it added is one of a playlist's, Translate when it says films have no translation, 打开 <library> and Undo after a film was moved to another library, 取消 while one is being copied there, and × to close it.
type notice struct{ a App }

// Render implements core.Node.
func (n notice) Render(cx *core.Scope) core.Node {
	a := n.a
	look := live.Memo(cx, a, func() noticeLook {
		bar := a.Notice()
		l := noticeLook{open: bar.Open, success: bar.Kind == "success", text: bar.Text, section: bar.Section, list: bar.List, translate: bar.Translate, library: bar.Library, undo: bar.Undo, stop: bar.Stop}
		if bar.Library != "" {
			l.libraryName = LibraryName(bar.Library)
		}
		// The film named may have been removed since the bar opened; then there is nothing to open and no button. It is a film of the library's, a job's or a video of its folder no job holds, or a job's the queue runs for another library.
		if bar.Film != "" {
			var ok bool
			if l.film, ok = Find(a.Films(), bar.Film); !ok {
				l.film, _ = a.JobFilm(bar.Film)
			}
		}
		return l
	})
	return core.Show{When: func() bool { return look().open }, Then: func(cx *core.Scope) core.Node { return noticeBar(cx, a, look) }}
}

// noticeBar is the message bar in the theme's colors for it (theme.Of): its fill, hairline and ink, amber for a warning and green for news that something worked.
func noticeBar(cx *core.Scope, a App, look func() noticeLook) core.Node {
	k := theme.Of(cx)
	ink := func() core.Color {
		if look().success {
			return k.OKText()
		}
		return k.WarnText()
	}
	mark := core.SVGView{Style: core.Style{Width: core.Pt(16), Height: core.Pt(16), FlexShrink: 0}, SVGFunc: func() *core.SVG {
		if look().success {
			return okMark
		}
		return warningMark
	}}
	text := core.Text{ContentFunc: func() string { return look().text }, Style: core.Style{FontSize: 13, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
		StyleFunc: func() core.Style { return core.Style{Color: ink()} }}
	settings := core.Show{When: func() bool { return look().section != "" }, Then: func(*core.Scope) core.Node {
		return textButton(locales.FilmsOpenSettings, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "notice-settings",
			OnPress: live.Do(a, func() { a.OpenSettings(look().section) })})
	}}
	open := core.Show{When: func() bool { return look().film.Key != "" }, Then: func(*core.Scope) core.Node {
		return textButton(locales.FilmsNoticeOpen, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "notice-film",
			OnPress: live.Do(a, func() {
				item := look().film
				a.Notice().Open = false
				a.Open(item)
			})})
	}}
	list := core.Show{When: func() bool { return look().list != "" }, Then: func(*core.Scope) core.Node {
		return textButton(locales.PlaylistAddWhole, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "notice-list",
			OnPress: live.Do(a, func() { a.AddList(look().list) })})
	}}
	translate := core.Show{When: func() bool { return look().translate }, Then: func(*core.Scope) core.Node {
		return textButton(locales.FilmsTranslate, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "notice-translate",
			OnPress: live.Do(a, func() {
				a.Notice().Open = false
				a.TranslateFilms(NotTranslated(a.Films()))
			})})
	}}
	library := core.Show{When: func() bool { return look().library != "" }, Then: func(*core.Scope) core.Node {
		label := func(cx *core.Scope) string { return locales.FilmsNoticeOpenLibrary(cx, look().libraryName) }
		return textButton(label, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "notice-library",
			OnPress: live.Do(a, func() {
				dir := look().library
				a.Notice().Open = false
				a.Choose(dir)
			})})
	}}
	undo := core.Show{When: func() bool { return look().undo }, Then: func(*core.Scope) core.Node {
		return textButton(locales.FilmsNoticeUndo, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "notice-undo", OnPress: live.Do(a, a.UndoMove)})
	}}
	stop := core.Show{When: func() bool { return look().stop }, Then: func(*core.Scope) core.Node {
		return textButton(locales.FilmsNoticeStop, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "notice-stop", OnPress: live.Do(a, a.CancelMove)})
	}}
	close := base.Button{TestID: "notice-close", LabelFunc: func() string { return locales.FilmsCloseNotice(cx) },
		OnPress: live.On(a, func(core.PressEvent) { a.Notice().Open = false }),
		Style: core.Style{Width: core.Pt(24), Height: core.Pt(24), CornerRadius: 12, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Cursor: core.CursorDefault, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() core.Style { return core.Style{Hover: &core.Style{Background: k.Shade(0.06)}} },
		Children:  []core.Node{core.Text{Content: "×", Style: core.Style{FontSize: 15}, StyleFunc: func() core.Style { return core.Style{Color: ink()} }}}}
	// The bar is 42 points high with one line of text, its hairline included.
	return core.View{
		TestID:    "films-notice",
		Role:      core.RoleStatus,
		LabelFunc: func() string { return look().text },
		Style: core.Style{Direction: core.Row, MinHeight: core.Pt(42), Margin: core.Edges{Bottom: 14}, Padding: core.Edges{Top: 8.5, Right: 10, Bottom: 8.5, Left: 14}, Gap: 10,
			AlignItems: core.AlignCenter, CornerRadius: 14, BorderWidth: 0.5},
		StyleFunc: func() core.Style {
			if look().success {
				return core.Style{Background: k.OKFill(), BorderColor: k.OKRing()}
			}
			return core.Style{Background: k.WarnFill(), BorderColor: k.WarnRing()}
		},
		Children: []core.Node{mark, text, settings, open, list, translate, library, undo, stop, close},
	}
}
