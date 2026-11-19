package session

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"

	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// The computers' subscriptions over protocol v2 (kando/packages/sync, subscriptions.go): the session reads each paired computer's as it reads its shelf (RefreshDesktop), so the Library lists them, and turns a subscription's auto-download on or off, sends its waiting videos to the queue, unsubscribes and subscribes to a list link through the computer, each answered with the subscription as it is after. The checks stay the computer's; what they found shows at the next read. Away from the computer's network the reads and the actions go through its storage (relay.go), where the computer answers them within a minute or two, and the list is read there as the phone begins following it there.

// Subscription is a paired computer's subscription.
type Subscription struct {
	Desktop string
	sync.Subscription
}

// Key names the subscription among every computer's.
func (s Subscription) Key() string { return store.Key(s.Desktop, s.ID) }

// subscriptions are the session's part that reads and changes the computers' subscriptions. Its fields belong to the engine's goroutine.
type subscriptions struct {
	// lists are what each computer last listed, by its ID; a computer that does not share its subscriptions lists none.
	lists *core.Signal[map[string][]sync.Subscription]
	// busy are the subscriptions an action is under way for, by Key, and the computers subscribing to a link, by subscribingKey.
	busy *core.Signal[map[string]bool]
	// reads are the reads under way, by computer, each told from a later one by its generation.
	reads map[string]subsRead
	gen   uint64
}

// subsRead is a read of a computer's subscriptions under way, relayed when it waits in the computer's storage.
type subsRead struct {
	listing
	relayed bool
}

func newSubscriptions(cx *core.Scope) *subscriptions {
	return &subscriptions{
		lists: core.NewSignalFunc(cx, map[string][]sync.Subscription{}, nil),
		busy:  core.NewSignalFunc(cx, map[string]bool{}, nil),
		reads: map[string]subsRead{},
	}
}

// subscribingKey stands for a computer subscribing to a link among the busy subscriptions.
const subscribingKey = "\x00subscribe"

// Subscriptions are the paired computers' subscriptions, each computer's in its order, the computers in the order they were paired.
func (s *Session) Subscriptions() []Subscription {
	lists := s.subs.lists.Get()
	var out []Subscription
	for _, d := range s.desktops.Get() {
		for _, sub := range lists[d.ID] {
			out = append(out, Subscription{Desktop: d.ID, Subscription: sub})
		}
	}
	return out
}

// Subscription is the subscription Key names, false once it is gone.
func (s *Session) Subscription(key string) (Subscription, bool) {
	for _, sub := range s.Subscriptions() {
		if sub.Key() == key {
			return sub, true
		}
	}
	return Subscription{}, false
}

// SubscriptionBusy reports whether an action on sub is under way.
func (s *Session) SubscriptionBusy(sub Subscription) bool { return s.subs.busy.Get()[sub.Key()] }

// Subscribing reports whether desktop was sent a link to subscribe to and has not answered yet.
func (s *Session) Subscribing(desktop string) bool {
	return s.subs.busy.Get()[store.Key(desktop, subscribingKey)]
}

// readSubscriptions reads computer id's subscriptions again, in place of a read under way, keeping what it last listed until the answer comes. A computer that does not share them (an older Kando, which answers unsupported or has no v2 call) lists none; one that cannot be reached keeps what it listed, as its shelf says why.
func (s *Session) readSubscriptions(id string) {
	c := s.Client(id)
	if c == nil {
		return
	}
	relayed := s.relayed(id)
	if r, running := s.subs.reads[id]; running {
		if relayed && r.relayed {
			// One read waits in the storage already, and answers what it lists then.
			return
		}
		r.cancel()
	}
	s.subs.gen++
	gen := s.subs.gen
	answered := func(got sync.SubscriptionList, err error) {
		if cur, ok := s.subs.reads[id]; !ok || cur.gen != gen {
			return
		}
		delete(s.subs.reads, id)
		var e *sync.Error
		switch {
		case err == nil:
			s.setSubscriptions(id, got.Subscriptions)
		case errors.As(err, &e) && (e.Code == sync.CodeUnsupported || e.Status == http.StatusNotFound):
			s.setSubscriptions(id, nil)
		}
	}
	if relayed {
		// Through the storage, from a computer that lists them there: an older one keeps what it listed on its network.
		if !slices.Contains(s.remote.queues.GetUntracked()[id].Ops, sync.OpSubscriptions) {
			return
		}
		s.subs.reads[id] = subsRead{listing: listing{cancel: func() {}, gen: gen}, relayed: true}
		relayCallResult(s, id, sync.OpSubscriptions, nil, answered)
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, listFor)
	s.subs.reads[id] = subsRead{listing: listing{cancel: cancel, gen: gen}}
	s.work.Go(func() {
		defer cancel()
		got, err := c.Subscriptions(ctx)
		s.cx.Post(func() { answered(got, err) })
	})
}

// forgetSubscriptions stops reading computer id's subscriptions and drops them, as it is unpaired or paired again.
func (s *Session) forgetSubscriptions(id string) {
	if r, running := s.subs.reads[id]; running {
		r.cancel()
		delete(s.subs.reads, id)
	}
	s.setSubscriptions(id, nil)
}

func (s *Session) setSubscriptions(id string, list []sync.Subscription) {
	m := maps.Clone(s.subs.lists.GetUntracked())
	if list == nil {
		delete(m, id)
	} else {
		m[id] = list
	}
	s.subs.lists.Set(m)
}

// putSubscription puts sub, as the computer answered it, in place of the one with its ID on desktop's list, or last.
func (s *Session) putSubscription(desktop string, sub sync.Subscription) {
	list := slices.Clone(s.subs.lists.GetUntracked()[desktop])
	if i := slices.IndexFunc(list, func(x sync.Subscription) bool { return x.ID == sub.ID }); i >= 0 {
		list[i] = sub
	} else {
		list = append(list, sub)
	}
	s.setSubscriptions(desktop, list)
}

func (s *Session) setSubscriptionBusy(key string, on bool) {
	m := maps.Clone(s.subs.busy.GetUntracked())
	if on {
		m[key] = true
	} else {
		delete(m, key)
	}
	s.subs.busy.Set(m)
}

// actOnSubscription runs op with args on sub's computer, off the engine's goroutine, while sub is busy: the subscription it answers takes sub's place, and one unsubscribed leaves the list; an error is told as a queue's action that failed (QueueEvents.Failed), after which a computer that answered is read again. A subscription the computer no longer has leaves the list. Away from the computer's network the call goes through its storage, and sub stays busy until the computer answered there.
func (s *Session) actOnSubscription(sub Subscription, op string, args any) {
	c := s.Client(sub.Desktop)
	key := sub.Key()
	if c == nil || s.subs.busy.GetUntracked()[key] {
		return
	}
	s.setSubscriptionBusy(key, true)
	answered := func(got sync.Subscription, err error) {
		s.setSubscriptionBusy(key, false)
		if _, paired := s.Desktop(sub.Desktop); !paired {
			// Unpaired while the call waited in its storage.
			return
		}
		var e *sync.Error
		switch {
		case errors.As(err, &e) && e.Code == sync.CodeNotFound:
			s.dropSubscription(sub.Desktop, sub.ID)
			s.queueFailed(sub.Desktop, err)
		case err != nil:
			s.queueFailed(sub.Desktop, err)
			// A computer that did part of it, as queueing some of the waiting videos and not the others, is read again for what it did.
			if errors.As(err, &e) {
				s.readSubscriptions(sub.Desktop)
			}
		case op == sync.OpUnsubscribe:
			s.dropSubscription(sub.Desktop, sub.ID)
		case got.ID != "":
			s.putSubscription(sub.Desktop, got)
		}
	}
	if s.relayed(sub.Desktop) {
		relayCallResult(s, sub.Desktop, op, args, answered)
		return
	}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, actFor)
		defer cancel()
		var got sync.Subscription
		err := c.Call(ctx, op, args, &got)
		s.cx.Post(func() { answered(got, err) })
	})
}

// dropSubscription takes subscription id off desktop's list.
func (s *Session) dropSubscription(desktop, id string) {
	if list, listed := s.subs.lists.GetUntracked()[desktop]; listed {
		s.setSubscriptions(desktop, slices.DeleteFunc(slices.Clone(list), func(x sync.Subscription) bool { return x.ID == id }))
	}
}

// SetSubscriptionAuto turns sub's auto-download on or off on its computer.
func (s *Session) SetSubscriptionAuto(sub Subscription, auto bool) {
	s.actOnSubscription(sub, sync.OpSubscriptionSet, sync.SubscriptionSet{ID: sub.ID, Auto: &auto})
}

// TakeSubscription sends every video waiting on sub to its computer's queue.
func (s *Session) TakeSubscription(sub Subscription) {
	s.actOnSubscription(sub, sync.OpSubscriptionTake, sync.SubscriptionTake{ID: sub.ID})
}

// Unsubscribe removes sub from its computer, which keeps the videos it added.
func (s *Session) Unsubscribe(sub Subscription) {
	s.actOnSubscription(sub, sync.OpUnsubscribe, sync.SubscriptionRef{ID: sub.ID})
}

// Subscribe subscribes desktop's open library to the channel, space, collection or playlist at link. The computer reads the whole list before it answers (sync.SubscribeWait); its answer is told through QueueEvents.Subscribed, and a failure through Failed. Away from the computer's network the link goes through its storage, and the computer subscribes once it answered there.
func (s *Session) Subscribe(desktop, link string) {
	c := s.Client(desktop)
	key := store.Key(desktop, subscribingKey)
	if c == nil || s.subs.busy.GetUntracked()[key] {
		return
	}
	s.setSubscriptionBusy(key, true)
	answered := func(got sync.Subscribed, err error) {
		s.setSubscriptionBusy(key, false)
		if _, paired := s.Desktop(desktop); !paired {
			// Unpaired while the link waited in its storage.
			return
		}
		if err != nil {
			s.queueFailed(desktop, err)
			return
		}
		s.putSubscription(desktop, got.Subscription)
		if s.remote.events.Subscribed != nil {
			s.remote.events.Subscribed(desktop, got)
		}
	}
	if s.relayed(desktop) {
		relayCallResult(s, desktop, sync.OpSubscribe, sync.SubscriptionAdd{Link: link}, answered)
		return
	}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, sync.SubscribeWait)
		defer cancel()
		got, err := c.Subscribe(ctx, link)
		s.cx.Post(func() { answered(got, err) })
	})
}
