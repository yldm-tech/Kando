package session

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	gosync "sync"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
)

// The storage relay (kando/packages/sync, relay.go; yldm-tech/Kando#219): a computer whose relay is on hands the phone, on its network, a key and links to the user's own storage (sync.OpRelayJoin). Away from that network, the phone follows the computer's queue by reading the state.json the computer writes there, and leaves its links and its actions on jobs there for the computer to answer, which it does within a minute or two while it runs. A link left there while the computer is off waits in the storage and starts once the computer is back. Back on the computer's network, the phone talks to it directly again, and a link left in the storage goes there too, under the same call ID, so the computer takes it once.

// lanTry is how long the phone waits for a computer on its network before reading its relay, when it has one: away from the network, a request to the computer's address may hang rather than fail.
const lanTry = 5 * time.Second

// relayLink is the phone's side of a computer's relay. Its fields are guarded by mu, as the follower and the sends use it from goroutines of their own.
type relayLink struct {
	mu gosync.Mutex
	// loaded is whether the store and the secure store were read for it; phone is the relay, nil while the phone has not joined; lapsed a grant whose links the storage refused.
	loaded bool
	phone  *sync.RelayPhone
	lapsed bool
	// sent counts the messages put in the storage, so a turn of the follower knows which of the calls the session waits for it saw sent (relayCall.gen).
	sent uint64
}

// relayCall is a call left in a computer's storage whose reply the session waits for: its computer, the busy key it holds ("" for a link), and the count of messages sent when it was (relayLink.sent).
type relayCall struct {
	desktop, busy string
	gen           uint64
}

// relayEvery is how often a computer's state.json is read away from its network.
func (s *Session) relayEvery() time.Duration { return cmp.Or(s.cfg.RelayEvery, sync.RelayPoll) }

// relayOf is desktop id's relay, made the first time it is asked for. Engine goroutine only.
func (s *Session) relayOf(id string) *relayLink {
	rl, ok := s.remote.relays[id]
	if !ok {
		rl = &relayLink{}
		s.remote.relays[id] = rl
	}
	return rl
}

// load reads desktop's relay from the store and its key from the secure store, the first time; a relay kept for another pairing with the computer (device) is left out. rl.mu is held.
func (rl *relayLink) load(s *Session, desktop, device string) {
	if rl.loaded {
		return
	}
	rl.loaded = true
	rec, ok, err := s.st.Relay(desktop)
	if !ok || err != nil || rec.Device != device {
		return
	}
	encoded, ok, err := s.secrets.Get(store.RelayKeyAccount(desktop))
	if !ok || err != nil {
		return
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != sync.RelayKeySize {
		return
	}
	rl.phone = &sync.RelayPhone{Device: rec.Device, Key: key, Grant: rec.Grant, Spent: rec.Spent, Seq: rec.Seq, Waiting: rec.Waiting, Now: s.cfg.Now}
	rl.lapsed = rec.Lapsed
}

// save keeps desktop's relay, without its key. rl.mu is held.
func (rl *relayLink) save(s *Session, desktop string) {
	p := rl.phone
	if p == nil {
		return
	}
	_ = s.st.SaveRelay(desktop, store.Relay{Device: p.Device, Grant: p.Grant, Spent: p.Spent, Seq: p.Seq, Waiting: p.Waiting, Lapsed: rl.lapsed})
}

// joined reports whether the phone holds links to desktop's relay that the storage did not refuse.
func (rl *relayLink) joined(s *Session, desktop, device string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.load(s, desktop, device)
	return rl.phone != nil && !rl.lapsed
}

// relayJoin joins desktop's relay through c, on its network, when the phone holds no links to it, the storage refused them, they are sync.RelayRegrant old or of another pairing (device); a computer that offers no relay (hello) has the phone forget the one it held. It runs on the follower's goroutine.
func (s *Session) relayJoin(ctx context.Context, desktop, device string, c *sync.Client, hello sync.Hello, rl *relayLink) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.load(s, desktop, device)
	if !slices.Contains(hello.Ops, sync.OpRelayJoin) {
		if rl.phone != nil {
			rl.phone = nil
			rl.forget(s, desktop)
		}
		return
	}
	// A relay of another pairing with the computer, as before the phone paired again, names folders the computer no longer answers.
	if p := rl.phone; p != nil && p.Device == device && !rl.lapsed && s.cfg.Now().Sub(p.Grant.Minted) < sync.RelayRegrant {
		return
	}
	var joined sync.RelayJoin
	if err := c.Call(ctx, sync.OpRelayJoin, nil, &joined); err != nil || len(joined.Key) != sync.RelayKeySize {
		return
	}
	if err := s.secrets.Set(store.RelayKeyAccount(desktop), base64.StdEncoding.EncodeToString(joined.Key)); err != nil {
		return
	}
	p := &sync.RelayPhone{Device: device, Key: joined.Key, Grant: joined.Grant, Now: s.cfg.Now}
	// The messages are numbered on from the last, and the replies to calls already sent are still read by their slots' links.
	if old := rl.phone; old != nil && old.Device == device {
		p.Seq, p.Waiting = old.Seq, old.Waiting
	}
	rl.phone, rl.lapsed = p, false
	rl.save(s, desktop)
}

// forget deletes what the phone keeps of desktop's relay. rl.mu is held.
func (rl *relayLink) forget(s *Session, desktop string) {
	_ = s.secrets.Delete(store.RelayKeyAccount(desktop))
	_ = s.st.DeleteRelay(desktop)
}

// forgetRelay forgets desktop id's relay, as it is unpaired. Engine goroutine only.
func (s *Session) forgetRelay(id string) {
	rl, ok := s.remote.relays[id]
	delete(s.remote.relays, id)
	if !ok {
		rl = &relayLink{loaded: true}
	}
	for call, rc := range s.remote.relayCalls {
		if rc.desktop == id {
			delete(s.remote.relayCalls, call)
		}
	}
	s.work.Go(func() {
		rl.mu.Lock()
		defer rl.mu.Unlock()
		rl.phone = nil
		rl.forget(s, id)
	})
}

// followRelay takes a turn of desktop's relay on the follower's goroutine, as its computer does not answer on its network: it reads the computer's state.json, then the replies to the calls left in the storage, and posts what it learned. It reports false when the phone holds no relay to the computer, the storage refused its links or cannot be reached, so the computer is taken to be out of reach as before.
func (s *Session) followRelay(ctx context.Context, desktop, device string, rl *relayLink, post func(func())) bool {
	rl.mu.Lock()
	rl.load(s, desktop, device)
	p := rl.phone
	if p == nil || rl.lapsed {
		rl.mu.Unlock()
		return false
	}
	state, err := p.State(ctx)
	if errors.Is(err, sync.ErrRelayLapsed) {
		rl.lapsed = true
		rl.save(s, desktop)
		rl.mu.Unlock()
		return false
	}
	// A state.json the computer has not written yet is a computer that has not run since the phone joined: the links still go into the storage.
	if err != nil && !errors.Is(err, sync.ErrNoObject) {
		rl.mu.Unlock()
		return false
	}
	var replies []sync.Reply
	for _, slot := range slices.Sorted(maps.Keys(p.Waiting)) {
		reply, ok, rerr := p.Reply(ctx, slot)
		switch {
		case ok:
			replies = append(replies, reply)
		case errors.Is(rerr, sync.ErrRelayLapsed):
			// A link that lapsed reaches the reply no more: its call goes again (relayReplies).
			delete(p.Waiting, slot)
		}
	}
	waiting := map[string]bool{}
	for _, w := range p.Waiting {
		waiting[w.Call] = true
	}
	gen := rl.sent
	rl.save(s, desktop)
	rl.mu.Unlock()
	written := err == nil
	fresh := written && s.cfg.Now().Sub(state.Written) < sync.RelayStale
	post(func() {
		s.setQueue(desktop, func(q Queue) Queue {
			q.Relayed = true
			q.Reach = Offline
			if fresh {
				q.Reach = Online
			}
			if written {
				// events.wait does not wait over the relay: the phone follows the changes by reading state.json.
				q.Ops = slices.DeleteFunc(slices.Clone(state.Hello.Ops), func(op string) bool { return op == sync.OpEvents })
				q.Jobs, q.Seen = state.Jobs.Jobs, state.Written
			}
			return q
		})
		s.relayReplies(desktop, replies, waiting, gen)
		s.sendWaiting(desktop)
	})
	return true
}

// relaySend leaves call in desktop's storage on a goroutine of its own, and tells done on the engine's goroutine whether it went, with the count of messages sent by then. A call that did not go is sent again by the follower's next turn.
func (s *Session) relaySend(desktop, device string, call sync.Call, done func(gen uint64, err error)) {
	rl := s.relayOf(desktop)
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.remote.ctx, actFor)
		defer cancel()
		rl.mu.Lock()
		rl.load(s, desktop, device)
		err := errors.New("the phone holds no relay to the computer")
		if rl.phone != nil && !rl.lapsed {
			var slot string
			slot, err = rl.phone.Send(ctx, call)
			if err != nil && slot != "" {
				// Not in the storage, it has no reply to wait for.
				delete(rl.phone.Waiting, slot)
			}
			if err == nil {
				rl.sent++
			}
			rl.save(s, desktop)
		}
		gen := rl.sent
		rl.mu.Unlock()
		s.cx.Post(func() { done(gen, err) })
	})
}

// relayCallOf is a call of op with args under id.
func relayCallOf(id, op string, args any) sync.Call {
	call := sync.Call{ID: id, V: sync.CallVersion, Op: op}
	if args != nil {
		call.Args, _ = json.Marshal(args)
	}
	return call
}

// relayLeave leaves link l in its computer's storage unless it is there or going already.
func (s *Session) relayLeave(l store.Link) {
	if l.Relayed || s.remote.sending[l.ID] {
		return
	}
	d, ok := s.Desktop(l.Desktop)
	if !ok {
		return
	}
	s.remote.sending[l.ID] = true
	s.relaySend(l.Desktop, d.DeviceID, relayCallOf(l.ID, sync.OpJobCreate, sync.JobCreate{Link: l.URL}), func(gen uint64, err error) {
		delete(s.remote.sending, l.ID)
		if err != nil {
			return
		}
		s.remote.relayCalls[l.ID] = relayCall{desktop: l.Desktop, gen: gen}
		s.setLinks(slices.Clone(s.remote.links.GetUntracked()), func(o *Outgoing) {
			if o.ID == l.ID {
				o.Relayed = true
			}
		})
	})
}

// relayAct leaves op with args in desktop's storage for its computer, holding busy key until the reply is read or the call is lost; a call that cannot go tells Failed why.
func (s *Session) relayAct(desktop, key, op string, args any) {
	d, ok := s.Desktop(desktop)
	if !ok || s.remote.busy.Get()[key] {
		return
	}
	s.setBusy(key, true)
	id := sync.NewCallID()
	s.relaySend(desktop, d.DeviceID, relayCallOf(id, op, args), func(gen uint64, err error) {
		if err != nil {
			s.setBusy(key, false)
			s.queueFailed(desktop, err)
			return
		}
		s.remote.relayCalls[id] = relayCall{desktop: desktop, busy: key, gen: gen}
	})
}

// relayLanded lets go of the calls left in desktop's storage as its computer answers on its network again: its queue is as it lists it there now, and the links left in the storage go to it directly, under the same IDs (sendWaiting), so it takes each once. Engine goroutine only.
func (s *Session) relayLanded(desktop string) {
	for id, rc := range s.remote.relayCalls {
		if rc.desktop == desktop {
			delete(s.remote.relayCalls, id)
			if rc.busy != "" {
				s.setBusy(rc.busy, false)
			}
		}
	}
}

// relayReplies takes the replies read from desktop's storage: a link taken leaves the phone or says why it was refused, a job acted on is as its reply says. A call left in the storage by the time of the turn (gen) that is no longer waited for, whose slot was used twice or whose link lapsed, frees what it held, and a link of it goes again. Engine goroutine only.
func (s *Session) relayReplies(desktop string, replies []sync.Reply, waiting map[string]bool, gen uint64) {
	for _, r := range replies {
		rc, ok := s.remote.relayCalls[r.ID]
		delete(s.remote.relayCalls, r.ID)
		if ok && rc.busy != "" {
			s.setBusy(rc.busy, false)
		}
		switch r.Op {
		case sync.OpJobCreate:
			i := slices.IndexFunc(s.remote.links.GetUntracked(), func(o Outgoing) bool { return o.ID == r.ID })
			if i < 0 {
				continue
			}
			var created sync.JobCreated
			err := r.Err()
			if err == nil {
				err = json.Unmarshal(r.Result, &created)
			}
			s.linkAnswered(s.remote.links.GetUntracked()[i].Link, created, err, true)
		case sync.OpJobCancel, sync.OpJobRetry, sync.OpJobPause, sync.OpJobResume:
			var job sync.Job
			err := r.Err()
			if err == nil {
				err = json.Unmarshal(r.Result, &job)
			}
			if err != nil {
				s.queueFailed(desktop, err)
				continue
			}
			s.upsertJobs(desktop, []sync.Job{job})
		case sync.OpUpdateYTDLP:
			if err := r.Err(); err != nil {
				s.queueFailed(desktop, err)
			}
		}
	}
	for id, rc := range s.remote.relayCalls {
		if rc.desktop == desktop && rc.gen <= gen && !waiting[id] {
			delete(s.remote.relayCalls, id)
			if rc.busy != "" {
				s.setBusy(rc.busy, false)
			}
		}
	}
	links := s.remote.links.GetUntracked()
	lost := func(o Outgoing) bool {
		_, sending := s.remote.relayCalls[o.ID]
		return o.Desktop == desktop && o.Relayed && !waiting[o.ID] && !sending
	}
	if slices.ContainsFunc(links, lost) {
		s.setLinks(slices.Clone(links), func(o *Outgoing) {
			if lost(*o) {
				o.Relayed = false
			}
		})
	}
}
