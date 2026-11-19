package main

import (
	"slices"

	"kando/apps/kando/plugins"
	"kando/apps/kando/saved"
	"kando/apps/kando/study"
)

// Export-template packs (docs/plugin-system.md E7): a pack's template is another way to export the saved lines, as CSV, TSV or Anki notes of its own note type, its columns made of text and the placeholders the host lists (saved.Placeholders), so a pack reads nothing of a line but what they name. The Saved Lines sheet offers the packs turned on beside Kando's own Anki notes (study.App.ExportTargets); the one chosen is kept in settings.json (savedPlugins.PluginExportTarget) and the export writes with it (pluginsExportTarget, saved.Target).

// ExportTargets are Kando's own Anki notes and the export-template packs turned on, each named in the interface's language (study.App).
func (a *app) ExportTargets() []study.ExportTarget {
	m := a.messages()
	out := []study.ExportTarget{{Label: m.SavedExportTargetAnki()}}
	for _, pack := range a.pluginsOn(plugins.KindExportTemplate) {
		out = append(out, study.ExportTarget{ID: pack.ID, Label: m.SavedExportTargetPlugin(pack.Name.In(m.Language()))})
	}
	return out
}

// ExportTarget is the export-template pack chosen while it is turned on, "" for Kando's own Anki notes (study.App).
func (a *app) ExportTarget() string {
	if _, ok := a.pluginsTemplate(a.plugins.exportTarget); ok {
		return a.plugins.exportTarget
	}
	return ""
}

// SetExportTarget chooses the export-template pack id, or Kando's own Anki notes with "", and saves the choice (study.App).
func (a *app) SetExportTarget(id string) {
	if id == a.plugins.exportTarget {
		return
	}
	if _, ok := a.pluginsTemplate(id); id != "" && !ok {
		return
	}
	a.plugins.exportTarget = id
	a.Save()
}

// pluginsTemplate is the export-template pack id when it is turned on.
func (a *app) pluginsTemplate(id string) (plugins.Pack, bool) {
	packs := a.pluginsOn(plugins.KindExportTemplate)
	i := slices.IndexFunc(packs, func(p plugins.Pack) bool { return p.ID == id })
	if id == "" || i < 0 {
		return plugins.Pack{}, false
	}
	return packs[i], true
}

// pluginsExportTarget is what the saved lines' export writes with (saved.Export): the template of the pack chosen, with its name in the interface's language, or the zero Target, Kando's own Anki notes, and "" while none is chosen or the one chosen is off or gone.
func (a *app) pluginsExportTarget() (saved.Target, string) {
	pack, ok := a.pluginsTemplate(a.plugins.exportTarget)
	if !ok {
		return saved.Target{}, ""
	}
	t := pack.Template
	target := saved.Target{ID: pack.ID, Format: t.Format, NoteType: t.NoteType}
	for _, c := range t.Columns {
		target.Headers = append(target.Headers, c.Header)
		target.Columns = append(target.Columns, c.Value)
	}
	return target, pack.Name.In(a.messages().Language())
}
