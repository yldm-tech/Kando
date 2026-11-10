package main

import (
	"context"
	"testing"

	"kando/apps/kando/cloud"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsAccount renders the player's upload capsule of the prototype's film, How Rivers Shape Landscapes (j1), with an R2 bucket set and signed in: while the upload runs, 40% done, with its Cancel button (upload-busy), and after Cancel was clicked (upload-canceled). The upload is the fake uploader, which reports its progress and then waits for the upload to be canceled.
func TestScreenshotsAccount(t *testing.T) {
	f := newScreenshotFixture(t)
	uploading := func(a *app, tt *ui.Tester, updates chan func()) {
		a.cloud.googleEmail = "reader@example.com"
		a.cloud.r2Endpoint = "account"
		a.cloud.r2Bucket = "films"
		a.cloud.r2AccessKey = "key"
		a.cloud.r2Secret = "secret"
		stop := make(chan struct{})
		t.Cleanup(func() { close(stop) })
		a.cloud.uploader = func(ctx context.Context, _ cloud.Store, _ []cloud.File, progress func(int64, int64)) error {
			progress(40, 100)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-stop:
				return context.Canceled
			}
		}
		screenshotOpen(t, a, tt, updates, "j1", 0)
		item, _ := films.Find(a.films(), "j1")
		screenshotClick(t, tt, a.UploadView(item).Label)
		screenshotWait(t, tt, updates, "upload progress", func() bool { return a.cloud.uploadWritten == 40 })
	}
	f.shoot(t, "upload-busy", uploading)
	f.shoot(t, "upload-canceled", func(a *app, tt *ui.Tester, updates chan func()) {
		uploading(a, tt, updates)
		screenshotClick(t, tt, locales.FilmUploadCancel())
		screenshotWait(t, tt, updates, "upload canceled", func() bool { return a.cloud.uploadPath == "" })
	})
}
