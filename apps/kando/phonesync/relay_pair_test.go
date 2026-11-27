package phonesync

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	psync "kando/packages/sync"
)

// linkedBucket is a memBucket whose presigned links a phone reaches over HTTP, as the store serves them.
type linkedBucket struct {
	*memBucket
	srv *httptest.Server
}

func newLinkedBucket(t *testing.T) *linkedBucket {
	t.Helper()
	b := &linkedBucket{memBucket: &memBucket{}}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/")
		switch r.Method {
		case http.MethodGet:
			body, err := b.Get(r.Context(), key)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = b.Put(r.Context(), key, body)
		}
	}))
	t.Cleanup(b.srv.Close)
	return b
}

// List lists the objects as written now: memBucket keeps no times, and the relay's sweep takes an object of no time for one long left behind.
func (b *linkedBucket) List(ctx context.Context, prefix string) ([]psync.RelayObject, error) {
	objects, err := b.memBucket.List(ctx, prefix)
	for i := range objects {
		objects[i].Modified = time.Now()
	}
	return objects, err
}

func (b *linkedBucket) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return b.srv.URL + "/" + key, nil
}

func (b *linkedBucket) PresignPut(_ context.Context, key string, _ time.Duration) (string, error) {
	return b.srv.URL + "/" + key, nil
}

// startRelayPairing starts a server whose relay is on in a linkedBucket, reading pairing mailboxes often.
func startRelayPairing(t *testing.T) (*Server, *linkedBucket) {
	t.Helper()
	every := relayPairEvery
	relayPairEvery = 5 * time.Millisecond
	t.Cleanup(func() { relayPairEvery = every })
	bucket := newLinkedBucket(t)
	srv, err := Start(Config{Dir: t.TempDir(), Name: "Test Mac", Source: fakeSource{dir: t.TempDir()}, Relay: bucket, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv, bucket
}

func pairThroughRelay(t *testing.T, r psync.PairRelay, code string) (psync.PairResponse, psync.RelayJoin, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return psync.PairThroughRelay(ctx, nil, r, psync.PairRequest{Code: code, Device: "Evan's iPhone", Platform: psync.PlatformIOS}, 5*time.Millisecond)
}

// A phone away from the computer's network pairs through the code's mailbox, and is joined to the relay with it: the computer lists it, and keeps the key it was given.
func TestAPhonePairsThroughTheRelay(t *testing.T) {
	srv, bucket := startRelayPairing(t)
	offer := srv.NewOffer()
	if offer.Relay == nil || len(offer.Relay.Key) != psync.RelayKeySize {
		t.Fatalf("the offer has no mailbox: %+v", offer)
	}
	res, join, err := pairThroughRelay(t, *offer.Relay, offer.Code)
	if err != nil {
		t.Fatal(err)
	}
	devs := srv.Devices()
	if len(devs) != 1 || devs[0].ID != res.DeviceID || res.Token == "" || res.Desktop != "Test Mac" {
		t.Fatalf("paired %+v, devices %+v", res, devs)
	}
	key, err := srv.devices.relayKey(res.DeviceID)
	if err != nil || string(key) != string(join.Key) || len(join.Grant.Slots) != psync.RelaySlots {
		t.Fatalf("joined with %x, the device keeps %x (%v)", join.Key, key, err)
	}
	if srv.Offer().State != OfferUsed {
		t.Fatalf("the code is %v", srv.Offer().State)
	}
	// The token pairs on the local network as one given there does.
	if _, err := client(t, srv, res.Token).Hello(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The mailbox goes as its code is taken back.
	srv.ClearOffer()
	waitFor(t, "the mailbox to go", func() bool {
		for _, k := range bucket.keys() {
			if strings.Contains(k, "/pair/") {
				return false
			}
		}
		return true
	})
}

// A code that does not pair through the relay counts against it as on the local network, and says why.
func TestAWrongCodeThroughTheRelayCounts(t *testing.T) {
	srv, _ := startRelayPairing(t)
	offer := srv.NewOffer()
	wrong := "000000"
	if offer.Code == wrong {
		wrong = "111111"
	}
	_, _, err := pairThroughRelay(t, *offer.Relay, wrong)
	wantCode(t, err, http.StatusForbidden, psync.CodeWrongCode)
	if len(srv.Devices()) != 0 {
		t.Fatal("a wrong code paired")
	}
	srv.mu.Lock()
	tries := srv.offer.tries
	srv.mu.Unlock()
	if tries != 1 {
		t.Fatalf("tries %d, want 1", tries)
	}
}

// The mailbox of a code replaced by a new one pairs nothing, and a server whose relay is off offers none.
func TestAnOldMailboxPairsNothing(t *testing.T) {
	srv, _ := startRelayPairing(t)
	old := srv.NewOffer()
	srv.NewOffer()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, _, err := psync.PairThroughRelay(ctx, nil, *old.Relay, psync.PairRequest{Code: old.Code}, 5*time.Millisecond); err == nil {
		t.Fatal("the old mailbox paired")
	}
	if len(srv.Devices()) != 0 {
		t.Fatal("a phone paired with the old code")
	}
	plain, _, _ := startTest(t)
	if o := plain.NewOffer(); o.Relay != nil {
		t.Fatalf("a server without the relay offers a mailbox: %+v", o.Relay)
	}
}

// slowSigning is a linkedBucket whose links take until the caller gives up to sign, as a bucket whose short-lived key is asked for from a service that does not answer.
type slowSigning struct{ *linkedBucket }

func (b slowSigning) PresignGet(ctx context.Context, _ string, _ time.Duration) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

// A code is offered on the UI thread with the server's lock held: a bucket slow to sign the mailbox's links holds it up for relayPairSignFor at most, and the code is then offered for the local network alone.
func TestACodeDoesNotWaitForASlowBucket(t *testing.T) {
	signFor := relayPairSignFor
	relayPairSignFor = 50 * time.Millisecond
	t.Cleanup(func() { relayPairSignFor = signFor })
	srv, err := Start(Config{Dir: t.TempDir(), Name: "Test Mac", Source: fakeSource{dir: t.TempDir()}, Relay: slowSigning{newLinkedBucket(t)}, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	start := time.Now()
	offer := srv.NewOffer()
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("the code waited %v", took)
	}
	if offer.Code == "" || offer.Relay != nil {
		t.Fatalf("offer %+v", offer)
	}
}
