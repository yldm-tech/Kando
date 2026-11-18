package synctest

import (
	"context"
	"testing"
	"time"

	"kando/packages/sync"
)

func TestTheLearningStateOverV2(t *testing.T) {
	ctx := context.Background()
	d := New(t, "Mac")
	d.AddLibrary("lib", "Library")
	d.Offer("123456")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	t.Cleanup(c.Close)
	paired, err := c.Pair(ctx, sync.PairRequest{Code: "123456", Device: "Pixel", Platform: sync.PlatformAndroid})
	if err != nil {
		t.Fatal(err)
	}
	c = sync.NewClient(d.Addr, d.Fingerprint, paired.Token)
	t.Cleanup(c.Close)

	at := time.Now().Add(-time.Hour)
	d.SetState("lib", at, sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1", Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(1000)}}})
	h, err := c.Hello(ctx)
	if err != nil || len(h.State) == 0 {
		t.Fatalf("hello %+v, %v", h, err)
	}
	got, err := c.State(ctx, "lib", "", 0)
	if err != nil || !got.Reset || len(got.Records) != 1 || got.Devices["desktop"] != "Mac" {
		t.Fatalf("state %+v, %v", got, err)
	}
	if _, err := c.State(ctx, "nope", "", 0); err == nil {
		t.Error("a library the desktop does not list answered")
	}
	res, err := c.PutState(ctx, "lib", []sync.Record{{Kind: sync.KindPosition, ID: "f1", Film: "f1", Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(2000), Stamp: sync.Stamp{At: time.Now()}}}}})
	if err != nil || len(res.Records) != 1 {
		t.Fatalf("put %+v, %v", res, err)
	}
	if r, _ := d.State("lib", sync.KindPosition, "f1"); string(r.Fields[sync.FieldPositionMS].Value) != "2000" {
		t.Errorf("the desktop's position %+v", r)
	}
}
