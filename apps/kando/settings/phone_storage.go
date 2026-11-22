package settings

import (
	"slices"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// PhoneStorage is what the films shared with the phones through the storage take there, as the 存储用量 card of the 手机同步 page shows it (yldm-tech/Kando#219): the computer puts each film's lines and poster in the storage, and its video unless the relay goes as subtitles only, so a phone away from this network streams or downloads them.
type PhoneStorage struct {
	// Measured is a listing of the storage that went through, which Videos, Lines and Mail measure in bytes: the videos, the lines and posters, and the phones' messages.
	Measured            bool
	Videos, Lines, Mail int64
	// Films is how many films are in the storage, WithVideo how many with their video.
	Films, WithVideo int
	// Uploading is the title of the film whose video goes up now, "" for none, Percent how far it is, and Waiting how many videos are still to go up, that one among them.
	Uploading string
	Percent   int
	Waiting   int
	// Err is why the last upload of the films failed, in the interface's language, "" for none.
	Err string
	// CleanDays is how many days after it was watched to its end a film's video is deleted from the storage, 0 for never (PhoneCleanDays).
	CleanDays int
	// Watched are the films whose video is in the storage and which were watched to their end, the longest ago first; Cleaning is a cleanup under way, and CleanErr why the last one failed, "" for none.
	Watched  []PhoneWatched
	Cleaning bool
	CleanErr string
	// Targets are the other storages set up in 账号与上传, which 换一个存储 moves to.
	Targets []PhoneStore
	// Moving is a move to the storage named MoveTo under way, from the one named MoveFrom, with Done of Total videos put there; Moved is one that is done. MoveErr is why the last move failed, "" for none.
	Moving, Moved    bool
	MoveTo, MoveFrom string
	Done, Total      int
	MoveErr          string
}

// PhoneWatched is a film whose video is in the storage and was watched to its end: its library's ID and its ID, its title, its video's size in bytes, and when it was last watched to its end, zero when that is not known.
type PhoneWatched struct {
	Library, ID, Title string
	Size               int64
	At                 time.Time
}

// PhoneStore is a storage set up in 账号与上传: its kind, as settings.json names it, and its name as the page shows it.
type PhoneStore struct {
	Kind, Name string
}

// PhoneCleanDays are the choices of 自动清理存储里的视频: never, then a month and a week after a film was watched to its end.
var PhoneCleanDays = []int{0, 30, 7}

// storageCard is the 存储用量 card, under the relay's while it is on with a storage set up: how much the films and the messages take, with the video uploading now; whether watched videos are cleaned up, and those that can be; and 换一个存储, which moves the films and the phones to another storage.
func storageCard(cx *core.Scope, a App) core.Node {
	colors := ui.ColorsOf(cx)
	k := theme.Of(cx)
	storage := func() PhoneStorage { return a.PhoneSync().Relay.Storage }
	measured := live.Func(a, func() bool { return storage().Measured })
	total := live.Func(a, func() string {
		if !measured() {
			return locales.SettingsPhoneStorageMeasuring(cx)
		}
		st := storage()
		return byteSize(st.Videos + st.Lines + st.Mail)
	})
	count := live.Func(a, func() string {
		st := storage()
		return locales.SettingsPhoneStorageFilms(cx, st.Films, st.WithVideo)
	})
	head := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, JustifyContent: core.JustifySpaceBetween, Gap: 10}, Children: []core.Node{
		core.Text{TestID: "settings-phone-storage-total", ContentFunc: total, Style: core.Style{FontSize: 20, LineHeight: 26, FontWeight: ui.Bold}, StyleFunc: ui.Colored(colors.Text)},
		core.Text{TestID: "settings-phone-storage-count", ContentFunc: count, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)},
	}}
	// The bar is the videos, the lines and posters and the messages, side by side in proportion; the two small ones keep a sliver so they show.
	part := func(size func(PhoneStorage) int64, min core.Points) func() core.Style {
		return func() core.Style {
			st := storage()
			sum := st.Videos + st.Lines + st.Mail
			if sum <= 0 || !st.Measured {
				return core.Style{Width: core.Pt(0)}
			}
			return core.Style{Width: core.Pct(float32(size(st)) * 100 / float32(sum)), MinWidth: core.Pt(min)}
		}
	}
	segment := func(color core.Color, size func(PhoneStorage) int64, min core.Points) core.Node {
		return core.View{Style: core.Style{Height: core.Pct(100), Background: color}, StyleFunc: part(size, min)}
	}
	bar := core.View{TestID: "settings-phone-storage-bar", Role: core.RoleImage,
		LabelFunc: live.Func(a, func() string {
			st := storage()
			return locales.SettingsPhoneStorageBar(cx, byteSize(st.Videos), byteSize(st.Lines), byteSize(st.Mail))
		}),
		Style: core.Style{Direction: core.Row, Height: core.Pt(8), Margin: core.Edges{Top: 8}, CornerRadius: 4, Overflow: core.Hidden}, StyleFunc: ui.Filled(func() core.Color { return k.Shade(0.08) }),
		Children: []core.Node{
			segment(theme.Accent, func(st PhoneStorage) int64 { return st.Videos }, 0),
			segment(theme.StatusOK, func(st PhoneStorage) int64 { return st.Lines }, 3),
			segment(theme.StatusWarn, func(st PhoneStorage) int64 { return st.Mail }, 3),
		}}
	key := func(color core.Color, text func() string, id string) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, Gap: 5, AlignItems: core.AlignCenter}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 2, Background: color}},
			core.Text{TestID: id, ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Text)},
		}}
	}
	legend := core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 4, ColumnGap: 14, Margin: core.Edges{Top: 8}}, Children: []core.Node{
		key(theme.Accent, live.Func(a, func() string { return locales.SettingsPhoneStorageVideos(cx, byteSize(storage().Videos)) }), "settings-phone-storage-videos"),
		key(theme.StatusOK, live.Func(a, func() string { return locales.SettingsPhoneStorageLines(cx, byteSize(storage().Lines)) }), "settings-phone-storage-lines"),
		key(theme.StatusWarn, live.Func(a, func() string { return locales.SettingsPhoneStorageMail(cx, byteSize(storage().Mail)) }), "settings-phone-storage-mail"),
	}}
	uploading := core.Show{When: live.Func(a, func() bool { return storage().Uploading != "" || storage().Err != "" }), Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "settings-phone-storage-uploading", Selectable: core.SelectableOn, LiveRegion: core.LiveRegionPolite,
			ContentFunc: live.Func(a, func() string {
				st := storage()
				switch {
				case st.Uploading != "" && st.Waiting > 1:
					return locales.SettingsPhoneStorageUploadingMore(cx, st.Uploading, st.Percent, st.Waiting-1)
				case st.Uploading != "":
					return locales.SettingsPhoneStorageUploading(cx, st.Uploading, st.Percent)
				}
				return locales.SettingsPhoneStorageFailed(cx, st.Err)
			}),
			Style: core.Style{FontSize: 11, LineHeight: 15, Margin: core.Edges{Top: 8}},
			StyleFunc: live.Func(a, func() core.Style {
				if storage().Uploading == "" {
					return core.Style{Color: colors.Danger()}
				}
				return core.Style{Color: colors.Muted()}
			})}
	}}
	usage := core.View{Style: core.Style{Padding: core.Symmetric(12, 14)}, Children: []core.Node{head, bar, legend, uploading}}
	return form.Card(usage, form.Separator(true), cleanRow(cx, a), form.Separator(true), watchedList(cx, a), form.Separator(true), moveRow(cx, a))
}

// cleanRow is 自动清理存储里的视频: never, or a month or a week after a film was watched to its end.
func cleanRow(cx *core.Scope, a App) core.Node {
	options := make([]ui.Option[int], len(PhoneCleanDays))
	for i, days := range PhoneCleanDays {
		label := locales.SettingsPhoneStorageNever(cx)
		if days > 0 {
			label = locales.SettingsPhoneStorageAfterDays(cx, days)
		}
		options[i] = ui.Option[int]{Value: days, Label: label}
	}
	choose := ui.Select[int]{Options: options, Label: locales.SettingsPhoneStorageAutoClean(cx), TestID: "settings-phone-storage-auto-clean",
		Value: live.Func(a, func() int { return a.PhoneSync().Relay.Storage.CleanDays }), OnChange: live.On(a, a.SetPhoneRelayClean), Style: core.Style{Width: core.Pt(180)}}
	return form.Row{Title: locales.SettingsPhoneStorageAutoClean(cx), Subtitle: locales.SettingsPhoneStorageAutoCleanHint(cx), NoGroup: true, Trailing: []core.Node{choose}}
}

// watchedList is 看完了、可以清理的视频: each film whose video is in the storage and was watched to its end, with when, its video's size and 清理, and 全部清理 for them all; or that there is none.
func watchedList(cx *core.Scope, a App) core.Node {
	colors := ui.ColorsOf(cx)
	watched := core.NewMemoFunc(cx, live.Func(a, func() []PhoneWatched { return a.PhoneSync().Relay.Storage.Watched }), slices.Equal[[]PhoneWatched])
	busy := live.Func(a, func() bool { return a.PhoneSync().Relay.Storage.Cleaning })
	all := core.Show{When: func() bool { return len(watched.Get()) > 0 }, Then: func(cx *core.Scope) core.Node {
		return form.Button{TestID: "settings-phone-storage-clean-all", Disabled: busy,
			TextFunc: func() string {
				var sum int64
				for _, w := range watched.Get() {
					sum += w.Size
				}
				return locales.SettingsPhoneStorageCleanAll(cx, byteSize(sum))
			},
			OnPress: live.Do(a, func() { a.CleanPhoneVideos(watched.GetUntracked()) })}
	}}
	head := form.Row{Title: locales.SettingsPhoneStorageWatched(cx), NoGroup: true, Trailing: []core.Node{all}}
	rows := core.Each(watched.Get, func(w PhoneWatched) string { return w.Library + "/" + w.ID }, func(cx *core.Scope, w func() PhoneWatched, _ func() int) core.Node {
		film := w()
		meta := live.Func(a, func() string {
			if w().At.IsZero() {
				return locales.SettingsPhoneStorageWatchedSome(cx)
			}
			return locales.SettingsPhoneStorageWatchedAgo(cx, ago(cx, w().At, a.PhoneNow()))
		})
		return core.View{TestID: "settings-phone-storage-watched-" + film.ID, Role: core.RoleGroup, Label: film.Title,
			Style: core.Style{Direction: core.Row, Wrap: core.Wrap, MinHeight: core.Pt(36), Padding: core.Edges{Top: 4, Right: 14, Bottom: 4, Left: 28}, Gap: 10, AlignItems: core.AlignCenter},
			Children: []core.Node{
				core.Text{Content: film.Title, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 12, LineHeight: 17, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(180), MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: meta, Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Muted)},
				core.Text{ContentFunc: func() string { return byteSize(w().Size) }, Style: core.Style{FontSize: 12, LineHeight: 17, Width: core.Pt(64), TextAlign: core.TextAlignRight}, StyleFunc: ui.Colored(colors.Muted)},
				ui.Button{Text: locales.SettingsPhoneStorageClean(cx), Label: locales.SettingsPhoneStorageCleanLabel(cx, film.Title), Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
					TestID: "settings-phone-storage-clean-" + film.ID, Disabled: busy, OnPress: live.Do(a, func() { a.CleanPhoneVideos([]PhoneWatched{w()}) })},
			}}
	})
	none := core.Show{When: func() bool { return len(watched.Get()) == 0 }, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "settings-phone-storage-none", Content: locales.SettingsPhoneStorageNothing(cx), Style: core.Style{FontSize: 12, LineHeight: 17, Padding: core.Edges{Right: 14, Bottom: 10, Left: 28}}, StyleFunc: ui.Colored(colors.Muted)}
	}}
	failed := core.Show{When: live.Func(a, func() bool { return a.PhoneSync().Relay.Storage.CleanErr != "" }), Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "settings-phone-storage-clean-failed", Selectable: core.SelectableOn,
			ContentFunc: live.Func(a, func() string {
				return locales.SettingsPhoneStorageCleanFailed(cx, a.PhoneSync().Relay.Storage.CleanErr)
			}),
			Style: core.Style{FontSize: 11, LineHeight: 15, Padding: core.Edges{Right: 14, Bottom: 10, Left: 28}}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	return core.Group{head, rows, none, failed}
}

// moveRow is 换一个存储: the other storages set up in 账号与上传 and 迁移…, which asks first under the row, then how the move goes; with no other storage, it says to set one up.
func moveRow(cx *core.Scope, a App) core.Node {
	colors := ui.ColorsOf(cx)
	storage := func() PhoneStorage { return a.PhoneSync().Relay.Storage }
	targets := core.NewMemoFunc(cx, live.Func(a, func() []PhoneStore { return storage().Targets }), slices.Equal[[]PhoneStore])
	picked := core.NewSignal(cx, "")
	asking := core.NewSignal(cx, false)
	target := func() PhoneStore {
		list := targets.Get()
		if i := slices.IndexFunc(list, func(s PhoneStore) bool { return s.Kind == picked.Get() }); i >= 0 {
			return list[i]
		}
		if len(list) > 0 {
			return list[0]
		}
		return PhoneStore{}
	}
	busy := live.Func(a, func() bool { return storage().Moving })
	options := func() []ui.Option[string] {
		list := targets.Get()
		options := make([]ui.Option[string], len(list))
		for i, s := range list {
			options[i] = ui.Option[string]{Value: s.Kind, Label: s.Name}
		}
		return options
	}
	// The drop-down follows the storages offered in place, keeping its focus; it gives way to a note while none is.
	choose := core.Show{
		When: func() bool { return len(targets.Get()) > 0 },
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
				ui.Select[string]{OptionsFunc: options, LabelFunc: func() string { return locales.SettingsPhoneStorageMoveTo(cx) }, TestID: "settings-phone-storage-move-to",
					Value: func() string { return target().Kind }, OnChange: func(kind string) { picked.Set(kind) }, Style: core.Style{Width: core.Pt(130)}},
				ui.Button{TextFunc: func() string { return locales.SettingsPhoneStorageMoveButton(cx) }, Size: ui.SizeSmall, TestID: "settings-phone-storage-move", Disabled: busy, OnPress: func() { asking.Set(true) }},
			}}
		},
		Else: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "settings-phone-storage-no-target", ContentFunc: func() string { return locales.SettingsPhoneStorageNoTarget(cx) }, Style: core.Style{FontSize: 12, LineHeight: 17, MaxWidth: core.Pt(220)}, StyleFunc: ui.Colored(colors.Muted)}
		},
	}
	row := form.Row{Title: locales.SettingsPhoneStorageMove(cx), Subtitle: locales.SettingsPhoneStorageMoveHint(cx), NoGroup: true, Trailing: []core.Node{choose}}
	ask := core.Show{When: func() bool { return asking.Get() && target().Kind != "" }, Then: func(cx *core.Scope) core.Node {
		k := theme.Of(cx)
		to := target()
		from := live.Func(a, func() string { return a.PhoneSync().Relay.Store })
		done := func() { asking.Set(false) }
		return core.View{Role: core.RoleGroup, Label: locales.SettingsPhoneStorageMoveAsk(cx, to.Name), TestID: "settings-phone-storage-move-ask",
			Style:     core.Style{FlexShrink: 0, Gap: 4, Margin: core.Edges{Right: 14, Bottom: 10, Left: 14}, Padding: core.Symmetric(10, 12), CornerRadius: 10},
			StyleFunc: ui.Filled(func() core.Color { return k.Shade(0.04) }),
			Children: []core.Node{
				core.Text{Content: locales.SettingsPhoneStorageMoveAsk(cx, to.Name), Role: core.RoleHeader, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: func() string { return locales.SettingsPhoneStorageMoveAskBody(cx, to.Name, from()) }, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Text)},
				core.View{Style: core.Style{Direction: core.Row, Gap: 8, Margin: core.Edges{Top: 4}}, Children: []core.Node{
					ui.Button{Text: locales.SettingsPhoneStorageMoveConfirm(cx), Size: ui.SizeSmall, TestID: "settings-phone-storage-move-confirm",
						OnPress: live.Do(a, func() {
							done()
							a.MovePhoneStorage(to.Kind)
						})},
					ui.Button{Text: locales.SettingsPhoneStorageMoveCancel(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "settings-phone-storage-move-cancel", OnPress: done},
				}},
			}}
	}}
	progress := core.Show{When: live.Func(a, func() bool { st := storage(); return st.Moving || st.Moved || st.MoveErr != "" }), Then: func(cx *core.Scope) core.Node {
		text := live.Func(a, func() string {
			st := storage()
			switch {
			case st.MoveErr != "":
				return locales.SettingsPhoneStorageMoveFailed(cx, st.MoveErr)
			case st.Moved:
				return locales.SettingsPhoneStorageMoved(cx, st.MoveTo, st.MoveFrom)
			case st.Total > 0:
				return locales.SettingsPhoneStorageMovingCount(cx, st.MoveTo, st.Done, st.Total)
			}
			return locales.SettingsPhoneStorageMoving(cx, st.MoveTo)
		})
		children := []core.Node{core.Text{TestID: "settings-phone-storage-move-status", ContentFunc: text, Selectable: core.SelectableOn, LiveRegion: core.LiveRegionPolite,
			Style: core.Style{FontSize: 11, LineHeight: 15},
			StyleFunc: live.Func(a, func() core.Style {
				if storage().MoveErr != "" {
					return core.Style{Color: colors.Danger()}
				}
				return core.Style{Color: colors.Muted()}
			})}}
		children = append(children, core.Show{When: busy, Then: func(cx *core.Scope) core.Node {
			return ui.Progress{TestID: "settings-phone-storage-move-progress", LabelFunc: func() string { return locales.SettingsPhoneStorageMove(cx) }, Value: live.Func(a, func() float32 {
				st := storage()
				if st.Total <= 0 {
					return 0
				}
				return float32(st.Done) * 100 / float32(st.Total)
			})}
		}})
		return core.View{Style: core.Style{Gap: 5, Padding: core.Edges{Right: 14, Bottom: 12, Left: 14}}, Children: children}
	}}
	return core.Group{row, ask, progress}
}
