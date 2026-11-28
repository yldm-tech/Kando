package mediasource

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// A source whose connecting takes longer than a request waits, as while macOS asks the user whether Kando may read the source's secrets from the keychain and the read waits on the answer, whatever its context says: each request gives up when its own context ends, none starts a connection of its own meanwhile, and once the connecting is done, the next request, and each after it, is served by that connection, including the request that gave up asking again.
func TestAPoolConnectingSlowlyHoldsNoRequest(t *testing.T) {
	release := make(chan struct{})
	var opens atomic.Int32
	k := testKind(nil)
	kinds := func(id string) (Kind, bool) { return k, id == "test" }
	open := func(context.Context, Kind, Source) (Driver, error) {
		opens.Add(1)
		<-release
		return &memDriver{}, nil
	}
	s := Source{ID: "0123456789abcdef", Kind: "test", Name: "Plex", Config: map[string]string{"endpoint": "http://plex.local:32400"}}
	var p Pool
	p.Set([]Source{s}, kinds, open)

	for range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		start := time.Now()
		_, err := p.Driver(ctx, s.ID)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
			t.Fatalf("a request while the source connects: %v after %v", err, time.Since(start))
		}
	}
	if n := opens.Load(); n != 1 {
		t.Fatalf("%d connections begun while one was being made", n)
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	a, err := p.Driver(ctx, s.ID)
	if err != nil {
		t.Fatalf("once the source connected: %v", err)
	}
	if b, err := p.Driver(ctx, s.ID); err != nil || b != a || opens.Load() != 1 {
		t.Fatalf("the next request: %v, %v, %d connections", b == a, err, opens.Load())
	}
}

// A connection that fails is not kept: the next request connects again.
func TestAPoolConnectingThatFailsIsTriedAgain(t *testing.T) {
	var opens atomic.Int32
	k := testKind(nil)
	kinds := func(id string) (Kind, bool) { return k, id == "test" }
	open := func(context.Context, Kind, Source) (Driver, error) {
		if opens.Add(1) == 1 {
			return nil, errors.New("the server did not answer")
		}
		return &memDriver{}, nil
	}
	s := Source{ID: "0123456789abcdef", Kind: "test", Name: "Plex"}
	var p Pool
	p.Set([]Source{s}, kinds, open)
	if _, err := p.Driver(context.Background(), s.ID); err == nil {
		t.Fatal("a connection that failed was kept")
	}
	if d, err := p.Driver(context.Background(), s.ID); err != nil || d == nil || opens.Load() != 2 {
		t.Fatalf("asked again: %v, %d connections", err, opens.Load())
	}
}
