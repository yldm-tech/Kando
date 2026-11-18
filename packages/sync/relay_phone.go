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

// RelayPhone is a phone's side of the storage relay (relay.go): it reads its state.json and sends calls through the slots of its grant, with plain HTTP requests to the grant's links. The phone keeps Device, Key, Grant, Spent and Seq between runs. It is not safe for concurrent use.
type RelayPhone struct {
	// Device is the phone's device ID (PairResponse.DeviceID), Key and Grant what it joined with (RelayJoin) or the newest grant since.
	Device string
	Key    []byte
	Grant  RelayGrant
	// Spent are the slots of Grant the phone has put a message in, Seq the number of its last message.
	Spent []string
	Seq   int64
	// HTTP is the client the links are requested with, nil for http.DefaultClient; Now the clock, nil for time.Now.
	HTTP *http.Client
	Now  func() time.Time
}

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

// State reads the phone's state.json, ErrNoObject while the desktop has not written one, and takes its grant when it is newer than the phone's, its slots all unspent.
func (p *RelayPhone) State(ctx context.Context) (RelayState, error) {
	sealed, err := p.get(ctx, p.Grant.State)
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
	p.Spent = append(p.Spent, slot.ID)
	p.Seq++
	plain, err := json.Marshal(RelayMessage{Seq: p.Seq, Sent: p.now(), Call: call})
	if err != nil {
		return "", err
	}
	sealed, err := SealRelay(p.Key, RelayInbox(p.Device, slot.ID), plain)
	if err != nil {
		return "", err
	}
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

// Reply reads the reply to the call in slot, false while the desktop has not answered it.
func (p *RelayPhone) Reply(ctx context.Context, slot string) (Reply, bool, error) {
	i := slices.IndexFunc(p.Grant.Slots, func(s RelaySlot) bool { return s.ID == slot })
	if i < 0 {
		return Reply{}, false, fmt.Errorf("no slot %s in the grant", slot)
	}
	sealed, err := p.get(ctx, p.Grant.Slots[i].Reply)
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
	return reply, true, nil
}

// get reads link, ErrNoObject for a 404.
func (p *RelayPhone) get(ctx context.Context, link string) ([]byte, error) {
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxRelayObject+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxRelayObject {
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
