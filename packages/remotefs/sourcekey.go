package remotefs

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
)

// SourceKeyLength is how many characters a SourceKey has: 22 of base64url, 132 bits of the hash.
const SourceKeyLength = 22

// SourceKey names a file of a NAS or a server by where it is and how big it is, the same on every device that reaches it, so the phone, which reaches a NAS itself, knows a film the computer made of the same file (docs/media-sources.md 10.5, sync.Film.SourceKey): the first SourceKeyLength characters of the unpadded base64url of the SHA-256 of six fields, each ended by a NUL byte, which none of them can hold:
//
//  1. kind: r.Kind, "smb", "sftp" or "webdav";
//  2. host: the server's name or address, trimmed, in lower case, without the brackets of an IPv6 address or a name's final dot; for WebDAV, the host of r.URL;
//  3. port: in decimal, the one named, else the kind's own: 445 for SMB, 22 for SFTP, and for WebDAV 443 for https:// and 80 for http://;
//  4. share: for SMB the share's name, trimmed of spaces and slashes, in lower case, as SMB's share names are; "" for the others;
//  5. path: where the file is on the server, p (slash-separated, relative to r.Root, as a Provider takes it) below the root, made plain (CleanPath) without a slash at either end: below the share for SMB; for SFTP starting with "/" when r.Root does, as a folder relative to the user's home is another place than the same path from the top; for WebDAV below the path of r.URL as well, unescaped;
//  6. size: the file's size in bytes, in decimal.
//
// Neither the user nor any secret is part of it, so two devices signed in as different users still agree, and a key says nothing of the server or the file to whoever reads it. A server named otherwise, as nas.local on one device and its address on another, gives another key on purpose: taking one file for another would be worse than not knowing it. SourceKey is "" for a remote it cannot name (an unknown kind, no host, an address WebDAV would not take), for a path that leaves the root or is the root itself, and for a negative size.
func SourceKey(r Remote, p string, size int64) string {
	if size < 0 {
		return ""
	}
	file, err := CleanFile(strings.Trim(p, "/"))
	if err != nil {
		return ""
	}
	root, err := remoteRoot(r.Root)
	if err != nil {
		return ""
	}
	var host, share, full string
	var port int
	switch r.Kind {
	case KindSMB:
		host, port = r.Host, r.Port
		if port == 0 {
			port = 445
		}
		share = strings.ToLower(strings.Trim(strings.TrimSpace(r.Share), `/\`))
		full = within(root, file)
	case KindSFTP:
		host, port = r.Host, r.Port
		if port == 0 {
			port = 22
		}
		full = within(root, file)
		if strings.HasPrefix(strings.TrimSpace(r.Root), "/") {
			full = "/" + full
		}
	case KindWebDAV:
		u, err := url.Parse(strings.TrimSpace(r.URL))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return ""
		}
		top, err := remoteRoot(u.Path)
		if err != nil {
			return ""
		}
		host = u.Hostname()
		if port, err = strconv.Atoi(u.Port()); u.Port() == "" || err != nil {
			port = 443
			if u.Scheme == "http" {
				port = 80
			}
		}
		full = within(within(top, root), file)
	default:
		return ""
	}
	host = strings.TrimSuffix(strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]"), ".")
	if host == "" || port < 1 || port > 65535 {
		return ""
	}
	h := sha256.New()
	for _, field := range []string{string(r.Kind), host, strconv.Itoa(port), share, full, strconv.FormatInt(size, 10)} {
		h.Write([]byte(field))
		h.Write([]byte{0})
	}
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))[:SourceKeyLength]
}
