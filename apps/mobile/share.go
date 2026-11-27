package main

import (
	"kando/apps/mobile/add"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
)

// sharedLinks hears what other apps share to Kando from the system's share sheet (Android's SEND intent for android.share_types, iOS's share extension for apple.share_types in nsgo.yaml), and hands open what the Add sheet's field takes from the texts of each share (add.Shared), once ready reports true; a share that came before, as one the app launched with or one iOS kept in the App Group while the app was closed, waits for it, and only the last waiting is opened. A share of files alone, as a picture or a video file, goes to files: the phone takes links, and makes nothing of a file itself.
func sharedLinks(cx *core.Scope, ready func() bool, open func(text string), files func()) {
	var waiting *string
	mobile.OnShared(cx, func(items []mobile.SharedItem) {
		var texts []string
		for _, it := range items {
			if it.Path == "" {
				texts = append(texts, it.Text)
			}
		}
		text := add.Shared(texts)
		switch {
		case text == "" && len(texts) == 0:
			files()
		case text == "":
		case ready():
			open(text)
		default:
			waiting = &text
		}
	})
	cx.Effect(func() {
		if !ready() || waiting == nil {
			return
		}
		text := *waiting
		waiting = nil
		cx.Post(func() { open(text) })
	})
}
