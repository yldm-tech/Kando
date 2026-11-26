package plexlink_test

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/mediasource/plexlink"
	"kando/apps/kando/mediasource/plexlink/plexlinktest"
)

func client(tv *plexlinktest.TV) *plexlink.Client {
	return &plexlink.Client{API: tv.URL, Product: "Kando", ID: "test-client"}
}

// TestSignIn takes a PIN, opens its page with the client and the code, waits for the person to sign in and gets the account's token.
func TestSignIn(t *testing.T) {
	tv := plexlinktest.New(t, "account-token")
	c := client(tv)
	ctx := context.Background()
	p, err := c.NewPIN(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Code == "" || time.Until(p.Expires) < 29*time.Minute {
		t.Fatalf("pin %+v", p)
	}
	page, err := url.Parse(c.AuthURL(p))
	if err != nil {
		t.Fatal(err)
	}
	if page.Scheme+"://"+page.Host+page.Path != plexlink.DefaultAuth {
		t.Errorf("the page is %s", page)
	}
	q, err := url.ParseQuery(strings.TrimPrefix(page.Fragment, "?"))
	if err != nil || q.Get("clientID") != "test-client" || q.Get("code") != p.Code || q.Get("context[device][product]") != "Kando" {
		t.Errorf("the page asks %v (%v)", q, err)
	}

	go func() {
		time.Sleep(30 * time.Millisecond)
		tv.SignIn(p.Code)
	}()
	token, err := c.Wait(ctx, p, 10*time.Millisecond)
	if err != nil || token != "account-token" {
		t.Fatalf("token %q, %v", token, err)
	}
	if n := tv.Count("GET", "/api/v2/pins/"); n < 2 {
		t.Errorf("asked %d times, want it to wait for the sign-in", n)
	}
}

// TestWaitExpired gives up once the PIN expired, and on the context's end.
func TestWaitExpired(t *testing.T) {
	tv := plexlinktest.New(t, "account-token")
	c := client(tv)
	p, err := c.NewPIN(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tv.Expire()
	if _, err := c.Wait(context.Background(), p, 10*time.Millisecond); !errors.Is(err, plexlink.ErrExpired) {
		t.Errorf("an expired PIN: %v", err)
	}

	tv2 := plexlinktest.New(t, "account-token")
	c2 := client(tv2)
	p3, _ := c2.NewPIN(context.Background())
	p3.Expires = time.Now().Add(40 * time.Millisecond)
	if _, err := c2.Wait(context.Background(), p3, 10*time.Millisecond); !errors.Is(err, plexlink.ErrExpired) {
		t.Errorf("a PIN past its time: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	p4, _ := c2.NewPIN(ctx)
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	if _, err := c2.Wait(ctx, p4, 10*time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Errorf("a sign-in stopped: %v", err)
	}
}

// TestServers lists the servers of the account, owned first, without its players, with the token each takes, and their addresses on the local network first, plain http after the plex.direct ones, without a relay.
func TestServers(t *testing.T) {
	tv := plexlinktest.New(t, "account-token")
	tv.Resources = []plexlinktest.Resource{
		{Name: "iPhone", ClientIdentifier: "phone", Provides: "client,player", Owned: true},
		{Name: "Friend", ClientIdentifier: "friend", Provides: "server", AccessToken: "friend-token", Connections: []plexlinktest.Connection{
			{Protocol: "https", Address: "203.0.113.9", Port: 32400, URI: "https://203-0-113-9.f.plex.direct:32400"},
		}},
		{Name: "nas", ClientIdentifier: "nas", Provides: "server", Owned: true, AccessToken: "nas-token", Connections: []plexlinktest.Connection{
			{Protocol: "https", Address: "198.51.100.4", Port: 32400, URI: "https://198-51-100-4.n.plex.direct:32400"},
			{Protocol: "https", Address: "172.104.89.189", Port: 8443, URI: "https://172-104-89-189.n.plex.direct:8443", Relay: true},
			{Protocol: "https", Address: "192.168.1.20", Port: 32400, URI: "https://192-168-1-20.n.plex.direct:32400", Local: true},
		}},
	}
	got, err := client(tv).Servers(context.Background(), "account-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "nas" || got[1].ID != "friend" {
		t.Fatalf("servers %+v", got)
	}
	if got[0].Token != "nas-token" || !got[0].Owned || got[1].Token != "friend-token" || got[1].Owned {
		t.Errorf("tokens %+v", got)
	}
	want := []string{"https://192-168-1-20.n.plex.direct:32400", "http://192.168.1.20:32400", "https://198-51-100-4.n.plex.direct:32400"}
	if !slices.Equal(got[0].Connections, want) {
		t.Errorf("addresses %v, want %v", got[0].Connections, want)
	}

	if _, err := client(tv).Servers(context.Background(), "other"); !errors.Is(err, plexlink.ErrRefused) {
		t.Errorf("a token plex.tv does not take: %v", err)
	}
}

// TestPick takes the first address at which the server answers as itself: not one that does not answer, nor another server's.
func TestPick(t *testing.T) {
	other := plexlinktest.Identity(t, "other")
	nas := plexlinktest.Identity(t, "nas")
	s := plexlink.Server{ID: "nas", Connections: []string{"http://127.0.0.1:1", other.URL, nas.URL}}
	if got, ok := plexlink.Pick(context.Background(), nil, s); !ok || got != nas.URL {
		t.Errorf("picked %s %v, want %s", got, ok, nas.URL)
	}
	s.Connections = []string{"http://127.0.0.1:1", other.URL}
	if got, ok := plexlink.Pick(context.Background(), nil, s); ok || got != "http://127.0.0.1:1" {
		t.Errorf("none answering: %s %v", got, ok)
	}
	if got, ok := plexlink.Pick(context.Background(), nil, plexlink.Server{ID: "nas"}); ok || got != "" {
		t.Errorf("no address: %s %v", got, ok)
	}
}
