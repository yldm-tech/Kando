package lock

import (
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Screen covers the whole window while Shown returns true, in place of a locked library: a lock, the library's name, its password field, and a way to another library. Its button, or Return in the field, runs OnUnlock with what was typed and empties the field; it is disabled while Look is Busy or the field is empty. Others are the items of the menu its second button opens, the other libraries and the ways to make or open one, built as it opens. Neither Escape nor a press on it closes it: only unlocking or opening another library does.
type Screen struct {
	Shown    func() bool
	Look     func() Look
	OnUnlock func(password string)
	Others   func() []nsdesktop.MenuItem
}

// Render implements core.Node.
func (s Screen) Render(cx *core.Scope) core.Node {
	look := core.NewMemo(cx, s.Look)
	name := core.NewMemo(cx, func() string { return look.Get().Name })
	colors := ui.ColorsOf(cx)
	return core.Show{When: s.Shown, Then: func(cx *core.Scope) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			n := name.Get()
			// TODO(nsgo#145): give base.Dialog a LabelFunc, ui.Button a TextFunc and core.Input a LabelFunc, so the words follow the language without building the screen again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			f := newForm(cx, Unlock)
			unlock := func() {
				if look.Get().Busy || !f.filled() {
					return
				}
				password, _, _ := f.take()
				if s.OnUnlock != nil {
					s.OnUnlock(password)
				}
			}
			// place is the row around the second button, which ui.Button takes no Ref for, so the menu opens under it.
			place := &core.Ref{}
			others := func() {
				if s.Others == nil {
					return
				}
				at := core.Point{}
				if b, ok := place.Bounds(); ok {
					at = core.Point{X: b.Origin.X, Y: b.Origin.Y + b.Size.Height + 4}
				}
				nsdesktop.PopupMenu(cx, s.Others(), at, nil)
			}
			heading := core.Text{Content: locales.LockScreenTitle(cx, n), Role: core.RoleHeader, TestID: "lock-screen-title", MaxLines: 2,
				Style: ui.TypeTitle.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)}
			field := core.View{Style: core.Style{AlignSelf: core.AlignStretch}, Children: f.fields(unlock)}
			unlockButton := ui.Button{Text: locales.LockUnlockButton(cx), TestID: "lock-screen-unlock", Disabled: func() bool { return look.Get().Busy || !f.filled() }, OnPress: unlock}
			otherButton := core.View{Ref: place, Children: []core.Node{
				ui.Button{Text: locales.LockScreenOthers(cx), Variant: ui.ButtonGhost, TestID: "lock-screen-others", OnPress: others},
			}}
			// The band under the title bar still moves the window, as the window's own does.
			band := core.View{Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn}, PointerEvents: core.PointerEventsBoxOnly,
				Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(40)}}
			return base.Dialog{
				TestID:                 "lock-screen",
				Label:                  locales.LockScreenTitle(cx, n),
				Open:                   func() bool { return true },
				NoCloseOnEscape:        true,
				NoCloseOnBackdropPress: true,
				BackdropStyleFunc:      ui.Filled(colors.Background),
				PopupStyle:             core.Style{Width: core.Pt(300), AlignItems: core.AlignCenter, Gap: 14},
				Before:                 []core.Node{band},
				Children: []core.Node{
					ui.Icon(lucide.LockKeyhole, 40, colors.Muted),
					heading,
					field,
					problemLine(cx, look.Get),
					unlockButton,
					otherButton,
				},
			}
		})
	}}
}
