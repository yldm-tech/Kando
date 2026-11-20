package subexport

import (
	"strings"

	"kando/apps/kando/film"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Dialog is the 导出字幕 dialog over the window while the app's SubexportOpen holds: the format, which lines, their order in a bilingual export, what a line without a translation becomes and whether the film's subtitle delay goes into the times, then what the export holds and where 导出到影片文件夹 writes it. 取消, Escape and closing it close it.
type Dialog struct{ A App }

// Render implements ns.Node.
func (d Dialog) Render(cx *ns.Scope) ns.Node {
	a := d.A
	return ns.Show{When: live.Func(a, a.SubexportOpen), Then: func(cx *ns.Scope) ns.Node {
		colors := ui.ColorsOf(cx)
		choice := live.Func(a, a.SubexportChoice)
		set := func(change func(c *Choice)) {
			live.Do(a, func() {
				c := a.SubexportChoice()
				change(&c)
				a.SetSubexportChoice(c)
			})()
		}
		tabs := func(id, label string, get func(Choice) string, put func(*Choice, string), options ...ui.Option[string]) ns.Node {
			return ui.Tabs[string]{TestID: id, Label: label, Size: ui.SizeSmall, Options: options,
				Value:    func() string { return get(choice()) },
				OnChange: func(v string) { set(func(c *Choice) { put(c, v) }) }}
		}
		// The plugins' formats come after Kando's own. Without any, Kando's own tabs are built once, as the other rows' are; with some, the tabs are built again only when they change.
		// TODO(nsgo#922): ui.Tabs still takes its Options as a slice only (ui/tabs.go:30), and its tab views, their frames and the column count are made once from it (ui/tabs.go:72-138), so tabs whose options change are built again and lose their focus; nsgo#848 gave ui.Select an OptionsFunc but not ui.Tabs; bind the formats once ui.Tabs has an OptionsFunc too https://github.com/nsgo-dev/nsgo/issues/922
		formats := ns.NewMemo(cx, live.Func(a, func() string {
			var out []string
			for _, f := range a.SubexportFormats() {
				out = append(out, f.Value+"\x00"+f.Label)
			}
			return strings.Join(out, "\x01")
		}))
		own := func() []ui.Option[string] {
			return []ui.Option[string]{
				{Value: core.FormatSRT, Label: "SRT", TestID: "export-format-srt"},
				{Value: core.FormatVTT, Label: "WebVTT", TestID: "export-format-vtt"},
				{Value: core.FormatASS, Label: "ASS", TestID: "export-format-ass"},
			}
		}
		formatTabs := func(options ...ui.Option[string]) ns.Node {
			return tabs("export-format", locales.SubexportFormat(cx), func(c Choice) string { return c.Format }, func(c *Choice, v string) { c.Format = v }, options...)
		}
		format := ns.Show{When: func() bool { return formats.Get() == "" }, Then: func(*ns.Scope) ns.Node { return formatTabs(own()...) }, Else: func(*ns.Scope) ns.Node {
			return ns.Dynamic(func(*ns.Scope) ns.Node {
				options := own()
				for _, item := range strings.Split(formats.Get(), "\x01") {
					value, label, _ := strings.Cut(item, "\x00")
					options = append(options, ui.Option[string]{Value: value, Label: label, TestID: "export-format-" + strings.ReplaceAll(strings.TrimPrefix(value, PluginPrefix), ":", "-")})
				}
				return formatTabs(options...)
			})
		}}
		content := tabs("export-content", locales.SubexportContent(cx), func(c Choice) string { return c.Content }, func(c *Choice, v string) { c.Content = v },
			ui.Option[string]{Value: core.SubOriginal, Label: locales.SubexportOriginal(cx), TestID: "export-content-original"},
			ui.Option[string]{Value: core.SubTranslation, Label: locales.SubexportTranslation(cx), TestID: "export-content-translation"},
			ui.Option[string]{Value: core.SubBilingual, Label: locales.SubexportBilingual(cx), TestID: "export-content-bilingual"})
		order := tabs("export-order", locales.SubexportOrder(cx), func(c Choice) string { return c.Order }, func(c *Choice, v string) { c.Order = v },
			ui.Option[string]{Value: core.SubTranslationFirst, Label: locales.SubexportTranslationFirst(cx), TestID: "export-order-translation"},
			ui.Option[string]{Value: core.SubOriginalFirst, Label: locales.SubexportOriginalFirst(cx), TestID: "export-order-original"})
		missing := ui.Select[string]{TestID: "export-missing", Label: locales.SubexportMissing(cx),
			Options: []ui.Option[string]{
				{Value: core.MissingSkip, Label: locales.SubexportMissingSkip(cx)},
				{Value: core.MissingPlaceholder, Label: locales.SubexportMissingPlaceholder(cx)},
				{Value: core.MissingOriginal, Label: locales.SubexportMissingOriginal(cx)},
			},
			Value:    func() string { return choice().Missing },
			OnChange: func(v string) { set(func(c *Choice) { c.Missing = v }) },
			Disabled: func() bool { return choice().Content == core.SubOriginal }}
		delay := ui.Switch{TestID: "export-delay", Label: locales.SubexportDelay(cx),
			Checked:  func() bool { return choice().Delay },
			OnChange: func(on bool) { set(func(c *Choice) { c.Delay = on }) }}
		delayText := func() string {
			return locales.SubexportDelayAt(cx, film.DelayText(cx, live.Func(a, a.SubexportDelay)()))
		}
		rows := []ns.Node{
			row(cx, func() string { return locales.SubexportFormat(cx) }, format),
			row(cx, func() string { return locales.SubexportContent(cx) }, content),
			ns.Show{When: func() bool { return choice().Content == core.SubBilingual }, Then: func(cx *ns.Scope) ns.Node {
				return row(cx, func() string { return locales.SubexportOrder(cx) }, order)
			}},
			row(cx, func() string { return locales.SubexportMissing(cx) }, missing),
			row(cx, delayText, delay),
		}
		summary := ns.Text{TestID: "export-count", ContentFunc: func() string { return countText(cx, choice(), live.Func(a, a.SubexportCount)()) },
			Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)}
		path := ns.Text{TestID: "export-path", ContentFunc: func() string { return Display(live.Func(a, a.SubexportPath)()) }, Selectable: ns.SelectableOn,
			Style: ui.TypeCaption.Merge(ns.Style{FontFamily: ui.Mono}), StyleFunc: ui.Colored(colors.Muted)}
		none := func() bool {
			return live.Func(a, a.SubexportPath)() == "" || live.Func(a, a.SubexportCount)().Cues == 0
		}
		return ui.Dialog{
			TestID:                 "export-dialog",
			NoCloseOnBackdropPress: true,
			Open:                   func() bool { return true },
			TitleFunc:              func() string { return locales.SubexportTitle(cx) },
			DescriptionFunc:        func() string { return locales.SubexportDescription(cx) },
			Children: []ns.Node{ns.View{Style: ns.Style{Gap: 12, AlignItems: ns.AlignStretch}, Children: []ns.Node{
				ns.View{Style: ns.Style{Gap: 10, AlignItems: ns.AlignStretch}, Children: rows},
				summary, path,
			}}},
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					live.Do(a, a.CloseSubexport)()
				}
			},
			Footer: []ns.Node{
				ui.Button{Text: locales.SubexportCancel(cx), Variant: ui.ButtonSecondary, TestID: "export-cancel", OnPress: live.Do(a, a.CloseSubexport)},
				ui.Button{Text: locales.SubexportSaveAs(cx), Variant: ui.ButtonSecondary, TestID: "export-save-as", Disabled: none, OnPress: live.Do(a, a.SubexportSaveAs)},
				ui.Button{Text: locales.SubexportToFolder(cx), TestID: "export-to-folder", Disabled: none, OnPress: live.Do(a, a.SubexportToFolder)},
			},
		}
	}}
}

// row is a row of the dialog: what it chooses, then its control at the end.
func row(cx *ns.Scope, label func() string, control ns.Node) ns.Node {
	return ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifySpaceBetween, Gap: 16, MinHeight: ns.Pt(28)}, Children: []ns.Node{
		ns.Text{ContentFunc: label, Style: ui.TypeBody.Merge(ns.Style{FlexShrink: 1})},
		control,
	}}
}

// countText is what an export as c holds, as count says: its lines, how many of them are translated, how many have no translation and what becomes of them, how many the user corrected and how many it leaves out as no speech.
func countText(cx *ns.Scope, c Choice, count core.ExportCount) string {
	parts := []string{locales.SubexportCountLines(cx, count.Lines), locales.SubexportCountTranslated(cx, count.Translated, count.Lines)}
	if count.Missing > 0 {
		how := locales.SubexportMissingSkip(cx)
		switch c.Missing {
		case core.MissingPlaceholder:
			how = locales.SubexportMissingPlaceholder(cx)
		case core.MissingOriginal:
			how = locales.SubexportMissingOriginal(cx)
		}
		parts = append(parts, locales.SubexportCountMissing(cx, count.Missing, how))
	}
	if count.Edited > 0 {
		parts = append(parts, locales.SubexportCountEdited(cx, count.Edited))
	}
	if count.NoSpeech > 0 {
		parts = append(parts, locales.SubexportCountNoSpeech(cx, count.NoSpeech))
	}
	return strings.Join(parts, " · ")
}
