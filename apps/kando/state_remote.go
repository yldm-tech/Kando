package main

import (
	"context"
	"errors"
	"log"

	"kando/apps/kando/remote"
)

// remoteUI is the app's end of the control socket (package remote), where the kando command line and its MCP server drive the running app. server is nil until the window is ready, and stays nil when another Kando already serves the socket or it could not be opened. ctx ends when the app quits, so a request still waiting for the UI thread gives up instead of holding the quit back until its timeout.
type remoteUI struct {
	server *remote.Server
	ctx    context.Context
	cancel context.CancelFunc
}

func newRemoteUI() remoteUI {
	ctx, cancel := context.WithCancel(context.Background())
	return remoteUI{ctx: ctx, cancel: cancel}
}

// remoteHooks: the socket opens once the window is ready, as its answers run on the UI thread through update, which posts to the window; it closes when the app quits. An app that cannot open it, as a second copy of Kando while the first one serves it, runs on without it and says why in the log.
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

// remoteListen opens the control socket at remote.SocketPath and answers its requests with remoteHandle (remote_api.go).
func (a *app) remoteListen() error {
	server, err := remote.Listen(remote.SocketPath(), a.remoteHandle)
	if err != nil {
		return err
	}
	a.remote.server = server
	return nil
}

// remoteClose ends the requests still waiting for the UI thread, then closes the socket and its connections. It runs on the UI thread, which those requests wait for, so it ends them first: Close waits for every answer to be written.
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
