package cloud

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpointAcceptsAnAccountIDOrURL(t *testing.T) {
	if got := Endpoint("abc123"); got != "https://abc123.r2.cloudflarestorage.com" {
		t.Fatalf("account id endpoint = %q", got)
	}
	if got := Endpoint("https://abc123.r2.cloudflarestorage.com/"); got != "https://abc123.r2.cloudflarestorage.com" {
		t.Fatalf("url endpoint = %q", got)
	}
	if Endpoint("  ") != "" {
		t.Fatal("blank endpoint")
	}
}

func TestObjectKeyUsesEmailLibraryAndVideoFolder(t *testing.T) {
	got := ObjectKey("a@b.c", "影视库", "/Movies/影视库.lingocut/Learn Rust/video.mp4")
	want := "a@b.c/影视库/Learn Rust/video.mp4"
	if got != want {
		t.Fatalf("key = %q", got)
	}
	sub := SiblingKey(got, "/Movies/影视库.lingocut/Learn Rust/video.bilingual.srt")
	if sub != "a@b.c/影视库/Learn Rust/video.bilingual.srt" {
		t.Fatalf("subtitle key = %q", sub)
	}
	got = ObjectKey("a/../b", "库", "/tmp/../x/video.mp4")
	for _, part := range strings.Split(got, "/") {
		if part == "." || part == ".." {
			t.Fatalf("segment %q in %q", part, got)
		}
	}
}

type memPut struct {
	keys []string
	body []byte
}

func (m *memPut) Put(_ context.Context, _, key, contentType string, body io.Reader, _ int64) error {
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	m.keys = append(m.keys, key+" "+contentType)
	m.body = append(m.body, buf...)
	return nil
}

func TestUploadReportsProgressAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	sub := filepath.Join(dir, "video.bilingual.srt")
	if err := os.WriteFile(video, []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sub, []byte("ef"), 0o644); err != nil {
		t.Fatal(err)
	}
	var last, total int64
	store := &memPut{}
	files := []File{
		{Key: "a/video.mp4", Path: video, ContentType: ContentType(video)},
		{Key: "a/video.bilingual.srt", Path: sub, ContentType: ContentType(sub)},
	}
	err := Upload(context.Background(), store, "bucket", files, func(written, all int64) {
		last, total = written, all
	})
	if err != nil {
		t.Fatal(err)
	}
	if last != 6 || total != 6 {
		t.Fatalf("progress %d/%d", last, total)
	}
	if strings.Join(store.keys, ",") != "a/video.mp4 video/mp4,a/video.bilingual.srt application/x-subrip" {
		t.Fatalf("puts %v", store.keys)
	}
	if string(store.body) != "abcdef" {
		t.Fatalf("body %q", store.body)
	}
}
