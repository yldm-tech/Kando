package main

import (
	"context"
	"errors"

	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
	"kando/apps/kando/subscribe"
	kcore "kando/packages/core"
	psync "kando/packages/sync"
)

// phoneSubscriptions is the subscriptions a paired phone follows and changes over protocol v2 (psync.SubscriptionService), as the sidebar's list and a subscription's page show and change them: those of every library the app knows, the locked ones left out, which a phone neither sees nor acts on. A phone subscribes the open library as the control socket does (remoteSubscribe); the checks stay the app's, on its own schedule.
type phoneSubscriptions struct{ a *app }

func (p phoneSubscriptions) Subscriptions(ctx context.Context, caller psync.Caller) (psync.SubscriptionList, error) {
	a := p.a
	return remote.OnUI(ctx, a.update, func() (psync.SubscriptionList, error) {
		out := psync.SubscriptionList{Subscriptions: []psync.Subscription{}}
		for _, sub := range a.subs.store.Subs {
			if a.phoneSubscriptionServed(sub, caller.DeviceID) {
				out.Subscriptions = append(out.Subscriptions, phoneSubscriptionOf(sub))
			}
		}
		return out, nil
	})
}

// Subscribe subscribes the open library to the list at req.Link, as the control socket's subscribe does: a link that is not a channel, a space, a collection or a playlist is invalid; without an open library, with it locked or without yt-dlp the desktop is not ready; a list the library follows already answers that subscription.
func (p phoneSubscriptions) Subscribe(ctx context.Context, caller psync.Caller, req psync.SubscriptionAdd) (psync.Subscribed, error) {
	a := p.a
	got, err := a.remoteSubscribe(ctx, remote.SubscribeParams{Source: req.Link})
	switch {
	case remote.IsCode(err, remote.CodeInvalid):
		var e *remote.Error
		errors.As(err, &e)
		return psync.Subscribed{}, psync.Errorf(psync.CodeInvalid, "%s", e.Message)
	case err != nil:
		return psync.Subscribed{}, phoneRemoteErr(err)
	}
	return remote.OnUI(ctx, a.update, func() (psync.Subscribed, error) {
		sub, ok := a.subscribeFind(got.Subscription.ID)
		if !ok || !a.phoneSubscriptionServed(*sub, caller.DeviceID) {
			return psync.Subscribed{}, psync.Errorf(psync.CodeNotFound, "subscription %q was removed", got.Subscription.ID)
		}
		if !got.Already {
			a.phoneLog(caller.Name, settings.ActivitySubscribed, sub.Name, 0, 0)
		}
		return psync.Subscribed{Subscription: phoneSubscriptionOf(*sub), Existing: got.Already}, nil
	})
}

// SetSubscription turns the auto-download of a subscription on or off, as its page's 新视频 does between 自动加入队列 and 先问我, and pauses or resumes its checks.
func (p phoneSubscriptions) SetSubscription(ctx context.Context, caller psync.Caller, req psync.SubscriptionSet) (psync.Subscription, error) {
	a := p.a
	return remote.OnUI(ctx, a.update, func() (psync.Subscription, error) {
		sub, err := a.phoneSubscription(req.ID, caller.DeviceID)
		if err != nil {
			return psync.Subscription{}, err
		}
		if req.Auto != nil {
			mode := subscribe.ModeAsk
			if *req.Auto {
				mode = subscribe.ModeAuto
			}
			if sub.Mode != mode {
				sub.Mode = mode
				a.subscribeSave()
			}
		}
		if req.Paused != nil {
			a.subscribePause(sub, *req.Paused)
		}
		return phoneSubscriptionOf(*sub), nil
	})
}

// TakeSubscription queues the videos waiting on a subscription, as its page's 全部加入 and 加入 do; those that could not be queued stay, and the answer says why, as the page does.
func (p phoneSubscriptions) TakeSubscription(ctx context.Context, caller psync.Caller, req psync.SubscriptionTake) (psync.Subscription, error) {
	a := p.a
	return remote.OnUI(ctx, a.update, func() (psync.Subscription, error) {
		sub, err := a.phoneSubscription(req.ID, caller.DeviceID)
		if err != nil {
			return psync.Subscription{}, err
		}
		before := len(sub.Added)
		why := a.subscribeAddWaiting(sub, req.Links)
		// The videos queued before one could not be are logged too, as the queue has them either way.
		if n := len(sub.Added) - before; n > 0 {
			a.phoneLog(caller.Name, settings.ActivitySubscriptionAdded, sub.Name, n, 0)
		}
		if why != nil {
			return psync.Subscription{}, psync.Errorf(psync.CodeNotReady, "%s", why(remoteMessages()))
		}
		return phoneSubscriptionOf(*sub), nil
	})
}

// Unsubscribe removes a subscription as its page's 取消订阅 does, keeping the videos it added; a page that shows it goes back to the library.
func (p phoneSubscriptions) Unsubscribe(ctx context.Context, caller psync.Caller, req psync.SubscriptionRef) error {
	a := p.a
	_, err := remote.OnUI(ctx, a.update, func() (struct{}, error) {
		sub, err := a.phoneSubscription(req.ID, caller.DeviceID)
		if err != nil {
			return struct{}{}, err
		}
		name := sub.Name
		a.subscribeRemove(sub.ID)
		a.phoneLog(caller.Name, settings.ActivityUnsubscribed, name, 0, 0)
		return struct{}{}, nil
	})
	return err
}

// phoneSubscriptionServed reports whether deviceID sees sub: its library is one the app knows and is not locked for deviceID. UI thread only.
func (a *app) phoneSubscriptionServed(sub subscribe.Sub, deviceID string) bool {
	return a.subscribeKnown(sub.Library) && !a.phoneLibraryLocked(sub.Library, deviceID)
}

// phoneSubscription is the subscription with id deviceID may act on, not found when it is gone or its library is forgotten or locked. UI thread only.
func (a *app) phoneSubscription(id, deviceID string) (*subscribe.Sub, error) {
	sub, ok := a.subscribeFind(id)
	if !ok || !a.phoneSubscriptionServed(*sub, deviceID) {
		return nil, psync.Errorf(psync.CodeNotFound, "no subscription %q", id)
	}
	return sub, nil
}

// phoneSubscriptionOf is sub as protocol v2 tells it, saying in English why its last check failed.
func phoneSubscriptionOf(sub subscribe.Sub) psync.Subscription {
	link := sub.Link()
	out := psync.Subscription{ID: sub.ID, Library: phoneLibraryID(sub.Library), Name: sub.Name, Source: sub.Source, Kind: string(subscribe.KindOf(link)),
		Auto: sub.Mode == subscribe.ModeAuto, AddPerCheck: subscribe.AddPerCheck, Paused: !sub.Checks(), Gone: sub.Gone, Checked: sub.Checked,
		Waiting: []psync.SubscriptionVideo{}, Added: len(sub.Added)}
	switch link.Platform {
	case kcore.PlatformYouTube:
		out.Platform = psync.PlatformYouTube
	case kcore.PlatformBilibili:
		out.Platform = psync.PlatformBilibili
	}
	if !sub.Gone {
		out.Error = subscribeFailure(sub)(remoteMessages())
	}
	for _, v := range sub.Pending {
		out.Waiting = append(out.Waiting, psync.SubscriptionVideo{URL: v.URL, Title: v.Title, DurationMS: v.Duration.Milliseconds()})
	}
	return out
}
