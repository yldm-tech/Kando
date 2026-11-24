package sync

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// Pairing through the relay (yldm-tech/Kando#468): a phone that cannot reach the desktop on its network pairs through the user's bucket, with the code the QR code carries, when the desktop's relay is on.
//
// The QR code is the secret channel. The phone's camera reads it off the desktop's screen, and it carries, besides the code and the certificate's fingerprint, a pairing mailbox: one link that GETs the mailbox's box, and a key of RelayKeySize random bytes made for this code alone (PairRelay). The desktop writes the box, sealed under that key, as it offers the code: it holds the link the phone PUTs its pairing request to (ask) and the one it GETs the answer from (answer). The desktop reads the ask while the code is on offer, checks its code as it checks one sent to PathPair, under the same limits (one use, CodeLifetime, CodeTries, the pairings per minute), and writes the answer, sealed under the same key: the phone's token and device ID, and its join of the relay (RelayJoin), so a phone paired this way follows the desktop through the relay from then on. The relay key thus travels only sealed under a key that only a holder of the QR code has; the store sees sealed bytes alone.
//
// Layout, below RelayPrefix, for a code's mailbox {id} (32 hex characters):
//
//	pair/{id}/box     the box (RelayPairBox), which the QR code's link reads
//	pair/{id}/ask     the phone's pairing request (PairRequest)
//	pair/{id}/answer  the desktop's answer (RelayPairAnswer)
//
// Each is sealed (SealRelay) under the mailbox's key with its part's name ("pair/box", "pair/ask", "pair/answer") as additional data. The links are good for CodeLifetime. A mailbox answers one ask: the desktop deletes the ask once it wrote the answer, and the whole mailbox once its code is no longer on offer (Remove); the relay's sweep deletes any mailbox older than RelayPairKeep.

// RelayPairKeep is how old a pairing mailbox the relay's sweep deletes is: one whose code ended long ago, as when the desktop stopped before it deleted it.
const RelayPairKeep = 2 * CodeLifetime

// RelayPairEvery is how often the desktop reads a mailbox's ask while its code is on offer, and the phone the answer while it waits.
const RelayPairEvery = 2 * time.Second

// The names, below RelayPrefix and as additional data, of a pairing mailbox's parts.
const (
	relayPairFolder = "pair/"
	relayPairBox    = "pair/box"
	relayPairAsk    = "pair/ask"
	relayPairAnswer = "pair/answer"
)

// PairRelay is the pairing mailbox a QR code carries: the link that GETs its box and the key its parts are sealed under.
type PairRelay struct {
	Box string
	Key []byte
}

// RelayPairBox is a mailbox's box: the link the phone PUTs its pairing request to and the one it GETs the answer from, and when they lapse.
type RelayPairBox struct {
	Ask     string    `json:"ask"`
	Answer  string    `json:"answer"`
	Expires time.Time `json:"expires"`
}

// RelayPairAnswer is the desktop's answer to a pairing through the relay: the pairing and the phone's join of the relay, or why it did not pair.
type RelayPairAnswer struct {
	Pair  *PairResponse `json:"pair,omitempty"`
	Join  *RelayJoin    `json:"join,omitempty"`
	Error *ErrorBody    `json:"error,omitempty"`
}

// RelayPairMailbox is the desktop's side of a code's pairing mailbox in its bucket.
type RelayPairMailbox struct {
	Bucket RelayBucket
	// Prefix is RelayPrefix of the desktop; ID names the mailbox, Key seals its parts.
	Prefix string
	ID     string
	Key    []byte
	// Link is the box's GET link, which the QR code carries; box what Publish writes.
	Link string
	box  RelayPairBox
}

// NewRelayPairMailbox makes a new mailbox in bucket under prefix, its links good for ttl from now. Making it does not talk to the network: Publish writes the box.
func NewRelayPairMailbox(ctx context.Context, bucket RelayBucket, prefix string, ttl time.Duration, now time.Time) (*RelayPairMailbox, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	m := &RelayPairMailbox{Bucket: bucket, Prefix: prefix, ID: hex.EncodeToString(id), Key: NewRelayKey()}
	var err error
	if m.Link, err = bucket.PresignGet(ctx, m.object("box"), ttl); err != nil {
		return nil, err
	}
	m.box.Expires = now.Add(ttl)
	if m.box.Ask, err = bucket.PresignPut(ctx, m.object("ask"), ttl); err != nil {
		return nil, err
	}
	if m.box.Answer, err = bucket.PresignGet(ctx, m.object("answer"), ttl); err != nil {
		return nil, err
	}
	return m, nil
}

// Relay is the mailbox as the QR code carries it.
func (m *RelayPairMailbox) Relay() PairRelay { return PairRelay{Box: m.Link, Key: m.Key} }

func (m *RelayPairMailbox) object(part string) string {
	return m.Prefix + relayPairFolder + m.ID + "/" + part
}

// Publish writes the sealed box.
func (m *RelayPairMailbox) Publish(ctx context.Context) error {
	plain, err := json.Marshal(m.box)
	if err != nil {
		return err
	}
	sealed, err := SealRelay(m.Key, relayPairBox, plain)
	if err != nil {
		return err
	}
	return m.Bucket.Put(ctx, m.object("box"), sealed)
}

// Ask reads the phone's pairing request, false while there is none. One that does not open or read is deleted, and reads as none.
func (m *RelayPairMailbox) Ask(ctx context.Context) (PairRequest, bool, error) {
	sealed, err := m.Bucket.Get(ctx, m.object("ask"))
	if errors.Is(err, ErrNoObject) {
		return PairRequest{}, false, nil
	}
	if err != nil && !errors.Is(err, ErrRelayTooLarge) {
		return PairRequest{}, false, err
	}
	var req PairRequest
	if err == nil {
		var plain []byte
		if plain, err = OpenRelay(m.Key, relayPairAsk, sealed); err == nil {
			err = json.Unmarshal(plain, &req)
		}
	}
	if err != nil {
		return PairRequest{}, false, m.Bucket.Delete(ctx, m.object("ask"))
	}
	return req, true, nil
}

// Answer writes the answer to the ask, then deletes the ask.
func (m *RelayPairMailbox) Answer(ctx context.Context, a RelayPairAnswer) error {
	plain, err := json.Marshal(a)
	if err != nil {
		return err
	}
	sealed, err := SealRelay(m.Key, relayPairAnswer, plain)
	if err != nil {
		return err
	}
	if err := m.Bucket.Put(ctx, m.object("answer"), sealed); err != nil {
		return err
	}
	return m.Bucket.Delete(ctx, m.object("ask"))
}

// Remove deletes the mailbox.
func (m *RelayPairMailbox) Remove(ctx context.Context) error {
	var errs []error
	for _, part := range []string{"box", "ask", "answer"} {
		errs = append(errs, m.Bucket.Delete(ctx, m.object(part)))
	}
	return errors.Join(errs...)
}

// Serve publishes the box and reads the ask every every, 0 for RelayPairEvery, until ctx ends: it answers the first ask with what pair answers it, writing that answer again until the store takes it, and then reads no more, as the mailbox answers one. It leaves the mailbox in the bucket for the caller to Remove. What failed of the store is tried again at the next turn; logf, when not nil, logs it.
func (m *RelayPairMailbox) Serve(ctx context.Context, every time.Duration, pair func(context.Context, PairRequest) RelayPairAnswer, logf func(string, ...any)) {
	if every <= 0 {
		every = RelayPairEvery
	}
	published, answered := false, false
	var answer *RelayPairAnswer
	// failing is what failed last, logged once until a turn goes through, so a store that refuses the desktop does not fill the log every turn for the code's whole life.
	failing := ""
	fail := func(what string, err error) {
		if err != nil && ctx.Err() == nil && what != failing {
			failing = what
			if logf != nil {
				logf("relay: pairing mailbox: %s: %v", what, err)
			}
		}
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		switch {
		case !published:
			if err := m.Publish(ctx); err != nil {
				fail("publish", err)
			} else {
				published, failing = true, ""
			}
		case !answered && answer != nil:
			// An answer the store did not take is written again as it was: the ask was paired once, and a second pairing of it would find the code used.
			if err := m.Answer(ctx, *answer); err != nil {
				fail("answer", err)
			} else {
				answered, failing = true, ""
			}
		case !answered:
			req, ok, err := m.Ask(ctx)
			if err != nil {
				fail("ask", err)
			}
			if ok {
				a := pair(ctx, req)
				answer = &a
				if err := m.Answer(ctx, a); err != nil {
					fail("answer", err)
				} else {
					answered, failing = true, ""
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// ErrRelayPairTimeout is PairThroughRelay's error when the desktop did not answer before the mailbox's links lapsed or ctx ended: it is off, or its relay does not reach the store.
var ErrRelayPairTimeout = errors.New("the desktop did not answer the pairing through the relay")

// PairThroughRelay pairs with the desktop through the mailbox r of its QR code: it reads the box, puts req in its ask and waits for the answer, reading it every every (0 for RelayPairEvery), until ctx ends or the links lapse. It answers the pairing and the phone's join of the relay; a code the desktop did not take is an *Error with its code, as Client.Pair's.
func PairThroughRelay(ctx context.Context, client *http.Client, r PairRelay, req PairRequest, every time.Duration) (PairResponse, RelayJoin, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if every <= 0 {
		every = RelayPairEvery
	}
	wait := func() error {
		select {
		case <-ctx.Done():
			return ErrRelayPairTimeout
		case <-time.After(every):
			return nil
		}
	}
	read := func(link, name string, v any) (bool, error) {
		sealed, err := relayGet(ctx, client, link, MaxRelayReply)
		if errors.Is(err, ErrNoObject) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		plain, err := OpenRelay(r.Key, name, sealed)
		if err != nil {
			return false, err
		}
		return true, json.Unmarshal(plain, v)
	}
	// The box: the desktop writes it as it offers the code, so it may not be there yet.
	var box RelayPairBox
	for {
		ok, err := read(r.Box, relayPairBox, &box)
		if errors.Is(err, ErrRelayLapsed) {
			return PairResponse{}, RelayJoin{}, ErrRelayPairTimeout
		}
		if err != nil && !isTransient(err) {
			return PairResponse{}, RelayJoin{}, err
		}
		if ok {
			break
		}
		if err := wait(); err != nil {
			return PairResponse{}, RelayJoin{}, err
		}
	}
	plain, err := json.Marshal(req)
	if err != nil {
		return PairResponse{}, RelayJoin{}, err
	}
	sealed, err := SealRelay(r.Key, relayPairAsk, plain)
	if err != nil {
		return PairResponse{}, RelayJoin{}, err
	}
	for {
		err := relayPut(ctx, client, box.Ask, sealed)
		if err == nil {
			break
		}
		if errors.Is(err, ErrRelayLapsed) {
			return PairResponse{}, RelayJoin{}, ErrRelayPairTimeout
		}
		if !isTransient(err) {
			return PairResponse{}, RelayJoin{}, err
		}
		if err := wait(); err != nil {
			return PairResponse{}, RelayJoin{}, err
		}
	}
	for {
		var a RelayPairAnswer
		ok, err := read(box.Answer, relayPairAnswer, &a)
		if errors.Is(err, ErrRelayLapsed) {
			return PairResponse{}, RelayJoin{}, ErrRelayPairTimeout
		}
		if err != nil && !isTransient(err) {
			return PairResponse{}, RelayJoin{}, err
		}
		if ok {
			switch {
			case a.Error != nil:
				return PairResponse{}, RelayJoin{}, &Error{Status: StatusOf(a.Error.Code), ErrorBody: *a.Error}
			case a.Pair == nil || a.Join == nil || len(a.Join.Key) != RelayKeySize:
				return PairResponse{}, RelayJoin{}, errors.New("the desktop's answer through the relay is incomplete")
			}
			return *a.Pair, *a.Join, nil
		}
		if err := wait(); err != nil {
			return PairResponse{}, RelayJoin{}, err
		}
	}
}

// isTransient reports whether err, of a request of a mailbox's link, may go through when tried again: one of the network, or of the store other than a refusal of the link.
func isTransient(err error) bool {
	var store *RelayStoreError
	if errors.As(err, &store) {
		return store.Status >= 500 || store.Status == http.StatusTooManyRequests
	}
	return !errors.Is(err, ErrRelaySealed) && !errors.Is(err, context.Canceled)
}

// relayGet reads link, at most limit bytes, ErrNoObject for a 404.
func relayGet(ctx context.Context, client *http.Client, link string, limit int) ([]byte, error) {
	return (&RelayPhone{HTTP: client}).get(ctx, link, limit)
}

// relayPut puts body at link.
func relayPut(ctx context.Context, client *http.Client, link string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, link, bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := storeErr(resp); err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return nil
}
