package cloud

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// onlyReader hides every method of r but Read, as a pipe or a network body is: it cannot go back to its start.
type onlyReader struct{ r io.Reader }

func (o onlyReader) Read(p []byte) (int, error) { return o.r.Read(p) }

// A body that cannot go back to its start, larger than a part, goes in parts read one at a time; a small one, or one that can go back, in one PUT.
func TestPutStreamSendsALargeBodyInParts(t *testing.T) {
	shortParts(t)
	b, fake := memBucket(t)
	ctx := context.Background()
	body := []byte("0123456789abcdefghijklm")
	if err := b.PutStream(ctx, "films/a/video.mp4", "", onlyReader{bytes.NewReader(body)}, int64(len(body))); err != nil {
		t.Fatal(err)
	}
	if got := fake.objects["films/a/video.mp4"]; !bytes.Equal(got, body) || fake.partPuts != 6 || len(fake.uploads) != 0 {
		t.Fatalf("the store holds %q after %d parts", got, fake.partPuts)
	}
	if err := b.PutStream(ctx, "small", "", onlyReader{strings.NewReader("abc")}, 3); err != nil || string(fake.objects["small"]) != "abc" {
		t.Fatalf("a small body: %v, %q", err, fake.objects["small"])
	}
	if err := b.PutStream(ctx, "seekable", "", strings.NewReader("0123456789"), 10); err != nil || string(fake.objects["seekable"]) != "0123456789" {
		t.Fatalf("a body that goes back: %v, %q", err, fake.objects["seekable"])
	}
	if fake.partPuts != 6 {
		t.Fatalf("%d parts", fake.partPuts)
	}
}

// An upload in parts that fails is dropped, so the store keeps no parts of it, and the object stays what it was.
func TestPutStreamDropsAFailedUpload(t *testing.T) {
	shortParts(t)
	b, fake := memBucket(t)
	ctx := context.Background()
	if err := b.Put(ctx, "v", []byte("old")); err != nil {
		t.Fatal(err)
	}
	fake.failPart = 2
	body := "0123456789abcdefghijklm"
	if err := b.PutStream(ctx, "v", "", onlyReader{strings.NewReader(body)}, int64(len(body))); err == nil {
		t.Fatal("the store failing a part did not fail the upload")
	}
	if len(fake.uploads) != 0 || string(fake.objects["v"]) != "old" {
		t.Fatalf("uploads %v, object %q", fake.uploads, fake.objects["v"])
	}
	// A body shorter than its size fails before it is complete.
	fake.failPart = 0
	if err := b.PutStream(ctx, "v", "", onlyReader{strings.NewReader(body)}, 40); err == nil {
		t.Fatal("a short body was stored")
	}
	if len(fake.uploads) != 0 || string(fake.objects["v"]) != "old" {
		t.Fatalf("uploads %v, object %q", fake.uploads, fake.objects["v"])
	}
	// So does one that can go back to its start, which the SDK would send short as it is.
	if err := b.PutStream(ctx, "v", "", strings.NewReader("new"), 8); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("a short seekable body: %v", err)
	}
	if string(fake.objects["v"]) != "old" {
		t.Fatalf("object %q", fake.objects["v"])
	}
}
