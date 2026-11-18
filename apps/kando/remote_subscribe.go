package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"kando/apps/kando/playlist"
	"kando/apps/kando/remote"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/subscribe"
	kcore "kando/packages/core"
	"kando/packages/media"
)

// The control socket's subscription methods (remote.MethodSubscriptions, MethodSubscribe, MethodSubscriptionCheck), as the app answers them: the open library's subscriptions, as the window's sidebar lists them. They read and change the subscriptions only on the UI thread (remote.OnUI), and list videos with yt-dlp outside it, as remoteLists does.

// remoteSubscribeHandle answers the subscription methods (remoteHandle).
func (a *app) remoteSubscribeHandle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case remote.MethodSubscribe:
		p, err := remote.Params[remote.SubscribeParams](params)
		if err != nil {
			return nil, err
		}
		return a.remoteSubscribe(ctx, p)
	case remote.MethodSubscriptionCheck:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		return a.remoteSubscriptionCheck(ctx, p.ID)
	}
	return a.remoteSubscriptions(ctx)
}

// remoteSubscription is sub as the socket gives it, saying in English why its last check failed.
func remoteSubscription(sub subscribe.Sub) remote.Subscription {
	link := sub.Link()
	r := remote.Subscription{
		ID: sub.ID, Name: sub.Name, Source: sub.Source, Kind: string(subscribe.KindOf(link)), Platform: string(link.Platform),
		Mode: string(sub.Mode), Filter: string(sub.Filter), Keywords: sub.Keywords, Cadence: string(sub.Cadence), Series: sub.Series, AddPerCheck: subscribe.AddPerCheck,
		Paused: sub.Paused, Gone: sub.Gone, Stopped: sub.Stopped, Renamed: sub.Renamed, Checked: sub.Checked,
		Waiting: []remote.ListVideo{}, Added: len(sub.Added),
	}
	if !sub.Gone {
		r.Error = subscribeFailure(sub)(remoteMessages())
	}
	for _, v := range sub.Pending {
		r.Waiting = append(r.Waiting, remote.ListVideo{URL: v.URL, Title: v.Title, Duration: v.Duration.Seconds()})
	}
	return r
}

// remoteSubscribeLibrary is the open library, which the subscription methods work in; without one, or while it is locked, they are not ready. remoteHandle's lock gate looks only before a method starts, and these look again after yt-dlp has read a list for minutes, in which the user may have locked it.
func (a *app) remoteSubscribeLibrary() (string, error) {
	dir := a.snap().LibraryDir
	switch {
	case strings.TrimSpace(dir) == "":
		return "", remote.Errorf(remote.CodeNotReady, "%s", remoteMessages().PrecheckNoLibrary())
	case a.lockShut():
		return "", remote.Errorf(remote.CodeNotReady, "%s", remoteMessages().LockLibraryIsLocked(sidebar.Title(dir)))
	}
	return dir, nil
}

// remoteSubscriptions is the open library's subscriptions, in the sidebar's order.
func (a *app) remoteSubscriptions(ctx context.Context) ([]remote.Subscription, error) {
	return remote.OnUI(ctx, a.update, func() ([]remote.Subscription, error) {
		dir, err := a.remoteSubscribeLibrary()
		if err != nil {
			return nil, err
		}
		out := []remote.Subscription{}
		for _, sub := range a.subs.store.In(dir) {
			out = append(out, remoteSubscription(sub))
		}
		return out, nil
	})
}

// remoteSubscribe subscribes the open library to the list at p.Source, as the window's dialog does (subscribeAskSubmit): a link that is not a channel, a collection or a playlist is refused; one the library follows already answers that subscription; any other is read with yt-dlp, and the videos it holds now count as seen. The window's sidebar lists it, but the window stays on the page it shows.
func (a *app) remoteSubscribe(ctx context.Context, p remote.SubscribeParams) (remote.Subscribed, error) {
	m := remoteMessages()
	source := strings.TrimSpace(p.Source)
	if link, ok := kcore.ParseLink(source); !ok || !link.IsList() {
		return remote.Subscribed{}, remote.Errorf(remote.CodeInvalid, "%s", m.SubscribeAddNotList())
	}
	type setup struct {
		dir    string
		lister media.Lister
		have   *remote.Subscribed
	}
	got, err := remote.OnUI(ctx, a.update, func() (setup, error) {
		dir, err := a.remoteSubscribeLibrary()
		if err != nil {
			return setup{}, err
		}
		if sub, ok := a.subs.store.Following(source, dir); ok {
			return setup{have: &remote.Subscribed{Subscription: remoteSubscription(*sub), Already: true}}, nil
		}
		lister := a.subscribeLister()
		if lister == nil {
			return setup{}, remote.Errorf(remote.CodeNotReady, "%s", m.SubscribeAddCannotList())
		}
		return setup{dir: dir, lister: lister}, nil
	})
	if err != nil {
		return remote.Subscribed{}, err
	}
	if got.have != nil {
		return *got.have, nil
	}
	list, err := playlist.ReadOne(ctx, got.lister, source)
	if err != nil {
		if ctx.Err() != nil {
			return remote.Subscribed{}, ctx.Err()
		}
		return remote.Subscribed{}, remote.Errorf(remote.CodeFailed, "%s", playlistError(m, err))
	}
	return remote.OnUI(ctx, a.update, func() (remote.Subscribed, error) {
		// The library may have been locked while yt-dlp read it, or another one opened and that one locked.
		if _, err := a.remoteSubscribeLibrary(); err != nil {
			return remote.Subscribed{}, err
		}
		if a.LibraryLocked(got.dir) {
			return remote.Subscribed{}, remote.Errorf(remote.CodeNotReady, "%s", m.LockLibraryIsLocked(sidebar.Title(got.dir)))
		}
		// The window may have subscribed to it while yt-dlp read it.
		if sub, ok := a.subs.store.Following(source, got.dir); ok {
			return remote.Subscribed{Subscription: remoteSubscription(*sub), Already: true}, nil
		}
		sub := subscribe.New(source, got.dir, list.Listing, time.Now())
		a.subs.store.Subs = append(a.subs.store.Subs, sub)
		a.subscribeSave()
		a.subs.notes[sub.ID] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteSubscribed, Text: sub.Name}}
		return remote.Subscribed{Subscription: remoteSubscription(sub)}, nil
	})
}

// remoteSubscribeFind is the open library's subscription with id, and why it cannot be checked: it is not there, it is paused, or it is kept after its site deleted the list.
func (a *app) remoteSubscribeFind(id string) (*subscribe.Sub, error) {
	if _, err := a.remoteSubscribeLibrary(); err != nil {
		return nil, err
	}
	sub, ok := a.subscribeFind(id)
	switch {
	case !ok || !a.subscribeOfOpen(*sub):
		return nil, remote.Errorf(remote.CodeNotFound, "no subscription %q in the open library", id)
	case sub.Paused:
		return nil, remote.Errorf(remote.CodeNotReady, "subscription %q is paused; resume it in the Kando window first", sub.Name)
	case sub.Stopped:
		return nil, remote.Errorf(remote.CodeNotReady, "subscription %q is no longer checked, as its site deleted the list", sub.Name)
	}
	return sub, nil
}

// remoteSubscriptionCheck checks the open library's subscription with id now, as the window's 立即检查 does, and says what it found. yt-dlp lists it outside the window's own line of checks, and what it found is folded in as a check of the window's is (subscribeTook), so the page says it too; a check the window runs at the same time finds nothing new after it.
func (a *app) remoteSubscriptionCheck(ctx context.Context, id string) (remote.SubscriptionChecked, error) {
	type setup struct {
		lister media.Lister
		source string
	}
	got, err := remote.OnUI(ctx, a.update, func() (setup, error) {
		sub, err := a.remoteSubscribeFind(id)
		if err != nil {
			return setup{}, err
		}
		lister := a.subscribeLister()
		if lister == nil {
			return setup{}, remote.Errorf(remote.CodeNotReady, "%s", remoteMessages().SubscribeAddCannotList())
		}
		return setup{lister: lister, source: sub.Source}, nil
	})
	if err != nil {
		return remote.SubscriptionChecked{}, err
	}
	list, listErr := playlist.ReadOne(ctx, got.lister, got.source)
	if listErr != nil && ctx.Err() != nil {
		return remote.SubscriptionChecked{}, ctx.Err()
	}
	return remote.OnUI(ctx, a.update, func() (remote.SubscriptionChecked, error) {
		// It may have been paused or removed while yt-dlp listed it.
		sub, err := a.remoteSubscribeFind(id)
		if err != nil {
			return remote.SubscriptionChecked{}, err
		}
		pending, added := len(sub.Pending), len(sub.Added)
		a.subscribeTook(id, list.Listing, listErr)
		m := remoteMessages()
		if listErr != nil {
			if sub.Gone {
				return remote.SubscriptionChecked{}, remote.Errorf(remote.CodeFailed, "%s", m.SubscribeNoteGone())
			}
			return remote.SubscriptionChecked{}, remote.Errorf(remote.CodeFailed, "%s", subscribeFailure(*sub)(m))
		}
		note := a.subs.notes[id]
		r := remote.SubscriptionChecked{Subscription: remoteSubscription(*sub), New: note.Count, Queued: len(sub.Added) - added, Held: len(sub.Pending) - pending, Skipped: note.Skipped}
		if note.Kind == subscribe.NoteNotQueued && note.why != nil {
			r.Note = note.why(m)
		}
		return r, nil
	})
}
