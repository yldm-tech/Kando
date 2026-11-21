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

// deviceName is the name pairing gives the phone, which the computer lists it by, so a computer paired with two phones of a kind tells them apart: the name its owner gave it, else on Android its maker and model, else the kind of device, as iOS keeps the name from apps since iOS 16. The kinds are product names, not words of the interface.
func deviceName(cx *core.Scope) string {
	env := cx.Env()
	if name := env.DeviceName(); name != "" {
		return name
	}
	android := env.OS() == core.OSAndroid
	if model := env.DeviceModel(); android && model != "" {
		return model
	}
	tablet := env.Idiom() == core.IdiomTablet
	switch {
	case android && tablet:
		return "Android tablet"
	case android:
		return "Android phone"
	case tablet:
		return "iPad"
	}
	return "iPhone"
}
