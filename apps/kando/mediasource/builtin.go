package mediasource

import "kando/apps/kando/storage"

// Builtin is the built-in kind of ID id, whatever the Registry holds, reporting false for any other ID, as a plugin's or one this version does not know.
func Builtin(id string) (Kind, bool) {
	switch id {
	case KindSMB:
		return SMB(), true
	case KindSFTP:
		return SFTP(), true
	}
	return Kind{}, false
}

// Remote is how storage reaches s, a source of a built-in kind, with its secrets by key, reporting false for a source of another kind, as a plugin's.
func Remote(s Source, secrets map[string]string) (storage.Remote, bool) {
	switch s.Kind {
	case KindSMB:
		return SMBRemote(s, secrets[SMBPassword]), true
	case KindSFTP:
		return SFTPRemote(s, secrets), true
	}
	return storage.Remote{}, false
}
