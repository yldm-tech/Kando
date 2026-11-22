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
		return core.Dynamic(func(cx *core.Scope) core.Node {
			l := look.Get()
			// TODO(nsgo#145): give ui.Dialog and ui.AlertDialog TitleFunc and DescriptionFunc twins so the words follow the drop and the language without building the sheet again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			title := locales.DropTitleSeveral(cx)
			if l.Folder != "" {
				title = locales.DropTitle(cx, l.Folder)
			}
			children := []core.Node{s.summary(cx, l), s.choice(cx)}
			if l.Categories && s.Category != nil {
				children = append(children, ui.Checkbox{Text: locales.DropCategory(cx, l.Folder), TestID: "drop-category", Checked: s.Category, OnChange: s.OnCategory})
			}
			if l.Disguised > 0 {
				children = append(children, disguised(cx, l))
			}
			return ui.Dialog{
				TestID:                 "drop-sheet",
				Role:                   core.RoleAlertDialog,
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				Title:                  title,
				Children:               children,
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Footer: []core.Node{
					ui.Button{Text: locales.DropCancel(cx), Variant: ui.ButtonSecondary, TestID: "drop-cancel", OnPress: cancel},
					s.addButton(cx, l),
				},
			}
		})
	}}
}

// addButton is Add, which says how many films it adds, or makes the subtitles of, as the choice changes, dimmed when that is none.
func (s Sheet) addButton(cx *core.Scope, l Look) core.Node {
	n := core.NewMemo(cx, func() int { return l.Adds(s.Generate()) })
	// TODO(nsgo#145): give ui.Button a TextFunc twin, so its label follows the number without building the button again https://github.com/nsgo-dev/nsgo/issues/145
	return core.Dynamic(func(cx *core.Scope) core.Node {
		count := n.Get()
		return ui.Button{Text: locales.DropAdd(cx, count), TestID: "drop-add", OnPress: s.OnAdd, Disabled: func() bool { return count == 0 }}
	})
}

// summary is what the sheet says under its title: how many films were found and the room they take, that those past the limit were left out, how many of them the library has, and how many files were not videos.
func (s Sheet) summary(cx *core.Scope, l Look) core.Node {
	colors := ui.ColorsOf(cx)
	line := func(text, id string) core.Node {
		return core.Text{Content: text, TestID: id, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)}
	}
	lines := []core.Node{line(locales.DropFound(cx, l.Videos, byteSize(l.Size)), "drop-found")}
	if l.Cut {
		lines = append(lines, line(locales.DropCut(cx, l.Videos), "drop-cut"))
	}
	if have := l.have(); have > 0 {
		lines = append(lines, line(locales.DropHave(cx, have), "drop-have"))
	}
	if l.Skipped > 0 {
		lines = append(lines, line(locales.DropSkipped(cx, l.Skipped), "drop-skipped"))
	}
	return core.View{Style: core.Style{Gap: 4}, Children: lines}
}

// choice is how the films are added: only into the library, which it starts at, or all into the queue.
func (s Sheet) choice(cx *core.Scope) core.Node {
	set := func(on bool) func(bool) {
		return func(checked bool) {
			if checked && s.OnGenerate != nil {
				s.OnGenerate(on)
			}
		}
	}
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
func disguised(cx *core.Scope, l Look) core.Node {
	colors := ui.ColorsOf(cx)
	text := locales.DropDisguised(cx, l.Disguised, l.Named[0])
	if l.Disguised > 1 && l.Named[1] != "" {
		text = locales.DropDisguisedMany(cx, l.Disguised, l.Named[0], l.Named[1])
	}
	return core.Text{Content: text, TestID: "drop-disguised", Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Danger)}
}

// byteSize writes n bytes as the size a person reads, in MB or GB, as Settings › 隐私 writes what it freed.
func byteSize(n int64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%.0f MB", max(1, float64(n)/(1<<20)))
}
