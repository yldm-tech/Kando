package main

import (
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"

	"github.com/nsgo-dev/nsgo/core"
)

// Kando updates itself from the signed releases of yldm-tech/Kando (updates in nsgo.yaml), through NSGo's updater (package updater): on macOS and Windows Sparkle and WinSparkle, which nsgo build bundles, check on their own schedule, show the newer version in their windows and install it; on Linux nsgo.Run reads the appcast at most once a day and asks, then installs the verified .deb through dpkg (pkexec) when dpkg installed the running copy, and offers the newer version's page otherwise, as for the Arch package and the AppImage. The words of NSGo's own alerts follow the interface's language, as Settings › 通用 sets the app's locale.

// updatesKeys is the app menu's 检查更新… row, which checks at once and shows what it finds. It has no key, so on Linux and Windows, which have no menu bar, the command palette is the way in, besides the button in Settings › 通用.
func (a *app) updatesKeys(cx *core.Scope) []shortcut {
	return []shortcut{
		{Group: groupApp, Label: locales.UpdatesCheck(cx), Run: a.CheckForUpdates},
	}
}

// CheckForUpdates checks for a newer version at once, under the app's scope, and shows what it finds: the updater's window on macOS and Windows, an alert elsewhere. It is settings.App's, for 通用's 检查更新….
//
// Offline mode refuses it and says so in the library's message bar; 通用's button is off meanwhile.
// TODO(nsgo#32): let the app turn the updater's own scheduled checks off and on while it runs (Sparkle's automaticallyChecksForUpdates, WinSparkle's, and the daily check nsgo.Run makes on Linux), so offline mode stops them too; until then it stops only the check the user starts, and Settings › 隐私 says the daily one goes on https://github.com/nsgo-dev/nsgo/issues/32
func (a *app) CheckForUpdates() {
	if a.privacy.offline {
		a.warn(a.messages().PrivacyNoUpdate(), sectionid.Privacy)
		return
	}
	a.updates.check(a.cx)
}
