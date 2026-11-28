package main

import (
	"context"
	"errors"
	"log"

	"kando/apps/kando/locales"
	"kando/apps/kando/playlist"
	"kando/apps/kando/remote"

	ns "github.com/nsgo-dev/nsgo/core"
)

// remoteUI is the app's end of the control socket (package remote), where the kando command line and its MCP server drive the running app. server is nil until the window is ready, and stays nil when another Kando already serves the socket or it could not be opened. ctx ends when the app quits, so a request still waiting for the UI thread, the engine's goroutine, gives up instead of holding the quit back until its timeout.
type remoteUI struct {
	server *remote.Server
	ctx    context.Context
	cancel context.CancelFunc
	// en is the scope the socket says the window's words under, in English (remoteScope), made on first use.
	en *ns.Scope
	// lists is the playlists and channels the socket listed (remoteLists), by the key of each's link, which the videos it then adds from them are placed in (nextKeepLists), as the playlist dialog places those it adds.
	lists map[string]playlist.List
}

func newRemoteUI() remoteUI {
	ctx, cancel := context.WithCancel(context.Background())
	return remoteUI{ctx: ctx, cancel: cancel}
}

// remoteHooks: the socket opens once the window is ready, as its answers run on the engine's goroutine through update (remote.OnUI), which the window's bindings then follow; it closes when the app quits. An app that cannot open it, as a second copy of Kando while the first one serves it, runs on without it and says why in the log.
func (a *app) remoteHooks() areaHooks {
	return areaHooks{
		ready: func() {
			if err := a.remoteListen(); errors.Is(err, remote.ErrInUse) {
				log.Printf("remote: %v; the command line drives that one", err)
			} else if err != nil {
				log.Printf("remote: no control socket: %v", err)
			}
		},
		quitting: a.remoteClose,
	}
}

// remoteListen opens the control socket at remote.SocketPath, its folder made the user's alone when it is Kando's own (remote.ListenDefault), and answers its requests with remoteHandle (remote_api.go).
func (a *app) remoteListen() error {
	server, err := remote.ListenDefault(a.remoteHandle)
	if err != nil {
		return err
	}
	a.remote.server = server
	return nil
}

// remoteClose ends the requests still waiting for the UI thread, then closes the socket and its connections. It runs once the engine has stopped (areaHooks.quitting), so nothing will run what those requests wait for, and it ends them first: Close waits for every answer to be written.
func (a *app) remoteClose() {
	if a.remote.cancel != nil {
		a.remote.cancel()
	}
	if a.remote.server == nil {
		return
	}
	if err := a.remote.server.Close(); err != nil {
		log.Printf("remote: close the control socket: %v", err)
	}
	a.remote.server = nil
}

// remoteScope is a scope that says Kando's words in English (remote.Language), for what the app says under a scope, as the sidebar's status line (sidebar.Readiness.Text). It is the root of a graph of its own, apart from the window's, as a scope's language is that of its whole app (Scope.SetLocale). UI thread only.
func (a *app) remoteScope() *ns.Scope {
	if a.remote.en == nil {
		a.remote.en = ns.NewRoot(ns.RootOptions{})
		a.remote.en.SetLocale(remote.Language)
	}
	return a.remote.en
}

// remoteMessages says Kando's words in English, the language the socket answers in (remote.Language), whatever language the window shows.
func remoteMessages() locales.Messages { return locales.In(remote.Language) }
