package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// keepFeed puts on the phone, besides the talk (f1, Morning Talk, added now), Morning Notes (f4, kept by its lines alone, half an hour before), Morning Walk (f3, an hour before) and Evening Talk (f2, two hours before): newest first, the Library lists them Talk, Notes, Walk, Evening.
func keepFeed(t *testing.T, dir string) store.Film {
	t.Helper()
	first := keepTalk(t, dir)
	st := store.Open(dir)
	keep := func(id, title string, ago time.Duration, video bool, cues []sync.Cue) {
		f := store.Film{Desktop: first.Desktop, Library: first.Library, Added: now().Add(-ago),
			Film: sync.Film{ID: id, Title: title, DurationMS: 10_000, Spoken: "ja", Translation: "简体中文"}}
		if video {
			f.Video, f.Film.VideoSize = "video.mp4", int64(len(talkVideo))
		}
		if err := st.SaveFilm(f); err != nil {
			t.Fatal(err)
		}
		if err := st.SaveCues(f.Desktop, id, cues); err != nil {
			t.Fatal(err)
		}
		if video {
			if err := os.WriteFile(filepath.Join(st.FilmDir(f.Desktop, id), f.Video), talkVideo, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	keep("f4", "Morning Notes", 30*time.Minute, false, talkCues)
	keep("f3", "Morning Walk", time.Hour, true, talkCues)
	// Its lines say nothing of the morning, so a search for it leaves the film out.
	keep("f2", "Evening Talk", 2*time.Hour, true, []sync.Cue{{StartMS: 1000, EndMS: 2000, Original: "Good evening.", Translation: "晚上好。"}})
	return first
}

// openFeed opens Morning Talk on an iPhone 17 from the Library searched for "Morning", its upright video immersive.
func openFeed(t *testing.T) *phone {
	t.Helper()
	dir := t.TempDir()
	f := keepFeed(t, dir)
	p := startPhone(t, dir, nsgotest.Size(402, 874))
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Type("id:films-search", "Morning")
	p.app.WaitForGone(filmRow(f.Desktop, "f2"), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.loadUpright()
	return p
}

// loadUpright reports the film playing loaded as an upright 9:16 video.
func (p *phone) loadUpright() {
	p.t.Helper()
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: videoSizes["9:16"]})
	p.app.WaitFor("id:player-mode-pill", wait)
}

// swipe drags the picture up (by less than 0) or down by dy, and lets the page slide.
func (p *phone) swipe(dy core.Points) {
	p.t.Helper()
	p.app.Pan("id:player-stage", core.Point{Y: dy / 4}, core.Point{Y: dy})
	p.app.Advance(400 * time.Millisecond)
}

// A swipe up plays the next film of the list the player was opened from, as it was searched, skipping one kept by its lines alone, and a swipe down the one before; at either end the film stays.
func TestASwipeGoesThroughTheListItWasOpenedFrom(t *testing.T) {
	p := openFeed(t)
	if !p.playing("f1") {
		t.Fatal("the talk does not play")
	}
	p.swipe(-400)
	p.app.WaitUntil(wait, func() bool { return p.playing("f3") })
	// The upright ratio carries over, so the next film is immersive before its video loads.
	if !p.app.Exists("id:player-mode-pill") {
		t.Fatal("the next film stacked before its video loaded")
	}
	p.loadUpright()
	// Morning Walk is the last of the search: Evening Talk is not in it.
	p.swipe(-400)
	if !p.playing("f3") {
		t.Fatal("a swipe up past the list's end went on")
	}
	p.swipe(400)
	p.app.WaitUntil(wait, func() bool { return p.playing("f1") })
	p.loadUpright()
	p.swipe(400)
	if !p.playing("f1") {
		t.Fatal("a swipe down before the list's start went on")
	}
	// A short drag springs back.
	p.swipe(-60)
	if !p.playing("f1") {
		t.Fatal("a short drag went on")
	}
}

// The swipe rests while the list of lines is up, in the stacked layout and with the phone on its side; the next film opens with its own loop and recap, the app's settings kept.
func TestTheSwipeRestsAndKeepsTheSettings(t *testing.T) {
	p := openFeed(t)
	p.app.Press("id:player-panel-toggle")
	p.app.WaitFor("id:player-lines-pane", wait)
	p.app.Pan("id:player-video", core.Point{Y: -100}, core.Point{Y: -400})
	p.app.Advance(400 * time.Millisecond)
	if !p.playing("f1") {
		t.Fatal("a swipe with the list up went on")
	}
	p.app.Press("id:player-panel-toggle")
	p.app.WaitForGone("id:player-lines-pane", wait)

	p.stacked()
	p.swipe(-400)
	if !p.playing("f1") {
		t.Fatal("a swipe in the stacked layout went on")
	}
	p.app.Press("id:player-fullscreen")
	p.app.WaitFor("id:player-mode-pill", wait)

	p.app.Resize(874, 402)
	p.app.WaitFor("id:player-panel-toggle", wait)
	p.swipe(-300)
	if !p.playing("f1") {
		t.Fatal("a swipe on its side went on")
	}
	p.app.Resize(402, 874)

	// The line looped is the film's; repeat and the speed are the app's.
	p.app.Press("id:player-more")
	p.app.Press("id:player-settings-repeat")
	p.app.Press("id:player-settings-speed-1.25×")
	p.app.Back()
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-loop")
	p.app.WaitFor("id:player-pill-loop", wait)
	p.swipe(-400)
	p.app.WaitUntil(wait, func() bool { return p.playing("f3") })
	p.loadUpright()
	if p.app.Exists("id:player-pill-loop") || p.app.Exists("id:player-recap") {
		t.Fatal("the next film kept the line looped or showed a recap")
	}
	if !p.app.Exists("id:player-pill-repeat") || p.texts("id:player-pill-speed") != "1.25×" {
		t.Fatal("the next film lost repeat or the speed")
	}
}

// Assistive technology goes to the next film and back by the picture's actions, without the swipe.
func TestThePicturesActionsGoThroughTheList(t *testing.T) {
	p := openFeed(t)
	p.app.AccessibilityAction("id:player-stage", "nextFilm")
	p.app.WaitUntil(wait, func() bool { return p.playing("f3") })
	p.loadUpright()
	p.app.AccessibilityAction("id:player-stage", "prevFilm")
	p.app.WaitUntil(wait, func() bool { return p.playing("f1") })
}
