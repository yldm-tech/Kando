package storage

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/cloudsoda/go-smb2"
)

// What OpenRemote's own servers answer reads as the failure a person can act on.
func TestDiagnoseReadsTheServersAnswers(t *testing.T) {
	ctx := context.Background()
	dav := newFakeDAV(t)
	ssh := newFakeSSH(t)
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refused := closed.Addr().String()
	closed.Close()
	host, port, _ := net.SplitHostPort(refused)
	n, _ := strconv.Atoi(port)

	wrongDAV := Remote{Kind: KindWebDAV, URL: dav.url, User: "kando", Password: "wrong"}
	missingDAV := Remote{Kind: KindWebDAV, URL: dav.url, Root: "missing", User: "kando", Password: "secret"}
	unknown := ssh.remote("")
	unknown.HostKey = ""
	wrongSSH := ssh.remote("")
	wrongSSH.Password = "wrong"
	missingSSH := ssh.remote("missing")
	badKey := ssh.remote("")
	badKey.PrivateKey = "not a key"
	climbing := ssh.remote("../..")
	tls := httptest.NewTLSServer(nil)
	t.Cleanup(tls.Close)
	for _, c := range []struct {
		name string
		cfg  Remote
		want Failure
	}{
		{"webdav wrong password", wrongDAV, FailSignIn},
		{"webdav missing root", missingDAV, FailNoRoot},
		{"webdav untrusted certificate", Remote{Kind: KindWebDAV, URL: tls.URL, User: "kando", Password: "secret"}, FailCertificate},
		{"webdav refused", Remote{Kind: KindWebDAV, URL: "http://" + refused, User: "kando", Password: "secret"}, FailUnreachable},
		{"sftp unknown key", unknown, FailHostKey},
		{"sftp wrong password", wrongSSH, FailSignIn},
		{"sftp missing root", missingSSH, FailNoRoot},
		{"sftp unreadable key", badKey, FailPrivateKey},
		{"sftp root above home", climbing, FailBadRoot},
		{"sftp refused", Remote{Kind: KindSFTP, Host: host, Port: n, User: "kando", Password: "secret", HostKey: ssh.hostKey}, FailUnreachable},
	} {
		_, closer, err := OpenRemote(ctx, c.cfg)
		if err == nil {
			closer.Close()
			t.Errorf("%s: opened", c.name)
			continue
		}
		if got := Diagnose(err); got != c.want {
			t.Errorf("%s: Diagnose(%v) = %d, want %d", c.name, err, got, c.want)
		}
	}
}

// An SMB server's status codes, wrapped as the providers wrap them, read as the failure they stand for.
func TestDiagnoseReadsSMBStatus(t *testing.T) {
	for code, want := range map[uint32]Failure{
		smbLogonFailure:   FailSignIn,
		smbAccessDenied:   FailDenied,
		smbBadNetworkName: FailNoShare,
		0xC0000001:        FailOther,
	} {
		err := fmt.Errorf("mount: %w", &smb2.ResponseError{Code: code})
		if got := Diagnose(err); got != want {
			t.Errorf("%#x = %d, want %d", code, got, want)
		}
	}
	if got := Diagnose(fmt.Errorf("x: %w", os.ErrPermission)); got != FailDenied {
		t.Errorf("permission = %d", got)
	}
	if got := Diagnose(x509.UnknownAuthorityError{}); got != FailCertificate {
		t.Errorf("certificate = %d", got)
	}
	if got := Diagnose(context.DeadlineExceeded); got != FailTimeout {
		t.Errorf("deadline = %d", got)
	}
	if got := Diagnose(errors.New("something else")); got != FailOther {
		t.Errorf("other = %d", got)
	}
}

// Fingerprint gives the form HostKeyError.Fingerprint has.
func TestFingerprintMatchesTheHostKeyError(t *testing.T) {
	f := newFakeSSH(t)
	cfg := f.remote("")
	cfg.HostKey = ""
	_, _, err := OpenRemote(context.Background(), cfg)
	var hk *HostKeyError
	if !errors.As(err, &hk) {
		t.Fatalf("err = %v", err)
	}
	if got := Fingerprint(hk.Key); got == "" || got != hk.Fingerprint {
		t.Fatalf("Fingerprint = %q, want %q", got, hk.Fingerprint)
	}
	if Fingerprint("not a key") != "" {
		t.Fatal("a bad key has a fingerprint")
	}
}
