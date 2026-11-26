package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/mediasource/plexlink"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// The sign-in of 设置 › 媒体来源's dialog to the account a kind's token field names (mediasource.Auth.SignIn, docs/media-sources.md 11.5): for Plex, Kando asks plex.tv for a PIN, opens its page in the browser, where the person signs in as their account does, waits for the token, lists the account's servers and fills the draft's address and token with the first that answers, which the person can change among the others.

// mediaSourceSignIn is the dialog's sign-in: gen numbers the sign-ins, so the answer of one started before another, or before the dialog closed or its kind changed, is dropped; busy while it waits for the person or plex.tv, cancel stopping it; servers the account's, with the address picked for each, chosen the ID of the one the draft was filled with; err why it failed. api and every, when set, are plex.tv's address and how often it is asked whether the person signed in, as a test's.
type mediaSourceSignIn struct {
	gen     int
	busy    bool
	cancel  context.CancelFunc
	servers []mediaSourceSignInServer
	chosen  string
	err     func(locales.Messages) string
	api     string
	every   time.Duration
}

// mediaSourceSignInServer is a server of the account signed in to, with the address the draft is filled with (address), and whether it answered there (reached).
type mediaSourceSignInServer struct {
	plexlink.Server
	address string
	reached bool
}

// The bounds of a sign-in: the person has as long as plex.tv keeps the PIN, half an hour, and Kando asks it every two seconds.
const (
	mediaSourceSignInTimeout = 30 * time.Minute
	mediaSourceSignInEvery   = 2 * time.Second
)

// mediaSourceSignInStop stops the dialog's sign-in and forgets what it found, as the dialog opens, closes or its kind changes.
func (a *app) mediaSourceSignInStop() {
	l := &a.sources.signIn
	if l.cancel != nil {
		l.cancel()
	}
	l.gen++
	l.busy, l.cancel, l.servers, l.chosen, l.err = false, nil, nil, "", nil
}

// mediaSourceSignInOffered is the account the draft's kind signs in to, and the key of the secret it fills; "" when it signs in to none.
func (a *app) mediaSourceSignInOffered() (link, key string) {
	k, ok := a.sources.kinds.Kind(a.sources.draft.Kind)
	if !ok {
		return "", ""
	}
	return k.SignIn()
}

// mediaSourceSignInStart signs in to the account the draft's kind names, starting over if a sign-in was waiting: off the engine's goroutine it asks plex.tv for a PIN, opens its page in the browser, waits for the token and lists the servers (mediaSourceSignedIn). Offline mode refuses it, as plex.tv is on the internet.
func (a *app) mediaSourceSignInStart() {
	if link, _ := a.mediaSourceSignInOffered(); link != mediasource.SignInPlex || a.cx == nil {
		return
	}
	a.mediaSourceSignInStop()
	l := &a.sources.signIn
	if a.privacy.offline {
		l.err = locales.Messages.SettingsSourcesPlexOffline
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), mediaSourceSignInTimeout)
	l.busy, l.cancel = true, cancel
	gen, every := l.gen, l.every
	if every == 0 {
		every = mediaSourceSignInEvery
	}
	c := &plexlink.Client{API: l.api, Product: a.messages().AppName(), ID: plexlink.NewID()}
	update := a.update
	go func() {
		defer cancel()
		pin, err := c.NewPIN(ctx)
		if err != nil {
			update(func() { a.mediaSourceSignedIn(gen, nil, err) })
			return
		}
		update(func() { a.mediaSourceSignInOpen(gen, c.AuthURL(pin)) })
		token, err := c.Wait(ctx, pin, every)
		if err != nil {
			update(func() { a.mediaSourceSignedIn(gen, nil, err) })
			return
		}
		servers, err := c.Servers(ctx, token)
		picked := mediaSourceSignInPick(ctx, servers)
		update(func() { a.mediaSourceSignedIn(gen, picked, err) })
	}()
}

// mediaSourceSignInPick is servers each with the address it answers at (plexlink.Pick), tried all at once.
func mediaSourceSignInPick(ctx context.Context, servers []plexlink.Server) []mediaSourceSignInServer {
	out := make([]mediaSourceSignInServer, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			address, ok := plexlink.Pick(ctx, http.DefaultClient, s)
			out[i] = mediaSourceSignInServer{Server: s, address: address, reached: ok}
		}()
	}
	wg.Wait()
	return out
}

// mediaSourceSignInOpen opens the page of the sign-in gen in the browser, unless another took its place; one that does not open ends it, as the person could not sign in.
func (a *app) mediaSourceSignInOpen(gen int, url string) {
	if a.sources.signIn.gen != gen || a.cx == nil {
		return
	}
	desktop.OpenURL(a.cx, url, live.On(a, func(opened bool) {
		if l := &a.sources.signIn; l.gen == gen && !opened {
			a.mediaSourceSignInStop()
			a.sources.signIn.err = locales.Messages.SettingsSourcesPlexNoBrowser
		}
	}))
}

// mediaSourceSignedIn records how the sign-in gen went, unless another took its place or the dialog closed: the servers found, the draft filled with the first that answered, or the first, and tested; or why it failed.
func (a *app) mediaSourceSignedIn(gen int, servers []mediaSourceSignInServer, err error) {
	l := &a.sources.signIn
	if l.gen != gen {
		return
	}
	l.busy, l.cancel = false, nil
	switch {
	case errors.Is(err, plexlink.ErrExpired), errors.Is(err, context.DeadlineExceeded):
		l.err = locales.Messages.SettingsSourcesPlexExpired
		return
	case errors.Is(err, media.ErrOffline):
		l.err = locales.Messages.SettingsSourcesPlexOffline
		return
	case err != nil:
		log.Printf("media source: signing in to Plex failed: %v", err)
		l.err = locales.Messages.SettingsSourcesPlexFailed
		return
	case len(servers) == 0:
		l.err = locales.Messages.SettingsSourcesPlexNoServer
		return
	}
	l.servers = servers
	pick := servers[0]
	for _, s := range servers {
		if s.reached {
			pick = s
			break
		}
	}
	a.mediaSourceSignInChoose(pick.ID)
}

// mediaSourceSignInChoose fills the draft with the server id of the account signed in to: its address and the token it takes, its name while the draft has none, and tests it.
func (a *app) mediaSourceSignInChoose(id string) {
	l := &a.sources.signIn
	_, key := a.mediaSourceSignInOffered()
	for _, s := range l.servers {
		if s.ID != id || key == "" {
			continue
		}
		d := &a.sources.draft
		if name := d.Name; name == "" || l.chosen != "" && name == a.mediaSourceSignInServerName(l.chosen) {
			d.Name = s.Name
		}
		l.chosen = id
		d.Values[mediasource.EndpointKey] = s.address
		d.Values[key] = s.Token
		delete(a.sources.tests, "")
		a.testMediaSource("")
		return
	}
}

// mediaSourceSignInServerName is the name of the server id of the account signed in to, "" for none.
func (a *app) mediaSourceSignInServerName(id string) string {
	for _, s := range a.sources.signIn.servers {
		if s.ID == id {
			return s.Name
		}
	}
	return ""
}

// mediaSourceSignInView is the sign-in as the dialog shows it, in m's language.
func (a *app) mediaSourceSignInView(m locales.Messages) settings.MediaSourceSignIn {
	link, _ := a.mediaSourceSignInOffered()
	if link == "" {
		return settings.MediaSourceSignIn{}
	}
	l := a.sources.signIn
	v := settings.MediaSourceSignIn{Offered: true, Busy: l.busy, Server: l.chosen}
	for _, s := range l.servers {
		v.Servers = append(v.Servers, settings.MediaSourceKind{ID: s.ID, Title: s.Name})
	}
	switch {
	case l.err != nil:
		v.Text, v.Failed = l.err(m), true
	case l.busy:
		v.Text = m.SettingsSourcesPlexWaiting()
	case l.chosen != "":
		for _, s := range l.servers {
			if s.ID != l.chosen {
				continue
			}
			if s.reached {
				v.Text = m.SettingsSourcesPlexSignedIn(s.Name)
			} else {
				v.Text, v.Failed = m.SettingsSourcesPlexUnreached(s.Name), true
			}
		}
	}
	return v
}
