package sources

import (
	"errors"
	"path"
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/nas"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/remotefs"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Browse are where a folder's page leads: Folder opens the folder at a path of the same source, Play plays the film kept on the phone under a key, Edit opens the source's form, Back leaves the page.
type Browse struct {
	Folder func(dir string)
	Play   func(key string)
	// Feed gives the player the folder's videos in its order as it lists them, which a video played from it goes on to by a swipe (player.Layer.SetFeed); nil for none.
	Feed func(keys func() []string)
	Edit func(id string)
	Back func()
}

// listed is a folder's listing: under way, read (its folders and videos), or why not.
type listed struct {
	busy    bool
	entries []remotefs.Info
	err     error
}

// BrowsePage is the folder dir of the source id, its root for "": its folders, which open their own page, then its videos, each with its size and where its download to the phone stands, which plays at once as it is pressed, streamed from the source until it is downloaded, and whose button downloads it, pauses its download or goes on with it. The folder is read as the page opens and as it is pulled; a folder that cannot be read says why, with Try Again and the way to the source's form, or to the system's settings for a local network the app may not reach.
func BrowsePage(cx *core.Scope, s *session.Session, id, dir string, act Browse) base.NavPage {
	colors := ui.ColorsOf(cx)
	list := core.NewSignalFunc(cx, listed{busy: true}, nil)
	pulled := core.NewSignal(cx, false)
	stop := func() {}
	load := func() {
		stop()
		list.Set(listed{busy: true, entries: list.GetUntracked().entries})
		stop = s.ListSource(id, dir, func(entries []remotefs.Info, err error) {
			list.Set(listed{entries: entries, err: err})
			pulled.Set(false)
		})
	}
	cx.Cleanup(func() { stop() })
	load()
	title := func() string {
		if dir != "" {
			return path.Base(dir)
		}
		src, _ := s.Source(id)
		return src.Name
	}
	source := func() store.Source { src, _ := s.Source(id); return src }
	ref := &core.Ref{}
	// videos are the folder's videos in its order, as the phone keeps them once one plays (session.SourceFilmKey).
	videos := func() []string {
		var keys []string
		for _, e := range list.GetUntracked().entries {
			if !e.Dir {
				keys = append(keys, session.SourceFilmKey(id, nas.Join(dir, e.Name)))
			}
		}
		return keys
	}
	rows := core.Each(func() []remotefs.Info { return list.Get().entries }, func(e remotefs.Info) string { return e.Name }, func(cx *core.Scope, e func() remotefs.Info, i func() int) core.Node {
		p := nas.Join(dir, e().Name)
		if e().Dir {
			return look.Row{TestID: "browse-folder-" + e().Name, Index: i, Chevron: true, TitleLines: 1,
				Leading: look.IconTile(cx, lucide.Folder, look.ToneAccent),
				Title:   func() string { return e().Name },
				OnPress: func() { act.Folder(p) }}
		}
		play := func(key string) {
			if act.Feed != nil {
				act.Feed(videos)
			}
			act.Play(key)
		}
		return videoRow(cx, s, id, p, e, i, play)
	})
	failed := func(cx *core.Scope) core.Node {
		denied := func() bool { return errors.Is(list.Get().err, session.ErrLocalNetwork) }
		return look.Empty{TestID: "browse-failed", Icon: lucide.ServerCrash, Title: func() string { return Failure(cx, source(), list.Get().err) },
			Action: core.View{Style: core.Style{Gap: 8, AlignSelf: core.AlignStretch}, Children: []core.Node{
				look.Button{TestID: "browse-retry", Kind: look.Tinted, Text: func() string { return locales.SourceRetry(cx) }, OnPress: load},
				core.Show{When: denied, Then: func(cx *core.Scope) core.Node {
					return look.Button{TestID: "browse-settings", Kind: look.Plain, Text: func() string { return locales.SourceOpenSettings(cx) },
						OnPress: func() { cx.OpenPermissionSettings(core.PermissionLocalNetwork, nil) }}
				}, Else: func(cx *core.Scope) core.Node {
					return look.Button{TestID: "browse-edit", Kind: look.Plain, Text: func() string { return locales.SourceEditSource(cx) }, OnPress: func() { act.Edit(id) }}
				}},
			}}}
	}
	body := core.Show{When: func() bool { l := list.Get(); return l.err != nil && !l.busy }, Then: failed,
		Else: func(cx *core.Scope) core.Node {
			return core.Show{When: func() bool { l := list.Get(); return !l.busy && len(l.entries) == 0 },
				Then: func(cx *core.Scope) core.Node {
					return look.Empty{TestID: "browse-empty", Icon: lucide.FolderOpen, Title: func() string { return locales.SourceEmpty(cx) }}
				},
				Else: func(cx *core.Scope) core.Node {
					return core.View{StyleFunc: look.Column(cx), Children: []core.Node{
						core.Show{When: func() bool { l := list.Get(); return l.busy && len(l.entries) == 0 && !pulled.Get() }, Then: func(cx *core.Scope) core.Node {
							return core.View{TestID: "browse-loading", Style: core.Style{Padding: core.All(32), AlignItems: core.AlignCenter}, Children: []core.Node{
								ui.Spinner{LabelFunc: func() string { return locales.SourceLoading(cx) }},
							}}
						}},
						look.Group{TestID: "browse-list", Children: []core.Node{rows}},
					}}
				}}
		}}
	return look.NavPage(cx, base.NavPage{TitleFunc: title, TestID: "browse-page", StyleFunc: ui.Filled(colors.Background), Children: []core.Node{
		core.Scroll{TestID: "browse-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1},
			OnRefresh: func() { pulled.Set(true); load() }, Refreshing: pulled.Get,
			Children: []core.Node{body}},
	}}, ui.NavHeader{OnBack: act.Back, Scroll: ref})
}

// videoRow is the video at p of the source id, as e lists it, at i in its folder: its name, its size and where its download to the phone stands, with the download's bar while it runs, and the button that downloads it, pauses or goes on with its download, or the mark that it is on the phone. Pressing the row plays it at once, streamed from the source (session.StreamSource), or from the phone once it is downloaded.
func videoRow(cx *core.Scope, s *session.Session, id, p string, e func() remotefs.Info, i func() int, play func(key string)) core.Node {
	colors := ui.ColorsOf(cx)
	key := session.SourceFilmKey(id, p)
	// A file the paired computer made a film of is on the phone once either copy is, the computer's downloaded from the computer among them (session.Copy), as the Library says.
	done := func() bool { f, ok := s.Film(s.Copy(key)); return ok && f.HasVideo() }
	running := func() bool { t, ok := s.Transfer(s.Copy(key)); return ok && t.State == session.Running }
	press := func() {
		if running() {
			s.Pause(s.Copy(key))
			return
		}
		s.DownloadSource(id, p, e())
	}
	detail := func() string {
		parts := []string{look.Size(e().Size)}
		if st := look.Status(cx, s, key); st != "" {
			parts = append(parts, st)
		}
		return strings.Join(parts, " · ")
	}
	name := func() string { return e().Name }
	trailing := core.Show{When: done,
		Then: func(cx *core.Scope) core.Node {
			return core.View{TestID: "browse-downloaded-" + e().Name, Style: core.Style{Width: core.Pt(44), Height: core.Pt(44), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				AccessibilityElementsHidden: func() bool { return true }, Children: []core.Node{ui.Icon(lucide.CircleCheck, 20, colors.Accent)}}
		},
		Else: func(cx *core.Scope) core.Node {
			button := func(icon func() *core.SVG, label func() string) core.Node {
				return ui.IconButton{TestID: "browse-download-" + e().Name, Variant: ui.IconButtonGhost, Size: 44, IconSize: 20, Icon: icon, LabelFunc: label, OnPress: press}
			}
			return core.Show{When: running,
				Then: func(cx *core.Scope) core.Node {
					return button(lucide.Pause, func() string { return locales.SourcePause(cx, name()) })
				},
				Else: func(cx *core.Scope) core.Node {
					return button(lucide.ArrowDownToLine, func() string { return locales.SourceDownload(cx, name()) })
				}}
		}}
	return look.Row{TestID: "browse-video-" + e().Name, Index: i, TitleLines: 2,
		Leading:  look.IconTile(cx, lucide.Film, look.ToneMuted),
		Title:    name,
		Detail:   detail,
		Trailing: trailing,
		Below: core.Show{When: running, Then: func(cx *core.Scope) core.Node {
			return ui.Progress{LabelFunc: func() string { return locales.SourceDownload(cx, name()) }, Value: func() float32 { t, _ := s.Transfer(s.Copy(key)); return t.Fraction() * 100 }, Style: core.Style{Margin: core.Edges{Top: 4}}}
		}},
		OnPress: func() { s.StreamSource(id, p, e(), play) }}
}
