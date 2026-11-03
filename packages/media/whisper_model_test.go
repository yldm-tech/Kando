package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSpeechModelsRecommendOneTurbo(t *testing.T) {
	models := SpeechModels()
	if len(models) != 7 {
		t.Fatalf("len = %d", len(models))
	}
	recommended := 0
	for _, model := range models {
		if model.Recommended {
			recommended++
			if model.ID != "large-v3-turbo-q5" || model.File != "ggml-large-v3-turbo-q5_0.bin" {
				t.Fatalf("recommended %#v", model)
			}
		}
	}
	if recommended != 1 {
		t.Fatalf("recommended = %d", recommended)
	}
}

func TestResolveSpeechModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ggml-small.bin")
	if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveSpeechModel("", []string{dir}, "ggml-small.bin"); got != path {
		t.Fatalf("found %q", got)
	}
	if got := ResolveSpeechModel("", []string{dir}, "ggml-tiny.bin"); got != "" {
		t.Fatalf("missing model resolved to %q", got)
	}
	other := filepath.Join(t.TempDir(), "ggml-small.bin")
	if err := os.WriteFile(other, []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveSpeechModel(other, []string{dir}, "ggml-small.bin"); got != other {
		t.Fatalf("configured = %q", got)
	}
	empty := filepath.Join(t.TempDir(), "ggml-tiny.bin")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveSpeechModel(empty, nil, "ggml-tiny.bin"); got != "" {
		t.Fatalf("empty file resolved to %q", got)
	}
}

func TestDownloadSpeechModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ggml-tiny.bin" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Length", "5")
		w.Write([]byte("hello"))
	}))
	defer srv.Close()
	var got int64
	dir := t.TempDir()
	path, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, func(written, total int64) {
		got = written
		if total != 5 {
			t.Errorf("total = %d", total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" || got != 5 {
		t.Fatalf("data %q written %d", data, got)
	}
	if _, err := os.Stat(path + ".partial"); !os.IsNotExist(err) {
		t.Fatal("partial file was left behind")
	}
}

func TestDownloadSpeechModelCancelRemovesPartial(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	dir := t.TempDir()
	_, err := DownloadSpeechModel(ctx, srv.URL, "ggml-tiny.bin", dir, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("left %d files", len(entries))
	}
}
