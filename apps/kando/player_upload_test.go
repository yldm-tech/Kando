package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/kando/cloud"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
)

// TestUploadSendsTheVideoAndSubtitle drives the player's upload button. It checks the upload through UploadView's Status and Fix and the texts it hands the capsule, not the wording of why an upload is blocked, which the settings own.
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
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "job1", Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete,
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
	refresh(tt, a)
	item, _ := films.Find(a.films(), "job1")
	openFilm(t, a, tt, updates, item)
	view := a.UploadView(item)
	if got := tt.Find("id:film-upload").Props.Label; got != view.Label || view.Status != film.UploadBlocked || view.Fix != "去登录" || view.Enabled || view.Detail == "" || a.player.film.uploadNote {
		t.Fatalf("upload %+v, button %q, note %v", view, got, a.player.film.uploadNote)
	}
	// The button stays clickable without an account, and opens a note on what is missing, with the way to fix it, instead of uploading.
	tt.Press("id:film-upload")
	if !a.player.film.uploadNote || !hasText(tt, view.Detail) || tt.Find("id:film-upload-fix").Props.Label != view.Fix || a.cloud.uploadPath != "" {
		t.Fatalf("note %v upload %q texts %q", a.player.film.uploadNote, a.cloud.uploadPath, tt.Texts())
	}
	tt.Press("id:film-upload-close")
	if a.player.film.uploadNote || tt.Exists("id:film-upload-capsule") {
		t.Fatalf("note stayed: %q", tt.Texts())
	}
	apply(tt, a, func() {
		a.cloud.googleEmail = "a@b.c"
		a.cloud.r2Endpoint = "acct"
		a.cloud.r2Bucket = "clips"
		a.cloud.r2AccessKey = "key"
		a.cloud.r2Secret = "secret-value"
	})
	view = a.UploadView(item)
	if !view.Enabled || view.Status != "" || view.Fix != "" {
		t.Fatalf("upload %+v", view)
	}
	tt.Press("id:film-upload")
	got := takeUpload(t, uploadedFiles)
	waitUntil(t, tt, a, updates, func() bool { return a.cloud.uploaded[cloud.KindR2+"\n"+video] != "" })
	want := cloud.ObjectKey("a@b.c", sidebar.Title(dir), video)
	if bucket != "clips" || len(got) != 2 || got[0].Key != want || got[1].Key != cloud.SiblingKey(want, srt) {
		t.Fatalf("bucket %q keys %#v", bucket, keysOf(got))
	}
	view = a.UploadView(item)
	if view.Status != film.UploadDone || !strings.Contains(view.Detail, want) || !hasText(tt, view.Detail) {
		t.Fatalf("upload %+v texts %q", view, tt.Texts())
	}
	tt.Press("id:film-upload")
	takeUpload(t, uploadedFiles)
}
