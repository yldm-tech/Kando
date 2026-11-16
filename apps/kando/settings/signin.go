package settings

import (
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// SignIn is the browser whose sign-in every download borrows, as the 浏览器登录 card of 通用 shows it.
type SignIn struct {
	// Browser is the chosen browser's --cookies-from-browser name, "" for 关闭. Browsers are the browsers the card offers besides 关闭: those on the computer, and the chosen one.
	Browser  string
	Browsers []media.Browser
	// Checking is a check of Browser's sign-in under way. Checked reports that one finished since it was chosen, and Trouble what it found, "" when yt-dlp read the sign-in.
	Checking bool
	Checked  bool
	Trouble  media.LoginTrouble
}

// SignInWhy says in the words of m why yt-dlp could not read the sign-in of browser (its --cookies-from-browser name) for trouble, and what to do, as both the card of a failed download and the 浏览器登录 row say it. platform is the system, as desktop.Platform names it.
func SignInWhy(m locales.Messages, platform, browser string, trouble media.LoginTrouble) string {
	name := media.BrowserName(browser)
	switch {
	case browser == "safari" && (trouble == media.LoginNoAccess || trouble == media.LoginNotFound):
		return m.QueueRemedySafariWhy()
	case trouble == media.LoginInUse:
		return m.QueueRemedyCookiesInUse(name)
	case trouble == media.LoginEncrypted:
		return m.QueueRemedyCookiesEncrypted(name)
	case trouble == media.LoginNoKey:
		return m.QueueRemedyCookiesNoKey(platform, name, media.KeychainItem(browser))
	case trouble == media.LoginNoAccess, trouble == media.LoginUnreadable:
		return m.QueueRemedyCookiesUnreadable(name)
	}
	return m.QueueRemedyCookiesWhy(name)
}

// signInState is what the end of the 浏览器登录 row shows.
type signInState uint8

const (
	signInNothing signInState = iota
	signInReady
	signInTrouble
	// signInDiskAccess is Safari's sign-in, which macOS keeps from an app without Full Disk Access.
	signInDiskAccess
)

// signIn is the 浏览器登录 card: the browser every download borrows the sign-in of, 关闭 or one of those on the computer, and what the check of its sign-in found, with 重新检查 when it failed.
type signIn struct{ s screen }

// Render implements core.Node.
func (g signIn) Render(cx *core.Scope) core.Node {
	a := g.s.a
	colors := ui.ColorsOf(cx)
	// ids is the browsers offered, joined, so the pop-up is built again only when they change.
	ids := core.NewMemo(cx, live.Func(a, func() string {
		var ids []string
		for _, b := range a.SignIn().Browsers {
			ids = append(ids, b.ID)
		}
		return strings.Join(ids, ",")
	}))
	choose := core.Dynamic(func(cx *core.Scope) core.Node {
		options := []ui.Option[string]{{Value: "", Label: locales.SettingsGeneralSignInOff(cx)}}
		if joined := ids.Get(); joined != "" {
			for _, id := range strings.Split(joined, ",") {
				options = append(options, ui.Option[string]{Value: id, Label: media.BrowserName(id)})
			}
		}
		return ui.Select[string]{Options: options, Label: locales.SettingsGeneralSignInUse(cx), TestID: "settings-signin-browser",
			Value: live.Func(a, func() string { return a.SignIn().Browser }),
			OnChange: live.On(a, func(id string) {
				if id != a.SignIn().Browser {
					a.SetSignInBrowser(id)
				}
			}),
			Style: core.Style{Width: core.Pt(140)}}
	})
	state := core.NewMemo(cx, live.Func(a, func() signInState {
		switch st := a.SignIn(); {
		case !st.Checked:
			return signInNothing
		case st.Trouble == "":
			return signInReady
		case st.Browser == "safari" && (st.Trouble == media.LoginNoAccess || st.Trouble == media.LoginNotFound):
			return signInDiskAccess
		}
		return signInTrouble
	}))
	end := core.Dynamic(func(cx *core.Scope) core.Node {
		check := form.Button{Text: locales.SettingsGeneralSignInCheck(cx), TestID: "settings-signin-check", OnPress: live.Do(a, a.CheckSignIn)}
		switch state.Get() {
		case signInReady:
			return form.Status(ui.Fixed(form.OK), colors.Muted, static(locales.SettingsGeneralSignInReady(cx)))
		case signInTrouble:
			return check
		case signInDiskAccess:
			return core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
				form.Button{Text: locales.SettingsGeneralSignInDiskAccess(cx), TestID: "settings-signin-disk-access", OnPress: live.Do(a, a.OpenFullDiskAccess)},
				check,
			}}
		}
		return core.Group(nil)
	})
	row := form.Row{Title: locales.SettingsGeneralSignInUse(cx), TestID: "settings-signin",
		SubtitleFunc: live.Func(a, func() string { return signInDetail(cx, a.SignIn()) }), Trailing: []core.Node{end, choose}}
	return core.Group([]core.Node{
		form.Label(locales.SettingsGeneralSignIn(cx), 14),
		form.Card(row),
	})
}

// signInDetail is the line under 借用此浏览器的登录, in the language the app shows under cx: what choosing a browser does while none is, that its sign-in is being read, why it could not be read, and otherwise that downloads sign in with it.
func signInDetail(cx *core.Scope, st SignIn) string {
	name := media.BrowserName(st.Browser)
	switch {
	case st.Browser == "":
		return locales.SettingsGeneralSignInOffHint(cx)
	case st.Checking:
		return locales.SettingsGeneralSignInChecking(cx, name)
	case st.Checked && st.Trouble != "":
		return SignInWhy(locales.For(cx), desktop.Platform(cx), st.Browser, st.Trouble)
	}
	return locales.SettingsGeneralSignInOn(cx, name)
}
