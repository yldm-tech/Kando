package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchThumbnailWritesThePicture(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("jpeg bytes"))
	}))
	t.Cleanup(srv.Close)
	dest := filepath.Join(t.TempDir(), "thumbs", "job.jpg")
	if err := FetchThumbnail(context.Background(), srv.URL+"/hq.jpg", dest); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "jpeg bytes" {
		t.Fatalf("wrote %q, %v", data, err)
	}
	other := filepath.Join(t.TempDir(), "other.jpg")
	if err := FetchThumbnail(context.Background(), srv.URL+"/missing.jpg", other); err == nil {
		t.Fatal("a missing picture was written")
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("a failed fetch left %s", other)
	}
	if err := FetchThumbnail(context.Background(), "file:///etc/passwd", other); err == nil {
		t.Fatal("a file address was read")
	}
}
