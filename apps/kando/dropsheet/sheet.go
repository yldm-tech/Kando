// Package dropsheet is the sheet a large drop opens: more videos than the window queues at once (films.ConfirmAbove), from folders dropped on the window or picked with 添加文件夹…. It says how many films were found and how much room they take, and asks how to add them: only into the library, where each film's 生成字幕 makes its subtitles when it is watched, which costs nothing now, or all into the queue, which costs minutes and the AI service's charges per film. For one folder dropped it offers to put the films in a category named after it. It names the programs found named as videos are, which are never added. The app keeps its state and adds the films (state_drop.go).
package dropsheet

import (
	"fmt"

	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Look is what the sheet shows of the drop.
type Look struct {
	// Folder is the name of the one folder dropped, "" when several things were.
	Folder string
	// Videos is how many films were found; New is how many of them the library does not have, and Unmade how many it has as films without subtitles, which 全部生成字幕 makes too. The others it has done, or being made.
	Videos int
	New    int
	Unmade int
	// Size is how many bytes the films take.
	Size int64
	// Skipped is how many files were left out because they are not videos.
	Skipped int
	// Disguised is how many programs were found named as videos are, and Named the names of the first two, "" past their count; Cut is whether the films past films.DropLimit were left out.
	Disguised int
	Named     [2]string
	Cut       bool
	// Categories is whether the sheet offers a category named after Folder: the library takes categories, and one folder was dropped.
	Categories bool
}

// Adds is how many films Add adds, or with generate makes the subtitles of: those found that the library does not have, and with generate also those it has without subtitles.
func (l Look) Adds(generate bool) int {
	if generate {
		return l.New + l.Unmade
	}
	return l.New
}

// have is how many of the films found the library has already.
func (l Look) have() int { return l.Videos - l.New }

// Sheet is the sheet over the window while Open returns true. Generate is whether Add queues the films to have their subtitles made, false for only adding them to the library, which it starts at; Category is whether Add puts them in a category named after the folder too. Add runs OnAdd; Cancel, Escape and closing the sheet run OnCancel.
type Sheet struct {
	Open       func() bool
	Look       func() Look
	Generate   func() bool
	OnGenerate func(on bool)
	Category   func() bool
	OnCategory func(on bool)
	OnAdd      func()
	OnCancel   func()
}

// Render implements core.Node.
func (s Sheet) Render(cx *core.Scope) core.Node {
	look := core.NewMemo(cx, s.Look)
	cancel := func() {
		if s.OnCancel != nil {
			s.OnCancel()
		}
	}
	return core.Show{When: s.Open, Then: func(cx *core.Scope) core.Node {
		return ui.Dialog{
			TestID:                 "drop-sheet",
			Role:                   core.RoleAlertDialog,
			NoCloseOnBackdropPress: true,
			Open:                   func() bool { return true },
			TitleFunc: func() string {
				if folder := look.Get().Folder; folder != "" {
					return locales.DropTitle(cx, folder)
				}
				return locales.DropTitleSeveral(cx)
			},
			Children: []core.Node{
				summary(cx, look.Get),
				s.choice(),
				core.Show{When: func() bool { return look.Get().Categories && s.Category != nil }, Then: func(cx *core.Scope) core.Node {
					return ui.Checkbox{TextFunc: func() string { return locales.DropCategory(cx, look.Get().Folder) }, TestID: "drop-category", Checked: s.Category, OnChange: s.OnCategory}
				}},
				core.Show{When: func() bool { return look.Get().Disguised > 0 }, Then: func(cx *core.Scope) core.Node { return disguised(cx, look.Get) }},
			},
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Footer: []core.Node{
				ui.Button{TextFunc: func() string { return locales.DropCancel(cx) }, Variant: ui.ButtonSecondary, TestID: "drop-cancel", OnPress: cancel},
				s.addButton(cx, look.Get),
			},
		}
	}}
}

// addButton is Add, which says how many films it adds, or makes the subtitles of, as the choice and the drop change, dimmed when that is none.
func (s Sheet) addButton(cx *core.Scope, look func() Look) core.Node {
	n := core.NewMemo(cx, func() int { return look().Adds(s.Generate()) })
	return ui.Button{TextFunc: func() string { return locales.DropAdd(cx, n.Get()) }, TestID: "drop-add", OnPress: s.OnAdd, Disabled: func() bool { return n.Get() == 0 }}
}

// summary is what the sheet says under its title: how many films were found and the room they take, that those past the limit were left out, how many of them the library has, and how many files were not videos.
func summary(cx *core.Scope, look func() Look) core.Node {
	colors := ui.ColorsOf(cx)
	line := func(id string, text func() string) core.Node {
		return core.Text{ContentFunc: text, TestID: id, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)}
	}
	// A line shown while when is true.
	shown := func(when func() bool, id string, text func() string) core.Node {
		return core.Show{When: when, Then: func(*core.Scope) core.Node { return line(id, text) }}
	}
	return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
		line("drop-found", func() string { l := look(); return locales.DropFound(cx, l.Videos, byteSize(l.Size)) }),
		shown(func() bool { return look().Cut }, "drop-cut", func() string { return locales.DropCut(cx, look().Videos) }),
		shown(func() bool { return look().have() > 0 }, "drop-have", func() string { return locales.DropHave(cx, look().have()) }),
		shown(func() bool { return look().Skipped > 0 }, "drop-skipped", func() string { return locales.DropSkipped(cx, look().Skipped) }),
	}}
}

// choice is how the films are added: only into the library, which it starts at, or all into the queue.
func (s Sheet) choice() core.Node {
	set := func(on bool) func(bool) {
		return func(checked bool) {
			if checked && s.OnGenerate != nil {
				s.OnGenerate(on)
			}
		}
	}
	// TODO(nsgo#2237): ui.Radio and ui.RadioGroup take their words and accessible names as strings only, so the group is built again as the language changes; pass TextFunc, LabelFunc and HintFunc and drop this Dynamic once they have them https://github.com/nsgo-dev/nsgo/issues/2237
	return core.Dynamic(func(cx *core.Scope) core.Node {
		return s.choiceGroup(cx, set)
	})
}

// choiceGroup is choice's radio group in the language the app shows now, set(on) choosing whether Add queues the films.
func (s Sheet) choiceGroup(cx *core.Scope, set func(on bool) func(bool)) core.Node {
	colors := ui.ColorsOf(cx)
	// A choice's title over its hint, which wraps within the sheet.
	hint := func(title, text string) []core.Node {
		return []core.Node{core.View{Style: core.Style{Gap: 2, FlexShrink: 1, FlexGrow: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			core.Text{Content: title, Style: ui.TypeBody},
			core.Text{Content: text, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
		}}}
	}
	return ui.RadioGroup{Label: locales.DropHow(cx), TestID: "drop-how", Style: core.Style{Gap: 10}, Children: []core.Node{
		ui.Radio{Label: locales.DropOnlyAdd(cx), Hint: locales.DropOnlyAddHint(cx), Children: hint(locales.DropOnlyAdd(cx), locales.DropOnlyAddHint(cx)), TestID: "drop-only-add",
			Checked: func() bool { return !s.Generate() }, OnChange: set(false)},
		ui.Radio{Label: locales.DropGenerate(cx), Hint: locales.DropGenerateHint(cx), Children: hint(locales.DropGenerate(cx), locales.DropGenerateHint(cx)), TestID: "drop-generate",
			Checked: s.Generate, OnChange: set(true)},
	}}
}

// disguised is the warning that names the programs found named as videos are, at most the first two, which are not added and are best left unopened.
func disguised(cx *core.Scope, look func() Look) core.Node {
	colors := ui.ColorsOf(cx)
	text := func() string {
		l := look()
		if l.Disguised > 1 && l.Named[1] != "" {
			return locales.DropDisguisedMany(cx, l.Disguised, l.Named[0], l.Named[1])
		}
		return locales.DropDisguised(cx, l.Disguised, l.Named[0])
	}
	return core.Text{ContentFunc: text, TestID: "drop-disguised", Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Danger)}
}

// byteSize writes n bytes as the size a person reads, in MB or GB, as Settings › 隐私 writes what it freed.
func byteSize(n int64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%.0f MB", max(1, float64(n)/(1<<20)))
}
