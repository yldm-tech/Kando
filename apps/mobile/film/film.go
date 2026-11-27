// Package film is a film's page: its poster where it has one, its title, what the phone keeps of it and its facts, each a group under its own header, and its main action at the page's foot. Its download starts, pauses, resumes and, once whole, plays from that action; the offline group says where it stands and cancels it, or once whole deletes it from the phone. A film its computer keeps no video of is kept by its lines, which play without the picture, and says what can be done to have the video (VideoPrompt), as the player does in the picture's place.
package film

import (
	"context"
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Page is film f of desktop's library, as the library listed it or as the phone keeps it; play plays it once downloaded.
func Page(cx *core.Scope, s *session.Session, desktop, library string, f sync.Film, play func(key string), back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	key := store.Key(desktop, f.ID)
	kept := func() (store.Film, bool) { return s.Film(key) }
	done := func() bool { k, ok := kept(); return ok && k.Done() }
	// noVideo is a film whose video is on neither the phone nor its computer, as the phone last heard: one kept by its lines, or one listed without a video. It is a memo because the page's actions are built again when it changes: read through kept() alone, they would be built again at every change of the film's record, and a press landing while they are would be lost.
	noVideo := core.NewMemo(cx, func() bool {
		if k, ok := kept(); ok {
			return k.LinesOnly && !k.HasVideo()
		}
		return f.SubtitlesOnly
	}).Get
	transfer := func() (session.Transfer, bool) { return s.Transfer(key) }
	state := func() (session.State, bool) { t, ok := transfer(); return t.State, ok }
	running := func() bool { st, ok := state(); return ok && st == session.Running }
	stopped := func() bool { st, ok := state(); return ok && st != session.Running }

	type posterSource struct{ c *sync.Client }
	remote := core.NewResource(cx, func() (posterSource, bool) {
		c := s.Client(desktop)
		k, ok := kept()
		return posterSource{c}, c != nil && f.Poster != "" && (!ok || k.Poster == "")
	}, func(ctx context.Context, src posterSource, _ core.ResourceInfo) ([]byte, error) {
		return s.Poster(ctx, src.c, desktop, f.ID)
	})
	// failed is a poster the platform could not draw.
	failed := core.NewSignal(cx, false)
	poster := func() core.ImageSource {
		if k, ok := kept(); ok && k.Poster != "" {
			return look.LocalPoster(s, k)
		}
		if data, ok := remote.Get(); ok {
			return core.ImageSource{Data: data}
		}
		return core.ImageSource{}
	}
	// hasPoster is a film with a poster to show, which takes no room without one.
	hasPoster := func() bool {
		if failed.Get() {
			return false
		}
		k, ok := kept()
		if ok && k.Poster != "" {
			return true
		}
		if data, got := remote.Get(); got && len(data) > 0 {
			return true
		}
		return f.Poster != "" && (!ok || k.Poster == "") && remote.Err() == nil
	}
	confirmDelete := func() {
		cx.ShowAlert(core.AlertConfig{Title: locales.FilmDeleteTitle(cx, f.Title), Actions: []core.AlertAction{
			{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
			{Label: locales.FilmDeleteConfirm(cx), Role: core.AlertDestructive},
		}}, func(i int) {
			if i == 1 {
				s.Delete(key)
				back()
			}
		})
	}
	// phase is which buttons the page shows, which changes far less often than the download's progress.
	const (
		notKept = iota
		downloading
		halted
		whole
	)
	phase := core.NewMemo(cx, func() int {
		switch {
		case done():
			return whole
		case running():
			return downloading
		case stopped():
			return halted
		}
		return notKept
	})
	// stored is what the user's storage holds of the film while the phone reaches its computer through it (session.RelayFilm): nothing, its lines alone, or its video too.
	const (
		notStored = iota
		linesStored
		videoStored
	)
	stored := core.NewMemo(cx, func() int {
		rf, ok := s.RelayFilm(desktop, f.ID)
		switch {
		case !ok:
			return notStored
		case rf.Media() == "":
			return linesStored
		}
		return videoStored
	})
	onNetwork := core.NewMemo(cx, func() bool { return s.OnNetwork(desktop) })
	// streamErr is why the film could not be streamed from its computer or storage, nil for none; streaming is a read under way.
	streamErr := core.NewSignal[error](cx, nil)
	streaming := core.NewSignal(cx, false)
	stream := func() {
		if streaming.GetUntracked() {
			return
		}
		streaming.Set(true)
		streamErr.Set(nil)
		s.Stream(desktop, library, f, func(key string) {
			streaming.Set(false)
			play(key)
		}, func(err error) {
			streaming.Set(false)
			streamErr.Set(err)
		})
	}
	text := func(say func(*core.Scope) string) func() string { return func() string { return say(cx) } }
	cancel := func() { s.Delete(key) }
	actions := core.Dynamic(func(cx *core.Scope) core.Node {
		switch phase.Get() {
		case whole:
			if noVideo() {
				return look.Button{Kind: look.Primary, Text: text(locales.FilmStudyLines), Icon: lucide.Captions, TestID: "film-play", OnPress: func() { play(key) }}
			}
			return look.Button{Kind: look.Primary, Text: text(locales.FilmPlay), Icon: lucide.Play, TestID: "film-play", OnPress: func() { play(key) }}
		case downloading:
			return look.Button{Kind: look.Tinted, Text: text(locales.FilmPause), Icon: lucide.Pause, TestID: "film-pause", OnPress: func() { s.Pause(key) }}
		case halted:
			return look.Button{Kind: look.Primary, Text: text(locales.FilmResume), TestID: "film-resume", OnPress: func() { s.Resume(key) }}
		}
		// A film being made plays from its computer on its network as it is made; it downloads once done.
		if f.Making && onNetwork.Get() {
			return look.Button{Kind: look.Primary, Text: text(locales.FilmPlay), Icon: lucide.Play, TestID: "film-stream", Busy: streaming.Get, OnPress: stream}
		}
		download := look.Button{Kind: look.Primary, Text: text(locales.FilmDownload), Icon: lucide.Download, TestID: "film-download", OnPress: func() { s.Download(desktop, library, f) }}
		if f.SubtitlesOnly {
			// Its computer keeps no video of it: downloading it keeps its lines.
			download.Text, download.Icon = text(locales.FilmSaveLines), lucide.Captions
		}
		// On its computer's network, a film with a video streams directly from the computer without downloading it first.
		if !f.SubtitlesOnly && onNetwork.Get() {
			download.Kind = look.Tinted
			return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				look.Button{Kind: look.Primary, Text: text(locales.FilmPlay), Icon: lucide.Play, TestID: "film-stream", Busy: streaming.Get, OnPress: stream}, download}}
		}
		// Away from its computer's network, the film plays from the user's storage, or opens for its lines alone when the storage holds no video of it.
		switch stored.Get() {
		case videoStored:
			// The two stack, as the sheet's rows do: side by side, the download's label does not fit a phone's half.
			download.Kind = look.Tinted
			return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				look.Button{Kind: look.Primary, Text: text(locales.FilmPlay), Icon: lucide.Play, TestID: "film-stream", Busy: streaming.Get, OnPress: stream}, download}}
		case linesStored:
			return look.Button{Kind: look.Primary, Text: text(locales.FilmLines), Icon: lucide.Captions, TestID: "film-lines", Busy: streaming.Get, OnPress: stream}
		}
		return download
	})
	prompt := core.Show{When: noVideo, Then: func(cx *core.Scope) core.Node {
		return core.View{StyleFunc: func() core.Style { return look.Card(cx) }, Children: []core.Node{VideoPrompt(cx, s, desktop, f, "film-no-video")}}
	}}
	// What the storage holds of a film whose video stays on the computer, and why it could not be read from there.
	storageNote := core.Show{When: func() bool { return stored.Get() == linesStored && phase.Get() != whole && !noVideo() }, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "film-lines-only", ContentFunc: func() string { d, _ := s.Desktop(desktop); return locales.FilmVideoAtHome(cx, d.Name) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)}
	}}
	makingNote := core.Show{When: func() bool { return f.Making && phase.Get() != whole }, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "film-making", ContentFunc: func() string { d, _ := s.Desktop(desktop); return locales.FilmMakingNote(cx, d.Name) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)}
	}}
	storageFailed := core.Show{When: func() bool { return streamErr.Get() != nil }, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "film-storage-failed", ContentFunc: func() string {
			if stored.Get() == notStored {
				d, _ := s.Desktop(desktop)
				return locales.FilmStreamFailed(cx, d.Name)
			}
			return locales.FilmStorageFailed(cx)
		}, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
	}}
	// offline is what the phone keeps of the film, a group under its own header: where its download stands, with its bar while it moves, over the row that cancels it, or once it is whole the row that deletes it from the phone with the room that frees.
	const (
		noRow = iota
		cancelRow
		deleteRow
	)
	last := core.NewMemo(cx, func() int {
		if _, ok := transfer(); ok {
			return cancelRow
		}
		if done() {
			return deleteRow
		}
		return noRow
	})
	second := func() int { return 1 }
	offline := core.Show{When: func() bool { return last.Get() != noRow }, Then: func(cx *core.Scope) core.Node {
		status := core.View{Style: core.Style{Gap: 10, Padding: core.Symmetric(15, 16)}, Children: []core.Node{
			core.Text{TestID: "film-status", ContentFunc: func() string { return look.Status(cx, s, key) }, LiveRegion: core.LiveRegionPolite, MaxLines: 2, Style: look.Body,
				StyleFunc: func() core.Style {
					if st, ok := state(); ok && st == session.Failed {
						return core.Style{Color: colors.Danger()}
					}
					return core.Style{Color: colors.Text()}
				}},
			core.Show{When: func() bool { _, ok := transfer(); return ok }, Then: func(cx *core.Scope) core.Node {
				return ui.Progress{TestID: "film-progress", LabelFunc: func() string { return locales.FilmDownload(cx) }, Value: func() float32 { t, _ := transfer(); return t.Fraction() * 100 }}
			}},
		}}
		row := core.Dynamic(func(cx *core.Scope) core.Node {
			if last.Get() == cancelRow {
				return look.Row{TestID: "film-cancel", Index: second, TitleTone: look.ToneDanger, Title: text(locales.FilmCancel), OnPress: cancel}
			}
			return look.Row{TestID: "film-delete", Index: second, TitleTone: look.ToneDanger, Title: text(locales.FilmDelete), OnPress: confirmDelete,
				Value: func() string {
					if k, ok := kept(); ok && k.HasVideo() && k.Film.VideoSize > 0 {
						return look.Size(k.Film.VideoSize)
					}
					return ""
				}}
		})
		return core.View{Style: core.Style{Gap: 7}, Children: []core.Node{look.GroupHeader(cx, text(locales.FilmOffline)), look.Group{Children: []core.Node{status, row}}}}
	}}
	art := core.Show{When: hasPoster, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "film-poster", Style: core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9, CornerRadius: look.CardRadius, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden},
			StyleFunc: func() core.Style {
				s := core.Style{Background: colors.Inset()}
				// A phone on its side keeps the poster to about a third of the screen's height, so the title shows under it above the action bar.
				if w := cx.Env().Window(); w.Width > w.Height {
					s.MaxWidth, s.AlignSelf = core.Pt(w.Height*0.35*16/9), core.AlignCenter
				}
				return s
			},
			Children: []core.Node{core.Image{SourceFunc: poster, ResizeMode: core.Cover, FadeDuration: 150 * time.Millisecond, ElementsHidden: true,
				OnError: func(error) { failed.Set(true) },
				Style:   core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}}}}
	}}
	// info is the film's facts as rows of a group under its own header, as the player's Info tab lists them: its source, languages, length and size, and the library it is in; a fact the film lacks takes no row.
	var facts []core.Node
	fact := func(id string, label func(*core.Scope) string, value func() string) {
		if value() == "" {
			return
		}
		i := len(facts)
		facts = append(facts, look.Row{TestID: "film-info-" + id, Index: func() int { return i }, Title: text(label), TitleLines: 1, Value: value})
	}
	is := func(v string) func() string { return func() string { return v } }
	fact("source", locales.FilmInfoSource, func() string { return sourceName(cx, catalog.SourceOf(f)) })
	fact("languages", locales.FilmInfoLanguages, is(look.Languages(f)))
	if f.DurationMS > 0 {
		fact("length", locales.FilmInfoLength, is(look.Duration(time.Duration(f.DurationMS)*time.Millisecond)))
	}
	if f.VideoSize > 0 {
		fact("size", locales.FilmInfoSize, is(look.Size(f.VideoSize)))
	}
	fact("library", locales.FilmInfoLibrary, is(library))
	info := core.View{TestID: "film-info", Style: core.Style{Gap: 7}, Children: []core.Node{look.GroupHeader(cx, text(locales.FilmInfo)), look.Group{Children: facts}}}
	ref := &core.Ref{}
	return look.NavPage(cx, base.NavPage{TestID: "film-page", Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
				core.View{Style: core.Style{Padding: core.Edges{Top: 8, Right: 16, Bottom: 24, Left: 16}, PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Left: core.EnvSafeArea},
					Gap: 20, MaxWidth: core.Pt(560), Width: core.Pct(100), AlignSelf: core.AlignCenter}, Children: []core.Node{
					art,
					core.Text{TestID: "film-title", Content: f.Title, Role: core.RoleHeader, MaxLines: 3, Style: look.Heading, StyleFunc: ui.Colored(colors.Text)},
					prompt,
					storageNote,
					makingNote,
					storageFailed,
					offline,
					info,
				}},
			}},
			look.ActionBar(cx, actions),
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// sourceName is the name of the site a film came from, as the Library's source chips say it.
func sourceName(cx *core.Scope, source string) string {
	switch source {
	case catalog.SourceYouTube:
		return locales.FilmsSourceYoutube(cx)
	case catalog.SourceBilibili:
		return locales.FilmsSourceBilibili(cx)
	case catalog.SourceLocal:
		return locales.FilmsSourceLocal(cx)
	}
	return locales.FilmsSourceWeb(cx)
}
