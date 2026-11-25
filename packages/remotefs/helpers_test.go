package remotefs

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
)

// The helpers and providers of this package's own tests; providertest has the same helpers for the tests outside it, which cannot be imported here as it imports this package.

// remoteProviders are a WebDAV and an SFTP provider over servers in the test and, when KANDO_TEST_SMB names a share, an SMB provider over a new folder of it, for the checks every provider passes (providertest.Run, in provider_test.go).
func remoteProviders(t *testing.T) map[string]Provider {
	dav, _ := webdavProvider(t)
	sf, _ := sftpProvider(t)
	out := map[string]Provider{"webdav": dav, "sftp": sf}
	if smb := smbProvider(t); smb != nil {
		out["smb"] = smb
	}
	return out
}

// write stores body as the file name of p, failing the test when it cannot.
func write(t *testing.T, p Provider, name, body string) {
	t.Helper()
	if err := p.Write(context.Background(), name, strings.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// read is length bytes of the file name of p from offset, to its end with length -1, failing the test when it cannot read them.
func read(t *testing.T, p Provider, name string, offset, length int64) string {
	t.Helper()
	r, err := p.Open(context.Background(), name, offset, length)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer r.Close()
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}

// names is what the folder dir of p holds, sorted, a folder with a slash after its name, failing the test when it cannot list it.
func names(t *testing.T, p Provider, dir string) []string {
	t.Helper()
	infos, err := p.List(context.Background(), dir)
	if err != nil {
		t.Fatalf("list %q: %v", dir, err)
	}
	var out []string
	for _, info := range infos {
		name := info.Name
		if info.Dir {
			name += "/"
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}
