package sync

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// A pairing URL carries the relay's pairing mailbox when there is one, and reads it back.
func TestPairURLCarriesTheRelaysMailbox(t *testing.T) {
	key := NewRelayKey()
	p := PairURL{Addrs: []netip.AddrPort{netip.MustParseAddrPort("192.168.1.20:52817")}, Code: "123456", Fingerprint: Fingerprint([]byte("c")), Name: "Mac",
		Relay: &PairRelay{Box: "https://store.example/b/kando-relay/x/pair/y/box?X-Amz-Signature=ab&x-id=GetObject", Key: key}}
	got, err := ParsePairURL(p.String())
	if err != nil {
		t.Fatal(err)
	}
	if got.Relay == nil || got.Relay.Box != p.Relay.Box || !bytes.Equal(got.Relay.Key, key) {
		t.Fatalf("read back the mailbox %+v, want %+v", got.Relay, p.Relay)
	}
	plain := p
	plain.Relay = nil
	if got, err := ParsePairURL(plain.String()); err != nil || got.Relay != nil {
		t.Fatalf("a URL without a mailbox read as %+v, %v", got.Relay, err)
	}
	s := p.String()
	for name, bad := range map[string]string{
		"no key":       s[:strings.Index(s, "&k=")],
		"a short key":  s[:strings.Index(s, "&k=")] + "&k=AAAA",
		"another link": strings.Replace(s, "r=https", "r=ftp", 1),
		"no link":      s[:strings.Index(s, "&r=")] + s[strings.Index(s, "&k="):],
	} {
		if _, err := ParsePairURL(bad); !errors.Is(err, ErrNotPairURL) {
			t.Errorf("%s: read with error %v", name, err)
		}
	}
}

// relayPairRig is a mailbox in a memRelay, served until the test ends with pair as the desktop's answer.
func relayPairRig(t *testing.T, pair func(context.Context, PairRequest) RelayPairAnswer) (*memRelay, *RelayPairMailbox) {
	t.Helper()
	store := newMemRelay(t, time.Now)
	m, err := NewRelayPairMailbox(context.Background(), store, "kando-relay/abc/", CodeLifetime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Serve(ctx, 5*time.Millisecond, pair, t.Logf)
	}()
	t.Cleanup(func() { cancel(); <-done })
	return store, m
}

// A phone pairs through the mailbox: the desktop reads its request, sealed, and its answer brings the token and the phone's join of the relay; the store holds nothing but sealed bytes.
func TestPairingThroughTheRelay(t *testing.T) {
	join := RelayJoin{Key: NewRelayKey(), Grant: RelayGrant{State: "https://store.example/state"}}
	var asked PairRequest
	store, m := relayPairRig(t, func(_ context.Context, req PairRequest) RelayPairAnswer {
		asked = req
		return RelayPairAnswer{Pair: &PairResponse{Token: "t", DeviceID: "d1", Desktop: "Studio"}, Join: &join}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, got, err := PairThroughRelay(ctx, nil, m.Relay(), PairRequest{Code: "123456", Device: "Pixel", Platform: PlatformAndroid}, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if asked.Code != "123456" || asked.Device != "Pixel" {
		t.Errorf("the desktop read %+v", asked)
	}
	if resp.Token != "t" || resp.DeviceID != "d1" || !bytes.Equal(got.Key, join.Key) || got.Grant.State != join.Grant.State {
		t.Errorf("paired %+v %+v", resp, got)
	}
	store.mu.Lock()
	for k, o := range store.objects {
		if bytes.Contains(o.body, []byte("123456")) || bytes.Contains(o.body, join.Key) || !bytes.HasPrefix(o.body, []byte(relayMagic)) {
			t.Errorf("%s is not sealed: %q", k, o.body)
		}
	}
	store.mu.Unlock()
}

// A code the desktop does not take says why, as on the local network.
func TestPairingThroughTheRelayWithACodeUsed(t *testing.T) {
	_, m := relayPairRig(t, func(context.Context, PairRequest) RelayPairAnswer {
		return RelayPairAnswer{Error: &ErrorBody{Code: CodeCodeUsed}}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, err := PairThroughRelay(ctx, nil, m.Relay(), PairRequest{Code: "123456"}, 5*time.Millisecond)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeCodeUsed || e.Status != 403 {
		t.Fatalf("error %v", err)
	}
}

// A desktop that does not answer, or a mailbox read with another key, ends the pairing.
func TestPairingThroughTheRelayThatDoesNotAnswer(t *testing.T) {
	store := newMemRelay(t, time.Now)
	m, err := NewRelayPairMailbox(context.Background(), store, "kando-relay/abc/", CodeLifetime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, _, err := PairThroughRelay(ctx, nil, m.Relay(), PairRequest{Code: "123456"}, 5*time.Millisecond); !errors.Is(err, ErrRelayPairTimeout) {
		t.Fatalf("no box: %v", err)
	}
	if err := m.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	other := m.Relay()
	other.Key = NewRelayKey()
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if _, _, err := PairThroughRelay(ctx2, nil, other, PairRequest{Code: "123456"}, 5*time.Millisecond); !errors.Is(err, ErrRelaySealed) {
		t.Fatalf("another key: %v", err)
	}
}

// A mailbox answers one ask, and Remove deletes it all.
func TestAPairingMailboxAnswersOnceAndGoes(t *testing.T) {
	store := newMemRelay(t, time.Now)
	m, err := NewRelayPairMailbox(context.Background(), store, "kando-relay/abc/", CodeLifetime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	asks := 0
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Serve(ctx, 5*time.Millisecond, func(context.Context, PairRequest) RelayPairAnswer {
			asks++
			return RelayPairAnswer{Error: &ErrorBody{Code: CodeWrongCode}}
		}, t.Logf)
	}()
	for range 2 {
		tctx, tcancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		_, _, _ = PairThroughRelay(tctx, nil, m.Relay(), PairRequest{Code: "000000"}, 5*time.Millisecond)
		tcancel()
	}
	cancel()
	<-done
	if err := m.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}
	if asks != 1 {
		t.Errorf("answered %d asks, want 1", asks)
	}
	if left := store.keys("kando-relay/abc/pair/"); len(left) != 0 {
		t.Errorf("left %v", left)
	}
}

// The relay's sweep deletes a pairing mailbox left behind once it is RelayPairKeep old.
func TestTheSweepDeletesOldPairingMailboxes(t *testing.T) {
	g := newRelayRig(t)
	m, err := NewRelayPairMailbox(context.Background(), g.store, g.relay.Prefix, CodeLifetime, g.clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := g.relay.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(g.store.keys(g.relay.Prefix+"pair/")) != 1 {
		t.Fatal("a new mailbox was swept")
	}
	g.clock = g.clock.Add(RelayPairKeep)
	if err := g.relay.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if left := g.store.keys(g.relay.Prefix + "pair/"); len(left) != 0 {
		t.Fatalf("left %v", left)
	}
}
