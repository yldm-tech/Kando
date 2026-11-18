package shelf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

func TestLibraryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	lib.Upsert(core.Job{ID: "a", Source: "https://example.com/a", Language: "简体中文", Stage: core.StageComplete, VideoPath: "/v/a.mp4", SubtitlePath: "/v/a.bilingual.srt"})
	lib.Upsert(core.Job{ID: "b", Source: "https://example.com/b", Stage: core.StageFailed, Err: context.Canceled})
	lib.Upsert(core.Job{ID: "a", Source: "https://example.com/a", Stage: core.StageComplete, SubtitlePath: "/v/a.bilingual.srt"})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Jobs) != 2 || loaded.Jobs[0].ID != "b" || loaded.Jobs[1].SubtitlePath != "/v/a.bilingual.srt" {
		t.Fatalf("jobs = %#v", loaded.Jobs)
	}
	if !errors.Is(loaded.Jobs[0].Err, context.Canceled) {
		t.Fatalf("err = %v", loaded.Jobs[0].Err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	want := os.FileMode(0o600)
	if runtime.GOOS == "windows" {
		// Windows has no permission bits: Go reports 0666 for any file that is not read-only; who may read the file is up to its ACL, which a mode does not show.
		want = 0o666
	}
	if info.Mode().Perm() != want {
		t.Fatalf("perm = %o, want %o", info.Mode().Perm(), want)
	}
}

func TestLoadLibraryMissingAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	missing, err := Load(filepath.Join(dir, "missing.json"))
	if err != nil || len(missing.Jobs) != 0 {
		t.Fatalf("missing: jobs=%d err=%v", len(missing.Jobs), err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("expected a corrupt file to fail")
	}
}

// TestLoadQueuesInterruptedJobs checks that a job the app quit in the middle of is read back as queued, so the queue starts it again, while finished jobs stay as they were.
func TestLoadQueuesInterruptedJobs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	done := time.Date(2026, 11, 6, 9, 30, 0, 0, time.UTC)
	lib.Jobs = []core.Job{
		{ID: "dl", Source: "https://example.com/dl", Stage: core.StageDownloading},
		{ID: "tr", Source: "https://example.com/tr", Stage: core.StageTranslating, VideoPath: "/v/tr.mp4"},
		{ID: "q", Source: "https://example.com/q", Stage: core.StageQueued},
		{ID: "ok", Source: "https://example.com/ok", Stage: core.StageComplete, CompletedAt: done},
		{ID: "bad", Source: "https://example.com/bad", Stage: core.StageFailed, Err: errors.New("boom")},
		{ID: "moved", Source: "https://example.com/moved", Stage: core.StageFailed, Err: core.ErrLibrarySwitched},
	}
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]core.Stage{"dl": core.StageQueued, "tr": core.StageQueued, "q": core.StageQueued, "ok": core.StageComplete, "bad": core.StageFailed, "moved": core.StageFailed}
	for _, job := range loaded.Jobs {
		if job.Stage != want[job.ID] {
			t.Errorf("job %s stage %q, want %q", job.ID, job.Stage, want[job.ID])
		}
	}
	if job, _ := loaded.Find("tr"); job.Err != nil || job.VideoPath != "/v/tr.mp4" {
		t.Fatalf("interrupted job %#v", job)
	}
	if job, _ := loaded.Find("ok"); !job.CompletedAt.Equal(done) {
		t.Fatalf("completed at %v, want %v", job.CompletedAt, done)
	}
	if job, _ := loaded.Find("bad"); job.Err == nil || job.Err.Error() != "boom" {
		t.Fatalf("failed job %#v", job)
	}
	if job, _ := loaded.Find("moved"); !errors.Is(job.Err, core.ErrLibrarySwitched) {
		t.Fatalf("stopped job err %v", job.Err)
	}
}

func TestPositionsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	lib.Positions["/v/a.mp4"] = 100*time.Second + 500*time.Millisecond
	lib.Positions["/v/zero.mp4"] = 0
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Positions) != 1 || loaded.Positions["/v/a.mp4"] != 100*time.Second+500*time.Millisecond {
		t.Fatalf("positions %v", loaded.Positions)
	}
}

// TestFurthestRoundTrip checks that how far each video was ever played is kept beside the resume positions, under its own key, and that a video played to the end keeps its furthest point when its resume position is dropped.
func TestFurthestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	lib.Furthest["/v/a.mp4"] = 95*time.Second + 250*time.Millisecond
	lib.Furthest["/v/zero.mp4"] = 0
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Furthest) != 1 || loaded.Furthest["/v/a.mp4"] != 95*time.Second+250*time.Millisecond {
		t.Fatalf("furthest %v", loaded.Furthest)
	}
	if len(loaded.Positions) != 0 {
		t.Fatalf("positions %v", loaded.Positions)
	}
}

// TestPlayedRoundTrip checks that when each video last played is kept under its own key, to the second at least, that a zero time is not written, and that a library.json written before it was kept still reads with none.
func TestPlayedRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 3, 21, 40, 5, 0, time.Local)
	lib.Played["/v/a.mp4"] = at
	lib.Played["/v/zero.mp4"] = time.Time{}
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"played"`) || strings.Contains(string(data), "zero.mp4") {
		t.Fatalf("library.json %s", data)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Played) != 1 || !loaded.Played["/v/a.mp4"].Equal(at) {
		t.Fatalf("played %v", loaded.Played)
	}
	if err := os.WriteFile(path, []byte(`{"jobs":[],"positions":{"/v/a.mp4":3}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	older, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Played) != 0 || older.Positions["/v/a.mp4"] != 3*time.Second {
		t.Fatalf("an older library.json reads played %v positions %v", older.Played, older.Positions)
	}
}

// TestDelaysRoundTrip checks that how far each video's subtitles were moved is kept under its own key, earlier as well as later, exactly to the tenth of a second the keys move them by, and that a video whose subtitles are back in place is not written.
func TestDelaysRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	lib.Delays["/v/a.mp4"] = 2300 * time.Millisecond
	lib.Delays["/v/b.mp4"] = -700 * time.Millisecond
	lib.Delays["/v/zero.mp4"] = 0
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"delays"`) || strings.Contains(string(data), "zero.mp4") {
		t.Fatalf("library.json %s", data)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Delays) != 2 || loaded.Delays["/v/a.mp4"] != 2300*time.Millisecond || loaded.Delays["/v/b.mp4"] != -700*time.Millisecond {
		t.Fatalf("delays %v", loaded.Delays)
	}
	if len(loaded.Positions) != 0 || len(loaded.Furthest) != 0 {
		t.Fatalf("positions %v furthest %v", loaded.Positions, loaded.Furthest)
	}
}

// TestLiftsRoundTrip checks that where each video's subtitles were moved to over its picture is kept under its own key, up as well as down, that a video whose subtitles are in their place is not written, and that a place past the picture's edge, as a hand-edited file may hold, is dropped on reading.
func TestLiftsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	lib.Lifts["/v/a.mp4"] = 0.25
	lib.Lifts["/v/b.mp4"] = -0.05
	lib.Lifts["/v/zero.mp4"] = 0
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"lifts"`) || strings.Contains(string(data), "zero.mp4") {
		t.Fatalf("library.json %s", data)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Lifts) != 2 || loaded.Lifts["/v/a.mp4"] != 0.25 || loaded.Lifts["/v/b.mp4"] != -0.05 {
		t.Fatalf("lifts %v", loaded.Lifts)
	}
	if len(loaded.Delays) != 0 {
		t.Fatalf("delays %v", loaded.Delays)
	}
	if err := os.WriteFile(path, []byte(`{"jobs":[],"lifts":{"/v/a.mp4":0.3,"/v/off.mp4":1.5,"/v/under.mp4":-1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if loaded, err = Load(path); err != nil || len(loaded.Lifts) != 1 || loaded.Lifts["/v/a.mp4"] != 0.3 {
		t.Fatalf("lifts %v (%v), want only a.mp4's", loaded.Lifts, err)
	}
}

// TestMoveVideosKeysByTheNewPath checks that when videos moved on disk, their resume positions, furthest points, subtitle delays and subtitle places are kept under their new paths, those of videos that did not move stay, and MoveVideos says whether anything moved.
func TestMoveVideosKeysByTheNewPath(t *testing.T) {
	lib := Library{
		Positions: map[string]time.Duration{"/old/a.mp4": 5 * time.Second, "/other/b.mp4": 7 * time.Second},
		Furthest:  map[string]time.Duration{"/old/a.mp4": 40 * time.Second, "/other/b.mp4": 9 * time.Second},
		Delays:    map[string]time.Duration{"/old/a.mp4": -300 * time.Millisecond, "/other/b.mp4": time.Second},
		Lifts:     map[string]float32{"/old/a.mp4": 0.2, "/other/b.mp4": -0.1},
		Played:    map[string]time.Time{"/old/a.mp4": time.Unix(100, 0)},
	}
	move := func(path string) (string, bool) {
		if rest, ok := strings.CutPrefix(path, "/old/"); ok {
			return "/new/" + rest, true
		}
		return "", false
	}
	if !lib.MoveVideos(move) {
		t.Fatal("MoveVideos said nothing moved")
	}
	if len(lib.Positions) != 2 || lib.Positions["/new/a.mp4"] != 5*time.Second || lib.Positions["/other/b.mp4"] != 7*time.Second {
		t.Fatalf("positions %v", lib.Positions)
	}
	if len(lib.Furthest) != 2 || lib.Furthest["/new/a.mp4"] != 40*time.Second || lib.Furthest["/other/b.mp4"] != 9*time.Second {
		t.Fatalf("furthest %v", lib.Furthest)
	}
	if len(lib.Delays) != 2 || lib.Delays["/new/a.mp4"] != -300*time.Millisecond || lib.Delays["/other/b.mp4"] != time.Second {
		t.Fatalf("delays %v", lib.Delays)
	}
	if len(lib.Lifts) != 2 || lib.Lifts["/new/a.mp4"] != 0.2 || lib.Lifts["/other/b.mp4"] != -0.1 {
		t.Fatalf("lifts %v", lib.Lifts)
	}
	if len(lib.Played) != 1 || !lib.Played["/new/a.mp4"].Equal(time.Unix(100, 0)) {
		t.Fatalf("played %v", lib.Played)
	}
	if lib.MoveVideos(func(string) (string, bool) { return "", false }) {
		t.Fatal("MoveVideos said something moved when nothing did")
	}
}

// TestLoadReadsTheOldList checks that library.json written before positions were kept, a bare list of jobs, still loads.
func TestLoadReadsTheOldList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	old := `[
  {"id": "a", "source": "https://example.com/a", "stage": "complete", "videoPath": "/v/a.mp4", "subtitlePath": "/v/a.bilingual.srt"},
  {"id": "b", "source": "https://example.com/b", "stage": "failed", "error": "canceled"}
]
`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Jobs) != 2 || lib.Jobs[0].SubtitlePath != "/v/a.bilingual.srt" || !errors.Is(lib.Jobs[1].Err, context.Canceled) {
		t.Fatalf("jobs %#v", lib.Jobs)
	}
	if lib.Positions == nil {
		t.Fatal("positions map is nil")
	}
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "{") {
		t.Fatalf("saved %s", data)
	}
}

func TestLibraryKeepsTheVideoNameButNotTheProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{
		ID: "a", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageFailed, Err: errors.New("download: no formats"),
		Title: "Me at the zoo", Thumbnail: "https://i.ytimg.com/vi/jNQXAC9IVRw/hqdefault.jpg", Progress: core.Fraction(0.4),
	})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	job := loaded.Jobs[0]
	if job.Title != "Me at the zoo" || job.Thumbnail != "https://i.ytimg.com/vi/jNQXAC9IVRw/hqdefault.jpg" || job.Progress != (core.Progress{}) {
		t.Fatalf("job = %+v", job)
	}
}

func TestLibraryKeepsWhyADownloadFailedAndTheBorrowedLogin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "a", Source: "https://www.bilibili.com/video/BV1xx411c7mD", Stage: core.StageFailed, Err: errors.New("download: login"), FailKind: core.FailLoginExpired, Cookies: "chrome"})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(path)
	if job := loaded.Jobs[0]; job.FailKind != core.FailLoginExpired || job.Cookies != "chrome" {
		t.Fatalf("job %+v", job)
	}
}

func TestLibraryKeepsHowManyLinesAreUntranslated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "a", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageComplete, Untranslated: 12, TranslationErr: "translate: KANDO_LLM_MODEL is not set"})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(path)
	if job := loaded.Jobs[0]; job.Untranslated != 12 || job.TranslationErr != "translate: KANDO_LLM_MODEL is not set" {
		t.Fatalf("job %+v", job)
	}
}

// TestLoadDamagedKeepsTheFileUntilAllowSave checks that a library.json cut short comes back as a *DamagedError holding its bytes, and that Save does not write over it until AllowSave.
func TestLoadDamagedKeepsTheFileUntilAllowSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	damaged := []byte(`{"jobs": [{"id": "a", "stage": "complete"}, {"id": "b`)
	if err := os.WriteFile(path, damaged, 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := Load(path)
	var bad *DamagedError
	if !errors.As(err, &bad) || string(bad.Data) != string(damaged) {
		t.Fatalf("Load error = %#v, want a *DamagedError with the file's bytes", err)
	}
	if len(lib.Jobs) != 0 {
		t.Fatalf("jobs of a file that is not JSON = %#v", lib.Jobs)
	}
	lib.Upsert(core.Job{ID: "new", Stage: core.StageQueued})
	if err := lib.Save(); !errors.Is(err, ErrUnread) {
		t.Fatalf("Save = %v, want ErrUnread", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(damaged) {
		t.Fatalf("library.json = %q (%v)", got, err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Save left %s.tmp: %v", path, err)
	}
	lib.AllowSave()
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil || len(again.Jobs) != 1 || again.Jobs[0].ID != "new" {
		t.Fatalf("after AllowSave: %#v %v", again.Jobs, err)
	}
}

// TestLoadDamagedKeepsWhatItCouldRead checks that a value of another type, as another version may write, leaves only that value out: the jobs and furthest points still read.
func TestLoadDamagedKeepsWhatItCouldRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	data := `{"jobs": [{"id": "a", "source": "https://example.com/a", "stage": "complete", "videoPath": "/v/a.mp4"}], "positions": "soon", "furthest": {"/v/a.mp4": 12}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := Load(path)
	var bad *DamagedError
	if !errors.As(err, &bad) {
		t.Fatalf("Load error = %v, want a *DamagedError", err)
	}
	if len(lib.Jobs) != 1 || lib.Jobs[0].VideoPath != "/v/a.mp4" || lib.Furthest["/v/a.mp4"] != 12*time.Second || len(lib.Positions) != 0 {
		t.Fatalf("jobs %#v furthest %v positions %v", lib.Jobs, lib.Furthest, lib.Positions)
	}
}

// A job the app quit in the middle of translating still knows its transcript was saved, so it resumes from the translation once it runs again.
func TestLibraryKeepsWhetherTheJobWasTranscribed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "tr", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageTranslating, VideoPath: "/v/tr.mp4", Transcribed: true})
	lib.Upsert(core.Job{ID: "dl", Source: "https://example.com/dl", Stage: core.StageDownloading})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(path)
	if job, _ := loaded.Find("tr"); job.Stage != core.StageQueued || !job.Transcribed || job.VideoPath != "/v/tr.mp4" {
		t.Fatalf("interrupted job %+v", job)
	}
	if job, _ := loaded.Find("dl"); job.Transcribed {
		t.Fatalf("job never transcribed %+v", job)
	}
}

// A job translating a done film again keeps which part it redoes, and into what language, across a restart, so the job read back as queued translates again rather than downloading and transcribing the film anew.
func TestLibraryKeepsWhatAJobRedoes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "a", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageTranslating, Redo: core.RedoTranslation, RedoLanguage: "日本語"})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(path)
	if job := loaded.Jobs[0]; job.Redo != core.RedoTranslation || job.RedoLanguage != "日本語" || job.Stage != core.StageQueued {
		t.Fatalf("job %+v", job)
	}
}

// TestLibraryKeepsTheSpokenLanguage checks that the language the transcription heard, and the one the user said the video is spoken in, are kept with its job.
func TestLibraryKeepsTheSpokenLanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "a", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageComplete, Language: "简体中文", SpokenLanguage: "ja", SpeechLanguage: "en"})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(path)
	if job := loaded.Jobs[0]; job.SpokenLanguage != "ja" || job.SpeechLanguage != "en" || job.Language != "简体中文" {
		t.Fatalf("job %+v", job)
	}
}

// A film that keeps no video is kept as one, and a job downloading its video says so after a restart.
func TestLibraryKeepsSubtitlesOnlyFilms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "a", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageQueued, Quality: core.QualityBest, SubtitlesOnly: true, Redo: core.RedoVideo})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"subtitlesOnly": true`) {
		t.Fatalf("library.json %s", data)
	}
	loaded, _ := Load(path)
	if job := loaded.Jobs[0]; !job.SubtitlesOnly || job.Redo != core.RedoVideo {
		t.Fatalf("job %+v", job)
	}
}

// The quality and the source of the original lines a job was given, and whether its film took its site's subtitles, are kept with it; a library written before them reads back without them.
func TestLibraryKeepsTheDownloadChoices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "a", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageComplete, Quality: core.Quality720, SubtitleSource: core.SubtitlesPlatform, PlatformSubtitles: true, LocalLines: 3})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(path)
	if job := loaded.Jobs[0]; job.Quality != core.Quality720 || job.SubtitleSource != core.SubtitlesPlatform || !job.PlatformSubtitles || job.LocalLines != 3 {
		t.Fatalf("job %+v", job)
	}

	old := filepath.Join(t.TempDir(), "library.json")
	if err := os.WriteFile(old, []byte(`{"jobs":[{"id":"b","source":"https://www.youtube.com/watch?v=jNQXAC9IVRw","stage":"complete"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, _ = Load(old)
	if job := loaded.Jobs[0]; job.Quality != "" || job.SubtitleSource != "" || job.PlatformSubtitles || job.LocalLines != 0 {
		t.Fatalf("old job %+v", job)
	}
}

// A job processing a video found in the library's folder stays one across a restart, so if the job read back as queued then fails, the video is still put back as the film it was rather than left as a failed card.
func TestLibraryKeepsAJobFromTheFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "a", Source: "file:///films/talk.mp4", VideoPath: "/films/talk.mp4", Stage: core.StageTranscribing, FromFolder: true})
	lib.Upsert(core.Job{ID: "b", Source: "file:///films/other.mp4", Stage: core.StageQueued})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(path)
	if a, _ := loaded.Find("a"); !a.FromFolder || a.Stage != core.StageQueued {
		t.Fatalf("job from the folder %+v", a)
	}
	if b, _ := loaded.Find("b"); b.FromFolder {
		t.Fatalf("job added %+v", b)
	}
}

// The paired phone a job's link came from, and the one that paused it, stay with the job across a restart, so its card still says where it came from and that it waits for the user.
func TestLibraryKeepsTheDeviceAJobCameFrom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "a", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageQueued, Device: "Evan's iPhone", PausedBy: "Evan's iPad"})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(path)
	if a, _ := loaded.Find("a"); a.Device != "Evan's iPhone" || a.PausedBy != "Evan's iPad" {
		t.Fatalf("job %+v", a)
	}
}

// The series the user said a film is part of stays with its job across a restart, so its translation keeps using the series' glossary.
func TestLibraryKeepsTheSeriesOfAJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, _ := Load(path)
	lib.Upsert(core.Job{ID: "a", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageComplete, Series: "sub-1"})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(path)
	if a, _ := loaded.Find("a"); a.Series != "sub-1" {
		t.Fatalf("job %+v", a)
	}
}

// TestRenameKeepsTheTitleGivenInKando checks that a title the user gives a video is kept trimmed, read back from library.json, dropped by an empty one, and that Rename says whether anything changed.
func TestRenameKeepsTheTitleGivenInKando(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !lib.Rename("/v/a.mp4", "  Linear Algebra 1  ") || !lib.Rename("/v/b.mp4", "Talk") {
		t.Fatal("Rename said nothing changed")
	}
	if lib.Rename("/v/a.mp4", "Linear Algebra 1") {
		t.Fatal("Rename to the same title said it changed")
	}
	if lib.Rename("", "Nothing") || lib.Rename("/v/c.mp4", "   ") {
		t.Fatal("Rename of no video, or to nothing for a video with no title, said it changed")
	}
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Names) != 2 || loaded.Names["/v/a.mp4"] != "Linear Algebra 1" || loaded.Names["/v/b.mp4"] != "Talk" {
		t.Fatalf("names %v", loaded.Names)
	}
	if !loaded.Rename("/v/b.mp4", "") || len(loaded.Names) != 1 {
		t.Fatalf("an empty title kept %v", loaded.Names)
	}
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "/v/b.mp4") {
		t.Fatalf("library.json still names the video whose title was dropped:\n%s", data)
	}

	var empty Library
	if !empty.Rename("/v/a.mp4", "Talk") || empty.Names["/v/a.mp4"] != "Talk" {
		t.Fatalf("a library with no titles yet kept %v", empty.Names)
	}
}

// TestMoveVideosMovesTheTitles checks that a title given in Kando follows its video when the video moves on disk, as a folder renamed in Finder moves it.
func TestMoveVideosMovesTheTitles(t *testing.T) {
	lib := Library{Names: map[string]string{"/old/a/video.mp4": "My talk", "/other/b.mp4": "B"}}
	moved := lib.MoveVideos(func(path string) (string, bool) {
		if path == "/old/a/video.mp4" {
			return "/old/renamed/video.mp4", true
		}
		return "", false
	})
	if !moved {
		t.Fatal("MoveVideos said nothing moved")
	}
	if len(lib.Names) != 2 || lib.Names["/old/renamed/video.mp4"] != "My talk" || lib.Names["/other/b.mp4"] != "B" {
		t.Fatalf("names %v", lib.Names)
	}
}

// TestUpsertCarriesTheTitleToTheNewVideo checks that when processing a job again leaves its video at another path, the title given to it in Kando goes with it, and that a job whose video stays where it was keeps its title.
func TestUpsertCarriesTheTitleToTheNewVideo(t *testing.T) {
	lib := Library{Names: map[string]string{"/lib/My folder/video.mp4": "My talk"}}
	lib.Jobs = []core.Job{{ID: "a", Stage: core.StageComplete, VideoPath: "/lib/My folder/video.mp4"}}
	lib.Upsert(core.Job{ID: "a", Stage: core.StageDownloading, VideoPath: "/lib/My folder/video.mp4", Title: "Site title"})
	if lib.Names["/lib/My folder/video.mp4"] != "My talk" {
		t.Fatalf("names %v", lib.Names)
	}
	lib.Upsert(core.Job{ID: "a", Stage: core.StageTranscribing, VideoPath: "/lib/Site title [x]/video.mp4", Title: "Site title"})
	if len(lib.Names) != 1 || lib.Names["/lib/Site title [x]/video.mp4"] != "My talk" {
		t.Fatalf("names %v", lib.Names)
	}
}

// TestStoredJobKeepsEveryField fills every field of core.Job but its live Progress, saves it and reads it back, so a field added to core.Job but not to storedJob, or copied one way only (storeJob, storedJob.job), fails here rather than being dropped from library.json on the next save. The job is a failed one, a stage Load keeps as it is.
func TestStoredJobKeepsEveryField(t *testing.T) {
	var job core.Job
	v := reflect.ValueOf(&job).Elem()
	for i := range v.NumField() {
		field, value := v.Type().Field(i), v.Field(i)
		switch {
		case field.Name == "Progress":
		case field.Name == "Stage":
			job.Stage = core.StageFailed
		case field.Name == "Err":
			job.Err = errors.New("the download stopped")
		case value.Kind() == reflect.String:
			value.SetString("kept " + field.Name)
		case value.Kind() == reflect.Bool:
			value.SetBool(true)
		case value.Kind() == reflect.Int:
			value.SetInt(int64(i + 1))
		case field.Type == reflect.TypeFor[time.Time]():
			value.Set(reflect.ValueOf(time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)))
		default:
			t.Fatalf("core.Job.%s is a %s, which this test does not fill; give it a value here", field.Name, field.Type)
		}
	}
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	lib.Upsert(job)
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || len(loaded.Jobs) != 1 {
		t.Fatalf("jobs %#v, %v", loaded.Jobs, err)
	}
	got := reflect.ValueOf(loaded.Jobs[0])
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		if name == "Progress" {
			continue
		}
		if name == "Err" {
			if loaded.Jobs[0].Err == nil || loaded.Jobs[0].Err.Error() != job.Err.Error() {
				t.Errorf("Err came back as %v, want %v", loaded.Jobs[0].Err, job.Err)
			}
			continue
		}
		if want, back := v.Field(i).Interface(), got.Field(i).Interface(); !reflect.DeepEqual(back, want) {
			t.Errorf("%s came back as %#v, want %#v", name, back, want)
		}
	}
}
