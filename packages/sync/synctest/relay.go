package synctest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	// gets are the GETs by link, each its key and its Range header, in their order.
	gets []string
	// refusal is what the bucket answers every link with while set (Refuse).
	refusal *refusal
}

type refusal struct {
	status int
	code   string
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
	if f := b.refusal; f != nil {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(f.status)
		_, _ = fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Error><Code>%s</Code><Message>refused</Message></Error>", f.code)
		return
	}
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
		// As S3 does, a GET answers a Range with the part asked for, and names the object by an ETag of its bytes.
		sum := sha256.Sum256(data)
		w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:8])+`"`)
		b.gets = append(b.gets, link.key+" "+r.Header.Get("Range"))
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	case http.MethodPut:
		b.objects[link.key] = body
	}
}

// Refuse has the bucket answer every link with status and an S3 error document naming code, as a store that no longer takes the key the links were signed with (403 SignatureDoesNotMatch), whose bucket was deleted (404 NoSuchBucket) or that is full (507); a status of 0 serves the links again. The desktop's own requests, with the store's credentials, still go through.
func (b *Bucket) Refuse(status int, code string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refusal = nil
	if status != 0 {
		b.refusal = &refusal{status: status, code: code}
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

// Gets are the GETs the bucket answered by link, each its key and its Range header, "" for none, in their order.
func (b *Bucket) Gets() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.gets)
}

// Delete deletes the object at key, as the user or a cleanup does.
func (b *Bucket) Delete(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.objects, key)
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
	relay *sync.Relay
	// films are what ShareFilms put in the bucket, nil before.
	films   *sync.RelayFiles
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
	d.relay.relay = &sync.Relay{Bucket: relayBucket{bucket}, Prefix: sync.RelayPrefix(d.Fingerprint), Desk: d.desk, Devices: d.relayDevices, Films: d.relayFilms, Now: now, Every: every, MinWrite: time.Millisecond}
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

// OfferRelay offers code for pairing as Offer does, and a pairing mailbox for it in the relay's bucket (package sync's relay_pair.go), served every every until the test ends, as Kando's desktop offers one with each code while its relay is on: a phone pairs through it as through the local network, with the same code, and is joined to the relay with it. It answers the mailbox the QR code carries. StartRelay comes first.
func (d *Desktop) OfferRelay(t testing.TB, code string, every time.Duration) sync.PairRelay {
	t.Helper()
	d.Offer(code)
	d.mu.Lock()
	r := d.relay.relay
	d.mu.Unlock()
	m, err := sync.NewRelayPairMailbox(context.Background(), r.Bucket, r.Prefix, sync.CodeLifetime, r.Bucket.(relayBucket).b.now())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Serve(ctx, every, func(ctx context.Context, req sync.PairRequest) sync.RelayPairAnswer {
			resp, code := d.tryPair(req)
			if code != "" {
				return sync.RelayPairAnswer{Error: &sync.ErrorBody{Code: code}}
			}
			join, err := relayJoiner{d}.Join(ctx, sync.Caller{DeviceID: resp.DeviceID, Name: req.Device})
			if err != nil {
				return sync.RelayPairAnswer{Error: &sync.ErrorBody{Code: sync.CodeFailed, Message: err.Error()}}
			}
			return sync.RelayPairAnswer{Pair: &resp, Join: &join}
		}, nil)
		_ = m.Remove(context.Background())
	}()
	t.Cleanup(func() { cancel(); <-done })
	return m.Relay()
}

// RelayJoins is how many times phones joined the relay.
func (d *Desktop) RelayJoins() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.relay.joins
}

// relayDevices are the phones that joined, read-only while the desktop makes every device so (SetReadOnly), as Kando's desktop reads each device's switch at every pass.
func (d *Desktop) relayDevices() []sync.RelayDevice {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []sync.RelayDevice
	for _, dev := range d.relay.devices {
		dev.ReadOnly = d.queue.readOnly
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

// ShareFilms puts the desktop's films in the relay's bucket as the desktop shares them with its phones away from its network (package sync's relay_films.go), each in a folder of its own, and lists them in the phones' state.json from the relay's next turn: its lines and poster always, its video when videos is set, else it is shared as lines alone. StartRelay comes first.
func (d *Desktop) ShareFilms(videos bool) {
	d.mu.Lock()
	r := d.relay.relay
	libs := slices.Clone(d.libs)
	films := slices.Clone(d.films)
	d.mu.Unlock()
	b := r.Bucket.(relayBucket).b
	files := &sync.RelayFiles{Libraries: libs}
	for i, f := range films {
		folder := fmt.Sprintf("f%02d", i)
		ff := sync.RelayFilmFiles{Library: f.Library, Film: f.Film, Subtitles: sync.RelayFilmObject(folder, sync.RelaySubtitlesFile)}
		lines, _ := json.Marshal(sync.Subtitles{Cues: append([]sync.Cue{}, f.Cues...)})
		put := func(name string, body []byte) {
			b.mu.Lock()
			b.objects[r.Prefix+name] = body
			b.mu.Unlock()
		}
		put(ff.Subtitles, lines)
		if videos && f.Video != nil {
			ff.Video = sync.RelayFilmObject(folder, "video.mp4")
			put(ff.Video, f.Video)
		}
		if f.Poster != nil {
			ff.Poster = sync.RelayFilmObject(folder, "poster.jpg")
			put(ff.Poster, f.Poster)
		}
		files.Films = append(files.Films, ff)
	}
	d.mu.Lock()
	d.relay.films = files
	d.mu.Unlock()
	r.Wake()
}

func (d *Desktop) relayFilms() *sync.RelayFiles {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.relay.films
}
