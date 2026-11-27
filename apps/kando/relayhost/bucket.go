package relayhost

import (
	"context"
	"errors"
	"strings"
	gosync "sync"
	"time"

	"kando/apps/kando/cloud"
	"kando/apps/kando/phonesync"
	psync "kando/packages/sync"
)

// Bucket is the hosted bucket as Phone Sync's relay uses it (phonesync.Bucket): every key goes under the account's folder (Credentials.Prefix), which the keys it lists leave out, and every request and link is signed with the newest key EveryAPI gave, asked for when there is none and again once half its life is spent. It asks nothing until it is first used, so building it does not talk to the network. It is safe for concurrent use.
type Bucket struct {
	client *Client
	now    func() time.Time
	told   func(State)

	// asking is held while a key is asked for, so one request goes to EveryAPI at a time; waiting for it gives up with the caller's context. mu guards the fields below it and is never held while EveryAPI is asked, so a link signed with the key held does not wait for a request under way.
	asking chan struct{}
	mu     gosync.Mutex
	cur    *session
	// failed is when the last request for a key failed, failure why, zero once one went through: EveryAPI is not asked again before retryAfter, so a store or a request that hangs is not asked again by every operation of a turn.
	failed  time.Time
	failure error
}

// State is what the last request for a key found, told to the app (NewBucket's told): the account's quota and use, whether it may write, and why the request failed, nil when it went through. Full is the storage full, as EveryAPI said it or as the key's numbers say.
type State struct {
	Quota, Used int64
	Writes      bool
	Full        bool
	Err         error
}

// session is a key EveryAPI gave, with the bucket it opens.
type session struct {
	creds   Credentials
	prefix  string
	bucket  phonesync.Bucket
	fetched time.Time
}

// ErrWritesStopped is the error of a write while EveryAPI does not let the account write (write_allowed false): its Kando Plus ended and the grace after it too. What is there stays, and is read.
var ErrWritesStopped = errors.New("the hosted relay takes no more writes")

// renewBefore is how long before it lapses a key is renewed at the latest, however short it is.
const renewBefore = 5 * time.Minute

// NewBucket is the hosted bucket of the account client asks with. now is the clock, nil for time.Now; told, when set, is told what each request for a key found, on the goroutine that asked.
func NewBucket(client *Client, now func() time.Time, told func(State)) *Bucket {
	if now == nil {
		now = time.Now
	}
	return &Bucket{client: client, now: now, told: told, asking: make(chan struct{}, 1)}
}

// retryAfter is how long after a request for a key failed EveryAPI is not asked again: the key held signs meanwhile while it is good, and without one the failure is answered again.
const retryAfter = 30 * time.Second

// fresh reports whether s needs no renewal at now: less than half its life is spent and it is more than renewBefore from lapsing.
func (s *session) fresh(now time.Time) bool {
	expires := s.creds.ExpiresAt.Time
	half := s.fetched.Add(expires.Sub(s.fetched) / 2)
	return now.Before(half) && now.Before(expires.Add(-renewBefore))
}

// held is the key held when it is fresh, else nil.
func (b *Bucket) held(now time.Time) *session {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cur != nil && b.cur.fresh(now) {
		return b.cur
	}
	return nil
}

// session is the key to sign with now: the one held while it is fresh, else a new one. A new one that cannot be had leaves the one held in use while it is good, but not once EveryAPI said the account has no Plus.
func (b *Bucket) session(ctx context.Context) (*session, error) {
	if s := b.held(b.now()); s != nil {
		return s, nil
	}
	select {
	case b.asking <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-b.asking }()
	now := b.now()
	b.mu.Lock()
	cur, failed, failure := b.cur, b.failed, b.failure
	b.mu.Unlock()
	if cur != nil && cur.fresh(now) {
		// Renewed while this waited.
		return cur, nil
	}
	if !failed.IsZero() && now.Sub(failed) < retryAfter {
		return b.fallback(cur, now, failure)
	}
	creds, err := b.client.Credentials(ctx)
	if err == nil {
		var next *session
		if next, err = open(creds, now); err == nil {
			b.mu.Lock()
			b.cur, b.failed, b.failure = next, time.Time{}, nil
			b.mu.Unlock()
			b.tell(State{Quota: creds.QuotaBytes, Used: creds.UsedBytes, Writes: creds.WriteAllowed, Full: creds.Full()})
			return next, nil
		}
	}
	if ctx.Err() != nil {
		// The caller gave up: that says nothing of EveryAPI.
		return b.fallback(cur, now, err)
	}
	b.mu.Lock()
	b.failed, b.failure = now, err
	b.mu.Unlock()
	st := State{Err: err, Full: errors.Is(err, ErrQuotaExceeded)}
	if kept, _ := b.fallback(cur, now, err); kept != nil {
		// The key held goes on: its numbers still stand.
		st.Quota, st.Used, st.Writes, st.Full = kept.creds.QuotaBytes, kept.creds.UsedBytes, kept.creds.WriteAllowed, st.Full || kept.creds.Full()
	}
	b.tell(st)
	return b.fallback(cur, now, err)
}

// fallback is the key held, cur, when a new one could not be had for err: while it is good for a minute more, unless EveryAPI said the account has no Plus.
func (b *Bucket) fallback(cur *session, now time.Time, err error) (*session, error) {
	if cur != nil && now.Before(cur.creds.ExpiresAt.Add(-time.Minute)) && !errors.Is(err, ErrPlusRequired) {
		return cur, nil
	}
	return nil, err
}

// signer is the key a link is signed with: the one held while it is more than renewBefore from lapsing, without asking EveryAPI to renew it, so signing a link, as a pairing code does on the UI thread, waits for no request; else session. The relay renews a key past half its life through LinksUntil, which it asks before it signs a grant or a film's links.
func (b *Bucket) signer(ctx context.Context) (*session, error) {
	now := b.now()
	b.mu.Lock()
	cur := b.cur
	b.mu.Unlock()
	if cur != nil && now.Before(cur.creds.ExpiresAt.Add(-renewBefore)) {
		return cur, nil
	}
	return b.session(ctx)
}

func (b *Bucket) tell(s State) {
	if b.told != nil {
		b.told(s)
	}
}

// open is the bucket creds opens, fetched at now.
func open(creds Credentials, now time.Time) (*session, error) {
	opened, err := cloud.OpenSessionBucket(creds.Endpoint, creds.Region, creds.Bucket, cloud.Session{AccessKey: creds.AccessKeyID, Secret: creds.SecretAccessKey, Token: creds.SessionToken, Expires: creds.ExpiresAt.Time})
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimLeft(creds.Prefix, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &session{creds: creds, prefix: prefix, bucket: phonesync.RelayBucket(opened), fetched: now}, nil
}

// writable is the key to write with: none while EveryAPI does not let the account write.
func (b *Bucket) writable(ctx context.Context) (*session, error) {
	s, err := b.session(ctx)
	if err != nil {
		return nil, err
	}
	if !s.creds.WriteAllowed {
		return nil, ErrWritesStopped
	}
	return s, nil
}

// LinksUntil is when the key the next link is signed with lapses, renewing it first when it is due, so the relay signs no link that outlives it (sync.Relay.LinksUntil).
func (b *Bucket) LinksUntil(ctx context.Context) (time.Time, error) {
	s, err := b.session(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return s.creds.ExpiresAt.Time, nil
}

func (b *Bucket) Get(ctx context.Context, key string) ([]byte, error) {
	s, err := b.session(ctx)
	if err != nil {
		return nil, err
	}
	return s.bucket.Get(ctx, s.prefix+key)
}

func (b *Bucket) Put(ctx context.Context, key string, body []byte) error {
	s, err := b.writable(ctx)
	if err != nil {
		return err
	}
	return s.bucket.Put(ctx, s.prefix+key, body)
}

func (b *Bucket) Exists(ctx context.Context, key string) (bool, error) {
	s, err := b.session(ctx)
	if err != nil {
		return false, err
	}
	return s.bucket.Exists(ctx, s.prefix+key)
}

func (b *Bucket) List(ctx context.Context, prefix string) ([]psync.RelayObject, error) {
	s, err := b.session(ctx)
	if err != nil {
		return nil, err
	}
	objects, err := s.bucket.List(ctx, s.prefix+prefix)
	if err != nil {
		return nil, err
	}
	for i := range objects {
		objects[i].Key = strings.TrimPrefix(objects[i].Key, s.prefix)
	}
	return objects, nil
}

// Delete removes the object at key. It goes through while writes are stopped too, as it only frees room, when EveryAPI's key lets it.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	s, err := b.session(ctx)
	if err != nil {
		return err
	}
	return s.bucket.Delete(ctx, s.prefix+key)
}

func (b *Bucket) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	s, err := b.signer(ctx)
	if err != nil {
		return "", err
	}
	return s.bucket.PresignGet(ctx, s.prefix+key, ttl)
}

// PresignPut is a link that PUTs key, none while writes are stopped: a phone's message would be written to a bucket the desktop no longer answers from.
func (b *Bucket) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	s, err := b.signer(ctx)
	if err != nil {
		return "", err
	}
	if !s.creds.WriteAllowed {
		return "", ErrWritesStopped
	}
	return s.bucket.PresignPut(ctx, s.prefix+key, ttl)
}

// PutFile puts a film's file, none while writes are stopped or the storage is full: a video would only fail at the store once the quota is reached.
func (b *Bucket) PutFile(ctx context.Context, key, path, contentType string, resume cloud.Resume, save func(cloud.Resume) error, progress func(done, total int64)) error {
	s, err := b.writable(ctx)
	if err != nil {
		return err
	}
	if s.creds.Full() {
		return ErrQuotaExceeded
	}
	return s.bucket.PutFile(ctx, s.prefix+key, path, contentType, resume, save, progress)
}

func (b *Bucket) Sizes(ctx context.Context, prefix string) ([]cloud.Object, error) {
	s, err := b.session(ctx)
	if err != nil {
		return nil, err
	}
	objects, err := s.bucket.Sizes(ctx, s.prefix+prefix)
	if err != nil {
		return nil, err
	}
	for i := range objects {
		objects[i].Key = strings.TrimPrefix(objects[i].Key, s.prefix)
	}
	return objects, nil
}

var _ phonesync.Bucket = (*Bucket)(nil)
