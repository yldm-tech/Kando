package phonesync

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"kando/apps/kando/cloud"
	psync "kando/packages/sync"
)

// The storage relay (psync's relay.go, yldm-tech/Kando#219): while it is on (Config.Relay), the server answers the calls its paired phones leave in the user's bucket through the same Desk as the local network's, writes each joined phone's state.json there, and hands a phone its key and first grant when it asks on the local network (psync.OpRelayJoin). A phone's key is kept with the device, and unpairing it deletes its folders in the bucket.

// forgetTimeout is how long unpairing a phone waits for the store to delete its folders.
const forgetTimeout = time.Minute

// relayEvery and relayMinWrite are how often the relay polls the inboxes, unless Config.RelayEvery says otherwise, and the least time between two of its turns (psync.Relay's Every and MinWrite). Tests shorten them.
var relayEvery, relayMinWrite = psync.RelayPoll, psync.RelayMinWrite

// startRelay starts the relay in bucket, under the folder named by the certificate's fingerprint, until Close. Start calls it before the server listens, as the handlers read s.relay and s.desk.Relay.
func (s *Server) startRelay(bucket psync.RelayBucket) {
	ctx, cancel := context.WithCancel(context.Background())
	s.relay = &psync.Relay{
		Bucket:     bucket,
		Prefix:     psync.RelayPrefix(s.fingerprint),
		Desk:       s.desk,
		Devices:    s.devices.relayDevices,
		Now:        s.cfg.Now,
		Logf:       s.cfg.Logf,
		Every:      cmp.Or(s.cfg.RelayEvery, relayEvery),
		MinWrite:   relayMinWrite,
		Health:     s.cfg.RelayHealth,
		LinksUntil: s.cfg.RelayLinksUntil,
	}
	s.desk.Relay = relayJoiner{s}
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		<-s.stop
		cancel()
	}()
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		s.relay.Run(ctx)
	}()
}

// forgetRelay deletes device's folders in the bucket, off the caller's goroutine.
func (s *Server) forgetRelay(device string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.relay == nil || s.closed {
		return
	}
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		// Close does not wait out a slow store: folders left behind are deleted by the relay's sweep, as a device it does not know.
		ctx, cancel := context.WithTimeout(context.Background(), forgetTimeout)
		defer cancel()
		go func() {
			select {
			case <-s.stop:
				cancel()
			case <-ctx.Done():
			}
		}()
		if err := s.relay.Forget(ctx, device); err != nil {
			s.cfg.Logf("phonesync: relay: forget %s: %v", device, err)
		}
	}()
}

// relayJoiner answers psync.OpRelayJoin: the phone's key, made the first time it joins, and a new grant, as the phone may have spent slots of the one before.
type relayJoiner struct{ s *Server }

func (j relayJoiner) Join(ctx context.Context, caller psync.Caller) (psync.RelayJoin, error) {
	key, err := j.s.devices.relayKey(caller.DeviceID)
	if err != nil {
		return psync.RelayJoin{}, err
	}
	grant, err := j.s.relay.NewGrant(ctx, caller.DeviceID)
	if err != nil {
		return psync.RelayJoin{}, err
	}
	// The phone's state.json is written at the relay's next turn, now, rather than at its next poll.
	j.s.relay.Wake()
	return psync.RelayJoin{Key: key, Grant: grant}, nil
}

// Bucket is the user's bucket as the relay and the films through it use it: the relay's objects and links, a file put from disk in parts as large as it is (cloud.Bucket.PutFile), and the objects under a prefix with their sizes.
type Bucket interface {
	psync.RelayBucket
	PutFile(ctx context.Context, key, path, contentType string, resume cloud.Resume, save func(cloud.Resume) error, progress func(done, total int64)) error
	Sizes(ctx context.Context, prefix string) ([]cloud.Object, error)
}

// RelayBucket is b as the relay and the films use it.
func RelayBucket(b *cloud.Bucket) Bucket { return relayBucket{b} }

type relayBucket struct{ b *cloud.Bucket }

func (r relayBucket) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := r.b.Get(ctx, key, psync.MaxRelayObject)
	switch {
	case errors.Is(err, cloud.ErrNoObject):
		return nil, psync.ErrNoObject
	case errors.Is(err, cloud.ErrObjectTooLarge):
		return nil, fmt.Errorf("%w: %w", psync.ErrRelayTooLarge, err)
	}
	return data, err
}

func (r relayBucket) Put(ctx context.Context, key string, body []byte) error {
	return r.b.Put(ctx, key, body)
}

func (r relayBucket) Exists(ctx context.Context, key string) (bool, error) {
	_, err := r.b.Head(ctx, key)
	if errors.Is(err, cloud.ErrNoObject) {
		return false, nil
	}
	return err == nil, err
}

func (r relayBucket) List(ctx context.Context, prefix string) ([]psync.RelayObject, error) {
	objects, err := r.b.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]psync.RelayObject, len(objects))
	for i, o := range objects {
		out[i] = psync.RelayObject{Key: o.Key, Modified: o.Modified}
	}
	return out, nil
}

func (r relayBucket) Delete(ctx context.Context, key string) error { return r.b.Delete(ctx, key) }

func (r relayBucket) PutFile(ctx context.Context, key, path, contentType string, resume cloud.Resume, save func(cloud.Resume) error, progress func(done, total int64)) error {
	return r.b.PutFile(ctx, key, path, contentType, resume, save, progress)
}

func (r relayBucket) Sizes(ctx context.Context, prefix string) ([]cloud.Object, error) {
	return r.b.List(ctx, prefix)
}

func (r relayBucket) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return r.b.PresignGet(ctx, key, ttl)
}

func (r relayBucket) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return r.b.PresignPut(ctx, key, ttl)
}
