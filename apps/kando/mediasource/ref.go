package mediasource

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"kando/packages/core"
)

// Ref is a video's address on a media source, kando-source://<source ID>/<path> (core.SourceScheme), which a film imported from the source keeps as its job's Source. Path is relative to the source's root, each segment escaped in the address (url.PathEscape). The address names the source by its ID, never by host name or user, so it says nothing of where the NAS is in library.json or to a phone; it is only Kando's, not an address anything opens.
type Ref struct {
	Source string
	Path   string
}

// String is the address of r, kando-source://<source ID>/<path>.
func (r Ref) String() string {
	segments := strings.Split(r.Path, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return core.SourceScheme + "://" + r.Source + "/" + strings.Join(segments, "/")
}

// ParseRef reads the address String writes. It fails with ErrInvalidPath for one whose path is no file under the source's root (CleanPath, written as it is clean), and otherwise for anything but that scheme with a source's ID (ValidID) and a path, without a port, user, query or fragment.
func ParseRef(raw string) (Ref, error) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, core.SourceScheme) || u.Opaque != "" {
		return Ref{}, fmt.Errorf("mediasource: %q is not a %s:// address", raw, core.SourceScheme)
	}
	if !ValidID(u.Host) || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return Ref{}, fmt.Errorf("mediasource: %q names no source's file", raw)
	}
	escaped, ok := strings.CutPrefix(u.EscapedPath(), "/")
	if !ok {
		return Ref{}, fmt.Errorf("mediasource: %q has no path", raw)
	}
	segments := strings.Split(escaped, "/")
	for i, s := range segments {
		seg, err := url.PathUnescape(s)
		// A segment is one name: an escaped slash in it would make it two.
		if err != nil || strings.Contains(seg, "/") {
			return Ref{}, fmt.Errorf("%q: %w", raw, ErrInvalidPath)
		}
		segments[i] = seg
	}
	p := strings.Join(segments, "/")
	clean, err := CleanPath(p)
	if err != nil || clean != p || clean == "" {
		return Ref{}, fmt.Errorf("%q: %w", raw, ErrInvalidPath)
	}
	return Ref{Source: u.Host, Path: p}, nil
}

// CleanPath is p, a path on a source, made plain, "a/./b/../c" as "a/c" and the root as "", or ErrInvalidPath for one that leaves the root, as remotefs's paths do (an absolute path, one with a backslash or NUL, one that climbs above the root with ".."), and for a name that is not UTF-8, which Kando could not name the same way again.
func CleanPath(p string) (string, error) {
	if strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\\x00") || !utf8.ValidString(p) {
		return "", fmt.Errorf("%q: %w", p, ErrInvalidPath)
	}
	parts := strings.Split(p, "/")
	out := parts[:0]
	for _, s := range parts {
		switch s {
		case "", ".":
		case "..":
			if len(out) == 0 {
				return "", fmt.Errorf("%q: %w", p, ErrInvalidPath)
			}
			out = out[:len(out)-1]
		default:
			out = append(out, s)
		}
	}
	return strings.Join(out, "/"), nil
}
