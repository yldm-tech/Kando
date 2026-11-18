package cloud

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
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
)

// memS3 is an S3-compatible server in memory, path style on plain http, as a MinIO is: it stores, reads, heads, lists by pages of pageSize keys, and deletes the objects of one bucket, answering a key it does not hold as S3 does. It notes which requests came signed by a presigned link rather than an Authorization header.
type memS3 struct {
	bucket   string
	pageSize int

	mu        sync.Mutex
	objects   map[string][]byte
	modified  map[string]time.Time
	presigned []string
	lists     int
}

func (m *memS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.objects == nil {
		m.objects, m.modified = map[string][]byte{}, map[string]time.Time{}
	}
	if r.URL.Query().Get("X-Amz-Signature") != "" {
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "both a header and a link signed", http.StatusBadRequest)
			return
		}
		m.presigned = append(m.presigned, r.Method+" "+r.URL.Path)
	} else if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		m.fail(w, http.StatusForbidden, "AccessDenied")
		return
	}
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if bucket != m.bucket {
		m.fail(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	switch {
	case r.Method == http.MethodGet && key == "" && r.URL.Query().Get("list-type") == "2":
		m.list(w, r)
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		m.objects[key] = body
		m.modified[key] = time.Date(2026, 11, 18, 9, 0, len(m.objects), 0, time.UTC)
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, len(body)))
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		body, ok := m.objects[key]
		if !ok {
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			m.fail(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("Last-Modified", m.modified[key].Format(http.TimeFormat))
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, len(body)))
		if r.Method == http.MethodGet {
			w.Write(body)
		}
	case r.Method == http.MethodDelete:
		delete(m.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected "+r.Method, http.StatusMethodNotAllowed)
	}
}

func (m *memS3) fail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>`+code+`</Code><Message>fake</Message></Error>`)
}

type listResult struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	Name                  string
	Prefix                string
	KeyCount              int
	IsTruncated           bool
	NextContinuationToken string `xml:",omitempty"`
	Contents              []listEntry
}

type listEntry struct {
	Key          string
	LastModified string
	ETag         string
	Size         int
}

// list answers a ListObjectsV2 page: the keys after the continuation token, which is the last key of the page before.
func (m *memS3) list(w http.ResponseWriter, r *http.Request) {
	m.lists++
	prefix, after := r.URL.Query().Get("prefix"), r.URL.Query().Get("continuation-token")
	var keys []string
	for k := range m.objects {
		if strings.HasPrefix(k, prefix) && k > after {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	out := listResult{Name: m.bucket, Prefix: prefix}
	if len(keys) > m.pageSize {
		keys = keys[:m.pageSize]
		out.IsTruncated, out.NextContinuationToken = true, keys[len(keys)-1]
	}
	for _, k := range keys {
		out.Contents = append(out.Contents, listEntry{Key: k, LastModified: m.modified[k].Format(time.RFC3339), ETag: fmt.Sprintf(`"%x"`, len(m.objects[k])), Size: len(m.objects[k])})
	}
	out.KeyCount = len(out.Contents)
	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(out)
}

// memBucket is a Bucket of an S3-compatible store served by a memS3 of bucket "relay" that lists two keys a page.
func memBucket(t *testing.T) (*Bucket, *memS3) {
	t.Helper()
	fake := &memS3{bucket: "relay", pageSize: 2}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	b, err := OpenBucket(Store{Kind: KindCompat, Endpoint: srv.URL, Region: "us-east-1", Bucket: "relay", AccessKey: "key", Secret: "secret-value", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	return b, fake
}

func TestBucketPutsGetsHeadsListsAndDeletes(t *testing.T) {
	ctx := context.Background()
	b, fake := memBucket(t)
	for _, key := range []string{"kando-relay/d/inbox/p1/a", "kando-relay/d/inbox/p1/b", "kando-relay/d/inbox/p2/c", "kando-relay/d/outbox/p1/state", "elsewhere/x"} {
		if err := b.Put(ctx, key, []byte("body of "+key)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := b.Get(ctx, "kando-relay/d/inbox/p1/b", 1<<10)
	if err != nil || string(got) != "body of kando-relay/d/inbox/p1/b" {
		t.Fatalf("get %q, %v", got, err)
	}
	if _, err := b.Get(ctx, "kando-relay/d/inbox/p1/b", 4); !errors.Is(err, ErrObjectTooLarge) {
		t.Fatalf("a get past its limit: %v", err)
	}
	if _, err := b.Get(ctx, "kando-relay/d/none", 1<<10); !errors.Is(err, ErrNoObject) {
		t.Fatalf("a get of nothing: %v", err)
	}
	head, err := b.Head(ctx, "kando-relay/d/outbox/p1/state")
	if err != nil || head.Size != int64(len("body of kando-relay/d/outbox/p1/state")) || head.Modified.IsZero() || head.ETag == "" {
		t.Fatalf("head %+v, %v", head, err)
	}
	if _, err := b.Head(ctx, "kando-relay/d/none"); !errors.Is(err, ErrNoObject) {
		t.Fatalf("a head of nothing: %v", err)
	}
	// Four keys under the prefix take two pages of two.
	listed, err := b.List(ctx, "kando-relay/d/")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, o := range listed {
		keys = append(keys, o.Key)
		if o.Size == 0 || o.Modified.IsZero() {
			t.Fatalf("listed %+v", o)
		}
	}
	if strings.Join(keys, ",") != "kando-relay/d/inbox/p1/a,kando-relay/d/inbox/p1/b,kando-relay/d/inbox/p2/c,kando-relay/d/outbox/p1/state" || fake.lists != 2 {
		t.Fatalf("listed %v in %d pages", keys, fake.lists)
	}
	if err := b.Delete(ctx, "kando-relay/d/inbox/p1/a"); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(ctx, "kando-relay/d/inbox/p1/a"); err != nil {
		t.Fatalf("deleting what is gone: %v", err)
	}
	if listed, _ := b.List(ctx, "kando-relay/d/inbox/p1/"); len(listed) != 1 {
		t.Fatalf("after delete %+v", listed)
	}
}

func TestBucketOfAnotherNameIsNoBucket(t *testing.T) {
	b, _ := memBucket(t)
	b.name = "other"
	_, err := b.Get(context.Background(), "k", 10)
	if errors.Is(err, ErrNoObject) || Diagnose(err).Reason != ReasonNoBucket {
		t.Fatalf("a missing bucket read as %v (%+v)", err, Diagnose(err))
	}
}

// TestPresignedLinksWorkWithoutAKey reads and writes through presigned links with plain requests, as a phone does that holds no key.
func TestPresignedLinksWorkWithoutAKey(t *testing.T) {
	ctx := context.Background()
	b, fake := memBucket(t)
	if err := b.Put(ctx, "kando-relay/d/outbox/p1/state", []byte("sealed state")); err != nil {
		t.Fatal(err)
	}
	get, err := b.PresignGet(ctx, "kando-relay/d/outbox/p1/state", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if q := mustQuery(t, get); q.Get("X-Amz-Expires") != "3600" || q.Get("X-Amz-Signature") == "" {
		t.Fatalf("get link %s", get)
	}
	resp, err := http.Get(get)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "sealed state" {
		t.Fatalf("GET %d %q", resp.StatusCode, body)
	}
	put, err := b.PresignPut(ctx, "kando-relay/d/inbox/p1/slot", 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if q := mustQuery(t, put); q.Get("X-Amz-Expires") != strconv.Itoa(int(MaxPresign/time.Second)) {
		t.Fatalf("a link asked for 30 days is good for %s seconds", q.Get("X-Amz-Expires"))
	}
	req, _ := http.NewRequest(http.MethodPut, put, bytes.NewReader([]byte("sealed call")))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT %d", resp.StatusCode)
	}
	if got, _ := b.Get(ctx, "kando-relay/d/inbox/p1/slot", 100); string(got) != "sealed call" {
		t.Fatalf("stored %q", got)
	}
	if strings.Join(fake.presigned, ",") != "GET /relay/kando-relay/d/outbox/p1/state,PUT /relay/kando-relay/d/inbox/p1/slot" {
		t.Fatalf("presigned requests %v", fake.presigned)
	}
}

// TestPresignAddressesEachKindOfStore signs links for every kind of store without the network, each at the address its store serves the bucket at.
func TestPresignAddressesEachKindOfStore(t *testing.T) {
	for _, tc := range []struct {
		cfg  Store
		want string
	}{
		{Store{Kind: KindR2, Endpoint: "acct", Bucket: "films"}, "https://acct.r2.cloudflarestorage.com/films/kando-relay/x"},
		{Store{Kind: KindS3, Region: "ap-northeast-1", Bucket: "films"}, "https://films.s3.ap-northeast-1.amazonaws.com/kando-relay/x"},
		{Store{Kind: KindOSS, Region: "cn-hangzhou", Bucket: "films"}, "https://films.s3.oss-cn-hangzhou.aliyuncs.com/kando-relay/x"},
		{Store{Kind: KindCOS, Region: "ap-guangzhou", Bucket: "films-1250000000"}, "https://films-1250000000.cos.ap-guangzhou.myqcloud.com/kando-relay/x"},
		{Store{Kind: KindCompat, Endpoint: "https://minio.home:9000", Region: "us-east-1", Bucket: "films", PathStyle: true}, "https://minio.home:9000/films/kando-relay/x"},
	} {
		t.Run(tc.cfg.Kind, func(t *testing.T) {
			tc.cfg.AccessKey, tc.cfg.Secret = "key", "secret-value"
			b, err := OpenBucket(tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			for _, presign := range []func(context.Context, string, time.Duration) (string, error){b.PresignGet, b.PresignPut} {
				link, err := presign(context.Background(), "kando-relay/x", time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				base, _, _ := strings.Cut(link, "?")
				if base != tc.want {
					t.Fatalf("link %s, want %s", link, tc.want)
				}
				if q := mustQuery(t, link); q.Get("X-Amz-Signature") == "" || q.Get("X-Amz-Credential") == "" || strings.Contains(link, "secret-value") {
					t.Fatalf("link %s", link)
				}
			}
		})
	}
}

func TestOpenBucketNeedsABucket(t *testing.T) {
	if _, err := OpenBucket(Store{Kind: KindS3, Region: "ap-northeast-1", AccessKey: "key", Secret: "secret"}); err == nil {
		t.Fatal("a store without a bucket opened")
	}
	if _, err := OpenBucket(Store{Kind: KindS3, Bucket: "films"}); err == nil {
		t.Fatal("a store without keys opened")
	}
}

func mustQuery(t *testing.T, link string) url.Values {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}
