package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// RelayPhone is a phone's side of the storage relay (relay.go): it reads its state.json and sends calls through the slots of its grant, with plain HTTP requests to the grant's links. The phone keeps Device, Key, Grant, Spent, Waiting and Seq between runs. It is not safe for concurrent use.
type RelayPhone struct {
	// Device is the phone's device ID (PairResponse.DeviceID), Key and Grant what it joined with (RelayJoin) or the newest grant since.
	Device string
	Key    []byte
	Grant  RelayGrant
	// Spent are the slots of Grant the phone has put a message in, Seq the number of its last message.
	Spent []string
	Seq   int64
	// Waiting are the calls the phone sent whose replies it has not read, by slot, from this grant or an older one: a newer grant replaces Grant and Spent, but the replies to calls already sent are still read by their old slots' links.
	Waiting map[string]RelayWaiting
	// HTTP is the client the links are requested with, nil for http.DefaultClient; Now the clock, nil for time.Now.
	HTTP *http.Client
	Now  func() time.Time
}

// RelayWaiting is a call sent in a slot whose reply the phone has not read: the call's ID, the slot's reply link, when that link lapses, and the number of its message (RelayMessage.Seq), by which the phone reads the replies in the order the desktop answered the calls; 0 for one kept by a phone that did not keep it.
type RelayWaiting struct {
	Call    string    `json:"call"`
	Reply   string    `json:"reply"`
	Expires time.Time `json:"expires"`
	Seq     int64     `json:"seq,omitempty"`
}

// ErrRelayCallTooLarge is Send's error for a call that, sealed, is larger than MaxRelayObject: the desktop would not read it.
var ErrRelayCallTooLarge = errors.New("the call is too large for the relay")

// ErrRelayReplyMismatch is Reply's error when the slot's reply answers another call than the one the phone sent in it, as when a slot was used twice: the desktop answered only the first, so the phone sends the call again in another slot.
var ErrRelayReplyMismatch = errors.New("the relay's reply answers another call")

// ErrRelaySlotsUsed is Send's error when every slot of the grant is spent: the phone reads its state.json for a newer grant.
var ErrRelaySlotsUsed = errors.New("every slot of the relay's grant is used")

// ErrRelayLapsed is the error of a link the store refuses (403), as one past RelayGrant.Expires: the phone joins again on the desktop's network.
var ErrRelayLapsed = errors.New("the relay's links have lapsed")

func (p *RelayPhone) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return http.DefaultClient
}

func (p *RelayPhone) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// State reads the phone's state.json, ErrNoObject while the desktop has not written one, and takes its grant when it is newer than the phone's, its slots all unspent. The calls still waiting for their replies keep their old slots' links (Waiting).
func (p *RelayPhone) State(ctx context.Context) (RelayState, error) {
	sealed, err := p.get(ctx, p.Grant.State, MaxRelayReply)
	if err != nil {
		return RelayState{}, err
	}
	plain, err := OpenRelay(p.Key, RelayStateName(p.Device), sealed)
	if err != nil {
		return RelayState{}, err
	}
	var state RelayState
	if err := json.Unmarshal(plain, &state); err != nil {
		return RelayState{}, err
	}
	if state.Grant.Minted.After(p.Grant.Minted) {
		p.Grant, p.Spent = state.Grant, nil
	}
	return state, nil
}

// Send puts call in the next unspent slot, numbered after the last message, and answers the slot, whose reply Reply reads. A call that failed to send is sent again, with the same ID, by another Send.
func (p *RelayPhone) Send(ctx context.Context, call Call) (string, error) {
	i := slices.IndexFunc(p.Grant.Slots, func(s RelaySlot) bool { return !slices.Contains(p.Spent, s.ID) })
	if i < 0 {
		return "", ErrRelaySlotsUsed
	}
	slot := p.Grant.Slots[i]
	plain, err := json.Marshal(RelayMessage{Seq: p.Seq + 1, Sent: p.now(), Call: call})
	if err != nil {
		return "", err
	}
	sealed, err := SealRelay(p.Key, RelayInbox(p.Device, slot.ID), plain)
	if err != nil {
		return "", err
	}
	if len(sealed) > MaxRelayObject {
		return "", ErrRelayCallTooLarge
	}
	p.Spent = append(p.Spent, slot.ID)
	p.Seq++
	now := p.now()
	for id, w := range p.Waiting {
		if !now.Before(w.Expires) {
			delete(p.Waiting, id)
		}
	}
	if p.Waiting == nil {
		p.Waiting = map[string]RelayWaiting{}
	}
	p.Waiting[slot.ID] = RelayWaiting{Call: call.ID, Reply: slot.Reply, Expires: p.Grant.Expires, Seq: p.Seq}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, slot.Put, bytes.NewReader(sealed))
	if err != nil {
		return "", err
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return slot.ID, err
	}
	defer resp.Body.Close()
	if err := storeErr(resp); err != nil {
		return slot.ID, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return slot.ID, nil
}

// Reply reads the reply to the call sent in slot, false while the desktop has not answered it. Once read, the slot is no longer waited on, and a reply that answers another call than the one sent in the slot is ErrRelayReplyMismatch.
func (p *RelayPhone) Reply(ctx context.Context, slot string) (Reply, bool, error) {
	w, ok := p.Waiting[slot]
	if !ok {
		return Reply{}, false, fmt.Errorf("no call waits in slot %s", slot)
	}
	sealed, err := p.get(ctx, w.Reply, MaxRelayReply)
	if errors.Is(err, ErrNoObject) {
		return Reply{}, false, nil
	}
	if err != nil {
		return Reply{}, false, err
	}
	plain, err := OpenRelay(p.Key, RelayReply(p.Device, slot), sealed)
	if err != nil {
		return Reply{}, false, err
	}
	var reply Reply
	if err := json.Unmarshal(plain, &reply); err != nil {
		return Reply{}, false, err
	}
	delete(p.Waiting, slot)
	if reply.ID != w.Call {
		return Reply{}, false, ErrRelayReplyMismatch
	}
	return reply, true, nil
}

// get reads link, at most limit bytes, ErrNoObject for a 404.
func (p *RelayPhone) get(ctx context.Context, link string, limit int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := storeErr(resp); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, errors.New("the relay's object is too large")
	}
	return data, nil
}

// RelayStoreError is the store's refusal of a link of the relay: the HTTP status it answered, and the error code and message its S3 error document names, "" where it sent none.
type RelayStoreError struct {
	Status  int
	Code    string
	Message string
}

func (e *RelayStoreError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("the store answered %d", e.Status)
	}
	return fmt.Sprintf("the store answered %d %s: %s", e.Status, e.Code, e.Message)
}

// Is reads the refusal as the relay's errors: a 403 is ErrRelayLapsed, as the store refuses links past their expiry or signed with a key it no longer takes, which only a new grant mends; a 404 of an object, not of the whole bucket, is ErrNoObject.
func (e *RelayStoreError) Is(target error) bool {
	switch target {
	case ErrRelayLapsed:
		return e.Status == http.StatusForbidden
	case ErrNoObject:
		return e.Status == http.StatusNotFound && e.Code != "NoSuchBucket"
	}
	return false
}

// storeErr is the error of resp, nil for a 2xx: a *RelayStoreError with the code and message of the S3 error document it carries.
func storeErr(resp *http.Response) error {
	if resp.StatusCode < 300 {
		return nil
	}
	e := &RelayStoreError{Status: resp.StatusCode}
	var doc struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	if data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10)); err == nil && xml.Unmarshal(data, &doc) == nil {
		e.Code, e.Message = strings.TrimSpace(doc.Code), strings.TrimSpace(doc.Message)
	}
	return e
}

// RelayFailure is why the store did not serve a phone's request of the relay, as the phone tells its user; RelayNoFailure for an error that is not the store's, or none.
type RelayFailure uint8

const (
	RelayNoFailure RelayFailure = iota
	// RelayLapsed is links past their expiry: the phone joins again on the desktop's network.
	RelayLapsed
	// RelayKeys is links signed with a key the store no longer takes, as once the desktop's user changed or deleted it.
	RelayKeys
	// RelayDenied is a key that may not reach the bucket.
	RelayDenied
	// RelayNoBucket is a bucket that is no longer there.
	RelayNoBucket
	// RelayFull is a store that takes nothing more: out of space, or over its quota.
	RelayFull
	// RelayUnreachable is a store the phone could not connect to, as with no network.
	RelayUnreachable
	// RelayFailed is any other answer of the store, as a 5xx.
	RelayFailed
)

// The S3 error codes that name a key the store does not take, as the desktop reads them (apps/kando/cloud's Diagnose), and those S3-compatible stores (MinIO among them) answer when they take nothing more, besides HTTP 507.
var (
	relayKeyCodes  = []string{"SignatureDoesNotMatch", "InvalidAccessKeyId", "InvalidSecurity", "InvalidToken", "ExpiredToken"}
	relayFullCodes = []string{"QuotaExceeded", "StorageFull", "XMinioStorageFull", "XMinioAdminBucketQuotaExceeded"}
)

// FailureOf is why err, an error of State, Send or Reply, is the store's: a 403 is RelayLapsed once the grant's links are past Grant.Expires by the phone's clock, or the store says the request expired.
func (p *RelayPhone) FailureOf(err error) RelayFailure {
	var e *RelayStoreError
	var u *url.Error
	switch {
	case err == nil || errors.Is(err, ErrNoObject) || errors.Is(err, context.Canceled):
		return RelayNoFailure
	case errors.As(err, &e):
		switch {
		case slices.Contains(relayKeyCodes, e.Code):
			return RelayKeys
		case e.Code == "NoSuchBucket":
			return RelayNoBucket
		case e.Status == http.StatusInsufficientStorage || slices.Contains(relayFullCodes, e.Code):
			return RelayFull
		case e.Status == http.StatusForbidden && (!p.now().Before(p.Grant.Expires) || strings.Contains(strings.ToLower(e.Message), "expired")):
			return RelayLapsed
		case e.Status == http.StatusForbidden:
			return RelayDenied
		}
		return RelayFailed
	case errors.As(err, &u):
		return RelayUnreachable
	}
	return RelayNoFailure
}

// Err is the error reply answers, as Client.CallID answers the same reply on the local network: an *Error with the status its code stands for, nil for a reply with a result.
func (r Reply) Err() error {
	if r.Error == nil {
		return nil
	}
	return &Error{Status: StatusOf(r.Error.Code), ErrorBody: *r.Error}
}
