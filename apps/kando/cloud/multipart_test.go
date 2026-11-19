package cloud

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

// memUpload is an upload in parts memS3 keeps: its key and the parts sent, by number.
type memUpload struct {
	key   string
	parts map[int32][]byte
}

type partEntry struct {
	PartNumber int32
	ETag       string
	Size       int
}

// multipart answers the requests of an upload in parts, as S3 does, and reports whether r was one.
func (m *memS3) multipart(w http.ResponseWriter, r *http.Request, key string) bool {
	q := r.URL.Query()
	_, begin := q["uploads"]
	id := q.Get("uploadId")
	if !begin && id == "" {
		return false
	}
	if m.uploads == nil {
		m.uploads = map[string]*memUpload{}
	}
	if begin && r.Method == http.MethodPost {
		id := rand.Text()
		m.uploads[id] = &memUpload{key: key, parts: map[int32][]byte{}}
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<InitiateMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><UploadId>%s</UploadId></InitiateMultipartUploadResult>`, m.bucket, key, id)
		return true
	}
	up, ok := m.uploads[id]
	if !ok || up.key != key {
		m.fail(w, http.StatusNotFound, "NoSuchUpload")
		return true
	}
	switch r.Method {
	case http.MethodPut:
		n, _ := strconv.Atoi(q.Get("partNumber"))
		if int32(n) == m.failPart {
			m.fail(w, http.StatusInternalServerError, "InternalError")
			return true
		}
		body, _ := io.ReadAll(r.Body)
		m.partPuts++
		up.parts[int32(n)] = body
		w.Header().Set("ETag", fmt.Sprintf(`"p%d-%d"`, n, len(body)))
	case http.MethodGet:
		type result struct {
			XMLName xml.Name `xml:"ListPartsResult"`
			Part    []partEntry
		}
		var out result
		for n, body := range up.parts {
			out.Part = append(out.Part, partEntry{PartNumber: n, ETag: fmt.Sprintf(`"p%d-%d"`, n, len(body)), Size: len(body)})
		}
		slices.SortFunc(out.Part, func(a, b partEntry) int { return int(a.PartNumber - b.PartNumber) })
		w.Header().Set("Content-Type", "application/xml")
		xml.NewEncoder(w).Encode(out)
	case http.MethodPost:
		var done struct {
			Part []struct {
				PartNumber int32
				ETag       string
			}
		}
		raw, _ := io.ReadAll(r.Body)
		if err := xml.Unmarshal(raw, &done); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}
		var whole []byte
		for i, p := range done.Part {
			body, ok := up.parts[p.PartNumber]
			if !ok || p.PartNumber != int32(i+1) || p.ETag != fmt.Sprintf(`"p%d-%d"`, p.PartNumber, len(body)) {
				m.fail(w, http.StatusBadRequest, "InvalidPart")
				return true
			}
			whole = append(whole, body...)
		}
		delete(m.uploads, id)
		m.objects[key] = whole
		m.modified[key] = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<CompleteMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><ETag>"whole"</ETag></CompleteMultipartUploadResult>`, m.bucket, key)
	case http.MethodDelete:
		delete(m.uploads, id)
		w.WriteHeader(http.StatusNoContent)
	}
	return true
}

// shortParts has a file larger than 10 bytes go in parts of 4 bytes for the test.
func shortParts(t *testing.T) {
	t.Helper()
	one, part := onePut, minPart
	onePut, minPart = 10, 4
	t.Cleanup(func() { onePut, minPart = one, part })
}

func writeFile(t *testing.T, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A file larger than one PUT takes goes in parts, which the store puts together in their order; a small one goes in one PUT, saving nothing to go on from.
func TestPutFileSendsALargeFileInParts(t *testing.T) {
	shortParts(t)
	b, fake := memBucket(t)
	ctx := context.Background()
	body := []byte("0123456789abcdefghijklm")
	var saves []Resume
	var told int64
	err := b.PutFile(ctx, "films/a/video.mp4", writeFile(t, body), "", Resume{}, func(r Resume) error { saves = append(saves, r); return nil }, func(done, total int64) { told = done })
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.objects["films/a/video.mp4"]; !bytes.Equal(got, body) {
		t.Fatalf("the store holds %q", got)
	}
	if fake.partPuts != 6 || told != int64(len(body)) || len(saves) != 7 || saves[0].UploadID == "" || saves[0].PartSize != 4 || len(saves[6].Parts) != 6 {
		t.Fatalf("%d parts, told %d, saves %+v", fake.partPuts, told, saves)
	}
	saves = nil
	if err := b.PutFile(ctx, "films/a/subtitles.json", writeFile(t, []byte("[]")), "application/json", Resume{}, func(r Resume) error { saves = append(saves, r); return nil }, nil); err != nil {
		t.Fatal(err)
	}
	if string(fake.objects["films/a/subtitles.json"]) != "[]" || len(saves) != 0 || fake.partPuts != 6 {
		t.Fatalf("a small file: saves %+v, %d parts", saves, fake.partPuts)
	}
}

// An upload that stopped goes on from the parts the store took; one of a file changed since starts anew, and the store drops the old one.
func TestPutFileGoesOnWhereItStopped(t *testing.T) {
	shortParts(t)
	b, fake := memBucket(t)
	ctx := context.Background()
	body := []byte("0123456789abcdefghijklm")
	path := writeFile(t, body)
	var kept Resume
	save := func(r Resume) error { kept = r; return nil }
	fake.failPart = 3
	if err := b.PutFile(ctx, "v", path, "", Resume{}, save, nil); err == nil {
		t.Fatal("the store failing a part did not fail the upload")
	}
	if len(kept.Parts) != 2 || fake.partPuts != 2 {
		t.Fatalf("kept %+v after %d parts", kept, fake.partPuts)
	}
	fake.failPart = 0
	if err := b.PutFile(ctx, "v", path, "", kept, save, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fake.objects["v"], body) || fake.partPuts != 6 {
		t.Fatalf("went on to %q after %d parts", fake.objects["v"], fake.partPuts)
	}

	fake.failPart = 2
	if err := b.PutFile(ctx, "w", path, "", Resume{}, save, nil); err == nil {
		t.Fatal("no failure")
	}
	old := kept.UploadID
	fake.failPart = 0
	changed := []byte("ZYXWVUTSRQPONMLKJIHGFEDCBA")
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.PutFile(ctx, "w", path, "", kept, save, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fake.objects["w"], changed) || kept.UploadID == old || fake.uploads[old] != nil {
		t.Fatalf("a changed file: %q, upload %s (was %s)", fake.objects["w"], kept.UploadID, old)
	}
	// An upload the store no longer knows starts anew too.
	if err := b.PutFile(ctx, "w", path, "", Resume{UploadID: "gone", PartSize: 4, Size: kept.Size, Modified: kept.Modified}, save, nil); err != nil || !bytes.Equal(fake.objects["w"], changed) {
		t.Fatalf("an upload the store forgot: %v", err)
	}
}

// The parts are 32 MiB, larger for a file of more than 10,000 of them, in whole MiB.
func TestPartSize(t *testing.T) {
	if got := partSize(6 << 30); got != 32<<20 {
		t.Errorf("6 GiB in parts of %d", got)
	}
	big := int64(500 << 30)
	got := partSize(big)
	if got%(1<<20) != 0 || (big+got-1)/got > maxParts {
		t.Errorf("500 GiB in parts of %d", got)
	}
}
