package films

import (
	"errors"
	"strconv"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// folderFills are the libraries' folder tiles in the sheet, a color each in turn.
var folderFills = []core.Color{theme.TileBlue, theme.TileOrange, theme.TileGreen, theme.TileViolet, theme.TileTeal}

// LibrarySheet is the sheet that picks the library the Library shows into library, or every library, while open reports true: the switcher's and a tablet's sidebar header's. Each library is a row with its folder (or lock for a password-protected library that is locked), its name and its computer, the one shown checked; picking a locked library opens the unlock password dialog and switches directly into it once unlocked; an unlocked password-protected library also offers a Lock button.
func LibrarySheet(cx *core.Scope, s *session.Session, library *core.Signal[catalog.Scope], open *core.Signal[bool]) core.Node {
	colors := ui.ColorsOf(cx)
	libs := core.NewMemoFunc(cx, func() []catalog.Scope { return catalog.Libraries(s.Computers(), s.Entries()) }, nil)
	unlocking := core.NewSignal(cx, catalog.Scope{})
	password := core.NewSignal(cx, "")
	busy := core.NewSignal(cx, false)
	problem := core.NewSignal(cx, "")
	pick := func(lib catalog.Scope) {
		if lib.Locked {
			open.Set(false)
			password.Set("")
			problem.Set("")
			busy.Set(false)
			unlocking.Set(lib)
			return
		}
		library.Set(lib)
		open.Set(false)
	}
	same := func(a, b catalog.Scope) bool { return a.Desktop == b.Desktop && a.ID == b.ID && a.Name == b.Name }
	check := func(on func() bool) core.Node {
		return core.Show{When: on, Then: func(cx *core.Scope) core.Node { return ui.Icon(lucide.Check, 16, colors.Accent) }}
	}
	lockLib := func(lib catalog.Scope) {
		s.LockLibrary(lib.Desktop, lib.ID, func(err error) {
			if err == nil && same(library.Get(), lib) {
				library.Set(catalog.Scope{})
			}
		})
	}
	all := look.Row{TestID: "films-library-option-0", Index: func() int { return 0 }, MinHeight: 56,
		Leading: libraryTile(lucide.Library, colors.Accent, colors.OnAccent),
		Title:   func() string { return locales.FilmsAllLibraries(cx) }, TitleStyle: core.Style{FontWeight: core.FontWeightSemibold},
		Checked: func() bool { return library.Get().Desktop == "" }, Trailing: check(func() bool { return library.Get().Desktop == "" }),
		OnPress: func() { pick(catalog.Scope{}) }}
	sheet := look.Dialog{TestID: "films-library-sheet", Open: open.Get, OnClose: func() { open.Set(false) }, Title: func() string { return locales.FilmsLibraries(cx) },
		Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.FilmsLibrariesNote(cx) }, Style: core.Style{FontSize: 13, LineHeight: 18, Margin: core.Edges{Top: -10}}, StyleFunc: ui.Colored(colors.Muted)},
			look.Group{Children: []core.Node{
				all,
				core.Each(libs.Get, func(l catalog.Scope) string { return l.Desktop + "/" + l.ID + "/" + l.Name }, func(cx *core.Scope, l func() catalog.Scope, i func() int) core.Node {
					on := func() bool { return !l().Locked && same(library.Get(), l()) }
					leading := core.Show{When: func() bool { return l().Locked },
						Then: func(cx *core.Scope) core.Node {
							return libraryTile(lucide.Lock, colors.Muted, colors.OnAccent)
						},
						Else: func(cx *core.Scope) core.Node {
							return libraryTile(lucide.Folder, func() core.Color { return folderFills[i()%len(folderFills)] }, colors.OnAccent)
						}}
					detail := func() string {
						cur := l()
						if cur.Locked {
							if cur.Computer != "" {
								return cur.Computer + " · " + locales.FilmsLibraryLocked(cx)
							}
							return locales.FilmsLibraryLocked(cx)
						}
						return cur.Computer
					}
					trailing := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
						core.Show{When: func() bool { return l().Locked }, Then: func(cx *core.Scope) core.Node {
							return ui.Icon(lucide.Lock, 15, colors.Muted)
						}},
						core.Show{When: func() bool { return !l().Locked && l().HasPassword }, Then: func(cx *core.Scope) core.Node {
							return ui.IconButton{TestID: "films-library-lock-" + l().ID,
								Icon:      lucide.LockOpen,
								Variant:   ui.IconButtonGhost,
								Size:      36,
								IconSize:  15,
								Tint:      colors.Muted,
								LabelFunc: func() string { return locales.FilmsLibraryLockAction(cx, l().Name) },
								OnPress:   func() { lockLib(l()) }}
						}},
						check(on),
					}}
					return look.Row{TestID: "films-library-option-" + strconv.Itoa(i()+1), Index: func() int { return i() + 1 }, MinHeight: 56,
						Leading: leading,
						Title:   func() string { return l().Name }, TitleLines: 1, TitleStyle: core.Style{FontWeight: core.FontWeightSemibold},
						// sync.Library carries no folder path (packages/sync/protocol.go:80-84), so the row says the library's computer where the design shows its folder.
						Detail:  detail,
						Checked: on, Trailing: trailing,
						OnPress: func() { pick(l()) }}
				}),
			}},
		}}

	closeUnlock := func() {
		if busy.GetUntracked() {
			return
		}
		password.Set("")
		problem.Set("")
		unlocking.Set(catalog.Scope{})
	}
	submitUnlock := func() {
		target := unlocking.GetUntracked()
		pw := password.GetUntracked()
		if target.Desktop == "" || pw == "" || busy.GetUntracked() {
			return
		}
		busy.Set(true)
		problem.Set("")
		s.UnlockLibrary(target.Desktop, target.ID, pw, func(err error) {
			busy.Set(false)
			if err != nil {
				problem.Set(unlockErrorText(cx, err))
				return
			}
			password.Set("")
			problem.Set("")
			unlocking.Set(catalog.Scope{})
			target.Locked = false
			target.HasPassword = true
			library.Set(target)
		})
	}
	unlockDialog := look.Dialog{
		TestID:  "films-unlock-sheet",
		Open:    func() bool { return unlocking.Get().Desktop != "" },
		OnClose: closeUnlock,
		Title:   func() string { return locales.FilmsUnlockTitle(cx, unlocking.Get().Name) },
		Nav: &look.SheetNav{
			Cancel:    func() string { return locales.FilmsUnlockCancel(cx) },
			Confirm:   func() string { return locales.FilmsUnlockConfirm(cx) },
			Disabled:  func() bool { return password.Get() == "" },
			Busy:      busy.Get,
			OnCancel:  closeUnlock,
			OnConfirm: submitUnlock,
			CancelID:  "films-unlock-cancel",
			ConfirmID: "films-unlock-confirm",
		},
		Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.FilmsUnlockNote(cx, unlocking.Get().Computer) },
				Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Muted)},
			ui.Input{
				TestID:          "films-unlock-password",
				LabelFunc:       func() string { return locales.FilmsUnlockPassword(cx) },
				PlaceholderFunc: func() string { return locales.FilmsUnlockPassword(cx) },
				AutoFocus:       true,
				Masked:          true,
				AutoCapitalize:  core.AutoCapitalizeNone,
				NoAutoCorrect:   true,
				Value:           password.Get,
				OnChange: func(v string) {
					password.Set(v)
					if problem.GetUntracked() != "" {
						problem.Set("")
					}
				},
				OnSubmit: func(string) { submitUnlock() },
				Style:    core.Style{MinHeight: core.Pt(44), FontSize: 16, CornerRadius: 12, Padding: core.Symmetric(10, 12)},
			},
			core.Show{When: func() bool { return problem.Get() != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "films-unlock-error", ContentFunc: problem.Get,
					Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Danger)}
			}},
		},
	}
	return core.Group{sheet, unlockDialog}
}

func unlockErrorText(cx *core.Scope, err error) string {
	var se *sync.Error
	if errors.As(err, &se) {
		switch se.Code {
		case sync.CodeWrongPassword:
			return locales.FilmsUnlockWrong(cx)
		case sync.CodeTooMany:
			return locales.FilmsUnlockTooMany(cx)
		}
	}
	return locales.FilmsUnlockFailed(cx)
}

// libraryTile is a library's tile in the sheet: a 32-point tile, 8 round, filled with fill, with icon at 16 in tint (design 16).
func libraryTile(icon func() *core.SVG, fill, tint func() core.Color) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(32), Height: core.Pt(32), CornerRadius: 8, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: ui.Filled(fill),
		Children:  []core.Node{ui.IconOf(icon, 16, tint)}}
}
