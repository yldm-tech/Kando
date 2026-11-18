// Package learn is the Learn tab: the words and lines the user saves, and their review. The phone keeps none yet; they come with the learning player and the study record's sync (yldm-tech/Kando#222, #224), and the tab says what will show there meanwhile.
package learn

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

// Page is the tab's root page.
func Page(cx *core.Scope) base.NavPage {
	title := func() string { return locales.LearnTitle(cx) }
	return look.TabRoot(cx, base.NavPage{TestID: "learn-page", Children: []core.Node{
		look.Empty{TestID: "learn-empty", Icon: lucide.GraduationCap, Title: func() string { return locales.LearnEmpty(cx) }},
	}}, title, nil, nil)
}
