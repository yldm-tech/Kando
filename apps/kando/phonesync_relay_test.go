package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/cloud"
	psync "kando/packages/sync"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// phoneRelayStore sets up a's storage as an S3-compatible bucket "relay" served by a server that answers every request 503, so the relay's turns fail without the network.
func phoneRelayStore(t *testing.T, a *app) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	t.Cleanup(srv.Close)
	a.cloud.storage = cloud.KindCompat
	a.cloud.compatEndpoint, a.cloud.compatRegion, a.cloud.compatBucket = srv.URL, "us-east-1", "relay"
	a.cloud.compatAccessKey, a.cloud.compatSecret, a.cloud.compatPath = "key", "secret-value", true
	return srv.URL
}

// The storage relay is off by default, even with a store set up: a paired phone is not offered to join it.
func TestPhoneRelayIsOffByDefault(t *testing.T) {
	a, tt, updates, _, _ := phoneSyncApp(t)
	phoneRelayStore(t, a)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || slices.Contains(h.Ops, psync.OpRelayJoin) {
		t.Fatalf("hello %+v, %v", h, err)
	}
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.RelayJoin, error) {
		var joined psync.RelayJoin
		return joined, c.Call(ctx, psync.OpRelayJoin, nil, &joined)
	})
	phoneWantCode(t, err, http.StatusNotImplemented, psync.CodeUnsupported)
}

// With settings.json's phoneRelay on, a phone joins the relay on the local network and gets links presigned for the store of 账号与上传, under the desktop's own folder.
func TestPhoneRelayHandsAPhoneLinksToTheStore(t *testing.T) {
	a, tt, updates, _, _ := phoneSyncApp(t)
	endpoint := phoneRelayStore(t, a)
	a.phone.relay = true
	phoneSyncOn(t, a, tt, updates)
	data, err := os.ReadFile(a.settingsPath)
	if err != nil || !strings.Contains(string(data), `"phoneRelay": true`) {
		t.Fatalf("settings.json: %s, %v", data, err)
	}
	c := phoneClient(t, a, "")
	paired, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.RelayJoin, error) {
		var joined psync.RelayJoin
		return joined, c.Call(ctx, psync.OpRelayJoin, nil, &joined)
	})
	if err != nil {
		t.Fatal(err)
	}
	folder := endpoint + "/relay/" + psync.RelayPrefix(a.phone.server.Fingerprint())
	if len(joined.Key) != psync.RelayKeySize || !strings.HasPrefix(joined.Grant.State, folder+psync.RelayStateName(paired.DeviceID)+"?") || len(joined.Grant.Slots) != psync.RelaySlots || !strings.HasPrefix(joined.Grant.Slots[0].Put, folder+"inbox/"+paired.DeviceID+"/") {
		t.Fatalf("joined %+v", joined.Grant)
	}
	if strings.Contains(joined.Grant.State, "secret-value") {
		t.Fatal("a link carries the store's secret")
	}
}

// With phoneRelay on but no store set up, the relay stays off.
func TestPhoneRelayNeedsAStore(t *testing.T) {
	a, tt, updates, _, _ := phoneSyncApp(t)
	a.phone.relay = true
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	if h, err := phoneCall(t, a, tt, updates, c.Hello); err != nil || slices.Contains(h.Ops, psync.OpRelayJoin) {
		t.Fatalf("hello %+v, %v", h, err)
	}
}

// phoneRelayRefusingStore sets up a's storage as phoneRelayStore does, served by a server that refuses the access key of every request, as S3 does, which the SDK does not retry.
func phoneRelayRefusingStore(t *testing.T, a *app) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>InvalidAccessKeyId</Code><Message>The access key does not exist.</Message></Error>`))
	}))
	t.Cleanup(srv.Close)
	phoneRelayStore(t, a)
	a.cloud.compatEndpoint = srv.URL
}

// relayJoinable reports whether c's desktop offers the relay to join.
func relayJoinable(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), c *psync.Client) bool {
	t.Helper()
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil {
		t.Fatal(err)
	}
	return slices.Contains(h.Ops, psync.OpRelayJoin)
}

// 经你的存储同步 on the 手机同步 page turns the relay on and off at once, and the page says why the storage does not answer; Phone Sync takes up the store's new settings once they settle, and stops the relay under 离线模式.
func TestPhoneRelayFollowsTheSwitchAndTheStore(t *testing.T) {
	a, tt, updates, _, _ := phoneSyncApp(t)
	phoneRelayRefusingStore(t, a)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	paired, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code)
	if err != nil {
		t.Fatal(err)
	}
	c = phoneClient(t, a, paired.Token)
	if relayJoinable(t, a, tt, updates, c) || tt.Exists("id:settings-phone-relay-status") {
		t.Fatal("the relay runs before its switch is on")
	}
	first := a.phone.server
	tt.Press("settings-phone-relay")
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil && a.phone.server != first && a.phone.relayErr != nil })
	data, err := os.ReadFile(a.settingsPath)
	if err != nil || !strings.Contains(string(data), `"phoneRelay": true`) {
		t.Fatalf("settings.json: %s, %v", data, err)
	}
	if !relayJoinable(t, a, tt, updates, c) {
		t.Fatal("the switch did not start the relay")
	}
	// The store refuses the access key: the page says so, in the words an upload's failure would.
	want := uploadFailure(a.messages(), a.phone.relayErr, a.currentStore())
	if st := a.PhoneSync().Relay; st.Err != want || st.Working || !tt.Exists("id:settings-phone-relay-status >> text:"+want) {
		t.Fatalf("relay %+v; texts %q", st, tt.Texts())
	}
	// Another bucket: the relay goes on in the old one until the settings settle, then Phone Sync starts again in the new one.
	running := a.phone.server
	a.cloud.compatBucket = "relay-2"
	refresh(tt, a)
	if a.phone.server != running {
		t.Fatal("Phone Sync started again at once, before the store's settings settled")
	}
	tt.Advance(phoneRelayRestartAfter)
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil && a.phone.server != running })
	if a.phone.relaySig != a.phoneRelaySig() || !relayJoinable(t, a, tt, updates, c) {
		t.Fatal("Phone Sync did not take up the new bucket")
	}
	// 离线模式 stops the relay without turning its switch off.
	running = a.phone.server
	a.privacy.offline = true
	refresh(tt, a)
	tt.Advance(phoneRelayRestartAfter)
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil && a.phone.server != running })
	if relayJoinable(t, a, tt, updates, c) || !a.PhoneSync().Relay.On || !a.PhoneSync().Relay.Offline {
		t.Fatalf("the relay under Offline mode: %+v", a.PhoneSync().Relay)
	}
	a.privacy.offline = false
	refresh(tt, a)
	tt.Advance(phoneRelayRestartAfter)
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil && a.phone.relaySig == a.phoneRelaySig() })
	// Off again, at once.
	running = a.phone.server
	tt.Press("settings-phone-relay")
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil && a.phone.server != running })
	if relayJoinable(t, a, tt, updates, c) || a.PhoneSync().Relay.On {
		t.Fatal("the switch did not stop the relay")
	}
}
