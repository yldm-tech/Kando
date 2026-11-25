package pair

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"syscall"
	"testing"
	"time"
)

// An address that holds the connection without answering does not keep the phone from the one that answers: the addresses are tried at once.
func TestReachTakesTheAddressThatAnswers(t *testing.T) {
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	go func() {
		for {
			c, err := silent.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	desktop := httptest.NewTLSServer(http.NotFoundHandler())
	defer desktop.Close()
	addrs := []netip.AddrPort{netip.MustParseAddrPort(silent.Addr().String()), netip.MustParseAddrPort(desktop.Listener.Addr().String())}
	start := time.Now()
	f, err := reach(context.Background(), addrs, reachFor)
	if err != nil {
		t.Fatal(err)
	}
	if f.addr != addrs[1] || len(f.fingerprint) == 0 {
		t.Fatalf("found %v, want %v with a fingerprint", f.addr, addrs[1])
	}
	if took := time.Since(start); took >= connectTimeout {
		t.Fatalf("took %v, as long as the silent address's timeout", took)
	}
}

// A connection the system refuses, as iOS does while it asks whether the app may use the local network, is blocked; one nothing answers is not, so it is not tried again.
func TestBlockedIsTheSystemsRefusal(t *testing.T) {
	op := func(errno syscall.Errno) error {
		return fmt.Errorf("pair: %w", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", errno)})
	}
	for _, errno := range []syscall.Errno{syscall.EHOSTUNREACH, syscall.ENETUNREACH, syscall.EPERM, syscall.EACCES} {
		if !blocked(op(errno)) {
			t.Errorf("%v is not taken for blocked", errno)
		}
	}
	for _, err := range []error{op(syscall.ECONNREFUSED), context.DeadlineExceeded, errors.New("tls: handshake failure")} {
		if blocked(err) {
			t.Errorf("%v is taken for blocked", err)
		}
	}
}
