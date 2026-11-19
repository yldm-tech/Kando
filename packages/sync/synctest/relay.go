package synctest

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"kando/packages/sync"
)

// Bucket is the user's bucket in memory for the storage relay (package sync's relay.go), with presigned links served over loopback as S3 serves them: a link names one method on one key until it lapses, by the bucket's clock, and needs no credentials; the bucket refuses it (403) after. A key it does not hold is 404. Its methods may be called from any goroutine.
type Bucket struct {
	srv *httptest.Server
	now func() time.Time

	mu      gosync.Mutex
	objects map[string][]byte
	links   map[string]bucketLink
}

type bucketLink struct {
	method, key string
	expires     time.Time
}

// NewBucket is an empty bucket whose links lapse by now, closed when the test ends.
func NewBucket(t testing.TB, now func() time.Time) *Bucket {
	t.Helper()
	b := &Bucket{now: now, objects: map[string][]byte{}, links: map[string]bucketLink{}}
	b.srv = httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.srv.Close)
	return b
}

func (b *Bucket) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	b.mu.Lock()
	defer b.mu.Unlock()
	link, ok := b.links[r.URL.Query().Get("sig")]
	if !ok || link.method != r.Method || "/o/"+link.key != r.URL.Path || !b.now().Before(link.expires) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		data, ok := b.objects[link.key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	case http.MethodPut:
		b.objects[link.key] = body
	}
}

// Keys are the keys the bucket holds that start with prefix, sorted.
func (b *Bucket) Keys(prefix string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for k := range b.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// relayBucket is b as the desktop's relay uses it, with the store's credentials.
type relayBucket struct{ b *Bucket }

func (r relayBucket) Get(_ context.Context, key string) ([]byte, error) {
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	data, ok := r.b.objects[key]
	if !ok {
		return nil, sync.ErrNoObject
	}
	if len(data) > sync.MaxRelayObject {
		return nil, sync.ErrRelayTooLarge
	}
	return slices.Clone(data), nil
}

func (r relayBucket) Put(_ context.Context, key string, body []byte) error {
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	r.b.objects[key] = slices.Clone(body)
	return nil
}

func (r relayBucket) Exists(_ context.Context, key string) (bool, error) {
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	_, ok := r.b.objects[key]
	return ok, nil
}

func (r relayBucket) List(_ context.Context, prefix string) ([]sync.RelayObject, error) {
	var out []sync.RelayObject
	for _, k := range r.b.Keys(prefix) {
		out = append(out, sync.RelayObject{Key: k, Modified: r.b.now()})
	}
	return out, nil
}

func (r relayBucket) Delete(_ context.Context, key string) error {
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	delete(r.b.objects, key)
	return nil
}

func (r relayBucket) presign(method, key string, ttl time.Duration) string {
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	sig := rand.Text()
	r.b.links[sig] = bucketLink{method: method, key: key, expires: r.b.now().Add(ttl)}
	return r.b.srv.URL + "/o/" + key + "?sig=" + sig
}

func (r relayBucket) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return r.presign(http.MethodGet, key, ttl), nil
}

func (r relayBucket) PresignPut(_ context.Context, key string, ttl time.Duration) (string, error) {
	return r.presign(http.MethodPut, key, ttl), nil
}

// relay is the stand-in's storage relay: the phones that joined, by device ID, and the relay running while it is on.
type relay struct {
	relay   *sync.Relay
	devices map[string]sync.RelayDevice
	joins   int
	stop    context.CancelFunc
	done    chan struct{}
}

// StartRelay turns the desktop's storage relay on in bucket, by the clock now, before the phone calls: it lists sync.OpRelayJoin and hands a phone that joins on the local network its key and a new grant, and from then on, every every, answers the messages phones leave in the bucket and writes their state.json, until StopRelay or the test ends. It stays on when the desktop is off (SetOff), as a computer whose phone left its network.
func (d *Desktop) StartRelay(t testing.TB, bucket *Bucket, now func() time.Time, every time.Duration) {
	t.Helper()
	d.mu.Lock()
	d.relay.devices = map[string]sync.RelayDevice{}
	d.relay.relay = &sync.Relay{Bucket: relayBucket{bucket}, Prefix: sync.RelayPrefix(d.Fingerprint), Desk: d.desk, Devices: d.relayDevices, Now: now, Every: every, MinWrite: time.Millisecond}
	d.mu.Unlock()
	d.desk.Relay = relayJoiner{d}
	d.runRelay()
	t.Cleanup(d.StopRelay)
}

func (d *Desktop) runRelay() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	d.mu.Lock()
	d.relay.stop, d.relay.done = cancel, done
	r := d.relay.relay
	d.mu.Unlock()
	go func() {
		defer close(done)
		r.Run(ctx)
	}()
}

// StopRelay stops answering the bucket and writing the phones' state.json, as a computer that is off or asleep: what the phones put in the bucket waits there until ResumeRelay.
func (d *Desktop) StopRelay() {
	d.mu.Lock()
	stop, done := d.relay.stop, d.relay.done
	d.relay.stop, d.relay.done = nil, nil
	d.mu.Unlock()
	if stop != nil {
		stop()
		<-done
	}
}

// ResumeRelay runs the relay StopRelay stopped again, as the computer waking.
func (d *Desktop) ResumeRelay() {
	d.mu.Lock()
	running := d.relay.stop != nil || d.relay.relay == nil
	d.mu.Unlock()
	if !running {
		d.runRelay()
	}
}

// RelayJoins is how many times phones joined the relay.
func (d *Desktop) RelayJoins() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.relay.joins
}

func (d *Desktop) relayDevices() []sync.RelayDevice {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []sync.RelayDevice
	for _, dev := range d.relay.devices {
		out = append(out, dev)
	}
	return out
}

// relayJoiner answers sync.OpRelayJoin as the desktop does: the phone's key, made the first time it joins, and a new grant.
type relayJoiner struct{ d *Desktop }

func (j relayJoiner) Join(ctx context.Context, caller sync.Caller) (sync.RelayJoin, error) {
	d := j.d
	d.mu.Lock()
	dev, ok := d.relay.devices[caller.DeviceID]
	if !ok {
		dev = sync.RelayDevice{ID: caller.DeviceID, Name: caller.Name, Key: sync.NewRelayKey()}
		d.relay.devices[caller.DeviceID] = dev
	}
	d.relay.joins++
	r := d.relay.relay
	d.mu.Unlock()
	grant, err := r.NewGrant(ctx, caller.DeviceID)
	if err != nil {
		return sync.RelayJoin{}, err
	}
	r.Wake()
	return sync.RelayJoin{Key: slices.Clone(dev.Key), Grant: grant}, nil
}
