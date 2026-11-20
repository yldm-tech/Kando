package settings

import (
	"context"
	"strconv"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	"kando/apps/mobile/widgets"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// widgetSize is the side of a small widget on an iPhone, and widgetWide the width of a medium one.
const (
	widgetSize = 158
	widgetWide = 322
)

// flame is the color of the streak's flame, as the widget draws it.
var flame = core.RGB(0xFF9500)

// WidgetsPage says how to add the app's widgets, over what each shows now: the streak, the film to go on with, the two together, and the Lock Screen's ring, from what the widgets are given (widgets.Read).
func WidgetsPage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	snap := core.NewMemo(cx, func() widgets.Snapshot { return widgets.Read(s, s.Now()) })
	film := core.NewMemo(cx, func() widgets.Film { return snap.Get().Film })
	// The line the film was left on and its poster are read from the disk, as the widgets read them.
	shown := core.NewResource(cx, func() (widgets.Film, bool) { return film.Get(), film.Get().Key != "" },
		func(_ context.Context, f widgets.Film, _ core.ResourceInfo) (widgets.Shown, error) {
			return widgets.Load(s, widgets.Snapshot{Film: f}), nil
		})
	w := func() widgets.Shown {
		got, _ := shown.Get()
		got.Snapshot = snap.Get()
		return got
	}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsWidgets(cx) }, TestID: "settings-widgets-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
					core.Text{TestID: "settings-widgets-how", ContentFunc: func() string { return locales.SettingsWidgetsHow(cx, locales.AppName(cx)) },
						Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Muted)},
					// The widgets sit on a picture of a Home Screen's wallpaper, as they show on one.
					core.View{TestID: "settings-widgets-wallpaper", Style: core.Style{AlignItems: core.AlignCenter, Gap: 16, Padding: core.Symmetric(22, 18), CornerRadius: 22, BorderCurve: core.BorderCurveContinuous,
						Gradients: []core.Gradient{wallpaper()}}, Children: []core.Node{
						streakPreview(cx, w),
						filmPreview(cx, w),
						widePreview(cx, w),
						lockPreview(cx, w),
					}},
				}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// tile is a widget's preview: a rounded card of the widget's size, the system's background behind its content unless it covers it.
func tile(cx *core.Scope, id string, width core.Points, label func() string, children ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: id, Role: core.RoleImage, LabelFunc: label,
		Style: core.Style{Width: core.Pt(width), Height: core.Pt(widgetSize), CornerRadius: 22, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden, Padding: core.All(14)},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Surface(), Shadows: []core.BoxShadow{{Offset: core.Point{Y: 8}, Blur: 22, Color: core.RGBA(0x000000, 0.18)}}}
		},
		Children: children}
}

// posterSource is the film's poster as the widgets show it: the phone's file, else the one fetched this session, none for neither.
func posterSource(w widgets.Shown) core.ImageSource {
	switch {
	case w.Film.PosterFile != "":
		return core.Src(kcore.FileURL(w.Film.PosterFile))
	case len(w.Poster) > 0:
		return core.ImageSource{Data: w.Poster}
	}
	return core.ImageSource{}
}

// streakPreview is the study widget: the streak under its flame, and what is due.
func streakPreview(cx *core.Scope, w func() widgets.Shown) core.Node {
	colors := ui.ColorsOf(cx)
	today := func() widgets.Day { return w().Days[0] }
	label := func() string {
		return locales.WidgetStreak(cx) + " " + strconv.Itoa(today().Streak) + " " + locales.WidgetDays(cx, today().Streak) + ", " + locales.WidgetDue(cx, today().Due)
	}
	return tile(cx, "settings-widget-streak", widgetSize, label,
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4}, Children: []core.Node{
			ui.IconOf(lucide.Flame, 14, func() core.Color { return flame }),
			core.Text{ContentFunc: func() string { return locales.WidgetStreak(cx) }, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, FontWeight: core.FontWeightBold, Color: theme.Streak}},
		}},
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignEnd, Gap: 3}, Children: []core.Node{
			core.Text{TestID: "settings-widget-streak-days", ContentFunc: func() string { return strconv.Itoa(today().Streak) }, MaxLines: 1,
				Style: core.Style{FontSize: 40, LineHeight: 44, FontWeight: 900, FontVariant: core.FontTabularNums}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return locales.WidgetDays(cx, today().Streak) }, MaxLines: 1,
				Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, Padding: core.Edges{Bottom: 6}}, StyleFunc: ui.Colored(colors.Text)},
		}},
		core.View{Style: core.Style{FlexGrow: 1}},
		core.Text{TestID: "settings-widget-streak-due", ContentFunc: func() string { return locales.WidgetDue(cx, today().Due) }, MaxLines: 1,
			Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Text)},
		core.Text{ContentFunc: func() string { return locales.WidgetTapToReview(cx) }, MaxLines: 1, Style: core.Style{FontSize: 11, LineHeight: 14}, StyleFunc: ui.Colored(colors.Muted)},
	)
}

// filmPreview is the small Continue widget: the film's poster with the line it was left on, else its title, and how far it played on a scrim, or that no film was left partway.
func filmPreview(cx *core.Scope, w func() widgets.Shown) core.Node {
	colors := ui.ColorsOf(cx)
	has := func() bool { return w().Film.Key != "" }
	text := func() string {
		if w().Line != "" {
			return w().Line
		}
		return w().Film.Title
	}
	label := func() string {
		if !has() {
			return locales.WidgetNoFilm(cx)
		}
		return locales.WidgetContinuePercent(cx, w().Film.Percent()) + ", " + w().Film.Title
	}
	white := core.RGB(0xFFFFFF)
	return tile(cx, "settings-widget-film", widgetSize, label, core.Show{When: has,
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), JustifyContent: core.JustifyEnd},
				StyleFunc: func() core.Style { return core.Style{Background: colors.Inset()} },
				Children: []core.Node{
					core.Image{SourceFunc: func() core.ImageSource { return posterSource(w()) }, ResizeMode: core.Cover, ElementsHidden: true,
						Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}},
					// A scrim darkens the poster from 30% of the way down to its foot, where the line is written, as the widget's does.
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Gradients: []core.Gradient{scrim()}}},
					core.View{Style: core.Style{Gap: 4, Padding: core.All(14)}, Children: []core.Node{
						core.Text{ContentFunc: func() string { return locales.WidgetContinue(cx) }, MaxLines: 1, Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightBold, Color: white.Faded(0.85)}},
						core.Text{TestID: "settings-widget-film-text", ContentFunc: text, MaxLines: 2, Style: core.Style{FontSize: 13, LineHeight: 17, FontWeight: core.FontWeightBold, Color: white}},
						progress(func() float32 { return float32(w().Film.Percent()) / 100 }, white),
					}},
				}}
		},
		Else: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{FlexGrow: 1, Gap: 4}, Children: []core.Node{
				ui.Icon(lucide.SquarePlay, 22, colors.Accent),
				core.View{Style: core.Style{FlexGrow: 1}},
				core.Text{TestID: "settings-widget-film-none", ContentFunc: func() string { return locales.WidgetNoFilm(cx) }, MaxLines: 2,
					Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightBold}), StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: func() string { return locales.WidgetNoFilmHint(cx) }, MaxLines: 2, Style: core.Style{FontSize: 11, LineHeight: 14}, StyleFunc: ui.Colored(colors.Muted)},
			}}
		}})
}

// widePreview is the medium Continue widget: the poster beside how far the film played, its title and the line it was left on, the streak and what is due at its foot. It shows while a film was left partway, as the medium widget alone has more to show then.
func widePreview(cx *core.Scope, w func() widgets.Shown) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { return w().Film.Key != "" }, Then: func(cx *core.Scope) core.Node {
		label := func() string { return locales.WidgetContinuePercent(cx, w().Film.Percent()) + ", " + w().Film.Title }
		return tile(cx, "settings-widget-wide", widgetWide, label,
			core.View{Style: core.Style{Direction: core.Row, Gap: 12, FlexGrow: 1}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(120), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden},
					StyleFunc: func() core.Style { return core.Style{Background: colors.Inset()} },
					Children: []core.Node{core.Image{SourceFunc: func() core.ImageSource { return posterSource(w()) }, ResizeMode: core.Cover, ElementsHidden: true,
						Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}}}},
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return locales.WidgetContinuePercent(cx, w().Film.Percent()) }, MaxLines: 1,
						Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Accent)},
					core.Text{ContentFunc: func() string { return w().Film.Title }, MaxLines: 2, Style: core.Style{FontSize: 14, LineHeight: 18, FontWeight: look.Heavy}, StyleFunc: ui.Colored(colors.Text)},
					core.Show{When: func() bool { return w().Line != "" }, Then: func(cx *core.Scope) core.Node {
						return core.Text{ContentFunc: func() string { return w().Line }, MaxLines: 2, Style: core.Style{FontSize: 12, LineHeight: 16, Padding: core.Edges{Top: 4}},
							StyleFunc: func() core.Style { return core.Style{Color: ui.Alpha(colors.Text(), 0.8)} }}
					}},
					core.View{Style: core.Style{FlexGrow: 1}},
					core.Text{TestID: "settings-widget-wide-foot", ContentFunc: func() string { return locales.WidgetFoot(cx, w().Days[0].Due, w().Days[0].Streak) }, MaxLines: 1,
						Style: core.Style{FontSize: 11, LineHeight: 14}, StyleFunc: ui.Colored(colors.Muted)},
				}},
			}})
	}}
}

// lockPreview is the Lock Screen's ring, how many words and lines are due, frosted over the wallpaper as a Lock Screen shows it.
func lockPreview(cx *core.Scope, w func() widgets.Shown) core.Node {
	white := core.RGB(0xFFFFFF)
	due := func() string { return strconv.Itoa(w().Days[0].Due) }
	return core.View{TestID: "settings-widget-lock", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 28},
		Children: []core.Node{
			core.View{Role: core.RoleImage, LabelFunc: func() string { return locales.WidgetDue(cx, w().Days[0].Due) },
				Style: core.Style{Width: core.Pt(72), Height: core.Pt(72), CornerRadius: 36, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: core.RGBA(0xFFFFFF, 0.35),
					BackdropFilters: []core.Filter{core.Blur(10)}},
				Children: []core.Node{
					core.Text{TestID: "settings-widget-lock-due", ContentFunc: due, MaxLines: 1, Style: core.Style{FontSize: 24, LineHeight: 26, FontWeight: look.Heavy, Color: white, FontVariant: core.FontTabularNums}},
					core.Text{ContentFunc: func() string { return locales.WidgetDueShort(cx) }, MaxLines: 1, Style: core.Style{FontSize: 10, LineHeight: 12, FontWeight: core.FontWeightBold, Color: white}},
				}},
			core.Text{ContentFunc: func() string { return locales.SettingsWidgetsLockScreen(cx) }, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, FontWeight: core.FontWeightSemibold, Color: white}},
		}}
}

// scrim is the Continue widget's shade over its poster: clear down to 30% of its height, then darkening to 0.75 black at its foot.
func scrim() core.Gradient {
	g := core.LinearGradient(180, core.RGBA(0x000000, 0), core.RGBA(0x000000, 0.75))
	g.Stops[0].At = core.Pct(30)
	return g
}

// wallpaper is the Home Screen wallpaper the previews sit on, its middle color 60% of the way down, as the design's.
func wallpaper() core.Gradient {
	g := core.LinearGradient(160, theme.WallpaperTop, theme.WallpaperMiddle, theme.WallpaperBottom)
	g.Stops[1].At = core.Pct(60)
	return g
}

// progress is a thin bar filled to the share value reports, in ink.
func progress(value func() float32, ink core.Color) core.Node {
	return core.View{Style: core.Style{Height: core.Pt(3), CornerRadius: 2, Background: core.RGBA(0xFFFFFF, 0.3), Overflow: core.Hidden}, Children: []core.Node{
		core.View{Style: core.Style{Height: core.Pt(3), Background: ink}, StyleFunc: func() core.Style { return core.Style{Width: core.Pct(100 * value())} }},
	}}
}
