package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
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

// modelServer serves data as ggml-tiny.bin the way Hugging Face's file servers do, with a strong ETag and byte ranges; requests records the Range header of every request.
func modelServer(t *testing.T, data []byte, etag string, requests *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.Header.Get("Range"))
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, "ggml-tiny.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// modelBytes is n bytes of a model that differ from place to place, so that a misplaced resume shows in the checksum.
func modelBytes(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i*7 + i/251)
	}
	return data
}

// leavePartial puts what an earlier download of ggml-tiny.bin left in dir: its first bytes and the server's name for the file.
func leavePartial(t *testing.T, dir string, data []byte, validator string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "ggml-tiny.bin.partial"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ggml-tiny.bin.partial.validator"), []byte(validator), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantModel(t *testing.T, dir string, data []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, "ggml-tiny.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if sha(got) != sha(data) {
		t.Fatalf("the model is %d bytes with checksum %s, want %d bytes with %s", len(got), sha(got), len(data), sha(data))
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("the folder holds %v, want the model alone", entries)
	}
}

func TestDownloadSpeechModelStallKeepsPartial(t *testing.T) {
	shortStall(t, 150*time.Millisecond)
	data := modelBytes(64 << 10)
	srv := stallingServer(t, data, 1000)
	dir := t.TempDir()
	_, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, nil)
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("err = %v, want ErrStalled", err)
	}
	partial, err := os.ReadFile(filepath.Join(dir, "ggml-tiny.bin.partial"))
	if err != nil || len(partial) != 1000 {
		t.Fatalf("partial holds %d bytes, err %v", len(partial), err)
	}
	if validator, _ := os.ReadFile(filepath.Join(dir, "ggml-tiny.bin.partial.validator")); string(validator) != `"v1"` {
		t.Fatalf("validator = %q", validator)
	}
}

func TestDownloadSpeechModelResumes(t *testing.T) {
	data := modelBytes(600 << 10)
	var requests []string
	dropped := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Header.Get("Range"))
		w.Header().Set("ETag", `"v1"`)
		if !dropped {
			// The first download breaks off a third of the way in.
			dropped = true
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			_, _ = w.Write(data[:200<<10])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
		http.ServeContent(w, r, "ggml-tiny.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	if _, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, nil); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("err = %v, want ErrDisconnected", err)
	}
	info, err := os.Stat(filepath.Join(dir, "ggml-tiny.bin.partial"))
	if err != nil || info.Size() == 0 {
		t.Fatalf("the dropped download kept nothing: %v", err)
	}
	kept := info.Size()

	var first, last, total int64 = -1, 0, 0
	path, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, func(written, whole int64) {
		if first < 0 {
			first = written
		}
		last, total = written, whole
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "ggml-tiny.bin") {
		t.Fatalf("path = %q", path)
	}
	wantModel(t, dir, data)
	if want := "bytes=" + strconv.FormatInt(kept, 10) + "-"; len(requests) != 2 || requests[1] != want {
		t.Fatalf("requests %q, want the second to ask for %q", requests, want)
	}
	// The progress counts what the first download kept, out of the whole file.
	if first != kept || last != int64(len(data)) || total != int64(len(data)) {
		t.Fatalf("progress from %d to %d of %d, want from %d to %d of %d", first, last, total, kept, len(data), len(data))
	}
}

func TestDownloadSpeechModelRestartsWhenRangeIsIgnored(t *testing.T) {
	data := modelBytes(300 << 10)
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Header.Get("Range"))
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	leavePartial(t, dir, data[:1000], `"v1"`)
	if _, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, nil); err != nil {
		t.Fatal(err)
	}
	wantModel(t, dir, data)
	if len(requests) != 1 || requests[0] != "bytes=1000-" {
		t.Fatalf("requests %q", requests)
	}
}

// A server that ignores If-Range, as Hugging Face's does, sends the rest of a file that changed since the partial one began; the download starts over.
func TestDownloadSpeechModelRestartsWhenTheFileChanged(t *testing.T) {
	data := modelBytes(300 << 10)
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Header.Get("Range"))
		w.Header().Set("ETag", `"v2"`)
		r.Header.Del("If-Range")
		http.ServeContent(w, r, "ggml-tiny.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	leavePartial(t, dir, []byte("an older model"), `"v1"`)
	if _, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, nil); err != nil {
		t.Fatal(err)
	}
	wantModel(t, dir, data)
	if len(requests) != 2 || requests[0] != "bytes=14-" || requests[1] != "" {
		t.Fatalf("requests %q, want a resume and then the whole file", requests)
	}
}

// A 206 that starts somewhere other than where the partial file ends would put its bytes in the wrong place, so the download starts over even though the file is the same.
func TestDownloadSpeechModelRestartsWhenTheRangeStartsElsewhere(t *testing.T) {
	data := modelBytes(200 << 10)
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Header.Get("Range"))
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(data)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	leavePartial(t, dir, data[:1000], `"v1"`)
	if _, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, nil); err != nil {
		t.Fatal(err)
	}
	wantModel(t, dir, data)
	if len(requests) != 2 || requests[0] != "bytes=1000-" || requests[1] != "" {
		t.Fatalf("requests %q, want a resume and then the whole file", requests)
	}
}

// A partial file nothing names is not resumed, since nothing would say the rest belongs to it.
func TestDownloadSpeechModelWithoutValidatorStartsOver(t *testing.T) {
	data := modelBytes(100 << 10)
	var requests []string
	srv := modelServer(t, data, `"v1"`, &requests)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ggml-tiny.bin.partial"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, nil); err != nil {
		t.Fatal(err)
	}
	wantModel(t, dir, data)
	if len(requests) != 1 || requests[0] != "" {
		t.Fatalf("requests %q", requests)
	}
}

// The last download got every byte and stopped before its rename: the server has nothing past the end, and the file is done.
func TestDownloadSpeechModelPartialAlreadyWhole(t *testing.T) {
	data := modelBytes(50 << 10)
	var requests []string
	srv := modelServer(t, data, `"v1"`, &requests)
	dir := t.TempDir()
	leavePartial(t, dir, data, `"v1"`)
	if _, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, nil); err != nil {
		t.Fatal(err)
	}
	wantModel(t, dir, data)
	if len(requests) != 1 {
		t.Fatalf("requests %q", requests)
	}
}

func TestDownloadSpeechModelStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "gone", http.StatusNotFound) }))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	_, err := DownloadSpeechModel(context.Background(), srv.URL, "ggml-tiny.bin", dir, nil)
	var status interface{ StatusCode() int }
	if !errors.As(err, &status) || status.StatusCode() != http.StatusNotFound {
		t.Fatalf("err = %v, want HTTP 404", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left %v", entries)
	}
}

// Canceling a resumed download removes what the earlier one kept too.
func TestDownloadSpeechModelCancelRemovesResumedPartial(t *testing.T) {
	data := modelBytes(64 << 10)
	srv := stallingServer(t, data, 1000)
	dir := t.TempDir()
	leavePartial(t, dir, data[:500], `"v1"`)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := DownloadSpeechModel(ctx, srv.URL, "ggml-tiny.bin", dir, func(written, total int64) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left %v", entries)
	}
}
