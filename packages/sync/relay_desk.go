package sync

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	gosync "sync"
	"time"
)

// RelayDevice is a phone that joined the relay: its device ID, its name as Caller names it, and its key.
type RelayDevice struct {
	ID, Name string
	Key      []byte
	// ReadOnly is the desktop user's read-only switch for the device (Caller.ReadOnly), read again with the device list at every pass.
	ReadOnly bool
}

// Relay is the desktop's side of the storage relay (relay.go): it answers the calls phones leave in their inboxes through Desk, and writes each phone's state.json. It is safe for concurrent use.
type Relay struct {
	Bucket RelayBucket
	// Prefix is RelayPrefix of the desktop.
	Prefix string
	Desk   *Desk
	// Devices are the phones that joined: paired, with a key. A message of any other device is deleted unread.
	Devices func() []RelayDevice
	// Now is the clock, nil for time.Now; Logf logs what failed, nil for nothing.
	Now  func() time.Time
	Logf func(format string, args ...any)
	// Every is how often Run polls, 0 for RelayPoll; MinWrite the least time between two of its turns, 0 for RelayMinWrite. Tests shorten them.
	Every, MinWrite time.Duration
	// Health, when set, is told after each of Run's turns what failed of its poll and its writes, nil for a turn that went through, so the desktop can say whether the store answers. It is called on Run's goroutine.
	Health func(err error)
	// Films, when set, is what the desktop put in the bucket for its phones (relay_films.go), listed in every phone's state.json with links to the files; nil, or a nil answer, lists none. It is called on Run's goroutine.
	Films func() *RelayFiles
	// LinksUntil, when set, is the latest a link signed now can stay good, as when the bucket's key is a short-lived one whose links lapse with it; zero, or a nil LinksUntil, is RelayURLLifetime from now. It may renew the key first, and is called on the goroutine that signs.
	LinksUntil func(ctx context.Context) (time.Time, error)

	// minting is held from looking at a phone's grant to storing a new one, so two turns (a join and Publish) never mint for the same phone at once.
	minting gosync.Mutex
	mu      gosync.Mutex
	grants  map[string]*relayGrant
	// written is what each phone's state.json last said, without when it was written, and when that was.
	written map[string]writtenState
	// wake has Run take its next turn at once.
	wake chan struct{}
	// filmLinks are the links to the films' files signed so far, by object name (shelf).
	filmLinks map[string]filmLink
	// publishing is held while state.json files are written, by Publish or Handoff, so a turn of Run never writes over the state.json a handoff wrote; handedOff is a Handoff that went through, after which this relay writes no state.json, so the phones keep the grant of links to the new bucket.
	publishing gosync.Mutex
	handedOff  bool
}

// relayGrant is a grant the desktop gave a phone, with the slots of it the phone has used.
type relayGrant struct {
	RelayGrant
	used map[string]bool
}

type writtenState struct {
	body []byte
	at   time.Time
}

func (r *Relay) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Relay) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

// Grant is device's newest grant, made anew when it has none, it is RelayRegrant old or half its links' life is spent (LinksUntil), or half its slots are used.
func (r *Relay) Grant(ctx context.Context, device string) (RelayGrant, error) {
	return r.grant(ctx, device, false)
}

// NewGrant makes device a new grant, as a join hands out: the phone may have spent slots of the one before.
func (r *Relay) NewGrant(ctx context.Context, device string) (RelayGrant, error) {
	return r.grant(ctx, device, true)
}

func (r *Relay) grant(ctx context.Context, device string, fresh bool) (RelayGrant, error) {
	r.minting.Lock()
	defer r.minting.Unlock()
	r.mu.Lock()
	g := r.grants[device]
	var current RelayGrant
	reuse := false
	if g != nil {
		current, reuse = g.RelayGrant, !fresh && r.now().Sub(g.Minted) < RelayRegrant && len(g.used) < RelaySlots/2 && !halfSpent(g.Minted, g.Expires, r.now())
	}
	r.mu.Unlock()
	if reuse {
		return current, nil
	}
	now := r.now()
	if !now.After(current.Minted) {
		// A phone keeps the grant minted last, so a new one is always minted after the one before, even within the clock's resolution.
		now = current.Minted.Add(time.Nanosecond)
	}
	minted, err := r.mint(ctx, device, now)
	if err != nil {
		return RelayGrant{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.grants == nil {
		r.grants = map[string]*relayGrant{}
	}
	r.grants[device] = &relayGrant{RelayGrant: minted, used: map[string]bool{}}
	return minted, nil
}

// halfSpent reports whether a link signed at minted that lapses at expires has lived half its life by now, after which it is signed anew: a grant or a film's link of RelayURLLifetime after RelayRegrant all the same, one cut short by LinksUntil sooner.
func halfSpent(minted, expires, now time.Time) bool {
	return !now.Before(minted.Add(expires.Sub(minted) / 2))
}

// linkLife is how long a link signed at now stays good: RelayURLLifetime, cut short by LinksUntil.
func (r *Relay) linkLife(ctx context.Context, now time.Time) (time.Duration, error) {
	if r.LinksUntil == nil {
		return RelayURLLifetime, nil
	}
	until, err := r.LinksUntil(ctx)
	if err != nil {
		return 0, err
	}
	if until.IsZero() {
		return RelayURLLifetime, nil
	}
	life := min(RelayURLLifetime, until.Sub(now))
	if life < time.Minute {
		// A key that lapses within the minute signs no link worth handing a phone: the bucket renews it before it signs again.
		return 0, ErrRelayKeyLapsing
	}
	return life, nil
}

// ErrRelayKeyLapsing is the error of signing a link with a short-lived key that lapses within the minute (LinksUntil).
var ErrRelayKeyLapsing = errors.New("the relay's key lapses too soon to sign a link")

// mint signs a new grant's links for device.
func (r *Relay) mint(ctx context.Context, device string, now time.Time) (RelayGrant, error) {
	life, err := r.linkLife(ctx, now)
	if err != nil {
		return RelayGrant{}, err
	}
	g := RelayGrant{Minted: now, Expires: now.Add(life)}
	if g.State, err = r.Bucket.PresignGet(ctx, r.Prefix+RelayStateName(device), life); err != nil {
		return RelayGrant{}, err
	}
	for range RelaySlots {
		s := RelaySlot{ID: newSlotID()}
		if s.Put, err = r.Bucket.PresignPut(ctx, r.Prefix+RelayInbox(device, s.ID), life); err != nil {
			return RelayGrant{}, err
		}
		if s.Reply, err = r.Bucket.PresignGet(ctx, r.Prefix+RelayReply(device, s.ID), life); err != nil {
			return RelayGrant{}, err
		}
		g.Slots = append(g.Slots, s)
	}
	return g, nil
}

// inboxed is a message found in an inbox, opened.
type inboxed struct {
	dev  RelayDevice
	slot string
	key  string
	msg  RelayMessage
}

// Poll answers the messages in the inboxes: in each phone's, in the order of their numbers, writing each one's reply before deleting it. A message of a device that did not join, one that does not open with its key or does not read as a message is deleted unanswered. It answers how many it answered; a message it could not answer or delete for the store's error stays for the next poll.
func (r *Relay) Poll(ctx context.Context) (int, error) {
	objects, err := r.Bucket.List(ctx, r.Prefix+"inbox/")
	if err != nil {
		return 0, err
	}
	devices := map[string]RelayDevice{}
	for _, d := range r.Devices() {
		devices[d.ID] = d
	}
	var found []inboxed
	var errs []error
	for _, o := range objects {
		device, slot, ok := strings.Cut(strings.TrimPrefix(o.Key, r.Prefix+"inbox/"), "/")
		dev, known := devices[device]
		if !ok || !known || !relaySlotID(slot) {
			errs = append(errs, r.Bucket.Delete(ctx, o.Key))
			continue
		}
		r.markUsed(device, slot)
		answered, err := r.Bucket.Exists(ctx, r.Prefix+RelayReply(device, slot))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if answered {
			errs = append(errs, r.Bucket.Delete(ctx, o.Key))
			continue
		}
		sealed, err := r.Bucket.Get(ctx, o.Key)
		if errors.Is(err, ErrNoObject) {
			continue
		}
		if errors.Is(err, ErrRelayTooLarge) {
			// A phone sends none this large (ErrRelayCallTooLarge); kept, it would be read at every poll for ever.
			r.logf("relay: a message of %s larger than %d bytes", device, MaxRelayObject)
			errs = append(errs, r.Bucket.Delete(ctx, o.Key))
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var msg RelayMessage
		plain, err := OpenRelay(dev.Key, RelayInbox(device, slot), sealed)
		if err == nil {
			err = json.Unmarshal(plain, &msg)
		}
		if err != nil {
			r.logf("relay: a message of %s that does not read: %v", device, err)
			errs = append(errs, r.Bucket.Delete(ctx, o.Key))
			continue
		}
		found = append(found, inboxed{dev: dev, slot: slot, key: o.Key, msg: msg})
	}
	slices.SortStableFunc(found, func(a, b inboxed) int {
		if c := strings.Compare(a.dev.ID, b.dev.ID); c != 0 {
			return c
		}
		return cmp.Compare(a.msg.Seq, b.msg.Seq)
	})
	n := 0
	for _, m := range found {
		if err := r.answer(ctx, m); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

// answer answers m, writes its reply and deletes it.
func (r *Relay) answer(ctx context.Context, m inboxed) error {
	call := m.msg.Call
	reply := r.Desk.Answer(ctx, Caller{DeviceID: m.dev.ID, Name: m.dev.Name, ReadOnly: m.dev.ReadOnly, Relay: true}, call)
	if ctx.Err() != nil {
		// A call cut short by the desktop's stopping is answered at the next poll.
		return ctx.Err()
	}
	plain, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	name := RelayReply(m.dev.ID, m.slot)
	sealed, err := SealRelay(m.dev.Key, name, plain)
	if err != nil {
		return err
	}
	if len(sealed) > MaxRelayReply {
		// The phone would not read it, and would wait for it for ever: it hears why instead.
		plain, err = json.Marshal(Reply{ID: call.ID, Op: call.Op, Error: &ErrorBody{Code: CodeInvalid, Message: "the reply is too long for the relay"}})
		if err != nil {
			return err
		}
		if sealed, err = SealRelay(m.dev.Key, name, plain); err != nil {
			return err
		}
	}
	if err := r.Bucket.Put(ctx, r.Prefix+name, sealed); err != nil {
		return err
	}
	return r.Bucket.Delete(ctx, m.key)
}

// markUsed notes that device used slot of its grant.
func (r *Relay) markUsed(device, slot string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g := r.grants[device]; g != nil && slices.ContainsFunc(g.Slots, func(s RelaySlot) bool { return s.ID == slot }) {
		g.used[slot] = true
	}
}

// Publish writes the state.json of each phone that joined whose state changed, whose grant is new, or whose state.json is RelayHeartbeat old.
func (r *Relay) Publish(ctx context.Context) error {
	r.publishing.Lock()
	defer r.publishing.Unlock()
	if r.handedOff {
		return nil
	}
	devices := r.Devices()
	if len(devices) == 0 {
		return nil
	}
	shelf, err := r.shelf(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, dev := range devices {
		errs = append(errs, r.publish(ctx, dev, shelf))
	}
	return errors.Join(errs...)
}

func (r *Relay) publish(ctx context.Context, dev RelayDevice, shelf *RelayShelf) error {
	grant, err := r.Grant(ctx, dev.ID)
	if err != nil {
		return err
	}
	return r.writeState(ctx, dev, grant, shelf, false)
}

// writeState writes dev's state.json with grant and shelf: when force, or when it says something else than the last one written, or that one is RelayHeartbeat old.
func (r *Relay) writeState(ctx context.Context, dev RelayDevice, grant RelayGrant, shelf *RelayShelf, force bool) error {
	caller := Caller{DeviceID: dev.ID, Name: dev.Name, ReadOnly: dev.ReadOnly, Relay: true}
	state := RelayState{V: RelayVersion, Desktop: r.Desk.Desktop, Jobs: r.Desk.Journal.List(), Learning: r.Desk.Journal.Learning(), Grant: grant, Shelf: shelf}
	hello := r.Desk.Answer(ctx, caller, Call{V: CallVersion, Op: OpHello})
	if hello.Error != nil {
		return fmt.Errorf("hello: %s", hello.Error.Message)
	}
	if err := json.Unmarshal(hello.Result, &state.Hello); err != nil {
		return err
	}
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	now := r.now()
	r.mu.Lock()
	last := r.written[dev.ID]
	r.mu.Unlock()
	if !force && bytes.Equal(body, last.body) && now.Sub(last.at) < RelayHeartbeat {
		return nil
	}
	state.Written = now
	plain, err := json.Marshal(state)
	if err != nil {
		return err
	}
	name := RelayStateName(dev.ID)
	sealed, err := SealRelay(dev.Key, name, plain)
	if err != nil {
		return err
	}
	if len(sealed) > MaxRelayReply {
		return fmt.Errorf("the state of %s is %d bytes, more than a phone reads", dev.ID, len(sealed))
	}
	if err := r.Bucket.Put(ctx, r.Prefix+name, sealed); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.written == nil {
		r.written = map[string]writtenState{}
	}
	r.written[dev.ID] = writtenState{body: body, at: now}
	return nil
}

// Forget deletes device's inbox and outbox and its grant, as when it is unpaired: its links reach nothing from then on.
func (r *Relay) Forget(ctx context.Context, device string) error {
	r.mu.Lock()
	delete(r.grants, device)
	delete(r.written, device)
	r.mu.Unlock()
	var errs []error
	for _, folder := range []string{"inbox/", "outbox/"} {
		objects, err := r.Bucket.List(ctx, r.Prefix+folder+device+"/")
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, o := range objects {
			errs = append(errs, r.Bucket.Delete(ctx, o.Key))
		}
	}
	return errors.Join(errs...)
}

// RelaySweepEvery is how often Run sweeps the outboxes.
const RelaySweepEvery = time.Hour

// Sweep deletes the replies older than ReplyMemory, which a phone has read or given up on, the inbox and outbox of any device that has not joined, as one unpaired while the relay was off, and the pairing mailboxes RelayPairKeep old.
func (r *Relay) Sweep(ctx context.Context) error {
	known := map[string]bool{}
	for _, d := range r.Devices() {
		known[d.ID] = true
	}
	now := r.now()
	var errs []error
	for _, folder := range []string{"inbox/", "outbox/"} {
		objects, err := r.Bucket.List(ctx, r.Prefix+folder)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, o := range objects {
			device, name, _ := strings.Cut(strings.TrimPrefix(o.Key, r.Prefix+folder), "/")
			oldReply := folder == "outbox/" && relaySlotID(name) && now.Sub(o.Modified) >= ReplyMemory
			if !known[device] || oldReply {
				errs = append(errs, r.Bucket.Delete(ctx, o.Key))
			}
		}
	}
	// A pairing mailbox (relay_pair.go) is deleted once its code ends; one left behind, as when the desktop stopped first, goes once it is RelayPairKeep old.
	mailboxes, err := r.Bucket.List(ctx, r.Prefix+relayPairFolder)
	if err != nil {
		errs = append(errs, err)
	}
	for _, o := range mailboxes {
		if now.Sub(o.Modified) >= RelayPairKeep {
			errs = append(errs, r.Bucket.Delete(ctx, o.Key))
		}
	}
	return errors.Join(errs...)
}

// Wake has Run take its next turn now rather than at the next poll, as when a phone joined and waits for its state.json.
func (r *Relay) Wake() {
	select {
	case r.wakeChan() <- struct{}{}:
	default:
	}
}

func (r *Relay) wakeChan() chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.wake == nil {
		r.wake = make(chan struct{}, 1)
	}
	return r.wake
}

// Run polls the inboxes every RelayPoll, and writes the phones' state.json as the jobs or the learning state change (at most every RelayMinWrite) and every RelayHeartbeat, until ctx ends. What fails is logged and tried again at the next turn.
func (r *Relay) Run(ctx context.Context) {
	every, minWrite := cmp.Or(r.Every, RelayPoll), cmp.Or(r.MinWrite, RelayMinWrite)
	wake := r.wakeChan()
	var swept time.Time
	for ctx.Err() == nil {
		began := time.Now()
		_, pollErr := r.Poll(ctx)
		if pollErr != nil && ctx.Err() == nil {
			r.logf("relay: inbox: %v", pollErr)
		}
		pubErr := r.Publish(ctx)
		if pubErr != nil && ctx.Err() == nil {
			r.logf("relay: state: %v", pubErr)
		}
		if r.Health != nil && ctx.Err() == nil {
			r.Health(errors.Join(pollErr, pubErr))
		}
		if r.now().Sub(swept) >= RelaySweepEvery {
			if err := r.Sweep(ctx); err != nil && ctx.Err() == nil {
				r.logf("relay: sweep: %v", err)
			}
			swept = r.now()
		}
		// The turn ends at the next change of the jobs or the learning state, a Wake, or the next poll, whichever comes first, but no sooner than minWrite after it began.
		list := r.Desk.Journal.List()
		waiting, stop := context.WithCancel(ctx)
		go func() {
			select {
			case <-wake:
				stop()
			case <-waiting.Done():
			}
		}()
		r.Desk.Journal.Wait(waiting, list.Epoch, list.Rev, every)
		stop()
		// began is by the wall clock, as the timers are, whatever clock Now is.
		if rest := minWrite - time.Since(began); rest > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(rest):
			}
		}
	}
}
