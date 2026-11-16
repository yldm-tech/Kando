// Command mobile is Kando for iOS and Android (yldm-tech/Kando#178, phase 2): it pairs with Kando on a computer on the local network (protocol v1, kando/packages/sync), lists the computer's libraries and films, downloads films to the phone and plays them offline with bilingual subtitles and the line list.
package main

import (
	"kando/apps/mobile/session"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() { nsgo.Run(Root) }

// Root is the app on this phone: its data in the app's own folder, and the phone as pairing names it.
func Root(cx *core.Scope) core.Node {
	return App(cx, session.Config{Dir: cx.DataDir(), Device: deviceName(cx), Platform: platform(cx)})
}

// platform is the platform pairing names, as the computer lists the phone.
func platform(cx *core.Scope) string {
	if cx.Env().OS() == core.OSAndroid {
		return sync.PlatformAndroid
	}
	return sync.PlatformIOS
}

// deviceName is the name pairing gives the phone, which the computer lists it by: the kind of device. They are product names, not words of the interface.
// TODO(nsgo): give apps the device's own name (Android's Settings.Global.DEVICE_NAME, or Build.MODEL), so a computer paired with two Android phones tells them apart; iOS gives apps only "iPhone" or "iPad" since iOS 16
func deviceName(cx *core.Scope) string {
	tablet := cx.Env().Idiom() == core.IdiomTablet
	switch {
	case cx.Env().OS() == core.OSAndroid && tablet:
		return "Android tablet"
	case cx.Env().OS() == core.OSAndroid:
		return "Android phone"
	case tablet:
		return "iPad"
	}
	return "iPhone"
}
