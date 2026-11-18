package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
)

// revisedCues are the talk's lines as its desktop has them after translating it again: the first line says something else in both languages.
var revisedCues = []sync.Cue{
	{StartMS: 1000, EndMS: 2000, Original: "Good morning!", Translation: "早安！"},
	{StartMS: 3000, EndMS: 4500, Original: "How are you?", Translation: "你好吗？"},
	{StartMS: 5000, EndMS: 6500, Original: "Fine, thanks.", Translation: "很好，谢谢。"},
	{StartMS: 7000, EndMS: 9000, Original: "See you.", Translation: "再见。"},
}

// keptFrom puts the talk on the phone as downloaded from d on the day now() names, with its lines as they were then (talkCues), and returns the phone's data folder, paired with d.
func keptFrom(t *testing.T, d *synctest.Desktop) (string, store.Film) {
	t.Helper()
	dir := t.TempDir()
	st := store.Open(dir)
	f := store.Film{Desktop: store.DesktopID(d.Fingerprint), Library: "Japanese Class", Video: "video.mp4", Added: now(),
		Film: sync.Film{ID: "f1", Title: "Morning Talk", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", VideoSize: int64(len(talkVideo)), Updated: now()}}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues(f.Desktop, f.Film.ID, talkCues); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir(f.Desktop, f.Film.ID), f.Video), talkVideo, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, f
}

// desktopWith is a desktop that serves the talk in its library with lines, as changed at updated.
func desktopWith(t *testing.T, lines []sync.Cue, updated time.Time) *synctest.Desktop {
	t.Helper()
	d := synctest.New(t, "Studio")
	d.AddLibrary("lib", "Japanese Class")
	d.AddFilm(synctest.Film{Library: "lib", Video: talkVideo, Cues: lines,
		Film: sync.Film{ID: "f1", Title: "Morning Talk", Source: "youtube", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", Updated: updated}})
	d.Offer("123456")
	return d
}

// playKept opens the talk kept in dir in the player and plays it to its first line, once the player has read the lines kept with it: they are read off the engine's goroutine, and until then no line plays and the subtitles are not shown at all.
func playKept(t *testing.T, d *synctest.Desktop, dir string) *phone {
	t.Helper()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.app.Press(talkRow(store.DesktopID(d.Fingerprint)))
	p.app.WaitFor("id:film-play", wait)
	p.app.Press("id:film-play")
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line") })
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(1500 * time.Millisecond)
	return p
}

// A film translated again on its desktop since the phone downloaded it plays with the desktop's lines once the desktop answers, and keeps them: the phone's copy of its lines and its record follow the desktop's, while its video stays as it was downloaded.
func TestAPlayedFilmFollowsItsDesktopsNewLines(t *testing.T) {
	d := desktopWith(t, revisedCues, now().Add(time.Hour))
	dir, f := keptFrom(t, d)
	p := playKept(t, d, dir)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:player-subtitles") == "早安！\nGood morning!" })

	st := store.Open(dir)
	if cues, ok, err := st.Cues(f.Desktop, f.Film.ID); err != nil || !ok || cues[0].Original != "Good morning!" {
		t.Fatalf("the phone kept lines %+v, %v, %v", cues, ok, err)
	}
	films, err := st.Films()
	if err != nil || len(films) != 1 || !films[0].Film.Updated.Equal(now().Add(time.Hour)) || films[0].Video != f.Video || films[0].Film.VideoSize != f.Film.VideoSize {
		t.Fatalf("the phone kept %+v, %v", films, err)
	}
}

// A film played while its desktop is slow to answer keeps the position it played to: the desktop's record, once it comes, is merged into the phone's as it then stands, not as it stood when the player opened.
func TestAFilmFollowedLateKeepsThePositionPlayedMeanwhile(t *testing.T) {
	d := desktopWith(t, revisedCues, now().Add(time.Hour))
	dir, f := keptFrom(t, d)
	release := d.Hold()
	t.Cleanup(release)
	p := playKept(t, d, dir)
	st := store.Open(dir)
	p.at(7 * time.Second)
	p.app.WaitUntil(wait, func() bool {
		films, err := st.Films()
		return err == nil && len(films) == 1 && films[0].PositionMS == 7000
	})

	release()
	p.app.WaitUntil(wait, func() bool {
		films, err := st.Films()
		return err == nil && len(films) == 1 && films[0].Film.Updated.Equal(now().Add(time.Hour))
	})
	films, _ := st.Films()
	if films[0].PositionMS != 7000 || films[0].Film.VideoSize != f.Film.VideoSize {
		t.Fatalf("the phone kept %+v", films[0])
	}
	if cues, ok, err := st.Cues(f.Desktop, f.Film.ID); err != nil || !ok || cues[0].Original != "Good morning!" {
		t.Fatalf("the phone kept lines %+v, %v, %v", cues, ok, err)
	}
}

// A desktop that does not answer does not hold the app up as it closes: the check is cancelled with the session rather than left to time out.
func TestAnUnansweredFollowDoesNotHoldTheAppClosing(t *testing.T) {
	d := desktopWith(t, revisedCues, now().Add(time.Hour))
	dir, _ := keptFrom(t, d)
	t.Cleanup(d.Hold())
	var closing time.Time
	// Cleanups run last first: this one runs after the phone has stopped, and the one below just before.
	t.Cleanup(func() {
		if took := time.Since(closing); took > 5*time.Second {
			t.Errorf("the app took %v to close", took)
		}
	})
	playKept(t, d, dir)
	t.Cleanup(func() { closing = time.Now() })
}
