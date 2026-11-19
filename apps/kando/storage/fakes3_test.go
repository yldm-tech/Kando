package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/cloud"
)

// fakeS3 is an S3-compatible server in memory, path style on plain http as a MinIO is, holding the objects of one bucket: it puts, gets (with a Range), heads, copies, deletes, lists by prefix and delimiter a page of pageSize entries at a time, and takes uploads in parts, answering errors with S3's codes. It counts the requests of each kind.
type fakeS3 struct {
	bucket   string
	pageSize int

	mu       sync.Mutex
	objects  map[string][]byte
	modified map[string]time.Time
	uploads  map[string]map[int32][]byte
	calls    map[string]int
	ranges   []string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") && r.URL.Query().Get("X-Amz-Signature") == "" {
		f.fail(w, http.StatusForbidden, "AccessDenied")
		return
	}
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if bucket != f.bucket {
		f.fail(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	q := r.URL.Query()
	_, begin := q["uploads"]
	id := q.Get("uploadId")
	switch {
	case r.Method == http.MethodGet && key == "" && q.Get("list-type") == "2":
		f.calls["list"]++
		f.list(w, q)
	case r.Method == http.MethodPost && begin:
		f.calls["create"]++
		id := rand.Text()
		f.uploads[id] = map[int32][]byte{}
		fmt.Fprintf(w, `<InitiateMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><UploadId>%s</UploadId></InitiateMultipartUploadResult>`, f.bucket, key, id)
	case r.Method == http.MethodPut && id != "":
		f.calls["part"]++
		parts, ok := f.uploads[id]
		if !ok {
			f.fail(w, http.StatusNotFound, "NoSuchUpload")
			return
		}
		n, _ := strconv.Atoi(q.Get("partNumber"))
		body, _ := io.ReadAll(r.Body)
		parts[int32(n)] = body
		w.Header().Set("ETag", fmt.Sprintf(`"p%d"`, n))
	case r.Method == http.MethodPost && id != "":
		f.calls["complete"]++
		parts, ok := f.uploads[id]
		if !ok {
			f.fail(w, http.StatusNotFound, "NoSuchUpload")
			return
		}
		var done struct {
			Part []struct {
				PartNumber int32
				ETag       string
			}
		}
		raw, _ := io.ReadAll(r.Body)
		if err := xml.Unmarshal(raw, &done); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var whole []byte
		for i, p := range done.Part {
			if p.PartNumber != int32(i+1) || p.ETag != fmt.Sprintf(`"p%d"`, p.PartNumber) {
				f.fail(w, http.StatusBadRequest, "InvalidPart")
				return
			}
			whole = append(whole, parts[p.PartNumber]...)
		}
		delete(f.uploads, id)
		f.store(key, whole)
		fmt.Fprintf(w, `<CompleteMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><ETag>"whole"</ETag></CompleteMultipartUploadResult>`, f.bucket, key)
	case r.Method == http.MethodDelete && id != "":
		f.calls["abort"]++
		delete(f.uploads, id)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut && r.Header.Get("X-Amz-Copy-Source") != "":
		f.calls["copy"]++
		source, err := url.PathUnescape(r.Header.Get("X-Amz-Copy-Source"))
		srcBucket, srcKey, _ := strings.Cut(strings.TrimPrefix(source, "/"), "/")
		body, ok := f.objects[srcKey]
		if err != nil || srcBucket != f.bucket || !ok {
			f.fail(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		f.store(key, slices.Clone(body))
		fmt.Fprintf(w, `<CopyObjectResult><ETag>%s</ETag><LastModified>%s</LastModified></CopyObjectResult>`, f.etag(key), f.modified[key].Format(time.RFC3339))
	case r.Method == http.MethodPut:
		f.calls["put"]++
		body, _ := io.ReadAll(r.Body)
		f.store(key, body)
		w.Header().Set("ETag", f.etag(key))
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		f.calls[strings.ToLower(r.Method)]++
		body, ok := f.objects[key]
		if !ok {
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			f.fail(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		if span := r.Header.Get("Range"); span != "" {
			f.ranges = append(f.ranges, span)
		}
		w.Header().Set("ETag", f.etag(key))
		http.ServeContent(w, r, "", f.modified[key], bytes.NewReader(body))
	case r.Method == http.MethodDelete:
		f.calls["delete"]++
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected "+r.Method, http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3) store(key string, body []byte) {
	f.objects[key] = body
	f.modified[key] = time.Date(2026, 11, 19, 9, 0, len(f.objects), 0, time.UTC)
}

func (f *fakeS3) etag(key string) string {
	return fmt.Sprintf(`"%d-%x"`, len(f.objects[key]), f.modified[key].Unix())
}

func (f *fakeS3) fail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>`+code+`</Code><Message>fake</Message></Error>`)
}

type fakeListResult struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	Name                  string
	Prefix                string
	KeyCount              int
	IsTruncated           bool
	NextContinuationToken string `xml:",omitempty"`
	Contents              []fakeListEntry
	CommonPrefixes        []struct{ Prefix string }
}

type fakeListEntry struct {
	Key          string
	LastModified string
	ETag         string
	Size         int
}

// list answers a ListObjectsV2 page: the keys and, with a delimiter, the common prefixes after the continuation token, which is the last entry of the page before.
func (f *fakeS3) list(w http.ResponseWriter, q url.Values) {
	prefix, delimiter, after := q.Get("prefix"), q.Get("delimiter"), q.Get("continuation-token")
	limit := f.pageSize
	if n, err := strconv.Atoi(q.Get("max-keys")); err == nil && n < limit {
		limit = n
	}
	isPrefix := map[string]bool{}
	var entries []string
	for k := range f.objects {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		entry := k
		if delimiter != "" {
			if i := strings.Index(k[len(prefix):], delimiter); i >= 0 {
				entry = k[:len(prefix)+i+len(delimiter)]
				isPrefix[entry] = true
			}
		}
		if entry > after && !slices.Contains(entries, entry) {
			entries = append(entries, entry)
		}
	}
	slices.Sort(entries)
	out := fakeListResult{Name: f.bucket, Prefix: prefix}
	if len(entries) > limit {
		entries = entries[:limit]
		out.IsTruncated, out.NextContinuationToken = true, entries[len(entries)-1]
	}
	for _, e := range entries {
		if isPrefix[e] {
			out.CommonPrefixes = append(out.CommonPrefixes, struct{ Prefix string }{e})
			continue
		}
		out.Contents = append(out.Contents, fakeListEntry{Key: e, LastModified: f.modified[e].Format(time.RFC3339), ETag: f.etag(e), Size: len(f.objects[e])})
	}
	out.KeyCount = len(entries)
	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(out)
}

func (f *fakeS3) object(key string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.objects[key]
	return body, ok
}

func (f *fakeS3) count(kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[kind]
}

// fakeStore is an ObjectStore rooted at root over a fakeS3 of bucket "films" that lists two entries a page, and the fake.
func fakeStore(t *testing.T, root string) (*ObjectStore, *fakeS3) {
	t.Helper()
	fake := &fakeS3{bucket: "films", pageSize: 2, objects: map[string][]byte{}, modified: map[string]time.Time{}, uploads: map[string]map[int32][]byte{}, calls: map[string]int{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	store, err := OpenObjectStore(cloud.Store{Kind: cloud.KindCompat, Endpoint: srv.URL, Region: "us-east-1", Bucket: "films", AccessKey: "key", Secret: "secret-value", PathStyle: true}, root)
	if err != nil {
		t.Fatal(err)
	}
	return store, fake
}
