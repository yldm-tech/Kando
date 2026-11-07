package shelf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o", info.Mode().Perm())
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
