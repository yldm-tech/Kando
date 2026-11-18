package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// fakeSubscriptions is a desktop's SubscriptionService with one subscription, which counts the changes it was asked for.
type fakeSubscriptions struct {
	sub     Subscription
	changes int
	gone    bool
}

func (f *fakeSubscriptions) Subscriptions(context.Context, Caller) (SubscriptionList, error) {
	if f.gone {
		return SubscriptionList{Subscriptions: []Subscription{}}, nil
	}
	return SubscriptionList{Subscriptions: []Subscription{f.sub}}, nil
}

func (f *fakeSubscriptions) Subscribe(_ context.Context, _ Caller, req SubscriptionAdd) (Subscribed, error) {
	f.changes++
	if req.Link == f.sub.Source {
		return Subscribed{Subscription: f.sub, Existing: true}, nil
	}
	return Subscribed{Subscription: Subscription{ID: "s2", Source: req.Link, Waiting: []SubscriptionVideo{}}}, nil
}

func (f *fakeSubscriptions) find(id string) error {
	if f.gone || id != f.sub.ID {
		return Errorf(CodeNotFound, "no subscription %q", id)
	}
	return nil
}

func (f *fakeSubscriptions) SetSubscription(_ context.Context, _ Caller, req SubscriptionSet) (Subscription, error) {
	if err := f.find(req.ID); err != nil {
		return Subscription{}, err
	}
	f.changes++
	if req.Auto != nil {
		f.sub.Auto = *req.Auto
	}
	if req.Paused != nil {
		f.sub.Paused = *req.Paused
	}
	return f.sub, nil
}

func (f *fakeSubscriptions) TakeSubscription(_ context.Context, _ Caller, req SubscriptionTake) (Subscription, error) {
	if err := f.find(req.ID); err != nil {
		return Subscription{}, err
	}
	f.changes++
	f.sub.Added += len(f.sub.Waiting)
	f.sub.Waiting = []SubscriptionVideo{}
	return f.sub, nil
}

func (f *fakeSubscriptions) Unsubscribe(_ context.Context, _ Caller, req SubscriptionRef) error {
	if err := f.find(req.ID); err != nil {
		return err
	}
	f.changes++
	f.gone = true
	return nil
}

// A phone lists the computer's subscriptions, turns one's auto-download off, sends its waiting videos to the queue and unsubscribes, each answered with the subscription as it is after; one gone since is not found.
func TestSubscriptionsOverV2(t *testing.T) {
	ctx := context.Background()
	f := &fakeSubscriptions{sub: Subscription{ID: "s1", Library: "l1", Name: "Earth Science", Source: "https://www.youtube.com/@earth", Kind: SubscriptionChannel, Platform: PlatformYouTube, Auto: true, AddPerCheck: 3,
		Waiting: []SubscriptionVideo{{URL: "https://youtu.be/a", Title: "Rivers", DurationMS: 600_000}, {URL: "https://youtu.be/b", Title: "Coasts"}}}}
	desk := newDesk(&fakeService{receive: ReceiveQueue})
	desk.Subscriptions = f
	c := v2Desktop(t, desk)
	list, err := c.Subscriptions(ctx)
	if err != nil || len(list.Subscriptions) != 1 || list.Subscriptions[0].Name != "Earth Science" || len(list.Subscriptions[0].Waiting) != 2 || list.Subscriptions[0].Waiting[0].DurationMS != 600_000 {
		t.Fatalf("subscriptions %+v %v", list, err)
	}
	off := false
	sub, err := c.SetSubscription(ctx, SubscriptionSet{ID: "s1", Auto: &off})
	if err != nil || sub.Auto || sub.Paused {
		t.Fatalf("set %+v %v", sub, err)
	}
	sub, err = c.TakeSubscription(ctx, "s1", nil)
	if err != nil || len(sub.Waiting) != 0 || sub.Added != 2 {
		t.Fatalf("take %+v %v", sub, err)
	}
	got, err := c.Subscribe(ctx, "https://www.youtube.com/@earth")
	if err != nil || !got.Existing || got.Subscription.ID != "s1" {
		t.Fatalf("subscribe to a list followed already %+v %v", got, err)
	}
	if err := c.Unsubscribe(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	_, err = c.SetSubscription(ctx, SubscriptionSet{ID: "s1", Auto: &off})
	wantError(t, err, http.StatusNotFound, CodeNotFound)
	if f.changes != 4 {
		t.Fatalf("%d changes reached the desktop, want 4", f.changes)
	}
}

// A desktop that does not share its subscriptions answers their calls unsupported, and a read-only device is refused every one that changes them before the desktop's work, though it lists them.
func TestSubscriptionsUnsupportedAndReadOnly(t *testing.T) {
	ctx := context.Background()
	desk := newDesk(&fakeService{receive: ReceiveQueue})
	_, err := v2Desktop(t, desk).Subscriptions(ctx)
	wantError(t, err, http.StatusNotImplemented, CodeUnsupported)

	f := &fakeSubscriptions{sub: Subscription{ID: "s1", Waiting: []SubscriptionVideo{}}}
	desk.Subscriptions = f
	reader := Caller{DeviceID: "d1", Name: "Pixel", ReadOnly: true}
	on := true
	writes := map[string]any{
		OpSubscribe:        SubscriptionAdd{Link: "https://www.youtube.com/@other"},
		OpSubscriptionSet:  SubscriptionSet{ID: "s1", Paused: &on},
		OpSubscriptionTake: SubscriptionTake{ID: "s1"},
		OpUnsubscribe:      SubscriptionRef{ID: "s1"},
	}
	for op, args := range writes {
		if !Mutates(op) {
			t.Fatalf("%s does not count as a change", op)
		}
		data, _ := json.Marshal(args)
		r := desk.Answer(ctx, reader, Call{ID: "w-" + op, V: CallVersion, Op: op, Args: data})
		if r.Error == nil || r.Error.Code != CodeReadOnly {
			t.Fatalf("%s of a read-only device answered %s %+v", op, r.Result, r.Error)
		}
	}
	if f.changes != 0 {
		t.Fatalf("%d changes of a read-only device reached the desktop", f.changes)
	}
	if r := desk.Answer(ctx, reader, Call{ID: "r", V: CallVersion, Op: OpSubscriptions}); r.Error != nil || Mutates(OpSubscriptions) {
		t.Fatalf("a read-only device's list: %+v", r.Error)
	}
}
