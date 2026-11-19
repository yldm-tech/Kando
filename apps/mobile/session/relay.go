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

// The storage relay (kando/packages/sync, relay.go; yldm-tech/Kando#219): a computer whose relay is on hands the phone, on its network, a key and links to the user's own storage (sync.OpRelayJoin). Away from that network, the phone follows the computer's queue by reading the state.json the computer writes there, and leaves its links, its actions on jobs, its changes of the learning state and the settings, and its actions on subscriptions there for the computer to answer, which it does within a minute or two while it runs (relay_study.go for the learning state, settings and subscriptions). A link left there while the computer is off waits in the storage and starts once the computer is back. Back on the computer's network, the phone talks to it directly again, and a link left in the storage goes there too, under the same call ID, so the computer takes it once. When the storage itself refuses the phone or cannot be reached, the queue says why (Queue.Storage), in the words the computer's Phone Sync page uses for the same failure.

// lanTry is how long the phone waits for a computer on its network before reading its relay, when it has one: away from the network, a request to the computer's address may hang rather than fail.
const lanTry = 5 * time.Second

// relayLink is the phone's side of a computer's relay. Its fields are guarded by mu, as the follower and the sends use it from goroutines of their own.
type relayLink struct {
	mu gosync.Mutex
	// loaded is whether the store and the secure store were read for it; phone is the relay, nil while the phone has not joined; lapsed a grant whose links the storage refused, and failure why.
	loaded  bool
	phone   *sync.RelayPhone
	lapsed  bool
	failure sync.RelayFailure
	// sent counts the messages put in the storage, so a turn of the follower knows which of the calls the session waits for it saw sent (relayCall.gen).
	sent uint64
}

// relayCall is a call left in a computer's storage whose reply the session waits for: its computer, the busy key it holds ("" for none), the count of messages sent when it was (relayLink.sent), and done, when not nil, which takes its reply on the engine's goroutine, or errRelayLost or errUnpaired when none will come (relayAsk).
type relayCall struct {
	desktop, busy string
	gen           uint64
	done          func(sync.Reply, error)
}

// relayAnswer is what came of a call left in the storage: its reply, or err when none will come.
type relayAnswer struct {
	reply sync.Reply
	err   error
}

// errRelayLost is the error of a call left in a computer's storage whose reply will not come through it: its slot was used twice or its link lapsed, or the phone is back on the computer's network.
var errRelayLost = errors.New("the call left in the storage was not answered")

// errNoRelay is the error of a call the phone cannot leave in a computer's storage: it holds no links to it that the storage did not refuse.
var errNoRelay = errors.New("the phone holds no relay to the computer")

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
	rl.lapsed, rl.failure = rec.Lapsed, rec.Failure
	if rl.lapsed && rl.failure == sync.RelayNoFailure {
		// Kept by a phone that did not keep why.
		rl.failure = sync.RelayLapsed
	}
}

// save keeps desktop's relay, without its key. rl.mu is held.
func (rl *relayLink) save(s *Session, desktop string) {
	p := rl.phone
	if p == nil {
		return
	}
	_ = s.st.SaveRelay(desktop, store.Relay{Device: p.Device, Grant: p.Grant, Spent: p.Spent, Seq: p.Seq, Waiting: p.Waiting, Lapsed: rl.lapsed, Failure: rl.failure})
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
	rl.phone, rl.lapsed, rl.failure = p, false, sync.RelayNoFailure
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
	delete(s.remote.relayStudies, id)
	// Once the computer is gone from the list, so what waited for its replies takes it as unpaired.
	s.cx.Post(func() {
		s.relayEnd(func(_ string, rc relayCall) bool { return rc.desktop == id }, errUnpaired)
	})
	s.work.Go(func() {
		rl.mu.Lock()
		defer rl.mu.Unlock()
		rl.phone = nil
		rl.forget(s, id)
	})
}

// followRelay takes a turn of desktop's relay on the follower's goroutine, as its computer does not answer on its network: it reads the computer's state.json, then the replies to the calls left in the storage, and posts what it learned. It reports false when the phone holds no relay to the computer, the storage refused its links or cannot be reached, so the computer is taken to be out of reach as before, with why the storage failed (Queue.Storage).
func (s *Session) followRelay(ctx context.Context, desktop, device string, rl *relayLink, post func(func())) bool {
	rl.mu.Lock()
	rl.load(s, desktop, device)
	p := rl.phone
	if p == nil {
		rl.mu.Unlock()
		return false
	}
	if rl.lapsed {
		failure := rl.failure
		rl.mu.Unlock()
		post(func() { s.setStorage(desktop, failure) })
		return false
	}
	state, err := p.State(ctx)
	if errors.Is(err, sync.ErrRelayLapsed) {
		rl.lapsed, rl.failure = true, p.FailureOf(err)
		failure := rl.failure
		rl.save(s, desktop)
		rl.mu.Unlock()
		post(func() { s.setStorage(desktop, failure) })
		return false
	}
	// A state.json the computer has not written yet is a computer that has not run since the phone joined: the links still go into the storage.
	if err != nil && !errors.Is(err, sync.ErrNoObject) {
		failure := p.FailureOf(err)
		rl.mu.Unlock()
		if failure != sync.RelayNoFailure {
			post(func() { s.setStorage(desktop, failure) })
		}
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
			// The storage answered: a store that was full stays so until it takes a call again (relaySend).
			if q.Storage != sync.RelayFull {
				q.Storage = sync.RelayNoFailure
			}
			if written {
				// events.wait does not wait over the relay: the phone follows the changes by reading state.json.
				q.Ops = slices.DeleteFunc(slices.Clone(state.Hello.Ops), func(op string) bool { return op == sync.OpEvents })
				q.ReadOnly = state.Hello.ReadOnly
				q.Jobs, q.Seen = state.Jobs.Jobs, state.Written
			}
			return q
		})
		if written {
			s.relayFollowStudy(desktop, state)
		}
		s.relayReplies(desktop, replies, waiting, gen)
		s.sendWaiting(desktop)
		if written {
			s.relayCatchUp(desktop)
		}
	})
	return true
}

// setStorage takes failure as why desktop's storage last failed the phone, sync.RelayNoFailure once it served it; a failure has the computer taken as followed through its storage, so its card says why the storage failed rather than only that the computer is out of reach. Engine goroutine only.
func (s *Session) setStorage(desktop string, failure sync.RelayFailure) {
	q := s.remote.queues.GetUntracked()[desktop]
	if q.Storage == failure && (failure == sync.RelayNoFailure || q.Relayed) {
		return
	}
	s.setQueue(desktop, func(q Queue) Queue {
		q.Storage = failure
		if failure != sync.RelayNoFailure {
			q.Relayed = true
		}
		return q
	})
}

// relaySend leaves calls in desktop's storage, in their order, on a goroutine of its own, and tells done on the engine's goroutine how many of them went, the count of messages sent by then, and why the rest did not go. A call that did not go is sent again by whoever sent it, as the follower's next turn does for a link. What the storage answered is told to the queue (setStorage): a store that refuses the links has the phone join again on the computer's network.
func (s *Session) relaySend(desktop, device string, calls []sync.Call, done func(gen uint64, sent int, err error)) {
	rl := s.relayOf(desktop)
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.remote.ctx, actFor)
		defer cancel()
		rl.mu.Lock()
		defer rl.mu.Unlock()
		rl.load(s, desktop, device)
		sent, err := 0, errNoRelay
		failure, served := sync.RelayNoFailure, false
		if rl.phone != nil && !rl.lapsed {
			err = nil
			for _, call := range calls {
				var slot string
				slot, err = rl.phone.Send(ctx, call)
				if err != nil {
					if slot != "" {
						// Not in the storage, it has no reply to wait for.
						delete(rl.phone.Waiting, slot)
					}
					break
				}
				rl.sent++
				sent++
			}
			failure, served = rl.phone.FailureOf(err), sent > 0
			if errors.Is(err, sync.ErrRelayLapsed) {
				rl.lapsed, rl.failure = true, failure
			}
			rl.save(s, desktop)
		}
		gen := rl.sent
		// Posted with rl.mu held, so a turn of the follower that reads a reply to one of these calls posts what it read after done has had the calls waited for.
		s.cx.Post(func() {
			switch {
			case failure != sync.RelayNoFailure:
				s.setStorage(desktop, failure)
			case served:
				s.setStorage(desktop, sync.RelayNoFailure)
			}
			done(gen, sent, err)
		})
	})
}

// relayAsk leaves calls in desktop's storage, in their order, and tells done on the engine's goroutine, once each call that went is answered or lost, what came of each by call ID, and why the calls not among them did not go. Engine goroutine only.
func (s *Session) relayAsk(desktop string, calls []sync.Call, done func(answers map[string]relayAnswer, err error)) {
	d, ok := s.Desktop(desktop)
	if !ok {
		done(nil, errUnpaired)
		return
	}
	s.relaySend(desktop, d.DeviceID, calls, func(gen uint64, sent int, err error) {
		answers := map[string]relayAnswer{}
		left := sent
		if left == 0 {
			done(answers, err)
			return
		}
		for _, call := range calls[:sent] {
			s.remote.relayCalls[call.ID] = relayCall{desktop: desktop, gen: gen, done: func(r sync.Reply, rerr error) {
				answers[call.ID] = relayAnswer{reply: r, err: rerr}
				left--
				if left == 0 {
					done(answers, err)
				}
			}}
		}
	})
}

// relayEnd ends the calls left in the storage that which picks out, as their replies will not come through it: what each held is let go, and done told err. Engine goroutine only.
func (s *Session) relayEnd(which func(id string, rc relayCall) bool, err error) {
	var ended []relayCall
	for id, rc := range s.remote.relayCalls {
		if which(id, rc) {
			delete(s.remote.relayCalls, id)
			ended = append(ended, rc)
		}
	}
	for _, rc := range ended {
		if rc.busy != "" {
			s.setBusy(rc.busy, false)
		}
		if rc.done != nil {
			rc.done(sync.Reply{}, err)
		}
	}
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
	s.relaySend(l.Desktop, d.DeviceID, []sync.Call{relayCallOf(l.ID, sync.OpJobCreate, sync.JobCreate{Link: l.URL})}, func(gen uint64, sent int, err error) {
		delete(s.remote.sending, l.ID)
		if sent == 0 {
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
	s.relaySend(desktop, d.DeviceID, []sync.Call{relayCallOf(id, op, args)}, func(gen uint64, sent int, err error) {
		if sent == 0 {
			s.setBusy(key, false)
			s.queueFailed(desktop, err)
			return
		}
		s.remote.relayCalls[id] = relayCall{desktop: desktop, busy: key, gen: gen}
	})
}

// relayLanded lets go of the calls left in desktop's storage as its computer answers on its network again: its queue is as it lists it there now, the links left in the storage go to it directly, under the same IDs (sendWaiting), so it takes each once, and the learning state, the settings and the subscriptions waiting for a reply there are asked for again on the network by those who wait (errRelayLost). Engine goroutine only.
func (s *Session) relayLanded(desktop string) {
	if rs := s.remote.relayStudies[desktop]; rs != nil {
		// Read on the network from now on, and through the storage again once the phone is away again.
		rs.listed = false
	}
	s.relayEnd(func(_ string, rc relayCall) bool { return rc.desktop == desktop }, errRelayLost)
}

// relayReplies takes the replies read from desktop's storage: a link taken leaves the phone or says why it was refused, a job acted on is as its reply says. A call left in the storage by the time of the turn (gen) that is no longer waited for, whose slot was used twice or whose link lapsed, frees what it held, and a link of it goes again. Engine goroutine only.
func (s *Session) relayReplies(desktop string, replies []sync.Reply, waiting map[string]bool, gen uint64) {
	for _, r := range replies {
		rc, ok := s.remote.relayCalls[r.ID]
		delete(s.remote.relayCalls, r.ID)
		if ok && rc.busy != "" {
			s.setBusy(rc.busy, false)
		}
		if ok && rc.done != nil {
			rc.done(r, nil)
			continue
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
	s.relayEnd(func(id string, rc relayCall) bool { return rc.desktop == desktop && rc.gen <= gen && !waiting[id] }, errRelayLost)
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
