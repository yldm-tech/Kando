package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// srtClock is a time as SubRip writes it, 00:00:01,500.
func srtClock(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

// TestScreenshotsPlayer renders the prototype's film, How Rivers Shape Landscapes (j1), from the library every scene starts from (screenshot_helpers_test.go), in the states the prototype's player screenshots show: opened and held at the start with the side panel on 字幕 (01), playing on the first line with the controls showing (02), the 片段 and 信息 tabs (04, 08), the 信息 tab with another spoken language named than the one heard, the subtitle mode menu (09), the film's reference layer with the menu that hides it, the style view (10), playing with the controls faded (11), playing with the subtitles dragged up, playing muted, the speed menu, playing at 1.5× and playing with its line looped, and, beyond the prototype, a line of the subtitle list right-clicked for its menu, the picture saying ⇧⌘C copied the playing line, the lines under the pointer showing they can be taken hold of, and the once-only tip that they can be dragged. It also renders the film on its first line (film), the film opened from the YouTube page, the film whose video the system's player cannot play, and a video no job has processed playing with the subtitles exported beside it.
func TestScreenshotsPlayer(t *testing.T) {
	f := newScreenshotFixture(t)
	// open opens film id at at and has the system's player report it loaded, held there.
	open := func(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), id string, at time.Duration) {
		screenshotOpen(t, a, tt, updates, id, at)
		// TODO(nsgo#1057): show the film's own picture, its frames decoded by the fake ffmpeg as MyGo's player showed them, once Kando plays what ffmpeg decodes (playerRemux in playback.go), which waits on an NSGo audio output fed decoded sound; the system's player plays no fake clip headless, so the stage shows the film's poster behind the report of a load https://github.com/nsgo-dev/nsgo/issues/1057
		screenshotLoad(tt, a)
	}
	f.shoot(t, "film", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, "j1", protoCues[0].Start)
	})
	held := func(steps func(t *testing.T, a *app, tt *nsgotest.App)) func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		return func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
			open(t, a, tt, updates, "j1", 0)
			if steps != nil {
				steps(t, a, tt)
			}
		}
	}
	f.shoot(t, "player-opened", held(nil))
	// The film played to its end, and the next of the list it was added from counting down on the Up Next card.
	f.shoot(t, "player-next", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		for i, job := range a.library.Jobs {
			switch job.ID {
			case "j1":
				a.library.Jobs[i].List, a.library.Jobs[i].ListIndex = "youtube.com/playlist?list=PLrivers", 1
			case "j8":
				a.library.Jobs[i].List, a.library.Jobs[i].ListIndex = "youtube.com/playlist?list=PLrivers", 2
			}
		}
		open(t, a, tt, updates, "j1", 0)
		screenshotProgress(tt, a, a.PlayLength())
		// On a platform the scene reports for the system's player, which plays nothing, that the film ended, as headless the progress to its end does.
		if nsgotest.Target() != "" {
			onEngine(a, a.VideoEnded)
		}
		refresh(tt, a)
		tt.Advance(3 * time.Second)
	})
	f.shoot(t, "player-clips", held(func(t *testing.T, _ *app, tt *nsgotest.App) { tt.Press("id:film-tab-clips") }))
	f.shoot(t, "player-info", held(func(t *testing.T, _ *app, tt *nsgotest.App) { tt.Press("id:film-tab-info") }))
	// The film heard in English, and 日语 named in the info tab as the language spoken in it, which offers to transcribe it again in Japanese.
	f.shoot(t, "player-info-spoken-language", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		for i, job := range a.library.Jobs {
			if job.ID == "j1" {
				a.library.Jobs[i].SpokenLanguage = "en"
			}
		}
		open(t, a, tt, updates, "j1", 0)
		tt.Press("id:film-tab-info")
		a.SetFilmSpeechLanguage("ja")
		refresh(tt, a)
		if !tt.Exists("id:film-info-transcribe-again") {
			t.Fatalf("no offer to transcribe again: %q", tt.Texts())
		}
	})
	// The film's original lines are its site's subtitles, three of whose lines did not match the speech and were recognized on this computer: the info tab says so.
	f.shoot(t, "player-info-platform-subtitles", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		for i, job := range a.library.Jobs {
			if job.ID == "j1" {
				a.library.Jobs[i].PlatformSubtitles, a.library.Jobs[i].LocalLines = true, 3
			}
		}
		open(t, a, tt, updates, "j1", 0)
		tt.Press("id:film-tab-info")
		if !tt.Exists("id:film-info-original") {
			t.Fatalf("no original lines in the info tab: %q", tt.Texts())
		}
	})
	// j1 kept no video (仅字幕): its sound was downloaded to make its subtitles and deleted. The fixture's video stays on disk for the other scenes, so the scenes say what the player says once it found none.
	subtitlesOnly := func(a *app) {
		for i, job := range a.library.Jobs {
			if job.ID == "j1" {
				a.library.Jobs[i].Quality, a.library.Jobs[i].SubtitlesOnly = core.QualitySubtitles, true
			}
		}
		a.player.streams = func(string) bool { return true }
	}
	// Opened offline, it cannot stream: the card over the picture offers to turn offline mode off or to download the video once online.
	f.shoot(t, "player-subtitles-only-offline", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		subtitlesOnly(a)
		a.player.panel = false
		screenshotOpen(t, a, tt, updates, "j1", protoCues[1].Start)
		apply(tt, a, func() {
			a.dropPlayback()
			a.privacy.offline = true
			a.player.playErr = film.ErrStreamOffline
		})
		if !tt.Exists("id:film-stream") {
			t.Fatalf("no card over the picture: %q", tt.Texts())
		}
	})
	// Where it cannot stream, as a bilibili film or any off macOS for now, the card offers to download the video.
	f.shoot(t, "player-subtitles-only-download", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		subtitlesOnly(a)
		a.player.streams = func(string) bool { return false }
		a.player.panel = false
		screenshotOpen(t, a, tt, updates, "j1", protoCues[1].Start)
		apply(tt, a, func() {
			a.dropPlayback()
			a.player.playErr = film.ErrStreamNone
		})
		if !tt.Exists("id:film-stream-download") {
			t.Fatalf("no download button: %q", tt.Texts())
		}
	})
	// The info tab says the film keeps no video and streams when watched, and offers to download it.
	f.shoot(t, "player-info-subtitles-only", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		subtitlesOnly(a)
		open(t, a, tt, updates, "j1", 0)
		tt.Press("id:film-tab-info")
		if !tt.Exists("id:film-info-stream") {
			t.Fatalf("no card in the info tab: %q", tt.Texts())
		}
	})
	// The film's YouTube description marks five chapters, whose titles were translated: the 片段 tab is cut at them, and the info tab says where they came from.
	chaptered := func(steps func(t *testing.T, tt *nsgotest.App)) func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		return func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
			job, ok := a.library.Find("j1")
			if !ok {
				t.Fatal("no j1")
			}
			s := time.Second
			kept := media.FilmChapters{Language: "简体中文", Chapters: []core.Chapter{
				{Start: 0, Title: "Intro", Translation: "开场"}, {Start: 24 * s, Title: "Where Rivers Begin", Translation: "河流的源头"}, {Start: 58 * s, Title: "Carving Valleys", Translation: "切出山谷"},
				{Start: 102 * s, Title: "Meanders and Oxbow Lakes", Translation: "河曲与牛轭湖"}, {Start: 146 * s, Title: "Building Deltas", Translation: "三角洲的形成"},
			}}
			if err := media.SaveChapters(job.VideoPath, kept); err != nil {
				t.Fatal(err)
			}
			open(t, a, tt, updates, "j1", 0)
			waitUntil(t, tt, a, updates, func() bool { return len(a.Info().Chapters) == 5 })
			steps(t, tt)
		}
	}
	f.shoot(t, "player-clips-chapters", chaptered(func(t *testing.T, tt *nsgotest.App) {
		tt.Press("id:film-tab-clips")
		if !tt.Exists("id:film-clips-note") {
			t.Fatalf("no chapters in the clips tab: %q", tt.Texts())
		}
	}))
	f.shoot(t, "player-info-chapters", chaptered(func(t *testing.T, tt *nsgotest.App) {
		tt.Press("id:film-tab-info")
		if !tt.Exists("id:film-info-chapters") {
			t.Fatalf("no chapters in the info tab: %q", tt.Texts())
		}
	}))
	f.shoot(t, "player-menu", held(func(t *testing.T, _ *app, tt *nsgotest.App) { tt.Press("id:film-mode") }))
	// The film's job kept YouTube's automatic captions as its reference layer: the line playing shows above the film's own, and the subtitle menu offers to hide it.
	f.shoot(t, "player-reference", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		job, ok := a.library.Find("j1")
		if !ok {
			t.Fatal("no j1")
		}
		var srt strings.Builder
		for i, cue := range protoCues[:3] {
			fmt.Fprintf(&srt, "%d\n%s --> %s\n%s\n\n", i+1, srtClock(cue.Start), srtClock(cue.End), strings.ToLower(strings.Trim(cue.Original, ".")))
		}
		if err := os.WriteFile(media.ReferencePath(job.VideoPath), []byte(srt.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		open(t, a, tt, updates, "j1", protoCues[0].Start+time.Second)
		waitUntil(t, tt, a, updates, func() bool { return len(a.Reference()) == 3 })
		tt.Press("id:film-mode")
		if !tt.Exists("id:film-reference") {
			t.Fatalf("no reference row: %q", tt.Texts())
		}
	})
	f.shoot(t, "player-style", held(func(t *testing.T, _ *app, tt *nsgotest.App) {
		tt.Press("id:film-mode")
		tt.Press("id:film-style")
	}))
	// Playing 3 seconds in, on the first line: with the pointer just moved the controls show, and once it has rested for the player's idle time they fade, the subtitles moving down into their room.
	playing := func(rest time.Duration) func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		return func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
			open(t, a, tt, updates, "j1", 3*time.Second)
			tt.Key("space")
			tt.HoverAt("id:film", ns.Point{X: 400, Y: 300})
			screenshotProgress(tt, a, 3*time.Second)
			refresh(tt, a)
			tt.Advance(rest)
			// Let the fade end.
			tt.Advance(500 * time.Millisecond)
		}
	}
	f.shoot(t, "player-playing", playing(0))
	f.shoot(t, "player-idle", playing(3*time.Second))
	// Playing with the film's subtitles dragged up a sixth of the picture, as off its own burned-in lines: they stay where they were let go as the controls fade.
	f.shoot(t, "player-dragged", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.setSubtitleLifts(map[string]float32{f.job(t, "j1").VideoPath: 1.0 / 6})
		playing(3*time.Second)(t, a, tt, updates)
	})
	// Held on the first line with the pointer over it: a backdrop behind the lines shows they can be taken hold of.
	f.shoot(t, "player-subtitles-hover", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, "j1", protoCues[0].Start)
		tt.Hover("id:film-subtitle-lines")
		if tt.Find("id:film-subtitle-lines").Style.Background == 0 {
			t.Fatal("no backdrop behind the lines under the pointer")
		}
	})
	// The first film played with subtitles: the tip over them says they can be dragged and double-clicked back.
	f.shoot(t, "player-subtitle-tip", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.player.subTipSeen = false
		playing(0)(t, a, tt, updates)
		if !tt.Exists("id:film-subtitle-tip") {
			t.Fatalf("no tip over the subtitles: %q", tt.Texts())
		}
	})
	// Playing with the sound muted by M: the speaker crossed out and the picture saying so.
	f.shoot(t, "player-muted", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		playing(0)(t, a, tt, updates)
		tt.Key("m")
	})
	f.shoot(t, "player-speed-menu", held(func(t *testing.T, _ *app, tt *nsgotest.App) { tt.Press("id:film-speed") }))
	// Playing at 1.5×, two presses of ⇧. up from 1×: the speed button and the picture say so.
	f.shoot(t, "player-speed", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		playing(0)(t, a, tt, updates)
		tt.Key("shift+.")
		tt.Key("shift+.")
	})
	// A video no job has processed, from a library copied without its job records (sidecarLibrary), plays with the subtitles exported beside it.
	f.shoot(t, "player-sidecar-subtitles", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		dir, video := f.sidecarLibrary(t)
		f.showLibrary(t, a, tt, updates, dir)
		open(t, a, tt, updates, video, protoCues[0].Start)
	})
	// Playing with the line looped by L: the transport's loop toggle lit and the picture saying so.
	f.shoot(t, "player-loop", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		playing(0)(t, a, tt, updates)
		tt.Key("l")
	})
	f.shoot(t, "player-youtube", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		apply(tt, a, func() { a.page = pageYouTube })
		open(t, a, tt, updates, "j1", protoCues[1].Start)
	})
	// A right click on the second line of the subtitle list opens its menu, 讲解这句 and 收藏这句, then 拷贝台词, 拷贝原文 and 拷贝译文; the menu is the system's, which the tester lists (tt.PopupMenu) rather than draws, so the picture shows the line under the pointer.
	f.shoot(t, "player-line-menu", held(func(t *testing.T, _ *app, tt *nsgotest.App) {
		tt.RightClick(whole(protoCues[1].Translation))
		var got []string
		for _, it := range tt.PopupMenu {
			got = append(got, it.Label)
		}
		msgs := testlocale.Messages()
		if want := []string{msgs.FilmExplainLine(), msgs.SavedKeySaveLine(), "", msgs.FilmCopyLine(), msgs.FilmCopyOriginal(), msgs.FilmCopyTranslation()}; !slices.Equal(got, want) {
			t.Fatalf("the line's menu is %q, want %q", got, want)
		}
	}))
	// ⇧⌘C on the first line: the picture says the line was copied.
	f.shoot(t, "player-line-copied", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, "j1", protoCues[0].Start)
		tt.Key("mod+shift+c")
		if !hasText(tt, "已拷贝") {
			t.Fatalf("⇧⌘C did not say it copied: %q", tt.Texts())
		}
	})
	// A video the system's player cannot play, which the app cannot copy for it either: the stage says so, and the side panel lists the film's lines.
	f.shoot(t, "player-no-picture", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.player.panel = false
		screenshotOpen(t, a, tt, updates, "j1", protoCues[1].Start)
		apply(tt, a, func() { a.player.playErr = playerErrUnplayable })
	})
	// Off macOS the player names the system's file manager, here the Linux one, and the style view's font menu offers a sans-serif and a serif font, shown open.
	usePlatform(t, ns.OSLinux)
	f.shoot(t, "player-info-linux", held(func(t *testing.T, _ *app, tt *nsgotest.App) { tt.Press("id:film-tab-info") }))
	f.shoot(t, "player-style-fonts-linux", held(func(t *testing.T, _ *app, tt *nsgotest.App) {
		tt.Press("id:film-mode")
		tt.Press("id:film-style")
		tt.Press("id:film-style-font")
	}))
}

// screenshotLoad has the system's player report the open film loaded, as a platform's does once it has opened the video, so the scene shows the film's transport as it plays. Headless nsgotest reports it to the video view (tt.Play); on a platform, where nsgotest leaves such reports to the platform and the scenes give the system's player nothing to play (screenshotOpen), the report goes to the app's handler of it on the engine's goroutine.
func screenshotLoad(tt *nsgotest.App, a *app) {
	if a.player.play == nil || a.player.playErr != "" {
		return
	}
	length := a.PlayLength()
	if length <= 0 {
		length = 5 * time.Minute
	}
	info := ns.VideoInfo{Duration: length, Size: ns.Size{Width: 1920, Height: 1080}}
	switch {
	case nsgotest.Target() != "":
		onEngine(a, func() { a.VideoLoaded(info) })
	case tt.Exists("id:film-video"):
		tt.Play("id:film-video", info)
	default:
		return
	}
	refresh(tt, a)
}

// screenshotProgress has the system's player report the open film played to position, as screenshotLoad reports its load.
func screenshotProgress(tt *nsgotest.App, a *app, position time.Duration) {
	if nsgotest.Target() == "" {
		tt.Progress("id:film-video", position, a.PlayLength())
		return
	}
	onEngine(a, func() { a.VideoProgress(position, a.PlayLength()) })
}
