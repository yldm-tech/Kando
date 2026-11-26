package sourcebrowser

import (
	"fmt"
	"slices"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Page is the 媒体来源 page beside the sidebar: its title, then the folder of the source the sidebar marks, under the path to it and 导入 N 个视频, or, while no source was added, what a source is and 添加来源…. It is built again when the interface's language changes, as the words its buttons take are plain strings; within it what changes follows in bindings, so the list keeps where it was scrolled to as a video is chosen.
type Page struct{ App App }

// Render implements core.Node.
func (p Page) Render(cx *core.Scope) core.Node {
	a := p.App
	return core.Dynamic(func(cx *core.Scope) core.Node {
		_ = i18n.Language(cx)
		return core.Func(func(cx *core.Scope) core.Node { return page(cx, a) })
	})
}

func page(cx *core.Scope, a App) core.Node {
	colors := ui.ColorsOf(cx)
	none := live.Memo(cx, a, func() bool { return len(a.MediaBrowseSources()) == 0 })
	title := core.Text{TestID: "media-sources-title", Role: core.RoleHeader, Content: locales.MediaSourceSidebar(cx), MaxLines: 1,
		Style: core.Style{FontSize: 20, LineHeight: 26, FontWeight: ui.Bold, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Text)}
	return core.View{TestID: "media-sources-page", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Gap: 14, AlignItems: core.AlignStretch}, Children: []core.Node{
		title,
		core.Show{When: none, Then: func(cx *core.Scope) core.Node { return empty(cx, a) }, Else: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), AlignItems: core.AlignStretch},
				Children: []core.Node{folder(cx, a)}}
		}},
	}}
}

// empty is the page while no source was added: the server's icon, what a source is for, and the button that adds one, in the middle of the page.
func empty(cx *core.Scope, a App) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{FlexGrow: 1, Gap: 14, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Edges{Bottom: 60}}, Children: []core.Node{
		ui.Icon(lucide.Server, 40, colors.Faint),
		core.Text{TestID: "media-sources-none", Content: locales.MediaSourceNone(cx), Style: core.Style{FontSize: 13, LineHeight: 19, MaxWidth: core.Pt(360), TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Muted)},
		ui.Button{TestID: "media-sources-add", Text: locales.MediaSourceAddSource(cx), OnPress: live.Do(a, a.AddMediaBrowseSource)},
	}}
}

// folder is the page under its title: 全选, the path to the folder shown and the filter of its rows, with 导入 N 个视频 at its end (导入所选 with folders chosen, which lists their videos first), what the last import did, and the folder's rows, or that it is being listed, empty, or could not be listed and why.
func folder(cx *core.Scope, a App) core.Node {
	colors := ui.ColorsOf(cx)
	chosen := live.Func(a, a.MediaBrowseChosen)
	// The path is plain strings, so it is built again for another folder.
	crumbs := core.NewMemoFunc(cx, live.Func(a, a.MediaBrowseCrumbs), slices.Equal[[]string])
	path := core.Dynamic(func(cx *core.Scope) core.Node {
		items := crumbs.Get()
		return ui.Breadcrumbs{TestID: "media-sources-path", Items: items, OnSelect: live.On(a, a.OpenMediaBrowseCrumb), Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}}
	})
	// The button shows while videos or folders are chosen, and its words count them; with folders it lists their videos first, and says so meanwhile.
	count := core.NewMemo(cx, chosen)
	folders := live.Func(a, a.MediaBrowseFoldersChosen)
	expanding := live.Func(a, a.MediaBrowseExpanding)
	// With a way of importing remembered the button says it and imports that way at once, and 更改方式… beside it asks again.
	way := live.Func(a, a.MediaBrowseImportWay)
	importButton := core.Show{When: func() bool { return count.Get() > 0 }, Then: func(cx *core.Scope) core.Node {
		button := ui.Button{TestID: "media-sources-import", TextFunc: func() string {
			text := locales.MediaSourceImportSome(cx, count.Get())
			switch {
			case expanding():
				return locales.MediaSourceListingFolders(cx)
			case folders():
				text = locales.MediaSourceImportChosen(cx, count.Get())
			}
			if w := way(); w != "" {
				return locales.MediaSourceImportWay(cx, text, w)
			}
			return text
		}, Disabled: expanding, OnPress: live.Do(a, a.ImportMediaBrowseChosen)}
		change := core.Show{When: func() bool { return way() != "" }, Then: func(cx *core.Scope) core.Node {
			return ui.Button{TestID: "media-sources-import-way-change", Text: locales.MediaSourceChangeWay(cx), Variant: ui.ButtonGhost, Size: ui.SizeSmall, Disabled: expanding,
				OnPress: live.Do(a, a.AskMediaBrowseImportWay)}
		}}
		return core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: []core.Node{change, button}}
	}}
	// 全选 chooses every folder and video of the folder that can be, or none once all are, half checked while some are.
	choice := core.NewMemo(cx, live.Func(a, func() [2]int { c, n := a.MediaBrowseChoice(); return [2]int{c, n} }))
	all := core.Show{When: func() bool { return choice.Get()[1] > 0 }, Then: func(cx *core.Scope) core.Node {
		return ui.Checkbox{TestID: "media-sources-choose-all", Text: locales.MediaSourceChooseAll(cx),
			Checked:       func() bool { c := choice.Get(); return c[0] > 0 && c[0] == c[1] },
			Indeterminate: func() bool { c := choice.Get(); return c[0] > 0 && c[0] < c[1] },
			Disabled:      expanding,
			OnChange:      live.On(a, func(bool) { a.ToggleMediaBrowseAll() }),
			Style:         core.Style{FlexShrink: 0, Padding: core.Edges{Left: 14}}}
	}}
	// The filter shows while the folder holds anything, and narrows its rows to the names holding its words.
	listed := live.Func(a, a.MediaBrowseListed)
	query := live.Func(a, a.MediaBrowseQuery)
	filter := core.Show{When: func() bool { return listed() > 0 }, Then: func(cx *core.Scope) core.Node {
		return ui.Input{TestID: "media-sources-filter", Label: locales.MediaSourceFilter(cx), Placeholder: locales.MediaSourceFilter(cx),
			NoAutoCorrect: true, AutoCapitalize: core.AutoCapitalizeNone,
			Value: query, OnChange: live.On(a, a.SetMediaBrowseQuery), Style: core.Style{Width: core.Pt(180), FlexShrink: 1}}
	}}
	bar := core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, FlexShrink: 0, MinHeight: core.Pt(32)}, Children: []core.Node{all, path, filter, importButton}}
	note := live.Func(a, a.MediaBrowseNote)
	noteLine := core.Show{When: func() bool { return note() != "" }, Then: func(*core.Scope) core.Node {
		return core.Text{TestID: "media-sources-note", ContentFunc: note, Style: core.Style{FontSize: 12, LineHeight: 17, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Muted)}
	}}
	loading := live.Func(a, a.MediaBrowseLoading)
	failed := live.Func(a, a.MediaBrowseErr)
	entries := core.NewMemoFunc(cx, live.Func(a, a.MediaBrowseEntries), slices.Equal[[]Entry])
	body := core.Show{When: loading, Then: func(cx *core.Scope) core.Node {
		return said(cx, "media-sources-loading", locales.MediaSourceListing(cx))
	}, Else: func(cx *core.Scope) core.Node {
		return core.Show{When: func() bool { return failed() != "" }, Then: func(cx *core.Scope) core.Node { return failure(cx, a, failed) }, Else: func(cx *core.Scope) core.Node {
			return core.Show{When: func() bool { return len(entries.Get()) == 0 }, Then: func(cx *core.Scope) core.Node {
				return core.Show{When: func() bool { return listed() > 0 }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "media-sources-no-match", ContentFunc: func() string { return locales.MediaSourceNoMatch(cx, query()) },
						Style: core.Style{FontSize: 13, LineHeight: 18, Padding: core.Symmetric(10, 4)}, StyleFunc: ui.Colored(colors.Muted)}
				}, Else: func(cx *core.Scope) core.Node { return said(cx, "media-sources-empty", locales.MediaSourceEmpty(cx)) }}
			}, Else: func(cx *core.Scope) core.Node { return rows(cx, a, entries.Get) }}
		}}
	}}
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 10, AlignItems: core.AlignStretch},
		Children: []core.Node{bar, noteLine, body}}
}

// said is a line the folder shows in place of its rows.
func said(cx *core.Scope, test, text string) core.Node {
	return core.Text{TestID: test, Content: text, Style: core.Style{FontSize: 13, LineHeight: 18, Padding: core.Symmetric(10, 4)}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)}
}

// failure is why the folder could not be listed, with 重试 and 编辑来源….
func failure(cx *core.Scope, a App, why func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignStart, Padding: core.Symmetric(10, 4)}, Children: []core.Node{
		core.Text{TestID: "media-sources-error", ContentFunc: why, Selectable: core.SelectableOn, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Danger)},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			ui.Button{TestID: "media-sources-retry", Text: locales.MediaSourceRetry(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: live.Do(a, a.RetryMediaBrowse)},
			ui.Button{TestID: "media-sources-edit", Text: locales.MediaSourceEditSource(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: live.Do(a, a.EditMediaBrowseSource)},
		}},
	}}
}

// rows are the folder's rows the filter keeps on a card, which scrolls.
func rows(cx *core.Scope, a App, entries func() []Entry) core.Node {
	k := theme.Of(cx)
	list := core.Each(entries, func(e Entry) string { return e.Path }, func(cx *core.Scope, e func() Entry, index func() int) core.Node {
		return core.View{Style: core.Style{AlignItems: core.AlignStretch, FlexShrink: 0}, Children: []core.Node{
			core.Show{When: func() bool { return index() > 0 }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Height: core.Pt(1), FlexShrink: 0, Margin: core.Edges{Left: 44}}, StyleFunc: func() core.Style { return core.Style{Background: k.Shade(0.08)} }}
			}},
			row(cx, a, e),
		}}
	})
	card := core.View{TestID: "media-sources-rows", Style: core.Style{AlignItems: core.AlignStretch, CornerRadius: 12, FlexShrink: 0},
		StyleFunc: func() core.Style {
			return core.Style{Background: k.Card(), Shadows: []core.BoxShadow{{Spread: 0.5, Color: k.Shade(0.07)}}}
		}, Children: []core.Node{list}}
	return core.Scroll{TestID: "media-sources-folder", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{core.View{Style: core.Style{AlignItems: core.AlignStretch, Padding: core.Edges{Bottom: 20}}, Children: []core.Node{card}}}}
}

// row is a folder or a video of the folder: a folder or a share opens with a press, has a check box that chooses it to import whole, says how many videos below it are in the library, and ends with the bell that follows it, lit while it is followed; a video has a check box that chooses it, its tags, its size and its time, and one already imported opens its film with a press instead.
func row(cx *core.Scope, a App, e func() Entry) core.Node {
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	path := e().Path
	opens := func() bool { x := e(); return x.Dir || x.InLibrary }
	// A press with Shift chooses the rows from the one chosen last to this one.
	choose := func(press core.PressEvent) {
		live.Do(a, func() {
			if press.Modifiers.Shift {
				a.ExtendMediaBrowseChoice(path)
			} else {
				a.ToggleMediaBrowseEntry(path)
			}
		})()
	}
	icon := func() *core.SVG {
		switch x := e(); {
		case x.Share:
			return lucide.HardDrive()
		case x.Dir:
			return lucide.Folder()
		}
		return lucide.Film()
	}
	name := core.Text{ContentFunc: func() string { return e().Name }, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle,
		Style: core.Style{FontSize: 13, LineHeight: 18, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Text)}
	// A folder, and a video not imported yet, is chosen with its check box, whose label is its name; a press on the rest of a folder's row opens it.
	lead := core.Show{When: func() bool { return !e().Choosable }, Then: func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(18), FlexShrink: 0}}
	}, Else: func(cx *core.Scope) core.Node {
		return ui.Checkbox{TestID: "media-sources-choose-" + path, Label: locales.MediaSourceSelect(cx, e().Name),
			Checked: func() bool { return e().Chosen }, OnChangeEvent: func(_ bool, press core.PressEvent) { choose(press) }}
	}}
	tag := func(test string, on func() bool, text string) core.Node {
		return core.Show{When: on, Then: func(*core.Scope) core.Node {
			return core.Text{TestID: test + path, Content: text, MaxLines: 1, Role: core.RoleNone,
				Style:     core.Style{FontSize: 10, FontWeight: ui.Bold, LineHeight: 16, CornerRadius: 8, Padding: core.Symmetric(0, 7), FlexShrink: 0},
				StyleFunc: func() core.Style { return core.Style{Background: k.Shade(0.07), Color: colors.Muted()} }}
		}}
	}
	detail := func(text func() string, width core.Points) core.Node {
		return core.Text{ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 17, Width: core.Pt(width), FlexShrink: 0, TextAlign: core.TextAlignRight},
			StyleFunc: ui.Colored(colors.Muted)}
	}
	return core.View{
		TestID: "media-sources-entry-" + path,
		Role:   core.RoleButton,
		LabelFunc: func() string {
			x := e()
			parts := []string{x.Name}
			if x.Subtitles {
				parts = append(parts, locales.MediaSourceHasSubtitles(cx))
			}
			if x.InLibrary {
				parts = append(parts, locales.MediaSourceInLibrary(cx))
			}
			if x.Imported > 0 {
				parts = append(parts, locales.MediaSourceImportedBelow(cx, x.Imported))
			}
			return strings.Join(parts, ", ")
		},
		OnPress: func(press core.PressEvent) {
			if opens() {
				live.Do(a, func() { a.OpenMediaBrowseEntry(path) })()
			} else {
				choose(press)
			}
		},
		Style: core.Style{Direction: core.Row, MinHeight: core.Pt(40), Padding: core.Symmetric(6, 14), Gap: 10, AlignItems: core.AlignCenter, FlexShrink: 0},
		StyleFunc: func() core.Style {
			return core.Style{Hover: &core.Style{Background: k.Shade(0.04)}}
		},
		Children: []core.Node{
			lead,
			ui.Icon(icon, 16, k.Glyph),
			name,
			tag("media-sources-subtitles-", func() bool { return e().Subtitles }, locales.MediaSourceHasSubtitles(cx)),
			tag("media-sources-in-library-", func() bool { return e().InLibrary }, locales.MediaSourceInLibrary(cx)),
			core.Show{When: func() bool { return e().Imported > 0 }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "media-sources-imported-" + path, ContentFunc: func() string { return locales.MediaSourceImportedBelow(cx, e().Imported) }, MaxLines: 1, Role: core.RoleNone,
					Style:     core.Style{FontSize: 10, FontWeight: ui.Bold, LineHeight: 16, CornerRadius: 8, Padding: core.Symmetric(0, 7), FlexShrink: 0},
					StyleFunc: func() core.Style { return core.Style{Background: k.Shade(0.07), Color: colors.Muted()} }}
			}},
			core.Show{When: func() bool { return e().Dir }, Then: func(cx *core.Scope) core.Node {
				follow := func() string {
					if x := e(); x.Followed {
						return locales.MediaSourceUnfollow(cx, x.Name)
					}
					return locales.MediaSourceFollow(cx, e().Name)
				}
				return ui.IconButton{TestID: "media-sources-follow-" + path, LabelFunc: follow, Tooltip: follow,
					Icon: func() *core.SVG {
						if e().Followed {
							return lucide.BellRing()
						}
						return lucide.BellPlus()
					},
					Selected: func() bool { return e().Followed },
					Tint: func() core.Color {
						if e().Followed {
							return colors.Accent()
						}
						return colors.Faint()
					},
					OnPress: live.Do(a, func() { a.FollowMediaBrowseEntry(path) })}
			}},
			detail(func() string {
				if x := e(); !x.Dir {
					return ByteSize(x.Size)
				}
				return ""
			}, 70),
			detail(func() string {
				if x := e(); !x.Dir && !x.Modified.IsZero() {
					return x.Modified.Local().Format("2006-01-02 15:04")
				}
				return ""
			}, 130),
		},
	}
}

// ImportDialog is the dialog over the window that imports the videos chosen on the page: how many and from where; whether they are copied into the library or only indexed and played from the source, with what each way means, the copies' room or that the source must be reachable; whether that way is remembered; with 取消 and 导入.
type ImportDialog struct{ App App }

// Render implements core.Node.
func (d ImportDialog) Render(cx *core.Scope) core.Node {
	a := d.App
	in := core.NewMemo(cx, live.Func(a, a.MediaBrowseImport))
	cancel := live.Do(a, a.CancelMediaBrowseImport)
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { return in.Get().Open }, Then: func(cx *core.Scope) core.Node {
		// Its words are read as it is built, so it is built again when the interface's language changes.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			_ = i18n.Language(cx)
			x := core.Untracked(cx, in.Get)
			choose := live.On(a, a.ChooseMediaBrowseImportLink)
			ways := ui.RadioGroup{Label: locales.MediaSourceImportTitle(cx, x.Count, x.Source), TestID: "media-sources-import-way", Children: []core.Node{
				ui.Radio{Text: locales.MediaSourceImportCopy(cx), TestID: "media-sources-import-copy", Checked: func() bool { return !in.Get().Link }, OnChange: func(bool) { choose(false) }},
				ui.Radio{Text: locales.MediaSourceImportLink(cx), TestID: "media-sources-import-link", Checked: func() bool { return in.Get().Link }, OnChange: func(bool) { choose(true) }},
			}}
			note := core.Text{TestID: "media-sources-import-note", Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted),
				ContentFunc: func() string {
					if x := in.Get(); x.Link {
						return locales.MediaSourceImportLinkNote(cx, x.Source)
					}
					return locales.MediaSourceImportCopyNote(cx, ByteSize(in.Get().Size))
				}}
			remember := ui.Checkbox{Text: locales.MediaSourceImportRemember(cx), TestID: "media-sources-import-remember", Checked: func() bool { return in.Get().Remember },
				OnChange: live.On(a, a.RememberMediaBrowseImport)}
			return ui.Dialog{TestID: "media-sources-import-dialog", Open: func() bool { return true }, NoCloseOnBackdropPress: true,
				Title: locales.MediaSourceImportTitle(cx, x.Count, x.Source),
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Style:    core.Style{Width: core.Pt(460)},
				Children: []core.Node{core.View{Style: core.Style{Gap: 12}, Children: []core.Node{ways, note, remember}}},
				Footer: []core.Node{
					ui.Button{Text: locales.MediaSourceCancel(cx), Variant: ui.ButtonSecondary, TestID: "media-sources-import-cancel", OnPress: cancel},
					ui.Button{Text: locales.MediaSourceImportStart(cx), TestID: "media-sources-import-start", OnPress: live.Do(a, a.StartMediaBrowseImport)},
				}}
		})
	}}
}

// ByteSize writes n bytes as the size a person reads: in KB, MB or GB, at least 1 KB.
func ByteSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.0f KB", max(1, float64(n)/(1<<10)))
}
