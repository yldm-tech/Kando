package settings

import (
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// PhoneStats is how much of the open library's learning state phones were last told: the words, saved lines and films with a place to continue from. Shown is false while there is no open library whose state phones were told, as before any call since Phone Sync started.
type PhoneStats struct {
	Shown                   bool
	Words, Lines, Positions int
}

// PhoneConflict is a sync conflict of the open library: one field changed on two devices apart, of which the later change was kept and the other set aside, until the user or a phone settles it.
type PhoneConflict struct {
	ID string
	// Field is what was changed twice: FieldOriginal, FieldTranslation, FieldNote or FieldMeaning.
	Field string
	// Film is the title of the film it belongs to, "" for none, and At where in the film the line is, Word the word it is of.
	Film string
	At   time.Duration
	Word string
	// Line is the line's original, shown over the two versions when they are not of it, "" for none.
	Line string
	// Kept is the version the sync kept, Lost the one it set aside.
	Kept, Lost PhoneVersion
	// Both is whether the two can be kept together, as two notes can.
	Both bool
}

// The fields a PhoneConflict is of (psync's names).
const (
	FieldOriginal    = "original"
	FieldTranslation = "translation"
	FieldNote        = "note"
	FieldMeaning     = "meaning"
)

// PhoneVersion is one of a conflict's versions: the device it was made on, "" for this computer, when, and its text.
type PhoneVersion struct {
	Device string
	At     time.Time
	Text   string
}

// PhoneKeep is which version of a conflict ResolvePhoneConflict keeps.
type PhoneKeep uint8

const (
	KeepKept PhoneKeep = iota
	KeepLost
	KeepBoth
)

// PhoneActivity is something a paired phone, or this computer for it, did lately: Kind says what, of the film or link Title, with Count things or at Position where that says.
type PhoneActivity struct {
	At       time.Time
	Device   string
	Kind     string
	Title    string
	Count    int
	Position time.Duration
}

// The kinds of PhoneActivity.
const (
	ActivityLink      = "link"
	ActivityPaused    = "paused"
	ActivityResumed   = "resumed"
	ActivityCanceled  = "canceled"
	ActivityRetried   = "retried"
	ActivityYTDLP     = "ytdlp"
	ActivityPosition  = "position"
	ActivityLines     = "lines"
	ActivityWords     = "words"
	ActivityEdits     = "edits"
	ActivityResolved  = "resolved"
	ActivityPaired    = "paired"
	ActivityRevoked   = "revoked"
	ActivityReadOnly  = "read-only"
	ActivityReadWrite = "read-write"
)

// learning is the page's part of what the phones sync with this computer: how much of the open library's learning state they were told, the sync conflicts left to settle with how many there are beside their label, and what the phones did lately.
func learning(cx *core.Scope, a App) []core.Node {
	stats := core.Show{When: live.Func(a, func() bool { return a.PhoneSync().Stats.Shown }), Then: func(cx *core.Scope) core.Node {
		return core.Group{
			form.Label(locales.SettingsPhoneLearning(cx), 14),
			form.Card(form.Row{Title: locales.SettingsPhoneSynced(cx), NoGroup: true, TestID: "settings-phone-stats",
				Trailing: []core.Node{form.Value(live.Func(a, func() string {
					st := a.PhoneSync().Stats
					return locales.SettingsPhoneSyncedCounts(cx, st.Words, st.Lines, st.Positions)
				}), 12)}}),
		}
	}}
	count := live.Func(a, func() int { return len(a.PhoneSync().Conflicts) })
	badge := core.Show{When: func() bool { return count() > 0 }, Then: func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: func() string { return strconv.Itoa(count()) }, TestID: "settings-phone-conflict-count",
			Style: core.Style{FontSize: 10, FontWeight: ui.Bold, LineHeight: 16, MinWidth: core.Pt(16), TextAlign: core.TextAlignCenter, CornerRadius: 8, Padding: core.Symmetric(0, 5)},
			// The palette has no ink for a danger fill; the one for an accent fill is the same white.
			StyleFunc: func() core.Style {
				colors := ui.ColorsOf(cx)
				return core.Style{Background: colors.Danger(), Color: colors.OnAccent()}
			}}
	}}
	return []core.Node{
		stats,
		form.Label(locales.SettingsPhoneConflicts(cx), 14, badge),
		form.Card(conflicts{a: a}),
		phoneError(cx, live.Func(a, func() string { return a.PhoneSync().ResolveErr }), locales.SettingsPhoneResolveFailed),
		form.Note(func() string { return locales.SettingsPhoneConflictsNote(cx) }, 0),
		form.Label(locales.SettingsPhoneActivity(cx), 14),
		form.Card(activity{a: a}),
	}
}

// conflicts is the card of the open library's sync conflicts, each with its two versions and 用这个 under each, and 两条都留 when both can be kept; or a line saying there is none.
type conflicts struct{ a App }

// Render implements core.Node.
func (c conflicts) Render(cx *core.Scope) core.Node {
	a := c.a
	// The conflicts are drawn again only when one comes or goes; a conflict does not change while it is open.
	list := core.NewMemoFunc(cx, live.Func(a, func() []PhoneConflict { return a.PhoneSync().Conflicts }), func(x, y []PhoneConflict) bool {
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if x[i].ID != y[i].ID {
				return false
			}
		}
		return true
	})
	return core.Dynamic(func(cx *core.Scope) core.Node {
		items := list.Get()
		if len(items) == 0 {
			return form.Row{Title: locales.SettingsPhoneNoConflicts(cx), NoGroup: true, TestID: "settings-phone-no-conflicts"}
		}
		var rows []core.Node
		for i, item := range items {
			if i > 0 {
				rows = append(rows, form.Separator(false))
			}
			rows = append(rows, conflictRow(cx, a, item))
		}
		return core.Group(rows)
	})
}

// conflictRow is conflict c: what was changed twice, of which film and where, the line it is of, and its two versions side by side, each with where and when it was made and 用这个.
func conflictRow(cx *core.Scope, a App, c PhoneConflict) core.Node {
	colors := ui.ColorsOf(cx)
	k := theme.Of(cx)
	where := []string{conflictField(cx, c.Field)}
	if c.Film != "" {
		where = append(where, locales.SettingsPhoneConflictFilm(cx, c.Film))
	}
	switch {
	case c.Word != "":
		where = append(where, locales.SettingsPhoneConflictWord(cx, c.Word))
	case c.At >= 0:
		where = append(where, clock(c.At))
	}
	head := core.Text{Content: strings.Join(where, " · "), Style: core.Style{FontSize: 12, LineHeight: 17, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)}
	children := []core.Node{head}
	if c.Line != "" {
		children = append(children, core.Text{Content: c.Line, MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)})
	}
	version := func(v PhoneVersion, keep PhoneKeep, testID string) core.Node {
		device := v.Device
		if device == "" {
			device = locales.SettingsPhoneThisComputer(cx)
		}
		return core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Gap: 6, Padding: core.Symmetric(9, 10), CornerRadius: 10}, StyleFunc: ui.Filled(k.Tile),
			Children: []core.Node{
				core.Text{ContentFunc: live.Func(a, func() string { return device + " · " + ago(cx, v.At, a.PhoneNow()) }), Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Muted)},
				core.Text{Content: v.Text, Selectable: core.SelectableOn, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Text)},
				core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
					form.Button{Text: locales.SettingsPhoneConflictUse(cx), Label: locales.SettingsPhoneConflictUseLabel(cx, device), TestID: testID,
						OnPress: live.Do(a, func() { a.ResolvePhoneConflict(c.ID, keep) })},
				}},
			}}
	}
	children = append(children, core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignStretch}, Children: []core.Node{
		version(c.Kept, KeepKept, "settings-phone-conflict-kept-"+c.ID),
		version(c.Lost, KeepLost, "settings-phone-conflict-lost-"+c.ID),
	}})
	if c.Both {
		children = append(children, core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
			form.Button{Text: locales.SettingsPhoneConflictBoth(cx), TestID: "settings-phone-conflict-both-" + c.ID, OnPress: live.Do(a, func() { a.ResolvePhoneConflict(c.ID, KeepBoth) })},
		}})
	}
	return core.View{Role: core.RoleGroup, Label: strings.Join(where, " · "), TestID: "settings-phone-conflict-" + c.ID,
		Style: core.Style{FlexShrink: 0, Gap: 8, Padding: core.Symmetric(12, 14)}, Children: children}
}

// conflictField is what a conflict's field is called on the page.
func conflictField(cx *core.Scope, field string) string {
	switch field {
	case FieldOriginal:
		return locales.SettingsPhoneConflictOriginal(cx)
	case FieldTranslation:
		return locales.SettingsPhoneConflictTranslation(cx)
	case FieldMeaning:
		return locales.SettingsPhoneConflictMeaning(cx)
	}
	return locales.SettingsPhoneConflictNote(cx)
}

// activity is the card of what the phones did lately, the latest first, each with when, which device and what; or a line saying they did nothing yet.
type activity struct{ a App }

// Render implements core.Node.
func (v activity) Render(cx *core.Scope) core.Node {
	a := v.a
	list := core.NewMemoFunc(cx, live.Func(a, func() []PhoneActivity { return a.PhoneSync().Activity }), func(x, y []PhoneActivity) bool {
		return len(x) == len(y) && (len(x) == 0 || x[0] == y[0])
	})
	return core.Dynamic(func(cx *core.Scope) core.Node {
		items := list.Get()
		if len(items) == 0 {
			return form.Row{Title: locales.SettingsPhoneNoActivity(cx), NoGroup: true, TestID: "settings-phone-no-activity"}
		}
		colors := ui.ColorsOf(cx)
		var rows []core.Node
		for i, item := range items {
			if i > 0 {
				rows = append(rows, form.Separator(false))
			}
			device := item.Device
			if device == "" {
				device = locales.SettingsPhoneThisComputer(cx)
			}
			rows = append(rows, core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(36), Padding: core.Symmetric(6, 14), Gap: 10, AlignItems: core.AlignCenter},
				Children: []core.Node{
					core.Text{ContentFunc: live.Func(a, func() string { return ago(cx, item.At, a.PhoneNow()) }), MaxLines: 1, Style: core.Style{FontSize: 11, Width: core.Pt(72), FlexShrink: 0}, StyleFunc: ui.Colored(colors.Muted)},
					core.Text{Content: device, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 12, FontWeight: ui.Semibold, MaxWidth: core.Pt(140), FlexShrink: 0}, StyleFunc: ui.Colored(colors.Text)},
					core.Text{Content: activityText(cx, item), MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 12, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Text)},
				}})
		}
		return core.Group(rows)
	})
}

// activityText is what item says the device did, in the interface's language.
func activityText(cx *core.Scope, item PhoneActivity) string {
	switch item.Kind {
	case ActivityLink:
		return locales.SettingsPhoneDidLink(cx, item.Title)
	case ActivityPaused:
		return locales.SettingsPhoneDidPause(cx, item.Title)
	case ActivityResumed:
		return locales.SettingsPhoneDidResume(cx, item.Title)
	case ActivityCanceled:
		return locales.SettingsPhoneDidCancel(cx, item.Title)
	case ActivityRetried:
		return locales.SettingsPhoneDidRetry(cx, item.Title)
	case ActivityYTDLP:
		return locales.SettingsPhoneDidUpdateYTDLP(cx)
	case ActivityPosition:
		return locales.SettingsPhoneDidWatch(cx, item.Title, clock(item.Position))
	case ActivityLines:
		return locales.SettingsPhoneDidLines(cx, item.Count)
	case ActivityWords:
		return locales.SettingsPhoneDidWords(cx, item.Count)
	case ActivityEdits:
		return locales.SettingsPhoneDidEdits(cx, item.Title, item.Count)
	case ActivityResolved:
		return locales.SettingsPhoneDidResolve(cx, item.Count)
	case ActivityPaired:
		return locales.SettingsPhoneDidPair(cx)
	case ActivityRevoked:
		return locales.SettingsPhoneDidRevoke(cx, item.Title)
	case ActivityReadOnly:
		return locales.SettingsPhoneDidReadOnly(cx, item.Title)
	case ActivityReadWrite:
		return locales.SettingsPhoneDidReadWrite(cx, item.Title)
	}
	return item.Kind
}

// ago is how long before now at was, in the interface's language: just now, minutes, hours, or days.
func ago(cx *core.Scope, at, now time.Time) string {
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return locales.PhoneSyncAgoNow(cx)
	case d < time.Hour:
		return locales.PhoneSyncAgoMinutes(cx, int(d/time.Minute))
	case d < 24*time.Hour:
		return locales.PhoneSyncAgoHours(cx, int(d/time.Hour))
	}
	return locales.PhoneSyncAgoDays(cx, int(d/(24*time.Hour)))
}

// clock is d as a film's clock: m:ss, or h:mm:ss from an hour on.
func clock(d time.Duration) string {
	s := int(d / time.Second)
	if s >= 3600 {
		return strconv.Itoa(s/3600) + ":" + pad2(s/60%60) + ":" + pad2(s%60)
	}
	return strconv.Itoa(s/60) + ":" + pad2(s%60)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
