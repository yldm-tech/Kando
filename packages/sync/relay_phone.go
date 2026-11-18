package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
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

// RelayWaiting is a call sent in a slot whose reply the phone has not read: the call's ID, the slot's reply link, and when that link lapses.
type RelayWaiting struct {
	Call    string    `json:"call"`
	Reply   string    `json:"reply"`
	Expires time.Time `json:"expires"`
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
	p.Waiting[slot.ID] = RelayWaiting{Call: call.ID, Reply: slot.Reply, Expires: p.Grant.Expires}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, slot.Put, bytes.NewReader(sealed))
	if err != nil {
		return "", err
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return slot.ID, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if err := statusErr(resp.StatusCode); err != nil {
		return slot.ID, err
	}
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
	if err := statusErr(resp.StatusCode); err != nil {
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

func statusErr(status int) error {
	switch {
	case status == http.StatusNotFound:
		return ErrNoObject
	case status == http.StatusForbidden:
		return ErrRelayLapsed
	case status >= 300:
		return fmt.Errorf("the store answered %d", status)
	}
	return nil
}

// Err is the error reply answers, as Client.CallID answers the same reply on the local network: an *Error with the status its code stands for, nil for a reply with a result.
func (r Reply) Err() error {
	if r.Error == nil {
		return nil
	}
	return &Error{Status: StatusOf(r.Error.Code), ErrorBody: *r.Error}
}
