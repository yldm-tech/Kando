// Package study draws the learner's saved lines across the library: the sheet that lists them (Sheet) and the review that goes through the ones due (Review).
package study

import (
	"slices"
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/saved"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// App is what the sheet reads of the saved lines and the clicks it reports. The sheet reads it inside its bindings through live.Func and calls its actions through live.Do, so what they change shows at once (package live).
type App interface {
	live.State
	// SavedEntries is the library's saved lines, newest first, as the sheet lists them.
	SavedEntries() []Entry
	// OpenSaved closes the sheet and plays the film of the saved line id from the line; RemoveSaved takes it out of the saved lines.
	OpenSaved(id string)
	RemoveSaved(id string)
	// CloseSaved closes the sheet.
	CloseSaved()
	// ExportSaved exports the saved lines for Anki into a folder the user picks; ExportState is how the export goes, StopExport stops the one running, and RevealExport shows its notes' file in the file manager. ExportSettings closes the sheet and opens the settings that install ffmpeg, which the export cuts the lines' audio with.
	ExportSaved()
	ExportState() Export
	StopExport()
	RevealExport()
	ExportSettings()
	// DueCount is how many saved lines are due for review; StartReview closes the sheet and reviews them (Review).
	DueCount() int
	StartReview()
}

// Export is how the export of the saved lines goes: Busy while it runs, Done lines of Total; once Finished, the Lines and the Sounds it wrote, or Err, why it failed, or Stopped once the user stopped it. NoFFmpeg reports that ffmpeg is not installed, so no line has its audio.
type Export struct {
	Busy, Finished bool
	Done, Total    int
	Lines, Sounds  int
	Err            string
	Stopped        bool
	NoFFmpeg       bool
}

// Entry is a saved line as the sheet lists it.
type Entry struct {
	// ID names the line for OpenSaved and RemoveSaved, and keeps its row as lines come and go.
	ID                          string
	Film, Original, Translation string
	Start                       time.Duration
	// Gone reports that the line's video is not in the library now, so it does not play.
	Gone bool
}

// The sheet's measures.
const (
	sheetWidth  core.Points = 560
	sheetHeight core.Points = 600
)

// Sheet lists the saved lines of the library, newest first: each with its film and where in it the line is, its original and its translation. A click plays the film from the line, the × beside it removes it, and Done, Escape and closing the sheet run CloseSaved. Export for Anki… under the list exports them, Cancel in its place stops an export running, and the line beside it says how the export goes and what it wrote (exportStatus), with Open Settings after an export that had no ffmpeg to cut the audio with; Review starts the review of the lines due, with how many are.
type Sheet struct {
	App App
}

// Render implements core.Node.
func (s Sheet) Render(cx *core.Scope) core.Node {
	a := s.App
	colors := ui.ColorsOf(cx)
	entries := core.NewMemoFunc(cx, live.Func(a, a.SavedEntries), slices.Equal[[]Entry]).Get
	title := func() string { return locales.SavedTitle(cx) }
	head := core.View{Style: core.Style{Gap: 2, FlexShrink: 0}, Children: []core.Node{
		core.View{Role: core.RoleHeader, AccessibilityLevel: func() int { return 1 }, LabelFunc: title, Children: []core.Node{
			core.Text{ContentFunc: title, Style: ui.TypeTitle, StyleFunc: ui.Colored(colors.Text)},
		}},
		core.Text{TestID: "saved-count", ContentFunc: func() string { return locales.SavedCount(cx, len(entries())) }, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
	}}
	list := core.Show{
		When: func() bool { return len(entries()) > 0 },
		Then: func(cx *core.Scope) core.Node {
			return core.List{TestID: "saved-lines", Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}, EstimatedSize: 64,
				Count: func() int { return len(entries()) },
				Key: func(i int) any {
					if list := entries(); i < len(list) {
						return list[i].ID
					}
					return i
				},
				Row: func(cx *core.Scope, row core.ListRow) core.Node {
					entry := func() Entry {
						if list := entries(); row.Index() < len(list) {
							return list[row.Index()]
						}
						return Entry{}
					}
					return entryRow(cx, a, entry)
				}}
		},
		Else: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.SavedEmpty(cx) }, Style: ui.TypeBody.Merge(core.Style{TextAlign: core.TextAlignCenter, MaxWidth: core.Pt(320)}), StyleFunc: ui.Colored(colors.Muted)},
			}}
		},
	}
	export := live.Func(a, a.ExportState)
	due := live.Memo(cx, a, a.DueCount)
	footer := core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: []core.Node{
		exportStatus(cx, export),
		core.Show{When: func() bool { e := export(); return e.Finished && e.Err == "" && !e.Stopped }, Then: func(cx *core.Scope) core.Node {
			return textButton(func(cx *core.Scope) string { return locales.FilmShowInFinder(cx, desktop.Platform(cx)) }, ui.Button{Variant: ui.ButtonSecondary, TestID: "saved-export-reveal", OnPress: live.Do(a, a.RevealExport)})
		}},
		core.Show{When: func() bool { e := export(); return e.Finished && e.NoFFmpeg && e.Err == "" && !e.Stopped }, Then: func(cx *core.Scope) core.Node {
			return textButton(locales.SavedExportOpenSettings, ui.Button{Variant: ui.ButtonSecondary, TestID: "saved-export-settings", OnPress: live.Do(a, a.ExportSettings)})
		}},
		core.Show{When: func() bool { return export().Busy },
			Then: func(cx *core.Scope) core.Node {
				return textButton(locales.SavedExportCancel, ui.Button{Variant: ui.ButtonSecondary, TestID: "saved-export-cancel", OnPress: live.Do(a, a.StopExport)})
			},
			Else: func(cx *core.Scope) core.Node {
				return textButton(locales.SavedExport, ui.Button{Variant: ui.ButtonSecondary, TestID: "saved-export", OnPress: live.Do(a, a.ExportSaved),
					Disabled: func() bool { return len(entries()) == 0 }})
			}},
		core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Button a TextFunc twin so its text follows the count without building the button again, which takes the keyboard focus off it; then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			n := due()
			return ui.Button{Text: locales.SavedReviewDue(cx, n), Variant: ui.ButtonSecondary, TestID: "saved-review", OnPress: live.Do(a, a.StartReview), Disabled: func() bool { return n == 0 }}
		}),
		textButton(locales.SavedDone, ui.Button{TestID: "saved-done", OnPress: live.Do(a, a.CloseSaved)}),
	}}
	// TODO(nsgo#145): give base.Dialog (ui.Dialog) LabelFunc and AccessibilityDescriptionFunc twins, so the sheet's accessible name follows the interface language as it changes while it is open; it is read as the sheet opens https://github.com/nsgo-dev/nsgo/issues/145
	return ui.Dialog{
		TestID: "saved",
		Label:  locales.SavedTitle(cx),
		Open:   func() bool { return true },
		Bare:   true,
		Style:  core.Style{Width: core.Pt(sheetWidth), Height: core.Pt(sheetHeight)},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				a.CloseSaved()
				a.Changed()
			}
		},
		Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Gap: 12, Padding: core.All(20)}, Children: []core.Node{head, list, footer}}},
	}
}

// exportStatus says how the export of the saved lines goes, in the room left of the sheet's buttons: how many lines it has done while it runs, what it wrote, how many of the lines with their audio, and where Anki wants it once it is done, that it was stopped, or why it failed; nothing before the first.
func exportStatus(cx *core.Scope, export func() Export) core.Node {
	colors := ui.ColorsOf(cx)
	text := func() string {
		e := export()
		switch {
		case e.Busy:
			return locales.SavedExporting(cx, e.Done, e.Total)
		case !e.Finished:
			return ""
		case e.Stopped:
			return locales.SavedExportStopped(cx)
		case e.Err != "":
			return locales.SavedExportFailed(cx, e.Err)
		case e.Sounds == 0:
			return locales.SavedExportedNoAudio(cx, e.Lines)
		case e.Sounds < e.Lines:
			return locales.SavedExportedSomeAudio(cx, e.Lines, e.Sounds)
		}
		return locales.SavedExported(cx, e.Lines)
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
		core.Text{TestID: "saved-export-status", ContentFunc: text, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// entryRow is a saved line of the sheet: its film and time over its original and its translation, which a click plays from, and the × that removes it. A line whose video is gone does not play and says so.
func entryRow(cx *core.Scope, a App, entry func() Entry) core.Node {
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	where := func() string {
		e := entry()
		if e.Gone {
			return locales.SavedWhereGone(cx, e.Film, saved.Clock(e.Start))
		}
		return locales.SavedWhere(cx, e.Film, saved.Clock(e.Start))
	}
	open := base.Button{
		LabelFunc: func() string { return strings.TrimSpace(where() + " " + entry().Original + " " + entry().Translation) },
		Disabled:  func() bool { return entry().Gone },
		OnPress:   live.On(a, func(core.PressEvent) { a.OpenSaved(entry().ID) }),
		Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2, Padding: core.Symmetric(8, 10), CornerRadius: 8, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() core.Style { return core.Style{Hover: &core.Style{Background: k.Shade(0.05)}} },
		Children: []core.Node{
			core.Text{ContentFunc: where, MaxLines: 1, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
			core.Text{ContentFunc: func() string { return entry().Original }, Style: core.Style{FontSize: 14, LineHeight: 19}, StyleFunc: ui.Colored(colors.Text)},
			core.Show{When: func() bool { return entry().Translation != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{ContentFunc: func() string { return entry().Translation }, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)}
			}},
		},
	}
	remove := ui.IconButton{Icon: lucide.X, LabelFunc: func() string { return locales.SavedRemoveLine(cx) }, Size: 24, TestID: "saved-remove-" + core.Untracked(cx, entry).ID,
		OnPress: live.Do(a, func() { a.RemoveSaved(entry().ID) })}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 4}, Children: []core.Node{open, core.View{Style: core.Style{Padding: core.Edges{Top: 6}}, Children: []core.Node{remove}}}}
}

// textButton is a ui.Button whose text follows the interface language.
func textButton(text func(cx *core.Scope) string, b ui.Button) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Button a TextFunc twin so its text follows the interface language without building the button again, which takes the keyboard focus off it; then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		b.Text = text(cx)
		return b
	})
}
