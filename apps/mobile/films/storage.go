package films

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// storageFailures are the paired computers whose films the user's storage no longer serves the phone, by ID, with why: its key no longer works, or it is full. The phone streams and downloads a computer's films from there away from its network, so those films play only once kept on the phone.
func (v *view) storageFailures() map[string]sync.RelayFailure {
	out := map[string]sync.RelayFailure{}
	for _, d := range v.s.Desktops() {
		if q := v.s.Queue(d.ID); q.Relayed && (q.Storage == sync.RelayKeys || q.Storage == sync.RelayFull) {
			out[d.ID] = q.Storage
		}
	}
	return out
}

// storageFailure is why the storage fails the phone, the key's failure before the full one's, as the banner says the first.
func (v *view) storageFailure() sync.RelayFailure {
	got := sync.RelayNoFailure
	for _, f := range v.storageFailures() {
		if f == sync.RelayKeys {
			return f
		}
		got = f
	}
	return got
}

// unreachable reports whether the film of desktop is out of the phone's reach as the storage fails it: not kept on the phone, while that storage fails. Its row fades.
func (v *view) unreachable(desktop, key string) bool {
	if _, failed := v.storageFailures()[desktop]; !failed {
		return false
	}
	k, ok := v.s.Film(key)
	return !ok || !k.Done()
}

// storageBanner is the warning under the jobs card while the user's storage fails the phone: what failed and what to do on the computer, and that the films kept on the phone still play.
func (v *view) storageBanner(cx *core.Scope) core.Node {
	warn := theme.Of(cx).WarnText
	keys := func() bool { return v.storageFailure() == sync.RelayKeys }
	title := func() string {
		if keys() {
			return locales.FilmsStorageKeysTitle(cx)
		}
		return locales.FilmsStorageFullTitle(cx)
	}
	body := func() string {
		if keys() {
			return locales.FilmsStorageKeysBody(cx)
		}
		return locales.FilmsStorageFullBody(cx)
	}
	return core.View{TestID: "films-storage", Role: core.RoleAlert, LabelFunc: func() string { return title() + ". " + body() },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 10, Padding: core.Symmetric(12, 14), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous,
			Background: theme.BadgeOrange.Faded(0.14)},
		Children: []core.Node{
			core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Padding: core.Edges{Top: 1}}, Children: []core.Node{
				core.Icon{SVG: warnIcon, Style: core.Style{Width: core.Pt(18), Height: core.Pt(18)}}}},
			core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
				core.Text{TestID: "films-storage-title", ContentFunc: title, Style: core.Style{FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(warn)},
				core.Text{ContentFunc: body, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(warn)},
			}},
		}}
}

// warnIcon is the design's warning: a solid orange triangle with a white exclamation mark, the same in either appearance.
var warnIcon = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 18 18"><path d="M7.7 2.3a1.5 1.5 0 0 1 2.6 0l6.6 11.9a1.5 1.5 0 0 1-1.3 2.2H2.4a1.5 1.5 0 0 1-1.3-2.2z" fill="#FF9500"/><path d="M9 6.6v4" stroke="#FFFFFF" stroke-width="2" stroke-linecap="round"/><circle cx="9" cy="13.4" r="1.1" fill="#FFFFFF"/></svg>`))
