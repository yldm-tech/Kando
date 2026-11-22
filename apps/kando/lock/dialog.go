// Package lock is the password of a library: the dialog that sets, changes or removes it or asks for it before another library opens, and the screen a locked library shows in place of its films. On macOS the password encrypts the library's files (package vault); elsewhere it is an app lock, not encryption. The dialog that sets it says which. The keys are made and checked by the app (shelf.Lock), never here.
package lock

import (
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Mode is what the dialog asks for.
type Mode int

const (
	// Set gives a library without a password one: the new password twice.
	Set Mode = iota + 1
	// Change gives it another: the current password, then the new one twice.
	Change
	// Remove takes it away: the current password.
	Remove
	// Unlock opens a locked library: its password.
	Unlock
)

// Problem is why the last try did not go through.
type Problem int

const (
	NoProblem Problem = iota
	// Wrong is a current password that is not the library's.
	Wrong
	// Mismatch is a new password typed differently the second time.
	Mismatch
	// SaveFailed is a password that could not be kept in library.json; Look.Detail says why.
	SaveFailed
	// Busy is a library a job is still downloading or processing a film into, whose files cannot be encrypted or decrypted, nor its image given another password, until it is done.
	Busy
	// NoSpace is a disk without room for the second copy of the library's files that encrypting or decrypting it makes first.
	NoSpace
	// VaultFailed is a library whose encrypted image could not be made, opened, given another password or removed for another reason than a wrong password; its files are as they were.
	VaultFailed
)

// Look is what the dialog and the screen show: what is asked for, the library's name, why the last try did not go through, and whether the password is being checked or its key made, which takes a moment.
type Look struct {
	Mode    Mode
	Name    string
	Problem Problem
	// Detail is why the password could not be kept, in the interface's language, with SaveFailed.
	Detail string
	Busy   bool
	// Encrypts is whether a password encrypts the library's files here (vault.Supported), which the dialog that sets it says.
	Encrypts bool
	// Copying is whether the library's files are being moved into its encrypted image (Set) or out of it (Remove), Percent how far that has gone.
	Copying bool
	Percent int
}

// Dialog asks for the passwords Mode needs while Open returns true, over the window. Its button, or Return in a field, runs OnSubmit with what was typed, "" for a field the mode does not have, and empties the fields, so a wrong password is typed again; it is disabled while Look is Busy or a field is empty. Cancel, Escape and closing it run OnCancel. A press outside it keeps it, so what was typed is not lost.
type Dialog struct {
	Open     func() bool
	Look     func() Look
	OnSubmit func(current, next, confirm string)
	OnCancel func()
}

// Render implements core.Node.
func (d Dialog) Render(cx *core.Scope) core.Node {
	// Look follows the app's whole revision; the memos let the dialog be built again only when what it asks for changes, not as the password is checked.
	look := core.NewMemo(cx, d.Look)
	type ask struct {
		mode     Mode
		name     string
		encrypts bool
	}
	asks := core.NewMemo(cx, func() ask { l := look.Get(); return ask{l.Mode, l.Name, l.Encrypts} })
	cancel := func() {
		if d.OnCancel != nil {
			d.OnCancel()
		}
	}
	return core.Show{When: d.Open, Then: func(cx *core.Scope) core.Node {
		// The dialog is built again for another question, which empties the fields; the words follow the language through their bindings.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			a := asks.Get()
			f := newForm(cx, a.mode)
			submit := func() {
				if look.Get().Busy || !f.filled() {
					return
				}
				current, next, confirm := f.take()
				if d.OnSubmit != nil {
					d.OnSubmit(current, next, confirm)
				}
			}
			description := func() string {
				switch {
				case a.mode == Set && a.encrypts:
					return locales.LockSetNoteEncrypted(cx)
				case a.mode == Set:
					return locales.LockSetNote(cx)
				}
				return ""
			}
			return ui.Dialog{
				TestID:                 "lock-dialog",
				Role:                   core.RoleAlertDialog,
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				TitleFunc:              func() string { return title(cx, a.mode, a.name) },
				DescriptionFunc:        description,
				Style:                  core.Style{Width: core.Pt(380)},
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Children: []core.Node{core.View{Style: core.Style{Gap: 8}, Children: append(f.fields(submit), problemLine(cx, look.Get), copyingLine(cx, look.Get))}},
				Footer: []core.Node{
					ui.Button{TextFunc: func() string { return locales.LockCancel(cx) }, Variant: ui.ButtonSecondary, TestID: "lock-cancel", Disabled: func() bool { return look.Get().Copying }, OnPress: cancel},
					ui.Button{TextFunc: func() string { return action(cx, a.mode) }, TestID: "lock-submit", Disabled: func() bool { return look.Get().Busy || !f.filled() }, OnPress: submit},
				},
			}
		})
	}}
}

// title is the dialog's title for mode, naming the library.
func title(cx *core.Scope, mode Mode, name string) string {
	switch mode {
	case Set:
		return locales.LockSetTitle(cx, name)
	case Change:
		return locales.LockChangeTitle(cx, name)
	case Remove:
		return locales.LockRemoveTitle(cx, name)
	}
	return locales.LockUnlockTitle(cx, name)
}

// action is the dialog's button for mode.
func action(cx *core.Scope, mode Mode) string {
	switch mode {
	case Set:
		return locales.LockSetButton(cx)
	case Change:
		return locales.LockChangeButton(cx)
	case Remove:
		return locales.LockRemoveButton(cx)
	}
	return locales.LockUnlockButton(cx)
}

// form is the dialog's password fields for its mode: the current password for Change, Remove and Unlock, the new one twice for Set and Change. Each field keeps what is typed in a signal of its own, which take empties.
type form struct {
	cx                     *core.Scope
	current, next, confirm *core.Signal[string]
	mode                   Mode
}

func newForm(cx *core.Scope, mode Mode) form {
	return form{cx: cx, mode: mode, current: core.NewSignal(cx, ""), next: core.NewSignal(cx, ""), confirm: core.NewSignal(cx, "")}
}

func (f form) asksCurrent() bool { return f.mode != Set }

func (f form) asksNew() bool { return f.mode == Set || f.mode == Change }

// filled reports whether every field the mode has holds something.
func (f form) filled() bool {
	if f.asksCurrent() && f.current.Get() == "" {
		return false
	}
	return !f.asksNew() || f.next.Get() != "" && f.confirm.Get() != ""
}

// take is what the fields hold, and empties them.
func (f form) take() (current, next, confirm string) {
	current, next, confirm = f.current.Get(), f.next.Get(), f.confirm.Get()
	f.current.Set("")
	f.next.Set("")
	f.confirm.Set("")
	return current, next, confirm
}

// fields are the mode's fields, the first holding the keyboard; Return in any of them runs submit.
func (f form) fields(submit func()) []core.Node {
	var out []core.Node
	if f.asksCurrent() {
		label := func() string {
			if f.mode == Unlock {
				return locales.LockFieldPassword(f.cx)
			}
			return locales.LockFieldCurrent(f.cx)
		}
		out = append(out, passwordField(f.current, "lock-current", label, core.AutoCompletePassword, true, submit))
	}
	if f.asksNew() {
		out = append(out,
			passwordField(f.next, "lock-new", func() string { return locales.LockFieldNew(f.cx) }, core.AutoCompleteNewPassword, !f.asksCurrent(), submit),
			passwordField(f.confirm, "lock-confirm", func() string { return locales.LockFieldConfirm(f.cx) }, core.AutoCompleteNewPassword, false, submit))
	}
	return out
}

// passwordField is a secure text field (Masked) that keeps what is typed in value, labelled and hinted with label.
func passwordField(value *core.Signal[string], id string, label func() string, fill core.AutoComplete, focus bool, submit func()) core.Node {
	return ui.Input{
		TestID:          id,
		LabelFunc:       label,
		PlaceholderFunc: label,
		Masked:          true,
		AutoComplete:    fill,
		AutoFocus:       focus,
		Value:           value.Get,
		OnChange:        value.Set,
		OnSubmit:        func(string) { submit() },
	}
}

// problemLine says why the last try did not go through, or nothing.
func problemLine(cx *core.Scope, look func() Look) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{
		When: func() bool { return look().Problem != NoProblem },
		Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "lock-problem", Role: core.RoleAlert, ContentFunc: func() string { return problemText(cx, look()) }, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Danger)}
		},
	}
}

func problemText(cx *core.Scope, l Look) string {
	switch l.Problem {
	case Wrong:
		return locales.LockWrong(cx)
	case Mismatch:
		return locales.LockMismatch(cx)
	case SaveFailed:
		return l.Detail
	case Busy:
		return locales.LockBusy(cx)
	case NoSpace:
		return locales.LockNoSpace(cx)
	case VaultFailed:
		return locales.LockVaultFailed(cx)
	}
	return ""
}

// copyingLine says how far moving the library's files into or out of its encrypted image has gone, while it runs.
func copyingLine(cx *core.Scope, look func() Look) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{
		When: func() bool { return look().Copying },
		Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "lock-copying", Role: core.RoleStatus, ContentFunc: func() string {
				l := look()
				if l.Mode == Remove {
					return locales.LockDecrypting(cx, l.Percent)
				}
				return locales.LockEncrypting(cx, l.Percent)
			}, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)}
		},
	}
}
