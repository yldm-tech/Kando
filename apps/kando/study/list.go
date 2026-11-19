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
	// ExportTargets are the ways the lines can be exported, Kando's own Anki notes first and the export-template plugins turned on after; ExportTarget is the id of the one chosen and SetExportTarget chooses one, which is saved.
	ExportTargets() []ExportTarget
	ExportTarget() string
	SetExportTarget(id string)
	// DueCount is how many saved lines are due for review; StartReview closes the sheet and reviews them (Review).
	DueCount() int
	StartReview()
	// NoteEditing is the saved line whose note the sheet is writing, by its ID, "" while none is. EditNote opens the note of line id, keeping the one open before; NoteDraft is the note as typed so far, which SetNoteDraft follows; SaveNote keeps it, an empty one taking the note away, and CancelNote drops it.
	NoteEditing() string
	EditNote(id string)
	NoteDraft() string
	SetNoteDraft(text string)
	SaveNote()
	CancelNote()
}

// Export is how the export of the saved lines goes: Busy while it runs, Done lines of Total; once Finished, the Lines and the Sounds it wrote, or Err, why it failed, or Stopped once the user stopped it. NoFFmpeg reports that ffmpeg is not installed, so no line has its audio. Template is the name of the export-template plugin it wrote with, "" for Kando's own Anki notes, and File the name of the file it wrote.
type Export struct {
	Busy, Finished bool
	Done, Total    int
	Lines, Sounds  int
	Err            string
	Stopped        bool
	NoFFmpeg       bool
	Template, File string
	// WithSound is whether the export writes the lines' audio: Kando's own notes do, a template only when it has a column for it.
	WithSound bool
}

// ExportTarget is a way the saved lines can be exported: ID "" for Kando's own Anki notes, else the export-template plugin's id; Label is what the sheet calls it, in the interface's language.
type ExportTarget struct {
	ID, Label string
}

// Entry is a saved line as the sheet lists it.
type Entry struct {
	// ID names the line for OpenSaved and RemoveSaved, and keeps its row as lines come and go.
	ID                          string
	Film, Original, Translation string
	Start                       time.Duration
	// Note is what the learner wrote about the line, "" for none.
	Note string
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
		exportPicker(cx, a),
		core.Show{When: func() bool { e := export(); return e.Finished && e.Err == "" && !e.Stopped }, Then: func(cx *core.Scope) core.Node {
			return textButton(func(cx *core.Scope) string { return locales.FilmShowInFinder(cx, desktop.Platform(cx)) }, ui.Button{Variant: ui.ButtonSecondary, TestID: "saved-export-reveal", OnPress: live.Do(a, a.RevealExport)})
		}},
		core.Show{When: func() bool {
			e := export()
			return e.Finished && e.NoFFmpeg && e.WithSound && e.Err == "" && !e.Stopped
		}, Then: func(cx *core.Scope) core.Node {
			return textButton(locales.SavedExportOpenSettings, ui.Button{Variant: ui.ButtonSecondary, TestID: "saved-export-settings", OnPress: live.Do(a, a.ExportSettings)})
		}},
		core.Show{When: func() bool { return export().Busy },
			Then: func(cx *core.Scope) core.Node {
				return textButton(locales.SavedExportCancel, ui.Button{Variant: ui.ButtonSecondary, TestID: "saved-export-cancel", OnPress: live.Do(a, a.StopExport)})
			},
			Else: func(cx *core.Scope) core.Node {
				// The button is built again when the way chosen changes, as its text says which.
				target := live.Func(a, a.ExportTarget)
				label := func(cx *core.Scope) string {
					if target() != "" {
						return locales.SavedExportTemplate(cx)
					}
					return locales.SavedExport(cx)
				}
				return textButton(label, ui.Button{Variant: ui.ButtonSecondary, TestID: "saved-export", OnPress: live.Do(a, a.ExportSaved),
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
		case e.Template != "" && e.WithSound && e.Sounds == 0:
			return locales.SavedExportedTemplateNoAudio(cx, e.Lines, e.File)
		case e.Template != "" && e.WithSound && e.Sounds < e.Lines:
			return locales.SavedExportedTemplateSomeAudio(cx, e.Lines, e.Sounds, e.File)
		case e.Template != "":
			return locales.SavedExportedTemplate(cx, e.Lines, e.File)
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

// exportPicker is the drop-down of the ways the lines can be exported, beside 导出, while an export-template plugin is turned on; nothing otherwise. It is built again only when the ways change.
func exportPicker(cx *core.Scope, a App) core.Node {
	targets := core.NewMemo(cx, live.Func(a, func() string {
		var out []string
		for _, t := range a.ExportTargets() {
			out = append(out, t.ID+"\x00"+t.Label)
		}
		return strings.Join(out, "\x01")
	}))
	return core.Show{When: func() bool { return strings.Contains(targets.Get(), "\x01") }, Then: func(cx *core.Scope) core.Node {
		// TODO(nsgo#848): give ui.Select its options as a binding, so a list that changes does not build the drop-down again and take its focus; it is built again only when the plugins turned on change, which is done elsewhere https://github.com/nsgo-dev/nsgo/issues/848
		return core.Dynamic(func(cx *core.Scope) core.Node { return exportSelect(cx, a, targets.Get()) })
	}}
}

// exportSelect is the drop-down of the ways joined lists, each its id and label.
func exportSelect(cx *core.Scope, a App, joined string) core.Node {
	var options []ui.Option[string]
	for _, item := range strings.Split(joined, "\x01") {
		id, label, _ := strings.Cut(item, "\x00")
		options = append(options, ui.Option[string]{Value: id, Label: label})
	}
	return ui.Select[string]{Options: options, Label: locales.SavedExportTarget(cx), TestID: "saved-export-target",
		Value: live.Func(a, a.ExportTarget), OnChange: live.On(a, a.SetExportTarget), Disabled: live.Func(a, func() bool { return a.ExportState().Busy }),
		Style: core.Style{Width: core.Pt(150)}}

}

// entryRow is a saved line of the sheet: its film and time over its original, its translation and the learner's note, which a click plays from, the pencil that opens the note's field under the line, and the × that removes it. A line whose video is gone does not play and says so.
func entryRow(cx *core.Scope, a App, entry func() Entry) core.Node {
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	id := core.Untracked(cx, entry).ID
	editing := live.Func(a, func() bool { return a.NoteEditing() != "" && a.NoteEditing() == entry().ID })
	where := func() string {
		e := entry()
		if e.Gone {
			return locales.SavedWhereGone(cx, e.Film, saved.Clock(e.Start))
		}
		return locales.SavedWhere(cx, e.Film, saved.Clock(e.Start))
	}
	open := base.Button{
		LabelFunc: func() string {
			e := entry()
			return strings.Join(slices.DeleteFunc([]string{where(), e.Original, e.Translation, e.Note}, func(s string) bool { return s == "" }), " ")
		},
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
			core.Show{When: func() bool { return entry().Note != "" && !editing() }, Then: func(cx *core.Scope) core.Node {
				k := theme.Of(cx)
				return core.View{TestID: "saved-note-" + id,
					Style:     core.Style{Margin: core.Edges{Top: 4}, Padding: core.Symmetric(6, 8), CornerRadius: 6, BorderWidth: 1},
					StyleFunc: func() core.Style { return core.Style{Background: k.WarnFill(), BorderColor: k.WarnRing()} },
					Children: []core.Node{
						core.Text{ContentFunc: func() string { return entry().Note }, Style: ui.TypeBody, StyleFunc: ui.Colored(k.WarnText)},
					}}
			}},
		},
	}
	note := ui.IconButton{Icon: lucide.NotebookPen, LabelFunc: func() string { return locales.SavedNoteEdit(cx) }, Size: 24, TestID: "saved-note-edit-" + id,
		OnPress: live.Do(a, func() { a.EditNote(entry().ID) })}
	remove := ui.IconButton{Icon: lucide.X, LabelFunc: func() string { return locales.SavedRemoveLine(cx) }, Size: 24, TestID: "saved-remove-" + id,
		OnPress: live.Do(a, func() { a.RemoveSaved(entry().ID) })}
	row := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 4}, Children: []core.Node{
		open,
		core.View{Style: core.Style{Direction: core.Row, Gap: 2, Padding: core.Edges{Top: 6}}, Children: []core.Node{note, remove}},
	}}
	return core.View{Children: []core.Node{
		row,
		core.Show{When: editing, Then: func(cx *core.Scope) core.Node { return noteEditor(cx, a, id) }},
	}}
}

// noteEditor is the field under saved line id where the learner writes a note on it, with Cancel and Save.
func noteEditor(cx *core.Scope, a App, id string) core.Node {
	field := ui.Input{TestID: "saved-note-field-" + id, Multiline: true, AutoFocus: true, LabelFunc: func() string { return locales.SavedNote(cx) },
		PlaceholderFunc: func() string { return locales.SavedNotePlaceholder(cx) },
		Value:           live.Func(a, a.NoteDraft),
		OnChange:        live.On(a, a.SetNoteDraft),
		Style:           core.Style{MinHeight: core.Pt(56)}}
	buttons := core.View{Style: core.Style{Direction: core.Row, Gap: 8, JustifyContent: core.JustifyEnd}, Children: []core.Node{
		textButton(locales.SavedNoteCancel, ui.Button{Variant: ui.ButtonSecondary, TestID: "saved-note-cancel", OnPress: live.Do(a, a.CancelNote)}),
		textButton(locales.SavedNoteSave, ui.Button{TestID: "saved-note-save", OnPress: live.Do(a, a.SaveNote)}),
	}}
	return core.View{Style: core.Style{Gap: 8, Padding: core.Edges{Top: 2, Right: 10, Bottom: 10, Left: 10}}, Children: []core.Node{field, buttons}}
}

// textButton is a ui.Button whose text follows the interface language.
func textButton(text func(cx *core.Scope) string, b ui.Button) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Button a TextFunc twin so its text follows the interface language without building the button again, which takes the keyboard focus off it; then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		b.Text = text(cx)
		return b
	})
}
