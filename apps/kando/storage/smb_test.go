package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// smbTestRemote is the share KANDO_TEST_SMB names, as smb://user:password@host:port/share, and false when it is not set: there is no SMB server in Go to run here, so these tests need a real one, such as a Samba container.
func smbTestRemote(t *testing.T) (Remote, bool) {
	t.Helper()
	raw := os.Getenv("KANDO_TEST_SMB")
	if raw == "" {
		return Remote{}, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "smb" || u.User == nil {
		t.Fatalf("KANDO_TEST_SMB=%q is not smb://user:password@host:port/share", raw)
	}
	password, _ := u.User.Password()
	port, _ := strconv.Atoi(u.Port())
	return Remote{Kind: KindSMB, Host: u.Hostname(), Port: port, Share: strings.Trim(u.Path, "/"), User: u.User.Username(), Password: password}, true
}

// smbProvider is a provider over a new folder of the share KANDO_TEST_SMB names, removed with all it holds when the test ends; nil when it is not set.
func smbProvider(t *testing.T) Provider {
	t.Helper()
	cfg, ok := smbTestRemote(t)
	if !ok {
		return nil
	}
	ctx := context.Background()
	top, closer, err := OpenRemote(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Root = "kando-test-" + rand.Text()
	if err := top.MkdirAll(ctx, cfg.Root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		share, _, err := top.(*smbStore).s.get(ctx)
		if err == nil {
			err = share.RemoveAll(cfg.Root)
		}
		if err != nil {
			t.Errorf("remove %s: %v", cfg.Root, err)
		}
		closer.Close()
	})
	p, pc, err := OpenRemote(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	return p
}

func TestSMBConnectsAgainAfterTheConnectionBroke(t *testing.T) {
	p := smbProvider(t)
	if p == nil {
		t.Skip("KANDO_TEST_SMB is not set")
	}
	write(t, p, "Heat/video.mp4", "video")
	st := p.(*smbStore)
	cut := func() {
		st.s.mu.Lock()
		st.s.closer.(*smbConn).tcp.Close()
		st.s.mu.Unlock()
	}
	cut()
	if got := read(t, p, "Heat/video.mp4", 1, 3); got != "ide" {
		t.Fatalf("after the cut = %q", got)
	}
	cut()
	write(t, p, "Heat/video.srt", "subs")
	if got := names(t, p, "Heat"); strings.Join(got, ",") != "video.mp4,video.srt" {
		t.Fatalf("lists %q", got)
	}
}

func TestSMBRefusesAWrongPasswordAndAMissingShare(t *testing.T) {
	cfg, ok := smbTestRemote(t)
	if !ok {
		t.Skip("KANDO_TEST_SMB is not set")
	}
	ctx := context.Background()
	wrong := cfg
	wrong.Password = "wrong-" + rand.Text()
	if _, _, err := OpenRemote(ctx, wrong); err == nil {
		t.Fatal("a wrong password got in")
	}
	missing := cfg
	missing.Share = "no-such-share"
	if _, _, err := OpenRemote(ctx, missing); err == nil {
		t.Fatal("a missing share mounted")
	}
	root := cfg
	root.Root = "no-such-folder-" + rand.Text()
	if _, _, err := OpenRemote(ctx, root); !errors.Is(err, ErrNotExist) {
		t.Fatalf("a missing root = %v", err)
	}
}

func TestSMBNeedsAHostAShareAndAUser(t *testing.T) {
	ctx := context.Background()
	for _, cfg := range []Remote{
		{Kind: KindSMB, Share: "films", User: "kando"},
		{Kind: KindSMB, Host: "nas", User: "kando"},
		{Kind: KindSMB, Host: "nas", Share: "a/b", User: "kando"},
		{Kind: KindSMB, Host: "nas", Share: "films"},
		{Kind: KindSMB, Host: "nas", Share: "films", User: "kando", Root: "../x"},
	} {
		if _, _, err := OpenRemote(ctx, cfg); err == nil {
			t.Errorf("%+v opened", cfg)
		}
	}
}
