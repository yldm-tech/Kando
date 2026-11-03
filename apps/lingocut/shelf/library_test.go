package shelf

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
