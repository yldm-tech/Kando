package remotefs

import "testing"

// TestSourceKeyVectors pins SourceKey to its definition: each vector's key is the first 22 characters of the base64url SHA-256 of its six fields, each ended by a NUL, as an implementation elsewhere computes it (these were computed with Python's hashlib).
func TestSourceKeyVectors(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    Remote
		p    string
		size int64
		want string
	}{
		// smb ‖ nas.local ‖ 445 ‖ video ‖ Films/River.mp4 ‖ 1048576
		{"smb", Remote{Kind: KindSMB, Host: "nas.local", Share: "video", Root: "Films"}, "River.mp4", 1 << 20, "6aTKFGnkWxhnQ8vrl6Wj-G"},
		// The same file as the root is set otherwise, the host and share written in another case, with spaces, slashes and the default port named.
		{"smb written otherwise", Remote{Kind: KindSMB, Host: " NAS.local. ", Port: 445, Share: "/Video/", User: "anna", Password: "secret"}, "Films/River.mp4", 1 << 20, "6aTKFGnkWxhnQ8vrl6Wj-G"},
		// smb ‖ 192.168.1.10 ‖ 445 ‖ video ‖ Films/River.mp4 ‖ 1048576
		{"smb by address", Remote{Kind: KindSMB, Host: "192.168.1.10", Share: "video"}, "Films/River.mp4", 1 << 20, "5yxv73jsLeH0H0VMMMaEWD"},
		// smb ‖ fe80::1 ‖ 445 ‖ video ‖ Films/River.mp4 ‖ 1048576
		{"smb by IPv6", Remote{Kind: KindSMB, Host: "[fe80::1]", Share: "video"}, "Films/./River.mp4", 1 << 20, "TyoSsA_TPg8s2BDSzYpWty"},
		// sftp ‖ nas.local ‖ 2222 ‖ ‖ /volume1/video/Films/River.mp4 ‖ 1048576
		{"sftp from the top", Remote{Kind: KindSFTP, Host: "nas.local", Port: 2222, Root: "/volume1/video/"}, "Films/River.mp4", 1 << 20, "q2dxo4ryFruhxNuKthS38G"},
		// sftp ‖ nas.local ‖ 22 ‖ ‖ Films/River.mp4 ‖ 1048576
		{"sftp from home", Remote{Kind: KindSFTP, Host: "nas.local"}, "Films/River.mp4", 1 << 20, "hqLXtEhqZwjPUKZG-gm3Vi"},
		// webdav ‖ dav.example.com ‖ 443 ‖ ‖ remote.php/dav/files/me/Films/河流.mp4 ‖ 734003200
		{"webdav", Remote{Kind: KindWebDAV, URL: "https://DAV.example.com/remote.php/dav/files/me/", Root: "Films"}, "河流.mp4", 700 << 20, "QWfEzdfSmjMW-lPoB5inCs"},
		// The same file through an address with the port named and a part of its path escaped.
		{"webdav escaped", Remote{Kind: KindWebDAV, URL: "https://dav.example.com:443/remote.php/dav/files/%6De/Films"}, "河流.mp4", 700 << 20, "QWfEzdfSmjMW-lPoB5inCs"},
		// webdav ‖ nas.local ‖ 5005 ‖ ‖ Films/River.mp4 ‖ 1048576
		{"webdav on a port", Remote{Kind: KindWebDAV, URL: "http://nas.local:5005"}, "Films/River.mp4", 1 << 20, "T6iee7yrqMIh0AMdx7DfOD"},
	} {
		if got := SourceKey(tc.r, tc.p, tc.size); got != tc.want {
			t.Errorf("%s: SourceKey = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestSourceKeyTellsFilesApart checks that each field counts, that a server named otherwise is another server, and that a remote or path it cannot name has no key.
func TestSourceKeyTellsFilesApart(t *testing.T) {
	base := Remote{Kind: KindSMB, Host: "nas.local", Share: "video"}
	key := SourceKey(base, "River.mp4", 100)
	if len(key) != SourceKeyLength {
		t.Fatalf("SourceKey = %q, want %d characters", key, SourceKeyLength)
	}
	for name, other := range map[string]string{
		"another size":  SourceKey(base, "River.mp4", 101),
		"another file":  SourceKey(base, "Lake.mp4", 100),
		"another share": SourceKey(Remote{Kind: KindSMB, Host: "nas.local", Share: "music"}, "River.mp4", 100),
		"another port":  SourceKey(Remote{Kind: KindSMB, Host: "nas.local", Port: 4455, Share: "video"}, "River.mp4", 100),
		"another name":  SourceKey(Remote{Kind: KindSMB, Host: "192.168.1.10", Share: "video"}, "River.mp4", 100),
		"another kind":  SourceKey(Remote{Kind: KindSFTP, Host: "nas.local", Port: 445, Root: "video"}, "River.mp4", 100),
		"another case":  SourceKey(base, "river.mp4", 100),
	} {
		if other == key {
			t.Errorf("%s has the same key %q", name, key)
		}
	}
	if SourceKey(Remote{Kind: KindSFTP, Host: "nas.local", Root: "/video"}, "River.mp4", 100) == SourceKey(Remote{Kind: KindSFTP, Host: "nas.local", Root: "video"}, "River.mp4", 100) {
		t.Error("an SFTP folder from the top and one in the home folder have the same key")
	}
	for name, r := range map[string]Remote{
		"no kind":       {Host: "nas.local"},
		"no host":       {Kind: KindSMB, Share: "video"},
		"ftp address":   {Kind: KindWebDAV, URL: "ftp://nas.local/"},
		"a user in URL": {Kind: KindWebDAV, URL: "https://anna@nas.local/"},
		"root leaves":   {Kind: KindSMB, Host: "nas.local", Share: "video", Root: "../x"},
	} {
		if got := SourceKey(r, "River.mp4", 100); got != "" {
			t.Errorf("%s: SourceKey = %q, want none", name, got)
		}
	}
	for _, p := range []string{"", "/", "../River.mp4", `a\b.mp4`} {
		if got := SourceKey(base, p, 100); got != "" {
			t.Errorf("path %q: SourceKey = %q, want none", p, got)
		}
	}
	if got := SourceKey(base, "River.mp4", -1); got != "" {
		t.Errorf("a negative size has the key %q", got)
	}
}
