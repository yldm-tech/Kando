package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"time"

	"kando/apps/kando/mediasource"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// The media sources script plugins add (docs/media-sources.md E17, 11): each kind a plugin turned on contributes is registered with the media sources' Registry, so 设置 › 媒体来源 offers it and the 媒体来源 page browses it as it does a built-in one, and taken off as the plugin is turned off, removed or no longer contributes it; the sources of a kind taken off stay in the settings. A source of a plugin's kind is read by a mediasource.PluginDriver, which asks the plugin's list, stat and open on the engine's goroutine and reads a file itself, so a video's bytes never pass through the plugin's code; the plugin reaches its server only through kando.source.fetch, which Kando sends to the address the user filled in for the source, with its secrets, which the plugin never sees.

// pluginsSourceCallTimeout is how long Kando waits for a plugin's list, stat or open (docs/media-sources.md 11.2).
const pluginsSourceCallTimeout = 15 * time.Second

// pluginsSources is what the plugins' media sources hold on the engine's goroutine: the kinds registered, each with the source of the manifest it was made from, so a plugin updated with other fields is registered anew; the fetchers of the sources open, by plugin and the id the plugin knows the source by (pluginsSourceKey), which kando.source.fetch sends through; and seq, which numbers the fetches, so one plugin's fetches wait on its server side by side (pluginsWaiting).
type pluginsSources struct {
	kinds    map[string]plugins.MediaSource
	fetchers map[string]*mediasource.Fetcher
	seq      int
}

// pluginsSourceKey is the key of the fetcher of the source the plugin plugin knows as id.
func pluginsSourceKey(plugin, id string) string { return plugin + "\x00" + id }

// pluginsSourcesFollow registers the kinds of media source of the script plugins turned on, and takes off those of plugins turned off, removed or that no longer contribute them, after each change of the app's state. A plugin turned on that is still being verified or loaded has its kinds registered: a source of one waits for the plugin when it is read. It reports whether the kinds changed, which the settings and the 媒体来源 page read; the connections to the sources of a changed kind are dropped, as its Open changed.
func (a *app) pluginsSourcesFollow() bool {
	want := map[string]struct {
		plugin string
		src    plugins.MediaSource
	}{}
	if a.plugins.scanned {
		for _, p := range a.plugins.packs {
			if p.Kind != plugins.KindScript || p.Contributes == nil || !a.plugins.state.EnabledPack(p) {
				continue
			}
			for _, src := range p.Contributes.Sources {
				want[mediasource.PluginKindID(p.ID, src.ID)] = struct {
					plugin string
					src    plugins.MediaSource
				}{p.ID, src}
			}
		}
	}
	s := &a.plugins.sources
	changed := false
	for id, src := range s.kinds {
		if w, ok := want[id]; !ok || !pluginsSameSource(w.src, src) {
			a.sources.kinds.Unregister(id)
			delete(s.kinds, id)
			changed = true
		}
	}
	for _, id := range slices.Sorted(maps.Keys(want)) {
		w := want[id]
		if _, ok := s.kinds[id]; ok {
			continue
		}
		k := mediasource.PluginKind(w.plugin, w.src, nil)
		k.Open = a.pluginsSourceOpen(w.plugin, w.src, k)
		if err := a.sources.kinds.Register(k); err != nil {
			continue
		}
		if s.kinds == nil {
			s.kinds = map[string]plugins.MediaSource{}
		}
		s.kinds[id] = w.src
		changed = true
	}
	if changed {
		a.mediaSourcePoolSync()
	}
	return changed
}

// pluginsSameSource reports whether a and b declare the same kind of source, field for field.
func pluginsSameSource(a, b plugins.MediaSource) bool { return reflect.DeepEqual(a, b) }

// pluginsSourceOpen is how a source of the kind k, which the plugin plugin contributes as src, is opened, on any goroutine: its endpoint read (mediasource.ParseEndpoint), and, on the engine's goroutine, once the plugin runs (waiting pluginsTranslateWait at most while it is still being loaded, as at launch), a fetcher of its own kept for kando.source.fetch under the id the plugin knows the source by (pluginsSourceID), until the driver closes.
func (a *app) pluginsSourceOpen(plugin string, src plugins.MediaSource, k mediasource.Kind) func(ctx context.Context, s mediasource.Source, secrets map[string]string) (mediasource.Driver, error) {
	return func(ctx context.Context, s mediasource.Source, secrets map[string]string) (mediasource.Driver, error) {
		endpoint, err := mediasource.ParseEndpoint(s.Config[mediasource.EndpointKey])
		if err != nil {
			return nil, err
		}
		base := &mediasource.Fetcher{Endpoint: endpoint, Source: src, Config: maps.Clone(s.Config), Secrets: maps.Clone(secrets), Pin: s.Config[mediasource.PinKey]}
		base.Clone() // makes the note of refused certificates its clones share
		type opened struct {
			id  string
			err error
		}
		got := make(chan opened, 1)
		waitUntil := time.Now().Add(pluginsTranslateWait)
		var try func()
		try = func() {
			if err := ctx.Err(); err != nil {
				got <- opened{err: err}
				return
			}
			l := a.plugins.script.loaded[plugin]
			if l == nil {
				p, _ := a.pluginsPack(plugin)
				if (a.pluginsRunnable(p) || a.pluginsVerifying(p)) && a.plugins.state.EnabledPack(p) && time.Now().Before(waitUntil) {
					time.AfterFunc(pluginsTranslatePoll, func() { a.update(try) })
					return
				}
				got <- opened{err: &mediasource.PluginError{Code: pluginsNotRunningCode}}
				return
			}
			id := a.pluginsSourceID(l, s.ID)
			ps := &a.plugins.sources
			if ps.fetchers == nil {
				ps.fetchers = map[string]*mediasource.Fetcher{}
			}
			ps.fetchers[pluginsSourceKey(plugin, id)] = base
			got <- opened{id: id}
		}
		a.update(try)
		var o opened
		select {
		case o = <-got:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if o.err != nil {
			return nil, o.err
		}
		key := pluginsSourceKey(plugin, o.id)
		return &mediasource.PluginDriver{
			Call:    a.pluginsSourceCall(plugin),
			Source:  mediasource.PluginSourceArg(k, o.id, s),
			Fetcher: base,
			Done: func() {
				a.update(func() {
					if a.plugins.sources.fetchers[key] == base {
						delete(a.plugins.sources.fetchers, key)
					}
				})
			},
		}, nil
	}
}

// pluginsNotRunningCode is the code a source of a plugin's kind fails with while its plugin does not run.
const pluginsNotRunningCode = "not-running"

// pluginsSourceID is the id the plugin l knows the source of ID id by (docs/media-sources.md 11.2): derived from the plugins folder's secret (pluginsScripts.filmKey), the plugin, its signer and the source (HMAC, as plugins.FilmID), so the plugin can tell its sources apart and learns nothing of them, and two plugins cannot match theirs up. A source not saved yet, as the draft a Test tries, has one of its own each time.
func (a *app) pluginsSourceID(l *pluginsScript, id string) string {
	if id == "" {
		id = "draft:" + mediasource.NewID()
	}
	k := hmac.New(sha256.New, a.plugins.script.filmKey)
	k.Write([]byte(l.pack.ID + "\x00" + l.pack.Signer + "\x00source"))
	mac := hmac.New(sha256.New, k.Sum(nil))
	mac.Write([]byte(id))
	return "s_" + hex.EncodeToString(mac.Sum(nil))[:16]
}

// pluginsSourceCall calls an export of the plugin plugin for its source, from any goroutine: on the engine's goroutine through the runner, waiting pluginsSourceCallTimeout at most. A rejection with a code is a *mediasource.PluginError, as is a plugin not running.
func (a *app) pluginsSourceCall(plugin string) mediasource.PluginCall {
	return func(ctx context.Context, export string, args ...any) (any, error) {
		type answer struct {
			v   any
			err error
		}
		got := make(chan answer, 1)
		a.update(func() {
			if err := ctx.Err(); err != nil {
				got <- answer{err: err}
				return
			}
			a.pluginsContribCall(plugin, export, args, func(v any, err error) { got <- answer{v, pluginsSourceErr(err)} })
		})
		timer := time.NewTimer(pluginsSourceCallTimeout)
		defer timer.Stop()
		select {
		case r := <-got:
			return r.v, r.err
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, &mediasource.PluginError{Code: pluginsTimedOut}
		}
	}
}

// pluginsSourceErr is err, a failure of a plugin's list, stat or open, as a *mediasource.PluginError: a plugin not running, a rejection with a code as that code, anything else as failed.
func pluginsSourceErr(err error) error {
	var coded *ns.Error
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errPluginsNotRunning), errors.Is(err, script.ErrStopped):
		return &mediasource.PluginError{Code: pluginsNotRunningCode}
	case errors.As(err, &coded) && coded.Code != "":
		return &mediasource.PluginError{Code: coded.Code, Message: coded.Message}
	}
	return &mediasource.PluginError{Code: pluginsFailed, Message: err.Error()}
}

// pluginsSourceFetch answers kando.source.fetch (docs/media-sources.md 11.3): {source, method, path, query, header, body, login}, a request to the server of the plugin's source the plugin knows as source, which must be one of its sources open now. Kando sends it under the source's endpoint with its secrets (mediasource.Fetcher), offline only to the local network, and answers {status, header, body}. A request the source's rules do not let go is denied; one Kando cannot send, offline to the internet or to a server whose certificate is not trusted, unavailable.
func (a *app) pluginsSourceFetch(l *pluginsScript, c *ns.HostCall) {
	source, r, ok := mediasource.PluginRequest(c.Data)
	if !ok {
		pluginsRefuse(c, pluginsBad)
		return
	}
	base := a.plugins.sources.fetchers[pluginsSourceKey(l.pack.ID, source)]
	if base == nil {
		pluginsRefuse(c, pluginsDenied)
		return
	}
	f := base.Clone()
	f.Offline = a.privacy.offline
	s := &a.plugins.sources
	s.seq++
	pluginsWaiting(a, l, c, "source.fetch "+strconv.Itoa(s.seq), func(ctx context.Context) (mediasource.FetchReply, error) {
		return f.Fetch(ctx, r)
	}, func(got mediasource.FetchReply, err error) {
		var cert *mediasource.CertError
		switch {
		case errors.Is(err, mediasource.ErrFetchDenied):
			pluginsRefuse(c, pluginsDenied)
		case errors.Is(err, mediasource.ErrFetchTooLarge):
			pluginsRefuse(c, pluginsTooLarge)
		case errors.Is(err, media.ErrOffline), errors.As(err, &cert):
			pluginsRefuse(c, pluginsUnavailable)
		case errors.Is(err, context.DeadlineExceeded):
			pluginsRefuse(c, pluginsTimedOut)
		case err != nil:
			pluginsRefuse(c, pluginsUnavailable)
		default:
			c.Resolve(mediasource.PluginReply(got))
		}
	})
}
