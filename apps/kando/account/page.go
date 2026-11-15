// Package account draws the 账号与上传 category of the settings window: the Google account uploads are filed under, and the object storage they go to.
package account

import (
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// App is the 账号与上传 category of the settings window. It draws and reports edits. It does not sign in or upload by itself. The page reads it inside its bindings through live.Func and calls its actions through live.Do and live.On, so what they change shows at once (package live).
type App interface {
	live.State
	GoogleEmail() string
	GoogleBusy() bool
	// GoogleErr is why the last sign-in failed, in the interface's language, "" when nothing failed.
	GoogleErr() string
	GoogleConfigured() bool
	SignInGoogle()
	SignOutGoogle()
	Storage() *string
	StorageKind() string
	R2Endpoint() *string
	R2Bucket() *string
	R2AccessKey() *string
	R2Secret() *string
	S3Region() *string
	S3Bucket() *string
	S3AccessKey() *string
	S3Secret() *string
	OSSRegion() *string
	OSSBucket() *string
	OSSAccessKey() *string
	OSSSecret() *string
	COSRegion() *string
	COSBucket() *string
	COSAccessKey() *string
	COSSecret() *string
	CompatEndpoint() *string
	CompatRegion() *string
	CompatBucket() *string
	CompatAccessKey() *string
	CompatSecret() *string
	CompatPathStyle() *bool
	Save() error
}

// View is the 账号与上传 category: the Google account uploads are filed under, and the object storage they go to, laid out as the prototype's page.
type View struct {
	App App
}

// Render implements core.Node.
func (v View) Render(cx *core.Scope) core.Node {
	a := v.App
	colors := ui.ColorsOf(cx)
	failed := live.Func(a, a.GoogleErr)
	signInNote := live.Func(a, func() string {
		if !a.GoogleConfigured() && a.GoogleEmail() == "" {
			return locales.AccountNoClientNote(cx)
		}
		return locales.AccountSignInNote(cx)
	})
	return core.View{
		TestID: "account",
		Style:  core.Style{Width: core.Pct(100), Gap: 6, AlignItems: core.AlignStretch},
		Children: []core.Node{
			label(func() string { return locales.AccountSectionAccount(cx) }, 6),
			card("account-google", googleRow{a}),
			core.Show{When: func() bool { return failed() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{ContentFunc: failed, Style: core.Style{FontSize: 11, Padding: core.Symmetric(0, 12)}, StyleFunc: ui.Colored(colors.Danger)}
			}},
			note(signInNote, 10),
			label(func() string { return locales.AccountSectionStorage(cx) }, 6),
			card("account-storage", storageRow{a}, storageFields{a}),
			note(func() string { return locales.AccountUploadNote(cx) }, 0),
		},
	}
}

// storageKinds are the kinds of storage the drop-down offers, by the ids the setting keeps.
var storageKinds = []string{"r2", "s3", "oss", "cos", "compat"}

// storageLabel is the storage's name as the drop-down shows it, in the language the page shows under cx; the setting keeps the id.
func storageLabel(cx *core.Scope, id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "s3":
		return "Amazon S3"
	case "oss":
		return locales.AccountStoreOSS(cx)
	case "cos":
		return locales.AccountStoreCOS(cx)
	case "compat":
		return locales.AccountStoreCompat(cx)
	default:
		return "Cloudflare R2"
	}
}

// storageRow chooses the kind of storage; the drop-down shows the storages' names and the setting keeps the id.
type storageRow struct{ a App }

// Render implements core.Node.
func (r storageRow) Render(cx *core.Scope) core.Node {
	a := r.a
	title := func() string { return locales.AccountStorageKind(cx) }
	kind := live.Func(a, a.StorageKind)
	// The drop-down's names are set as it is built, so it is built again for another language.
	choose := core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Select LabelFunc twins for its name and its options' labels so they follow the interface language without building the drop-down again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		options := make([]ui.Option[string], len(storageKinds))
		for i, id := range storageKinds {
			options[i] = ui.Option[string]{Value: id, Label: storageLabel(cx, id)}
		}
		return ui.Select[string]{
			TestID: "storage-kind", Label: locales.AccountStorageKind(cx), Options: options, Value: kind,
			OnChange: live.On(a, func(id string) {
				*a.Storage() = id
				_ = a.Save()
			}),
		}
	})
	return row(title, none, choose)
}

// storageFields are the fields of the kind of storage chosen, built again when another is chosen.
type storageFields struct{ a App }

// Render implements core.Node.
func (f storageFields) Render(cx *core.Scope) core.Node {
	a := f.a
	kind := live.Memo(cx, a, a.StorageKind)
	hint := func(message func(*core.Scope) string) func() string { return func() string { return message(cx) } }
	return core.Dynamic(func(cx *core.Scope) core.Node {
		var rows []core.Node
		switch kind() {
		case "s3":
			rows = []core.Node{
				textRow(a, "s3-region", "Region", hint(locales.AccountS3RegionHint), a.S3Region()),
				textRow(a, "s3-bucket", "Bucket", none, a.S3Bucket()),
				textRow(a, "s3-access", "Access Key", none, a.S3AccessKey()),
				secretRow(a, "s3-secret", none, a.S3Secret()),
			}
		case "oss":
			rows = []core.Node{
				textRow(a, "oss-region", "Region", hint(locales.AccountOSSRegionHint), a.OSSRegion()),
				textRow(a, "oss-bucket", "Bucket", none, a.OSSBucket()),
				textRow(a, "oss-access", "Access Key", none, a.OSSAccessKey()),
				secretRow(a, "oss-secret", none, a.OSSSecret()),
			}
		case "cos":
			rows = []core.Node{
				textRow(a, "cos-region", "Region", hint(locales.AccountCOSRegionHint), a.COSRegion()),
				textRow(a, "cos-bucket", "Bucket", hint(locales.AccountCOSBucketHint), a.COSBucket()),
				textRow(a, "cos-access", "Access Key", hint(locales.AccountCOSAccessHint), a.COSAccessKey()),
				secretRow(a, "cos-secret", hint(locales.AccountCOSSecretHint), a.COSSecret()),
			}
		case "compat":
			rows = []core.Node{
				textRow(a, "compat-endpoint", "Endpoint", hint(locales.AccountCompatEndpointHint), a.CompatEndpoint()),
				textRow(a, "compat-region", "Region", hint(locales.AccountCompatRegionHint), a.CompatRegion()),
				textRow(a, "compat-bucket", "Bucket", none, a.CompatBucket()),
				textRow(a, "compat-access", "Access Key", none, a.CompatAccessKey()),
				secretRow(a, "compat-secret", none, a.CompatSecret()),
				separator(),
				pathStyleRow{a},
			}
		default:
			rows = []core.Node{
				textRow(a, "r2-endpoint", "Endpoint", hint(locales.AccountR2EndpointHint), a.R2Endpoint()),
				textRow(a, "r2-bucket", "Bucket", none, a.R2Bucket()),
				textRow(a, "r2-access", "Access Key", none, a.R2AccessKey()),
				secretRow(a, "r2-secret", none, a.R2Secret()),
			}
		}
		return core.Group(rows)
	})
}

// textRow edits one of the storage's fields in the prototype's column of fields, in a monospaced face, saving each edit. Its label is the field's name in the storage's own words, which no language translates.
func textRow(a App, test, title string, hint func() string, value *string) core.Node {
	input := field{Input: base.Input{
		TestID:   test,
		Label:    title,
		Value:    live.Func(a, func() string { return *value }),
		OnChange: live.On(a, func(text string) { *value = text; _ = a.Save() }),
	}, Face: core.Style{FontFamily: ui.Mono, FontSize: 12}}
	return core.Group{separator(), row(func() string { return title }, hint, input)}
}

// secretRow edits a storage key's secret in a password field, which keeps it hidden on screen, saving each edit.
func secretRow(a App, test string, hint func() string, value *string) core.Node {
	input := field{Input: base.Input{
		TestID:   test,
		Label:    "Secret",
		Masked:   true,
		Value:    live.Func(a, func() string { return *value }),
		OnChange: live.On(a, func(text string) { *value = text; _ = a.Save() }),
	}, Face: core.Style{FontSize: 13}}
	return core.Group{separator(), row(func() string { return "Secret" }, hint, input)}
}

// pathStyleRow is the S3-compatible storage's switch of path-style addresses: the title over the subtitle, padded 8 points above and below and 12 before the switch. The switch is named by the title, and a press on the title or the subtitle turns it too, as on a label of a check box.
type pathStyleRow struct{ a App }

// Render implements core.Node.
func (r pathStyleRow) Render(cx *core.Scope) core.Node {
	a := r.a
	on := live.Func(a, func() bool { return *a.CompatPathStyle() })
	set := live.On(a, func(v bool) {
		*a.CompatPathStyle() = v
		_ = a.Save()
	})
	title := func() string { return locales.AccountPathStyle(cx) }
	subtitle := func() string { return locales.AccountPathStyleHint(cx) }
	return core.View{
		Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(48), Padding: core.Symmetric(8, 14), Gap: 12, AlignItems: core.AlignCenter},
		Children: []core.Node{
			core.View{
				Style:    core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2},
				NoFocus:  true,
				OnPress:  func(core.PressEvent) { set(!on()) },
				Children: []core.Node{titles(title, subtitle)},
			},
			// The switch's name is set as it is built, so it is built again for another language.
			core.Dynamic(func(cx *core.Scope) core.Node {
				// TODO(nsgo#145): give ui.Switch a LabelFunc twin so its name follows the interface language without building the switch again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
				return ui.Switch{TestID: "compat-path", Label: locales.AccountPathStyle(cx), Variant: ui.SwitchSystem, Checked: on, OnChange: set}
			}),
		},
	}
}

// googleRow is the account card's row, as the prototype's: a gray avatar, the account over what it is for, and the button that signs in or out.
type googleRow struct{ a App }

// Render implements core.Node.
func (r googleRow) Render(cx *core.Scope) core.Node {
	a := r.a
	signedIn := live.Memo(cx, a, func() bool { return a.GoogleEmail() != "" })
	title := live.Func(a, func() string {
		if email := a.GoogleEmail(); email != "" {
			return email
		}
		return locales.AccountSignedOut(cx)
	})
	subtitle := live.Func(a, func() string {
		if a.GoogleEmail() != "" {
			return locales.AccountSignedInWith(cx)
		}
		return locales.AccountSignedOutHint(cx)
	})
	busy := live.Memo(cx, a, a.GoogleBusy)
	configured := live.Func(a, a.GoogleConfigured)
	// The buttons' words are set as they are built, so they are built again for another language, and 登录 for 正在登录.
	signOut := core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Button a TextFunc twin so its text follows the interface language without building the button again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		return ui.Button{Text: locales.AccountSignOut(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "google-sign-out", OnPress: live.Do(a, a.SignOutGoogle)}
	})
	signIn := core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Button a TextFunc twin so its text follows the sign-in and the interface language without building the button again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		text := locales.AccountSignIn(cx)
		if busy() {
			text = locales.AccountSigningIn(cx)
		}
		return ui.Button{Text: text, TestID: "google-sign-in",
			Disabled: func() bool { return busy() || !configured() }, OnPress: live.Do(a, a.SignInGoogle)}
	})
	// The row is built again as the account signs in or out, which its description, read as it is built, follows; the button is the only part of it that takes the keyboard, and goes with the account anyway.
	build := func(button core.Node) func(*core.Scope) core.Node {
		return func(*core.Scope) core.Node {
			// TODO(nsgo#145): give core.View an AccessibilityDescriptionFunc twin, so the row's description follows the interface language as it changes, as LabelFunc does its name; it is read as the row is built https://github.com/nsgo-dev/nsgo/issues/145
			return core.View{
				TestID:                   "google-row",
				Role:                     core.RoleGroup,
				LabelFunc:                title,
				AccessibilityDescription: core.Untracked(cx, subtitle),
				Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(62), Padding: core.Symmetric(10, 14), Gap: 12,
					AlignItems: core.AlignCenter},
				Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 20, FlexShrink: 0, Background: formIdle}},
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
						core.Text{ContentFunc: title, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, FontWeight: ui.Semibold, Color: formText}},
						core.Text{ContentFunc: subtitle, Style: core.Style{FontSize: 11, Color: formSecondary}},
					}},
					button,
				},
			}
		}
	}
	return core.Show{When: signedIn, Then: build(signOut), Else: build(signIn)}
}
