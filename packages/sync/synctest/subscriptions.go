package synctest

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"kando/packages/sync"
)

// subscriptions are the stand-in desktop's subscriptions (sync.SubscriptionService): the list, how many it made, and whether it shares them at all (SetNoSubscriptions).
type subscriptions struct {
	list []sync.Subscription
	made int
	none bool
}

// AddSubscription adds sub to the desktop's subscriptions, or puts it in place of the one with its ID, as a check on the desktop finding new videos.
func (d *Desktop) AddSubscription(sub sync.Subscription) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if sub.Waiting == nil {
		sub.Waiting = []sync.SubscriptionVideo{}
	}
	if i := slices.IndexFunc(d.subs.list, func(s sync.Subscription) bool { return s.ID == sub.ID }); i >= 0 {
		d.subs.list[i] = sub
		return
	}
	d.subs.list = append(d.subs.list, sub)
}

// Subscriptions are the desktop's subscriptions, in the order they were added.
func (d *Desktop) Subscriptions() []sync.Subscription {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.subs.list)
}

// SetNoSubscriptions makes the desktop answer the subscriptions' calls unsupported while none is set, as a Kando from before they were shared.
func (d *Desktop) SetNoSubscriptions(none bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.subs.none = none
}

// subscriptionService is the stand-in's subscriptions as v2's calls read and change them. Subscribing names the list by its link and adds it to the first library; taking a subscription's waiting videos queues them as links a phone sent would be.
type subscriptionService struct{ d *Desktop }

func (s subscriptionService) Subscriptions(context.Context, sync.Caller) (sync.SubscriptionList, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.subs.none {
		return sync.SubscriptionList{}, sync.Errorf(sync.CodeUnsupported, "no subscriptions")
	}
	out := sync.SubscriptionList{Subscriptions: []sync.Subscription{}}
	for _, sub := range d.subs.list {
		if slices.ContainsFunc(d.libs, func(l sync.Library) bool { return l.ID == sub.Library }) {
			out.Subscriptions = append(out.Subscriptions, sub)
		}
	}
	return out, nil
}

func (s subscriptionService) Subscribe(_ context.Context, _ sync.Caller, req sync.SubscriptionAdd) (sync.Subscribed, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	link := strings.TrimSpace(req.Link)
	switch {
	case d.subs.none:
		return sync.Subscribed{}, sync.Errorf(sync.CodeUnsupported, "no subscriptions")
	case link == "":
		return sync.Subscribed{}, sync.Errorf(sync.CodeInvalid, "no link")
	case len(d.libs) == 0:
		return sync.Subscribed{}, sync.Errorf(sync.CodeNotReady, "no library open")
	}
	for _, sub := range d.subs.list {
		if sub.Source == link {
			return sync.Subscribed{Subscription: sub, Existing: true}, nil
		}
	}
	d.subs.made++
	sub := sync.Subscription{ID: fmt.Sprintf("sub%d", d.subs.made), Library: d.libs[0].ID, Name: link, Source: link, Kind: sync.SubscriptionChannel, Auto: true, AddPerCheck: 3, Waiting: []sync.SubscriptionVideo{}}
	d.subs.list = append(d.subs.list, sub)
	return sync.Subscribed{Subscription: sub}, nil
}

func (s subscriptionService) SetSubscription(_ context.Context, _ sync.Caller, req sync.SubscriptionSet) (sync.Subscription, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	sub, err := d.subscriptionLocked(req.ID)
	if err != nil {
		return sync.Subscription{}, err
	}
	if req.Auto != nil {
		sub.Auto = *req.Auto
	}
	if req.Paused != nil {
		sub.Paused = *req.Paused
	}
	return *sub, nil
}

func (s subscriptionService) TakeSubscription(_ context.Context, caller sync.Caller, req sync.SubscriptionTake) (sync.Subscription, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	sub, err := d.subscriptionLocked(req.ID)
	if err != nil {
		return sync.Subscription{}, err
	}
	var kept []sync.SubscriptionVideo
	for _, v := range sub.Waiting {
		if req.Links != nil && !slices.Contains(req.Links, v.URL) {
			kept = append(kept, v)
			continue
		}
		d.queue.made++
		d.queue.jobs = append(d.queue.jobs, sync.Job{ID: fmt.Sprintf("job%d", d.queue.made), Link: v.URL, Title: v.Title, Library: sub.Library, Device: caller.Name, State: sync.JobWaiting, Steps: []sync.Step{
			{Name: sync.StepDownload, State: sync.StepWaiting}, {Name: sync.StepTranscribe, State: sync.StepWaiting}, {Name: sync.StepTranslate, State: sync.StepWaiting},
		}})
		sub.Added++
	}
	if kept == nil {
		kept = []sync.SubscriptionVideo{}
	}
	sub.Waiting = kept
	d.desk.Journal.PublishJobs(d.queue.jobs)
	return *sub, nil
}

func (s subscriptionService) Unsubscribe(_ context.Context, _ sync.Caller, req sync.SubscriptionRef) error {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.subscriptionLocked(req.ID); err != nil {
		return err
	}
	d.subs.list = slices.DeleteFunc(d.subs.list, func(s sync.Subscription) bool { return s.ID == req.ID })
	return nil
}

// subscriptionLocked is the subscription with id of a library the desktop lists, as one of a locked library is not found to a phone. d.mu is held.
func (d *Desktop) subscriptionLocked(id string) (*sync.Subscription, error) {
	if d.subs.none {
		return nil, sync.Errorf(sync.CodeUnsupported, "no subscriptions")
	}
	i := slices.IndexFunc(d.subs.list, func(s sync.Subscription) bool { return s.ID == id })
	if i < 0 || !slices.ContainsFunc(d.libs, func(l sync.Library) bool { return l.ID == d.subs.list[i].Library }) {
		return nil, sync.Errorf(sync.CodeNotFound, "no subscription %q", id)
	}
	return &d.subs.list[i], nil
}
