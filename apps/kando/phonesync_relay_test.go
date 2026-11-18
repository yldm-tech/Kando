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
