package mediasource

import "kando/packages/remotefs"

// Builtin is the built-in kind of ID id, whatever the Registry holds, reporting false for any other ID, as a plugin's or one this version does not know.
func Builtin(id string) (Kind, bool) {
	switch id {
	case KindSMB:
		return SMB(), true
	case KindSFTP:
		return SFTP(), true
	case KindWebDAV:
		return WebDAV(), true
	}
	return Kind{}, false
}

// Remote is how storage reaches s, a source of a built-in kind, with its secrets by key, reporting false for a source of another kind, as a plugin's.
func Remote(s Source, secrets map[string]string) (remotefs.Remote, bool) {
	switch s.Kind {
	case KindSMB:
		return SMBRemote(s, secrets[SMBPassword]), true
	case KindSFTP:
		return SFTPRemote(s, secrets), true
	case KindWebDAV:
		return WebDAVRemote(s, secrets[WebDAVPassword]), true
	}
	return remotefs.Remote{}, false
}

// SourceKey is the key a phone knows the file at ref, of one of sources, by (remotefs.SourceKey, docs/media-sources.md 10.5), size being its size: "" for a source not among them, or of a kind the phone does not reach itself, as a plugin's. No secret goes into it.
func SourceKey(sources []Source, ref Ref, size int64) string {
	for _, s := range sources {
		if s.ID != ref.Source {
			continue
		}
		r, ok := Remote(s, nil)
		if !ok {
			return ""
		}
		return remotefs.SourceKey(r, ref.Path, size)
	}
	return ""
}
