package media

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDownloaderRejectsEmptyInputBeforeLookingUpBinary(t *testing.T) {
	d := Downloader{Binary: "binary-that-does-not-exist"}
	_, err := d.Download(context.Background(), "", "video.%(ext)s")
	if err == nil || !strings.Contains(err.Error(), "video source is empty") {
		t.Fatal("expected an empty source error")
	}
}

func TestDownloaderReportsMissingYTDLP(t *testing.T) {
	d := Downloader{Binary: "binary-that-does-not-exist"}
	_, err := d.Download(context.Background(), "https://example.com/video", "video.%(ext)s")
	if !errors.Is(err, ErrDownloaderNotFound) {
		t.Fatalf("expected ErrDownloaderNotFound, got %v", err)
	}
}
