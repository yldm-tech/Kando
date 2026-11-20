package learn

import (
	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/packages/study"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// ExportSheet exports the vocabulary, with the saved lines when asked, from every computer's libraries, as the computer's Export Vocabulary writes a library's (learning.Export): in the format picked, Anki's notes, a CSV table or plain text, each word with the line it is from and that line's translation as asked; the choices are kept for the next export. The file goes out through the share sheet. It shows over Learn while open reports true; closing calls close.
func ExportSheet(cx *core.Scope, s *session.Session, open func() bool, close func()) core.Node {
	colors := ui.ColorsOf(cx)
	o := func() study.VocabOptions { return s.ExportOptions() }
	set := func(change func(*study.VocabOptions)) {
		v := s.ExportOptions()
		change(&v)
		s.SetExportOptions(v)
	}
	items := core.NewMemoFunc(cx, s.LearnItems, nil)
	counts := func() (int, int) {
		words, lines := learning.ExportCounts(items.Get(), o())
		return len(words), len(lines)
	}
	formats := []struct {
		id           string
		name, detail func() string
	}{
		{study.VocabAnki, func() string { return locales.ExportAnki(cx) }, func() string { return locales.ExportAnkiDetail(cx) }},
		{study.VocabCSV, func() string { return locales.ExportCSV(cx) }, func() string { return locales.ExportCSVDetail(cx) }},
		{study.VocabTSV, func() string { return locales.ExportTSV(cx) }, func() string { return locales.ExportTSVDetail(cx) }},
	}
	var formatRows []core.Node
	for i, f := range formats {
		// The format's name is in the design's heavier type, which look.Row's title does not take, so the row draws its texts below.
		texts := core.View{Style: core.Style{Gap: 2}, Children: []core.Node{
			core.Text{ContentFunc: f.name, MaxLines: 2, Style: core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightSemibold}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: f.detail, MaxLines: 2, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
		}}
		formatRows = append(formatRows, look.Row{TestID: "export-format-" + f.id, Index: func() int { return i },
			Below:   texts,
			Checked: func() bool { return o().Format == f.id },
			Trailing: core.Show{When: func() bool { return o().Format == f.id }, Then: func(*core.Scope) core.Node {
				return ui.Icon(lucide.Check, 18, colors.Accent)
			}},
			OnPress: func() { set(func(v *study.VocabOptions) { v.Format = f.id }) }})
	}
	toggle := func(i int, id string, title func() string, get func(study.VocabOptions) bool, put func(*study.VocabOptions, bool)) core.Node {
		return look.Row{TestID: "export-" + id, Index: func() int { return i }, TitleLines: 2, Title: title,
			Trailing: ui.Switch{TestID: "export-" + id + "-switch", Label: title(), Variant: ui.SwitchSystem,
				Checked: func() bool { return get(o()) }, OnChange: func(on bool) { set(func(v *study.VocabOptions) { put(v, on) }) }}}
	}
	anchor := &core.Ref{}
	exporting := core.NewSignal(cx, false)
	export := func() {
		if exporting.GetUntracked() {
			return
		}
		exporting.Set(true)
		s.ExportLearning(func(file string, err error) {
			if err != nil {
				exporting.Set(false)
				cx.ShowAlert(core.AlertConfig{Title: locales.LearnExportFailed(cx), Actions: []core.AlertAction{{Label: locales.CommonOK(cx), Role: core.AlertCancel}}}, nil)
				return
			}
			cx.Share(core.ShareOptions{Files: []core.SharedFile{{Path: file}}, Anchor: anchor}, func(bool, error) { exporting.Set(false) })
		})
	}
	note := func(id string, text func() string, style core.Style) core.Text {
		return core.Text{TestID: id, ContentFunc: text, Style: style.Merge(core.Style{Padding: core.Edges{Right: 4, Left: 4}}), StyleFunc: ui.Colored(colors.Muted)}
	}
	share := core.View{Ref: anchor, Children: []core.Node{
		look.Button{TestID: "export-share", Kind: look.Primary, Text: func() string { return locales.LearnExport(cx) },
			Busy:     exporting.Get,
			Disabled: func() bool { w, l := counts(); return w+l == 0 },
			OnPress:  export},
	}}
	return look.Dialog{TestID: "export-page", Open: open, OnClose: close, TitleHidden: true, Title: func() string { return locales.ExportTitle(cx) }, Children: []core.Node{
		core.View{Style: core.Style{Gap: 14}, Children: []core.Node{
			ui.Tabular(note("export-count", func() string { w, l := counts(); return locales.ExportCount(cx, w, l) }, core.Style{FontSize: 13, LineHeight: 19})),
			look.Group{Children: []core.Node{
				core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.ExportFormat(cx) }, Children: formatRows},
			}},
			look.Group{Children: []core.Node{
				toggle(0, "sentence", func() string { return locales.ExportWithSentence(cx) },
					func(v study.VocabOptions) bool { return v.Context }, func(v *study.VocabOptions, on bool) { v.Context = on }),
				toggle(1, "translation", func() string { return locales.ExportWithTranslation(cx) },
					func(v study.VocabOptions) bool { return v.Translation }, func(v *study.VocabOptions, on bool) { v.Translation = on }),
				toggle(2, "lines", func() string { return locales.ExportWithLines(cx) },
					func(v study.VocabOptions) bool { return v.Lines }, func(v *study.VocabOptions, on bool) { v.Lines = on }),
			}},
			share,
			note("export-share-note", func() string { return locales.ExportShareNote(cx) }, look.Footnote),
		}},
	}}
}
