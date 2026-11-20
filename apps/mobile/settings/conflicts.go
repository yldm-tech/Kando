package settings

import (
	"strings"
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/study"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// ConflictsPage is Settings › Sync Conflicts: what two devices changed apart, a translation, a note or a meaning, each with its two versions, where and when each was made and Use This beside it, and Keep Both where both can be kept; or that none is left. A version kept goes to every device through the computer, and the conflict leaves the list once the computer took it.
func ConflictsPage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	failure := func() string {
		f := s.ResolveFailed()
		switch {
		case f.Computer == "":
			return ""
		case f.ReadOnly:
			return locales.SettingsConflictReadOnly(cx, f.Computer)
		case f.Problem == session.Unpaired || f.Problem == session.Certificate:
			return locales.SettingsConflictUnpaired(cx, f.Computer)
		}
		return locales.SettingsConflictUnreachable(cx, f.Computer)
	}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsConflicts(cx) }, TestID: "conflicts-page", Children: []core.Node{
		core.Scroll{TestID: "conflicts-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.SettingsConflictsNote(cx) }, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Muted)},
				core.Show{When: func() bool { return failure() != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "conflicts-failure", ContentFunc: failure, Role: core.RoleAlert, Style: look.Subhead.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Danger)}
				}},
				core.Each(s.Conflicts, func(c session.SyncConflict) string { return c.Key() }, func(cx *core.Scope, c func() session.SyncConflict, _ func() int) core.Node {
					return conflictCard(cx, s, c)
				}),
				core.Show{When: func() bool { return len(s.Conflicts()) == 0 }, Then: func(cx *core.Scope) core.Node { return conflictsDone(cx) }},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// conflictCard is conflict c: what was changed twice, of which film, where in it, the line it is of, and its two versions, each with Use This, and Keep Both where both can be kept. While it is being resolved its choices take no presses.
func conflictCard(cx *core.Scope, s *session.Session, c func() session.SyncConflict) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	id := c().Key()
	busy := func() bool { return s.Resolving(c()) }
	children := []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
			core.Text{TestID: "conflict-kind-" + id, ContentFunc: func() string { return conflictKind(cx, c().Field) }, MaxLines: 1,
				Style:     core.Style{FontSize: 12, FontWeight: ui.Bold, LineHeight: 20, Padding: core.Symmetric(0, 7), CornerRadius: 5, FlexShrink: 0},
				StyleFunc: func() core.Style { return core.Style{Background: tints.Warn(), Color: theme.Of(cx).WarnText()} }},
			core.Text{TestID: "conflict-film-" + id, ContentFunc: func() string { return c().Title }, MaxLines: 1, EllipsizeMode: core.EllipsizeTail,
				Style: look.Subhead.Merge(core.Style{FontWeight: ui.Semibold, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Text)},
		}},
		core.Show{When: func() bool { return conflictWhere(cx, c().Conflict) != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "conflict-where-" + id, ContentFunc: func() string { return conflictWhere(cx, c().Conflict) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}
		}},
		core.Show{When: func() bool { return c().Line != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "conflict-line-" + id, ContentFunc: func() string { return c().Line }, Style: look.Subhead.Merge(core.Style{Margin: core.Edges{Top: 4}}), StyleFunc: ui.Colored(colors.Text)}
		}},
		conflictVersion(cx, s, c, func(x session.SyncConflict) study.Version { return x.Kept }, session.KeepKept, "conflict-kept-"+id, busy),
		conflictVersion(cx, s, c, func(x session.SyncConflict) study.Version { return x.Lost }, session.KeepLost, "conflict-lost-"+id, busy),
		core.Show{When: func() bool { return c().Both }, Then: func(cx *core.Scope) core.Node {
			return core.View{TestID: "conflict-both-" + id, Role: core.RoleButton, LabelFunc: func() string { return locales.SettingsConflictBoth(cx) },
				Disabled: busy, OnPress: func(core.PressEvent) { s.ResolveConflict(c(), session.KeepBoth) },
				Style: core.Style{Height: core.Pt(40), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Active: &core.Style{Background: colors.Pressed()}},
				Children: []core.Node{core.Text{ContentFunc: func() string { return locales.SettingsConflictBoth(cx) }, Style: look.Subhead.Merge(core.Style{FontWeight: ui.Semibold}), StyleFunc: ui.Colored(colors.Accent)}}}
		}},
	}
	return core.View{TestID: "conflict-" + id, Style: core.Style{Gap: 4, Padding: core.Symmetric(14, 16), CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style {
			st := core.Style{Background: colors.Surface()}
			if busy() {
				st.Opacity = core.Alpha(0.6)
			}
			return st
		},
		Children: children}
}

// conflictVersion is a version of conflict c, the one of picks: the device it was made on and when, its text, and Use This, which keeps it.
func conflictVersion(cx *core.Scope, s *session.Session, c func() session.SyncConflict, of func(session.SyncConflict) study.Version, keep session.Keep, testID string, busy func() bool) core.Node {
	colors := ui.ColorsOf(cx)
	device := func() string { return versionDevice(cx, of(c())) }
	meta := func() string { return device() + " · " + versionWhen(cx, of(c()).At, s.Now()) }
	return core.View{TestID: testID, Role: core.RoleButton, Disabled: busy,
		LabelFunc: func() string { return locales.SettingsConflictUseLabel(cx, device()) + ", " + of(c()).Text },
		OnPress:   func(core.PressEvent) { s.ResolveConflict(c(), keep) },
		Style:     core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Margin: core.Edges{Top: 4}, Padding: core.Symmetric(10, 12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style {
			return core.Style{Background: ui.Alpha(colors.Text(), 0.06), Active: &core.Style{Background: colors.Pressed()}}
		},
		Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{TestID: testID + "-meta", ContentFunc: meta, MaxLines: 1, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
				core.Text{TestID: testID + "-text", ContentFunc: func() string { return of(c()).Text }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Text)},
			}},
			core.Text{ContentFunc: func() string { return locales.SettingsConflictUse(cx) }, MaxLines: 1, Style: look.Subhead.Merge(core.Style{FontWeight: ui.Semibold, FlexShrink: 0}), StyleFunc: ui.Colored(colors.Accent)},
		}}
}

// conflictsDone is what the page shows once no conflict is left.
func conflictsDone(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "conflicts-done", Style: core.Style{AlignItems: core.AlignCenter, Gap: 6, Padding: core.Symmetric(28, 16), CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} },
		Children: []core.Node{
			ui.Icon(lucide.CircleCheck, 30, colors.Up),
			core.Text{ContentFunc: func() string { return locales.SettingsConflictsDone(cx) }, Role: core.RoleHeader, Style: look.Title.Merge(core.Style{FontWeight: ui.Bold, TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return locales.SettingsConflictsDoneDetail(cx) }, Style: look.Subhead.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
		}}
}

// conflictKind is the badge of a conflict of field.
func conflictKind(cx *core.Scope, field string) string {
	switch field {
	case sync.FieldOriginal:
		return locales.SettingsConflictOriginal(cx)
	case sync.FieldNote:
		return locales.SettingsConflictNote(cx)
	case sync.FieldMeaning:
		return locales.SettingsConflictMeaning(cx)
	}
	return locales.SettingsConflictTranslation(cx)
}

// conflictWhere is where in its film c is: the word it is of, or the time of the line, "" for neither.
func conflictWhere(cx *core.Scope, c study.Conflict) string {
	var parts []string
	switch {
	case c.Word != "":
		parts = append(parts, locales.SettingsConflictWord(cx, c.Word))
	case c.At >= 0:
		parts = append(parts, look.Duration(c.At))
	}
	return strings.Join(parts, " · ")
}

// versionDevice is the device v was made on as the page names it: this phone or tablet, the device the computer names, or another device.
func versionDevice(cx *core.Scope, v study.Version) string {
	switch {
	case v.Mine && look.Tablet(cx):
		return locales.SettingsConflictThisTablet(cx)
	case v.Mine:
		return locales.SettingsConflictThisPhone(cx)
	case v.Device != "":
		return v.Device
	}
	return locales.SettingsConflictOtherDevice(cx)
}

// versionWhen is when a version was made, at, as the page says it against now: today's or yesterday's time, else the day and time.
func versionWhen(cx *core.Scope, at, now time.Time) string {
	at, now = at.Local(), now.Local()
	day := func(t time.Time) time.Time { y, m, d := t.Date(); return time.Date(y, m, d, 0, 0, 0, 0, time.Local) }
	switch day(at) {
	case day(now):
		return locales.SettingsConflictToday(cx, at)
	case day(now).AddDate(0, 0, -1):
		return locales.SettingsConflictYesterday(cx, at)
	}
	return locales.SettingsConflictDate(cx, at)
}
