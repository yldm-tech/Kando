package main

import (
	"errors"
	"fmt"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// The screenshot tests render the main scenes to PNG files in the directory named by KANDO_SCREENSHOTS, so a visual change can be compared before and after, and with the prototype. They are skipped when the variable is unset. Each area renders its own scenes in its own screenshot_<area>_test.go (TestScreenshots<Area>), from the library this file makes; `go test -run TestScreenshots` renders them all. The window is light, as main sets it, and the size of the prototype's window (1304×782) unless a scene asks for another.

// screenshotFixture is the library every scene starts from: the prototype's (screenshot_prototype_test.go). Its two libraries, 我的影片 (open) and 英语听力, are ~/Movies/<name>.kando in a new empty home folder; its films are jobs whose IDs are the prototype's (j1 How Rivers Shape Landscapes, j2 Morning Market in Kyoto, …), each in the prototype's stage, with the prototype's error for a failed one. A film that has its video has it in a folder named after its title, a placeholder the fake ffmpeg plays as the film's scene picture (writeFakeScene); a finished one has the prototype's 18 subtitles beside it, as the pipeline exports them. A film not downloaded yet has no video, as in the app. The scenes do not see this machine: the pipeline runs the fake ffmpeg, yt-dlp and whisper-cli in ~/bin (fake_tools_test.go), so every path a scene shows starts with ~ and the versions and the pictures are the same everywhere. A scene picks a film by its prototype ID (screenshotOpen, f.job) and changes the state it needs in its steps; a scene that needs another folder makes it under home.
type screenshotFixture struct {
	out  string
	home string
	// lib is 我的影片, the library open; libs are the prototype's libraries by ID ("main", "listening").
	lib      string
	libs     map[string]string
	pipeline media.Pipeline
	jobs     []core.Job
}

// newScreenshotFixture makes the prototype's library in a fake home folder (screenshotHome), or skips t when KANDO_SCREENSHOTS is unset. The scenes read the interface in KANDO_SCREENSHOTS_LANG, one of locales.Interface, or in locales.Default without it.
func newScreenshotFixture(t *testing.T) *screenshotFixture {
	t.Helper()
	out := os.Getenv("KANDO_SCREENSHOTS")
	if out == "" {
		t.Skip("set KANDO_SCREENSHOTS=<dir> to render the screenshots")
	}
	if lang := os.Getenv("KANDO_SCREENSHOTS_LANG"); lang != "" {
		if !locales.Supported(lang) {
			t.Fatalf("KANDO_SCREENSHOTS_LANG=%s is not one of %q", lang, locales.Interface)
		}
		testlocale.Use(t, lang)
		useProtoContent(t, lang)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	f := newPrototypeLibrary(t, screenshotHome(t))
	f.out = out
	return f
}

// newPrototypeLibrary writes the prototype's libraries, videos and subtitles into home and returns the fixture without a folder for its pictures.
func newPrototypeLibrary(t *testing.T, home string) *screenshotFixture {
	t.Helper()
	libs := map[string]string{}
	for _, lib := range protoLibraries {
		dir := filepath.Join(home, "Movies", lib.Name+".kando")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		libs[lib.ID] = dir
	}
	lib := libs[protoLibraries[0].ID]
	jobs := make([]core.Job, 0, len(protoFilms))
	for _, film := range protoFilms {
		job := core.Job{ID: film.ID, Language: protoTarget, Stage: film.Stage, LibraryDir: libs[film.Library]}
		if film.Seconds > 0 {
			job.VideoPath = filepath.Join(job.LibraryDir, film.Title, "video.mp4")
			writeFakeScene(t, job.VideoPath, film.Seconds, film.Scene)
		}
		switch film.Platform {
		case core.PlatformYouTube:
			job.Source = "https://www.youtube.com/watch?v=" + film.VideoID
		case core.PlatformBilibili:
			job.Source = "https://www.bilibili.com/video/" + film.VideoID
		case core.PlatformLocal:
			job.Source = (&url.URL{Scheme: "file", Path: job.VideoPath}).String()
		}
		if film.Stage == core.StageComplete {
			job.SubtitlePath = writeProtoSubtitles(t, job.VideoPath)
		}
		if film.Queue != nil && film.Queue.Err != "" {
			job.Err = errors.New(film.Queue.Err)
		}
		jobs = append(jobs, job)
	}
	bin := filepath.Join(home, "bin")
	pipeline := media.Tools{
		OutputDir:  lib,
		YTDLP:      fakeTool(t, bin, "yt-dlp"),
		FFmpeg:     fakeTool(t, bin, "ffmpeg"),
		Whisper:    fakeTool(t, bin, "whisper-cli"),
		Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel},
	}
	return &screenshotFixture{home: home, lib: lib, libs: libs, pipeline: pipeline, jobs: jobs}
}

// writeProtoSubtitles writes the prototype's subtitles beside video as the pipeline exports them, video.srt with the original lines and video.bilingual.srt with each translation above its original, and returns the bilingual one. A cue whose translation failed has only its original line.
func writeProtoSubtitles(t *testing.T, video string) string {
	t.Helper()
	clock := func(d time.Duration) string {
		ms := d.Milliseconds()
		return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
	}
	var original, bilingual strings.Builder
	for i, cue := range protoCues {
		span := clock(cue.Start) + " --> " + clock(cue.End)
		fmt.Fprintf(&original, "%d\n%s\n%s\n\n", i+1, span, cue.Original)
		text := cue.Original
		if cue.Translation != "" {
			text = cue.Translation + "\n" + cue.Original
		}
		fmt.Fprintf(&bilingual, "%d\n%s\n%s\n\n", i+1, span, text)
	}
	base := strings.TrimSuffix(video, filepath.Ext(video))
	if err := os.WriteFile(base+".srt", []byte(original.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".bilingual.srt", []byte(bilingual.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return base + ".bilingual.srt"
}

// sidecarLibrary makes ~/Movies/Copied Films.kando, a library copied from another Mac without its job records: Copied Lecture, the newer, has the subtitles the pipeline exported for it beside its video (writeProtoSubtitles), and Raw Recording has none. It returns the library and Copied Lecture's video, whose path is its card's key.
func (f *screenshotFixture) sidecarLibrary(t *testing.T) (dir, video string) {
	t.Helper()
	dir = filepath.Join(f.home, "Movies", "Copied Films.kando")
	video = filepath.Join(dir, "Copied Lecture", "video.mp4")
	raw := filepath.Join(dir, "Raw Recording", "video.mp4")
	writeFakeClip(t, video, 180, "2F4A6B", "C9D6E3")
	writeProtoSubtitles(t, video)
	writeFakeClip(t, raw, 95, "5B4632", "E3D2BC")
	// The shelf lists the newest video first.
	for path, at := range map[string]time.Time{video: time.Unix(1_700_000_200, 0), raw: time.Unix(1_700_000_100, 0)} {
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	return dir, video
}

// showLibrary makes dir the open library, as picking it in the sidebar does, and returns once the shelf has read it and its cards' posters show.
func (f *screenshotFixture) showLibrary(t *testing.T, a *app, tt *ui.Tester, updates chan func(), dir string) {
	t.Helper()
	a.edit(func(s *session) {
		s.LibraryDir = dir
		s.Libraries = append(s.Libraries, dir)
	})
	a.refreshLibrary()
	tt.Frame()
	screenshotWait(t, tt, updates, "posters", func() bool { return len(a.lib.posterBusy) == 0 })
	tt.Frame()
}

// job is the fixture's job for the prototype's film id.
func (f *screenshotFixture) job(t *testing.T, id string) core.Job {
	t.Helper()
	for _, job := range f.jobs {
		if job.ID == id {
			return job
		}
	}
	t.Fatalf("the fixture has no film %s", id)
	return core.Job{}
}

// screenshotHome cuts t off from this machine until it ends and returns the fake home folder: HOME (USERPROFILE on Windows) is a new empty folder, so the config, cache and speech model folders are empty ones in it and the window shows paths in it as ~; the XDG folders follow HOME; PATH is only ~/bin, where the fixture puts the fake tools, so nothing finds a tool installed here; and every KANDO_ and EVERYAPI_ variable is blank, so no credentials, endpoints, models or library folder of this machine reach a scene.
func screenshotHome(t *testing.T) string {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "KANDO_") || strings.HasPrefix(name, "EVERYAPI_") {
			t.Setenv(name, "")
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(name, "")
	}
	t.Setenv("PATH", filepath.Join(home, "bin"))
	return home
}

// shoot opens the window on 我的影片 at the prototype's size, 1304×782, runs steps and saves what it shows as name.png.
func (f *screenshotFixture) shoot(t *testing.T, name string, steps func(a *app, tt *ui.Tester, updates chan func())) {
	t.Helper()
	f.shootSize(t, name, 1304, 782, steps)
}

// shootSize is shoot at width×height. Before steps run, the cards' posters have been grabbed and the window shows them, and the player remembers where the prototype stopped each film (protoFilm.Watched).
func (f *screenshotFixture) shootSize(t *testing.T, name string, width, height int, steps func(a *app, tt *ui.Tester, updates chan func())) {
	t.Helper()
	updates := make(chan func(), 64)
	a := newApp(f.pipeline, func(fn func()) { updates <- fn })
	a.prefs.listModels = offlineModels
	a.prefs.interfaceLanguage = testlocale.Current()
	a.edit(func(s *session) {
		s.LibraryDir = f.lib
		for _, lib := range protoLibraries {
			s.Libraries = append(s.Libraries, f.libs[lib.ID])
		}
		s.LibraryParent = filepath.Dir(f.lib)
	})
	a.library.Jobs = append([]core.Job(nil), f.jobs...)
	watched := map[string]time.Duration{}
	for _, film := range protoFilms {
		if film.Watched > 0 {
			watched[f.job(t, film.ID).VideoPath] = film.Watched
		}
	}
	a.setResumePositions(watched)
	a.refreshLibrary()
	t.Cleanup(a.stopPlayback)
	tt := ui.NewTester(a.view, width, height)
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
	tt.Frame()
	screenshotWait(t, tt, updates, "posters", func() bool { return len(a.lib.posterBusy) == 0 })
	tt.Frame()
	steps(a, tt, updates)
	tt.Frame()
	file, err := os.Create(filepath.Join(f.out, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// screenshotWait runs the window's updates and draws a frame after each until done reports true, or fails t after ten seconds, naming what it waited for.
func screenshotWait(t *testing.T, tt *ui.Tester, updates chan func(), what string, done func() bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !done() {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("%s did not load; texts %q", what, tt.Texts())
		}
		tt.Frame()
	}
}

// screenshotOpen opens the prototype's film id on the player, held at at, and returns once its subtitles and, when it has a video, its picture have loaded and the first frame shows. Cue i of protoCues shows from protoCues[i].Start.
func screenshotOpen(t *testing.T, a *app, tt *ui.Tester, updates chan func(), id string, at time.Duration) {
	t.Helper()
	item, ok := films.Find(a.films(), id)
	if !ok {
		t.Fatalf("film %s is not on the open library's shelf", id)
	}
	a.player.playFrom = at
	a.Open(item)
	tt.Frame()
	picture := a.ffmpegPath() != "" && item.VideoPath != ""
	screenshotWait(t, tt, updates, "film "+id, func() bool {
		return a.player.subs.ready && (!picture || a.player.play != nil || a.player.playErr != "")
	})
	if a.player.playErr != "" {
		t.Fatalf("film %s: %s", id, a.player.playErr)
	}
	if !picture {
		return
	}
	// The picture's first frame comes from the decoder in the background.
	for range 20 {
		tt.Advance(50 * time.Millisecond)
		tt.Frame()
		time.Sleep(20 * time.Millisecond)
	}
}

// screenshotClick clicks the control labeled label and draws the next frame.
func screenshotClick(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
}

// TestScreenshotFixtureMatchesPrototype checks, without rendering anything, that the fixture's library shows what the prototype counts: 14 films in 我的影片 by the sidebar's sources, 1 in 英语听力, every film that has its video under the prototype's title, and every finished one with the prototype's 18 subtitles at their times.
func TestScreenshotFixtureMatchesPrototype(t *testing.T) {
	f := newPrototypeLibrary(t, screenshotHome(t))
	for _, lib := range protoLibraries {
		dir := f.libs[lib.ID]
		videos, err := media.ListVideos(dir)
		if err != nil {
			t.Fatal(err)
		}
		shelf := films.List(dir, f.jobs, videos, jobName, statusText)
		if len(shelf) != lib.Count {
			t.Errorf("%s has %d films, want %d", lib.Name, len(shelf), lib.Count)
		}
		if lib.ID != protoLibraries[0].ID {
			continue
		}
		for source, want := range protoSourceCounts {
			if got := len(films.BySource(shelf, source)); got != want {
				t.Errorf("%s: %d films from %s, want %d", lib.Name, got, source, want)
			}
		}
	}
	for _, film := range protoFilms {
		job := f.job(t, film.ID)
		if job.VideoPath != "" && jobName(job) != film.Title {
			t.Errorf("%s is named %q, want %q", film.ID, jobName(job), film.Title)
		}
		if film.Stage != core.StageComplete {
			continue
		}
		data, err := os.ReadFile(job.SubtitlePath)
		if err != nil {
			t.Fatal(err)
		}
		cues, err := media.ParseSRT(data)
		if err != nil || len(cues) != len(protoCues) {
			t.Fatalf("%s: %d cues, %v", film.ID, len(cues), err)
		}
		for i, cue := range cues {
			want := strings.TrimPrefix(protoCues[i].Translation+"\n"+protoCues[i].Original, "\n")
			if cue.Start != protoCues[i].Start || cue.End != protoCues[i].End || cue.Text != want {
				t.Errorf("%s cue %d is %v-%v %q", film.ID, i, cue.Start, cue.End, cue.Text)
			}
		}
	}
}
