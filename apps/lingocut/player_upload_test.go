package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lingocut/apps/lingocut/cloud"
	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/core"
)

// TestUploadSendsTheVideoAndSubtitle drives the player's upload button through UploadView: blocked without an account, then the video and its subtitle sent to the bucket, then done, with the object's key in its line. It does not check the wording of why an upload is blocked, which the settings own.
func TestUploadSendsTheVideoAndSubtitle(t *testing.T) {
	dir := t.TempDir()
	videoDir := filepath.Join(dir, "talk")
	if err := os.Mkdir(videoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(videoDir, "talk.mp4")
	srt := filepath.Join(videoDir, "talk.bilingual.srt")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "job1", Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: srt,
	}}
	uploadedFiles := make(chan []cloud.File, 2)
	var bucket string
	a.cloud.uploader = func(_ context.Context, cfg cloud.Store, files []cloud.File, progress func(int64, int64)) error {
		if cfg.Kind != cloud.KindR2 || cfg.Secret != "secret-value" || cfg.AccessKey != "key" {
			return errors.New("credentials were not passed")
		}
		bucket = cfg.Bucket
		if progress != nil {
			progress(1, 1)
		}
		uploadedFiles <- append([]cloud.File(nil), files...)
		return nil
	}
	tt.Frame()
	if err := tt.Click("talk"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	waitUpdate(t, updates)
	tt.Frame()
	// Without an account the upload is blocked: the button shows, disabled, with a line saying why. The wording of that line is the settings', so only that the window shows one is checked here.
	item, _ := films.Find(a.films(), a.openKey)
	view := a.UploadView(item)
	if !view.Show || view.Enabled || view.Detail == "" || !tt.HasText(view.Label) || !tt.HasText(view.Detail) {
		t.Fatalf("upload %+v texts %q", view, tt.Texts())
	}
	a.cloud.googleEmail = "a@b.c"
	a.cloud.r2Endpoint = "acct"
	a.cloud.r2Bucket = "clips"
	a.cloud.r2AccessKey = "key"
	a.cloud.r2Secret = "secret-value"
	tt.Frame()
	if view = a.UploadView(item); !view.Enabled {
		t.Fatalf("upload %+v", view)
	}
	if err := tt.Click(view.Label); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	got := takeUpload(t, uploadedFiles)
	waitUntil(t, updates, func() bool { return a.cloud.uploaded[cloud.KindR2+"\n"+video] != "" })
	tt.Frame()
	want := cloud.ObjectKey("a@b.c", sidebar.Title(dir), video)
	if bucket != "clips" || len(got) != 2 || got[0].Key != want || got[1].Key != cloud.SiblingKey(want, srt) {
		t.Fatalf("bucket %q keys %#v", bucket, keysOf(got))
	}
	if view = a.UploadView(item); !view.Enabled || !strings.Contains(view.Detail, want) || !tt.HasText(view.Detail) {
		t.Fatalf("upload %+v texts %q", view, tt.Texts())
	}
	if err := tt.Click(view.Label); err != nil {
		t.Fatal(err)
	}
	takeUpload(t, uploadedFiles)
}
