package films

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// removeLook is what the confirmation of 从资料库移除 says: the film's name, and which of its files go to the Trash; or, for several films, how many they are (count).
type removeLook struct {
	name            string
	outside, folder bool
	count           int
	// stored names the storage location the film's video is on, whose copy there is deleted too (Film.Stored), "" for a video in the library's folder; of several films, one of those whose video is on one.
	stored string
}

// removeDialog confirms 从资料库移除 before the film's folder, or only its video and subtitles, goes to the Trash, or those of each of several films selected. 取消 is the default answer, so Enter keeps the film, as do Escape and closing the dialog.
type removeDialog struct{ a App }

// Render implements core.Node.
func (r removeDialog) Render(cx *core.Scope) core.Node {
	a := r.a
	colors := ui.ColorsOf(cx)
	open := live.Func(a, func() bool {
		open, _, _ := a.Removing()
		return *open
	})
	// The confirmation is built again only when what it says changes, not on every change of the app's state.
	look := live.Memo(cx, a, func() removeLook {
		_, items, folder := a.Removing()
		if len(items) != 1 {
			l := removeLook{count: len(items)}
			for _, item := range items {
				if place := a.StoredPlace(item); place != "" {
					l.stored = place
				}
			}
			return l
		}
		item := items[0]
		return removeLook{name: filmName(item), outside: !InDir(a.LibraryDir(), item.VideoPath), folder: folder, count: 1, stored: a.StoredPlace(item)}
	})
	cancel := live.Do(a, a.CloseRemove)
	trash := live.Do(a, func() {
		open, items, _ := a.Removing()
		if *open {
			a.TrashFilms(items...)
		}
	})
	return core.Show{When: open, Then: func(cx *core.Scope) core.Node {
		// Enter answers 取消, the default answer, wherever the keyboard focus is in the dialog; the binding goes when the dialog does.
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways, OverModals: true}, cancel)
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Dialog and ui.AlertDialog TitleFunc and DescriptionFunc twins so the words follow the film and the language without building the dialog again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			l := look()
			title, message := locales.FilmsRemoveConfirm(cx, l.name), locales.FilmsRemoveFiles(cx)
			switch {
			case l.count > 1 && l.stored != "":
				title, message = locales.FilmsRemoveConfirmMany(cx, l.count), locales.FilmsRemoveManyStored(cx)
			case l.count > 1:
				title, message = locales.FilmsRemoveConfirmMany(cx, l.count), locales.FilmsRemoveManyMessage(cx)
			case l.stored != "":
				message = locales.FilmsRemoveStored(cx, l.stored)
			case l.outside:
				message = locales.FilmsRemoveOutside(cx)
			case l.folder:
				message = locales.FilmsRemoveFolder(cx)
			}
			return ui.Dialog{
				TestID:                 "remove-confirm",
				Role:                   core.RoleAlertDialog,
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				Title:                  title,
				Description:            message,
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Footer: []core.Node{
					ui.Button{Text: locales.FilmsCancel(cx), Variant: ui.ButtonSecondary, TestID: "remove-cancel", OnPress: cancel},
					ui.Button{Text: locales.FilmsRemove(cx), Color: colors.Danger, TestID: "remove-confirm-trash", OnPress: trash},
				},
			}
		})
	}}
}
