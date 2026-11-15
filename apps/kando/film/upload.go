package film

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The upload capsule's dots (prototype template 583-595).
var uploadDots = map[string]core.Color{
	UploadBusy:     core.RGB(0x64D2FF),
	UploadDone:     core.RGB(0x30D158),
	UploadFailed:   core.RGB(0xFFD60A),
	UploadCanceled: core.RGB(0x8E8E93),
	UploadBlocked:  core.RGB(0xFFD60A),
}

// uploadGlass is the capsule's glass.
var uploadGlass = glassSpec{fill: core.RGBA(0x1E1E22, 0.72), blur: 24, ring: 0.2, radius: 18}

// uploadCapsule says how the film's upload goes, under the title bar: while it runs with a bar of its progress and a button that cancels it, then whether it worked or was canceled, or, when the upload button found nowhere to upload, why and a button to 账号 to fix it. It shows while an upload of this film runs, and after the upload button was clicked until closed; the close button only hides it and leaves an upload running. It is centered in the room the side panel leaves, as the big play button is: over the whole width, a wide capsule in a narrow window reached under the panel, which hid its close button.
func (s *screen) uploadCapsule() core.Node {
	a := s.a
	view := live.Func(a, a.FilmUpload)
	shown := live.Func(a, func() bool {
		v := a.FilmUpload()
		return v.Show && v.Detail != "" && (*a.UploadNote() || v.Status == UploadBusy)
	})
	return core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
		busy := func() bool { return view().Status == UploadBusy }
		fix := live.Memo(cx, a, func() string { return a.FilmUpload().Fix })
		closeNote := live.Do(a, func() { *a.UploadNote() = false })
		dot := core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4, FlexShrink: 0}, StyleFunc: func() core.Style {
			c, ok := uploadDots[view().Status]
			if !ok {
				c = uploadDots[UploadDone]
			}
			return core.Style{Background: c}
		}}
		// TODO(nsgo): give the capsule core.RoleStatus, as MyGo's RoleStatus had assistive technology read it as a status; NSGo has no status role, so it is a polite live region here
		capsule := core.View{TestID: "film-upload-capsule", LiveRegion: core.LiveRegionPolite,
			Style: uploadGlass.style().Merge(core.Style{Direction: core.Row, Wrap: core.Wrap, MaxWidth: core.Pct(70), Gap: 10, Padding: core.Edges{Top: 8, Right: 8, Bottom: 8, Left: 16}, AlignItems: core.AlignCenter}),
			Children: []core.Node{
				dot,
				core.Text{TestID: "film-upload-detail", ContentFunc: func() string { return view().Detail }, Style: core.Style{FontSize: 13, LineHeight: 18, FlexShrink: 1, Color: onGlass}},
				core.Show{When: busy, Then: func(cx *core.Scope) core.Node {
					return row(10,
						ui.Progress{Value: func() float32 { return float32(view().Progress * 100) }, Style: core.Style{Width: core.Pt(120), Height: core.Pt(4)}},
						core.Dynamic(func(cx *core.Scope) core.Node {
							return ui.Button{TestID: "film-upload-cancel", Text: locales.FilmUploadCancel(cx), Variant: ui.ButtonSecondary,
								Disabled: func() bool { return view().Canceling },
								OnPress: live.Do(a, func() {
									if !a.FilmUpload().Canceling {
										a.CancelUpload()
									}
								})}
						}),
					)
				}},
				core.Show{When: func() bool { return fix() != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Dynamic(func(cx *core.Scope) core.Node {
						return ui.Button{TestID: "film-upload-fix", Text: fix(), Variant: ui.ButtonSecondary, OnPress: live.Do(a, func() {
							*a.UploadNote() = false
							a.OpenSettings(sectionid.Account)
						})}
					})
				}},
				ui.IconButton{TestID: "film-upload-close", Icon: svg(iconClose), LabelFunc: func() string { return locales.FilmDismiss(cx) }, Size: 26,
					Tint: func() core.Color { return onGlass }, OnPress: closeNote},
			}}
		return core.View{PointerEvents: core.PointerEventsBoxNone,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(64), Direction: core.Row, JustifyContent: core.JustifyCenter},
			StyleFunc: func() core.Style {
				return core.Style{Right: core.Pt(s.roomRight()), Transition: roomTransition}
			},
			Children: []core.Node{capsule}}
	}}
}
