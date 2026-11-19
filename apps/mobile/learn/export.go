package learn

import (
	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/packages/study"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// ExportPage exports the vocabulary, with the saved lines when asked, from every computer's libraries, as the computer's Export Vocabulary writes a library's (learning.Export): in the format picked, Anki's notes, a CSV table or plain text, each word with the line it is from and that line's translation as asked; the choices are kept for the next export. The file goes out through the share sheet.
func ExportPage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
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
		formatRows = append(formatRows, look.Row{TestID: "export-format-" + f.id, Index: func() int { return i },
			Title: f.name, Detail: f.detail,
			Checked: func() bool { return o().Format == f.id },
			Trailing: core.Show{When: func() bool { return o().Format == f.id }, Then: func(*core.Scope) core.Node {
				return ui.Icon(lucide.Check, 20, colors.Accent)
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
	section := func(title func() string, rows ...core.Node) core.Node {
		return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{look.SectionTitle(cx, title), look.Group{Children: rows}}}
	}
	ref := &core.Ref{}
	body := core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{StyleFunc: look.Column(cx), Children: []core.Node{
			ui.Tabular(core.Text{TestID: "export-count", ContentFunc: func() string { w, l := counts(); return locales.ExportCount(cx, w, l) },
				Style: look.Subhead.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Muted)}),
			section(func() string { return locales.ExportFormat(cx) },
				core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.ExportFormat(cx) }, Children: formatRows}),
			section(func() string { return locales.ExportInclude(cx) },
				toggle(0, "sentence", func() string { return locales.ExportWithSentence(cx) },
					func(v study.VocabOptions) bool { return v.Context }, func(v *study.VocabOptions, on bool) { v.Context = on }),
				toggle(1, "translation", func() string { return locales.ExportWithTranslation(cx) },
					func(v study.VocabOptions) bool { return v.Translation }, func(v *study.VocabOptions, on bool) { v.Translation = on }),
				toggle(2, "lines", func() string { return locales.ExportWithLines(cx) },
					func(v study.VocabOptions) bool { return v.Lines }, func(v *study.VocabOptions, on bool) { v.Lines = on })),
		}},
	}}
	share := core.View{Ref: anchor, Children: []core.Node{
		look.Button{TestID: "export-share", Kind: look.Primary, Text: func() string { return locales.LearnExport(cx) },
			Busy:     exporting.Get,
			Disabled: func() bool { w, l := counts(); return w+l == 0 },
			OnPress:  export},
	}}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.ExportTitle(cx) }, TestID: "export-page", Children: []core.Node{
		body, look.ActionBar(cx, share),
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}
