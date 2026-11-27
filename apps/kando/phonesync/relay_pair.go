package phonesync

import (
	"context"
	"time"

	psync "kando/packages/sync"
)

// Pairing through the relay (psync's relay_pair.go, yldm-tech/Kando#468): while the relay is on, each code on offer has a pairing mailbox in the bucket, which the QR code carries, so a phone away from the computer's network pairs by scanning it. The server reads the mailbox while the code is on offer, checks its code as it checks one sent on the local network, under the same limits, and answers the phone's token with its join of the relay. The mailbox is deleted as its code ends.

// relayPairEvery is how often the server reads a pairing mailbox while its code is on offer. Tests shorten it.
var relayPairEvery = psync.RelayPairEvery

// relayPairSignFor is the longest making a pairing mailbox waits for the bucket to sign its links (tests shorten it): no time for a bucket that signs with a key it holds, but a bucket whose short-lived key must first be asked for (relayhost) talks to the network, and a code is offered on the UI thread with mu held. A code whose mailbox is not signed by then is offered for the local network alone.
var relayPairSignFor = 5 * time.Second

// offerRelay gives o a pairing mailbox in the relay's bucket and serves it until o ends, while the relay is on. The caller holds mu.
func (s *Server) offerRelay(o *offer) {
	if s.relay == nil {
		return
	}
	sctx, scancel := context.WithTimeout(context.Background(), relayPairSignFor)
	m, err := psync.NewRelayPairMailbox(sctx, s.relay.Bucket, s.relay.Prefix, psync.CodeLifetime, s.cfg.Now())
	scancel()
	if err != nil {
		s.cfg.Logf("phonesync: relay: pairing mailbox: %v", err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	relay := m.Relay()
	o.mailbox, o.stopMailbox = &relay, cancel
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		// The code ends at its lifetime even when nothing takes it back, as when the settings stay open.
		timer := time.AfterFunc(psync.CodeLifetime, cancel)
		defer timer.Stop()
		m.Serve(ctx, relayPairEvery, func(ctx context.Context, req psync.PairRequest) psync.RelayPairAnswer {
			return s.pairRelay(ctx, o, req)
		}, s.cfg.Logf)
		// Close does not wait out a slow store: a mailbox left behind is deleted by the relay's sweep.
		rctx, rcancel := context.WithTimeout(context.Background(), forgetTimeout)
		defer rcancel()
		go func() {
			select {
			case <-s.stop:
				rcancel()
			case <-rctx.Done():
			}
		}()
		if err := m.Remove(rctx); err != nil {
			s.cfg.Logf("phonesync: relay: pairing mailbox: remove: %v", err)
		}
	}()
}

// pairRelay pairs the phone whose request came through o's mailbox, as pair does one on the local network, and joins it to the relay: its key and first grant go back with its token, sealed under the mailbox's key.
func (s *Server) pairRelay(ctx context.Context, o *offer, req psync.PairRequest) psync.RelayPairAnswer {
	code, changed := s.tryPair(req.Code, true, o)
	if code != "" {
		if changed {
			// The last try the code had went wrong: the settings say it expired.
			s.cfg.Changed()
		}
		s.cfg.Logf("phonesync: pair through the relay: %s", code)
		return psync.RelayPairAnswer{Error: &psync.ErrorBody{Code: code, Message: "the code does not pair"}}
	}
	dev, token, err := s.devices.add(req.Device, req.Platform, s.cfg.Now())
	s.cfg.Changed()
	if err != nil {
		s.cfg.Logf("phonesync: pair through the relay: save the device: %v", err)
		return psync.RelayPairAnswer{Error: &psync.ErrorBody{Code: psync.CodeFailed, Message: "could not save the device"}}
	}
	join, err := relayJoiner{s}.Join(ctx, psync.Caller{DeviceID: dev.ID, Name: dev.Name})
	if err != nil {
		// A phone paired with no way to reach the computer away from its network is unpaired again: it pairs anew with the next code.
		s.cfg.Logf("phonesync: pair through the relay: join: %v", err)
		if err := s.Revoke(dev.ID); err != nil {
			s.cfg.Logf("phonesync: pair through the relay: unpair: %v", err)
		}
		return psync.RelayPairAnswer{Error: &psync.ErrorBody{Code: psync.CodeFailed, Message: "could not join the relay"}}
	}
	s.cfg.Logf("phonesync: paired %q (%s) through the relay", dev.Name, dev.Platform)
	return psync.RelayPairAnswer{Pair: &psync.PairResponse{Token: token, DeviceID: dev.ID, Desktop: s.cfg.Name}, Join: &join}
}
