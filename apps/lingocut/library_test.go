package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"lingocut/packages/core"
)

func TestLibraryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib := library{path: path}
	lib.upsert(core.Job{ID: "a", Source: "https://example.com/a", Language: "简体中文", Stage: core.StageComplete, VideoPath: "/v/a.mp4", SubtitlePath: "/v/a.bilingual.srt"})
	lib.upsert(core.Job{ID: "b", Source: "https://example.com/b", Stage: core.StageFailed, Err: context.Canceled})
	lib.upsert(core.Job{ID: "a", Source: "https://example.com/a", Stage: core.StageComplete, SubtitlePath: "/v/a.bilingual.srt"})
	if err := lib.save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.jobs) != 2 || loaded.jobs[0].ID != "b" || loaded.jobs[1].SubtitlePath != "/v/a.bilingual.srt" {
		t.Fatalf("jobs = %#v", loaded.jobs)
	}
	if !errors.Is(loaded.jobs[0].Err, context.Canceled) {
		t.Fatalf("err = %v", loaded.jobs[0].Err)
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
	missing, err := loadLibrary(filepath.Join(dir, "missing.json"))
	if err != nil || len(missing.jobs) != 0 {
		t.Fatalf("missing: jobs=%d err=%v", len(missing.jobs), err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLibrary(bad); err == nil {
		t.Fatal("expected a corrupt file to fail")
	}
}
