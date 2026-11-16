package cloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is an S3-compatible server on plain http, as a MinIO on the home network is: it keeps the body of each PUT by its path, and answers the first fails PUTs with the error XML of status and code.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	puts    int
	hashes  []string
	fails   int
	status  int
	code    string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != http.MethodPut {
		http.Error(w, "unexpected "+r.Method, http.StatusMethodNotAllowed)
		return
	}
	f.puts++
	f.hashes = append(f.hashes, r.Header.Get("X-Amz-Content-Sha256"))
	if f.puts <= f.fails {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(f.status)
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>`+f.code+`</Code><Message>fake `+f.code+`</Message><RequestId>1</RequestId></Error>`)
		return
	}
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	f.objects[r.URL.Path] = body
	w.Header().Set("ETag", `"etag"`)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeS3) object(path string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.objects[path]
}

// fakeS3Upload uploads a file of size bytes to fake through an S3-compatible client on its http address, path style, and returns the file's bytes, the error and the last progress reported.
func fakeS3Upload(t *testing.T, fake *fakeS3, size int) (want []byte, err error, written, total int64) {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	client, err := NewCompat(srv.URL, "us-east-1", "key", "secret-value", true)
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.Repeat([]byte("kando-"), size/6+1)[:size]
	video := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(video, want, 0o644); err != nil {
		t.Fatal(err)
	}
	err = Upload(context.Background(), client, "clips", []File{{Key: "a/video.mp4", Path: video}}, func(w, all int64) {
		if w > all {
			t.Errorf("progress %d over %d", w, all)
		}
		written, total = w, all
	})
	return want, err, written, total
}

// TestUploadToAnHTTPEndpoint puts a film on an S3-compatible endpoint that is http, not https: the SDK cannot leave the payload unsigned there unless told to, and a body it cannot rewind failed every such upload after reading the whole file.
func TestUploadToAnHTTPEndpoint(t *testing.T) {
	fake := &fakeS3{}
	want, err, written, total := fakeS3Upload(t, fake, 3<<20)
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.object("/clips/a/video.mp4"); !bytes.Equal(got, want) {
		t.Fatalf("stored %d bytes, want %d", len(got), len(want))
	}
	if written != int64(len(want)) || total != int64(len(want)) {
		t.Fatalf("progress %d/%d, want %d", written, total, len(want))
	}
	// Unsigned, the file is read once, as it is sent, and the progress follows the bytes sent: signing the payload would read the whole file to hash it first.
	if strings.Join(fake.hashes, ",") != "UNSIGNED-PAYLOAD" {
		t.Fatalf("payload hashes %q", fake.hashes)
	}
}

// TestUploadRetriesFromTheStart has the storage fail the first PUT with a 500, which the SDK retries: the body goes back to its start for the retry, so the object holds the whole file once, and the progress does not count the failed attempt.
func TestUploadRetriesFromTheStart(t *testing.T) {
	fake := &fakeS3{fails: 1, status: http.StatusInternalServerError, code: "InternalError"}
	want, err, written, total := fakeS3Upload(t, fake, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if fake.puts != 2 {
		t.Fatalf("%d PUTs, want 2", fake.puts)
	}
	if got := fake.object("/clips/a/video.mp4"); !bytes.Equal(got, want) {
		t.Fatalf("stored %d bytes, want %d", len(got), len(want))
	}
	if written != total || total != int64(len(want)) {
		t.Fatalf("progress %d/%d", written, total)
	}
}

// TestUploadErrorsSayWhy has the storage refuse the PUT with the codes S3 and its look-alikes send, and reads each error back as its reason.
func TestUploadErrorsSayWhy(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   Reason
	}{
		{http.StatusForbidden, "SignatureDoesNotMatch", ReasonKeys},
		{http.StatusForbidden, "InvalidAccessKeyId", ReasonKeys},
		{http.StatusForbidden, "AccessDenied", ReasonDenied},
		{http.StatusNotFound, "NoSuchBucket", ReasonNoBucket},
		{http.StatusMovedPermanently, "PermanentRedirect", ReasonRegion},
		{http.StatusBadRequest, "AuthorizationHeaderMalformed", ReasonRegion},
		{http.StatusBadRequest, "EntityTooLarge", ReasonTooLarge},
	} {
		t.Run(tc.code, func(t *testing.T) {
			fake := &fakeS3{fails: 1, status: tc.status, code: tc.code}
			_, err, _, _ := fakeS3Upload(t, fake, 16)
			if err == nil {
				t.Fatal("the upload did not fail")
			}
			if got := Diagnose(err); got.Reason != tc.want {
				t.Fatalf("%v read as %+v, want reason %d", err, got, tc.want)
			}
		})
	}
	fake := &fakeS3{fails: 1, status: http.StatusBadRequest, code: "InvalidBucketName"}
	_, err, _, _ := fakeS3Upload(t, fake, 16)
	if got := Diagnose(err); got.Reason != ReasonOther || got.Detail != "InvalidBucketName: fake InvalidBucketName" {
		t.Fatalf("unknown code read as %+v", got)
	}
}

// TestUploadErrorsOfTheConnection reads the errors of a storage that cannot be reached: nothing listening, an https endpoint whose certificate is not trusted, and a host that does not resolve.
func TestUploadErrorsOfTheConnection(t *testing.T) {
	video := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	upload := func(endpoint string) error {
		client, err := NewCompat(endpoint, "us-east-1", "key", "secret-value", true)
		if err != nil {
			t.Fatal(err)
		}
		return Upload(context.Background(), client, "clips", []File{{Key: "a/video.mp4", Path: video}}, nil)
	}

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	if got := Diagnose(upload(closed.URL)); got.Reason != ReasonOffline {
		t.Fatalf("nothing listening read as %+v", got)
	}

	tlsSrv := httptest.NewUnstartedServer(&fakeS3{})
	tlsSrv.Config.ErrorLog = log.New(io.Discard, "", 0)
	tlsSrv.StartTLS()
	defer tlsSrv.Close()
	host := strings.TrimPrefix(tlsSrv.URL, "https://")
	if got := Diagnose(upload(tlsSrv.URL)); got.Reason != ReasonInsecure || got.Host != host {
		t.Fatalf("untrusted certificate read as %+v, want host %s", got, host)
	}

	plain := httptest.NewServer(&fakeS3{})
	defer plain.Close()
	host = strings.TrimPrefix(plain.URL, "http://")
	if got := Diagnose(upload("https://" + host)); got.Reason != ReasonInsecure || got.Host != host {
		t.Fatalf("https to an http server read as %+v, want host %s", got, host)
	}

	// A lookup that found no such host, as the SDK's transport wraps it; a real lookup would depend on the machine's resolver.
	dns := &url.Error{Op: "Put", URL: "http://clips.minio.invalid:9000/a/video.mp4", Err: &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "clips.minio.invalid", IsNotFound: true}}}
	if got := Diagnose(fmt.Errorf("operation error S3: PutObject, %w", dns)); got.Reason != ReasonUnreachable || got.Host != "clips.minio.invalid:9000" {
		t.Fatalf("no such host read as %+v", got)
	}
}

// TestUploadStallsWhenTheStorageStopsAnswering has a storage take the request and never answer: the upload fails with ErrStalled once nothing moved for stallTimeout, not with the caller's cancel and not after a fixed deadline.
func TestUploadStallsWhenTheStorageStopsAnswering(t *testing.T) {
	was := stallTimeout
	stallTimeout = 300 * time.Millisecond
	t.Cleanup(func() { stallTimeout = was })
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	client, err := NewCompat(srv.URL, "us-east-1", "key", "secret-value", true)
	if err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Upload(context.Background(), client, "clips", []File{{Key: "a/video.mp4", Path: video}}, nil)
	if !errors.Is(err, ErrStalled) || errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want ErrStalled alone", err)
	}
	if got := Diagnose(err); got.Reason != ReasonStalled {
		t.Fatalf("stall read as %+v", got)
	}
}

// TestUploadRefusesAFileLargerThanOnePut fails before a byte is sent when a file is larger than one PUT stores.
func TestUploadRefusesAFileLargerThanOnePut(t *testing.T) {
	was := maxPut
	maxPut = 4
	t.Cleanup(func() { maxPut = was })
	fake := &fakeS3{}
	_, err, _, _ := fakeS3Upload(t, fake, 5)
	if !errors.Is(err, ErrTooLarge) || Diagnose(err).Reason != ReasonTooLarge {
		t.Fatalf("err = %v", err)
	}
	if fake.puts != 0 {
		t.Fatalf("%d PUTs", fake.puts)
	}
}
