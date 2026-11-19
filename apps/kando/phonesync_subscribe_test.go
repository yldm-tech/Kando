package main

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/apps/kando/subscribe"
	"kando/packages/core"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// A phone subscribes the open library to a channel, sees the videos a check held for the user, turns the subscription's auto-download off and pauses it, sends one waiting video to the queue and unsubscribes; the page's list of what the phones did says so. A video link is not a list, and a subscription of a locked library is neither listed nor acted on.
func TestPhoneSyncSharesTheSubscriptions(t *testing.T) {
	a, tt, updates, dir, c := phonePaired(t)
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubOld}}})
	a.pipeline = p
	// The library's queued clip would run once the video the phone queues is stopped as the test ends, and block the queue in its turn.
	a.library.Jobs = slices.DeleteFunc(a.library.Jobs, func(j core.Job) bool { return j.ID == "clip" })
	ask := func(call func(ctx context.Context) (any, error)) (any, error) {
		return phoneCall(t, a, tt, updates, call)
	}
	list := func() []psync.Subscription {
		t.Helper()
		got, err := phoneCall(t, a, tt, updates, c.Subscriptions)
		if err != nil {
			t.Fatal(err)
		}
		return got.Subscriptions
	}
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || !slices.Contains(h.Ops, psync.OpSubscriptions) || !slices.Contains(h.Ops, psync.OpUnsubscribe) {
		t.Fatalf("hello %+v, %v; want the subscriptions among the operations", h, err)
	}
	if subs := list(); subs == nil || len(subs) != 0 {
		t.Fatalf("subscriptions %+v, want none", subs)
	}
	_, err = ask(func(ctx context.Context) (any, error) { return c.Subscribe(ctx, testSubOld.URL) })
	phoneWantCode(t, err, http.StatusBadRequest, psync.CodeInvalid)

	got, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Subscribed, error) { return c.Subscribe(ctx, testSubChannel) })
	if err != nil {
		t.Fatal(err)
	}
	sub := got.Subscription
	if got.Existing || sub.Name != "Earth Science" || sub.Library != phoneLibraryID(dir) || sub.Kind != psync.SubscriptionChannel || sub.Platform != psync.PlatformYouTube || !sub.Auto || sub.AddPerCheck != subscribe.AddPerCheck || sub.Waiting == nil {
		t.Fatalf("subscribed %+v", got)
	}
	if nextCall(t, p).source != testSubChannel || len(a.subs.store.Subs) != 1 {
		t.Fatalf("subscriptions %+v", a.subs.store.Subs)
	}
	if act := a.phone.activity; len(act) == 0 || act[0].Kind != settings.ActivitySubscribed || act[0].Title != "Earth Science" {
		t.Fatalf("activity %+v", act)
	}
	again, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Subscribed, error) { return c.Subscribe(ctx, testSubChannel) })
	if err != nil || !again.Existing || again.Subscription.ID != sub.ID {
		t.Fatalf("subscribed again %+v, %v", again, err)
	}

	// A check held two videos for the user.
	onEngine(a, func() {
		a.subs.store.Subs[0].Hold([]subscribe.Video{{URL: testSubLong.URL, Title: testSubLong.Title, Duration: testSubLong.Duration}, {URL: testSubShort.URL, Title: testSubShort.Title}})
	})
	subs := list()
	if len(subs) != 1 || len(subs[0].Waiting) != 2 || subs[0].Waiting[0].Title != testSubLong.Title || subs[0].Waiting[0].DurationMS != testSubLong.Duration.Milliseconds() {
		t.Fatalf("subscriptions %+v", subs)
	}

	off, on := false, true
	set, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Subscription, error) {
		return c.SetSubscription(ctx, psync.SubscriptionSet{ID: sub.ID, Auto: &off, Paused: &on})
	})
	if err != nil || set.Auto || !set.Paused || a.subs.store.Subs[0].Mode != subscribe.ModeAsk || !a.subs.store.Subs[0].Paused {
		t.Fatalf("set %+v, %v; the desktop's %+v", set, err, a.subs.store.Subs[0].Rules)
	}

	took, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Subscription, error) {
		return c.TakeSubscription(ctx, sub.ID, []string{testSubLong.URL})
	})
	if err != nil || len(took.Waiting) != 1 || took.Waiting[0].URL != testSubShort.URL || took.Added != 1 {
		t.Fatalf("took %+v, %v", took, err)
	}
	if !slices.Contains(playlistJobSources(a), testSubLong.URL) {
		t.Fatalf("jobs %q", playlistJobSources(a))
	}
	if act := a.phone.activity[0]; act.Kind != settings.ActivitySubscriptionAdded || act.Count != 1 {
		t.Fatalf("activity %+v", act)
	}

	// Locked, the library's subscriptions are not there for a phone.
	l, err := shelf.NewLock("pw")
	if err != nil {
		t.Fatal(err)
	}
	onEngine(a, func() { a.library.SetLock(dir, l) })
	if subs := list(); len(subs) != 0 {
		t.Fatalf("subscriptions of a locked library %+v", subs)
	}
	_, err = ask(func(ctx context.Context) (any, error) { return nil, c.Unsubscribe(ctx, sub.ID) })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	onEngine(a, func() { a.library.SetLock(dir, shelf.Lock{}) })

	if _, err := ask(func(ctx context.Context) (any, error) { return nil, c.Unsubscribe(ctx, sub.ID) }); err != nil {
		t.Fatal(err)
	}
	if len(a.subs.store.Subs) != 0 || !slices.Contains(playlistJobSources(a), testSubLong.URL) {
		t.Fatalf("subscriptions %+v, jobs %q; want none, and the video it added kept", a.subs.store.Subs, playlistJobSources(a))
	}
	if act := a.phone.activity[0]; act.Kind != settings.ActivityUnsubscribed || act.Title != "Earth Science" {
		t.Fatalf("activity %+v", act)
	}
}
