package plugins

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// InboxName is the folder of root the command line drops a pack file into (`kando plugin install`, Drop), for the app to offer it in its install confirmation: nothing outside the app installs a pack, as only the app shows what a pack is before it is installed (docs/plugin-system.md 6.5). Scan does not read it, as it is no kind's folder.
const InboxName = "inbox"

// Drop copies the pack file src, a zip, into root's inbox as a new file named after it with a random prefix, so two drops of the same name never meet, and returns the copy. It is copied through a hidden file renamed into place, so the app never reads half of it. src must be a regular file no larger than the largest zip Stage takes; Drop does not validate it, which the caller does first (Stage into a folder of its own) and the app does again as it stages the copy.
func Drop(root, src string) (string, error) {
	in, err := openRegular(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return "", err
	}
	if max := limitsOf(KindDictionary).zip; info.Size() > max {
		return "", fmt.Errorf("%w: the file is over %d MiB", ErrTooLarge, max>>20)
	}
	inbox := filepath.Join(root, InboxName)
	if err := os.MkdirAll(inbox, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(inbox, ".drop-*")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(tmp, in)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	dst := filepath.Join(inbox, randomName("")+"-"+inboxBase(src))
	if err == nil {
		err = os.Rename(tmp.Name(), dst)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return dst, nil
}

// inboxBaseMax is the most bytes inboxBase keeps of a name, so the inbox's file, its random prefix before it, stays under the 255 bytes most file systems allow a name.
const inboxBaseMax = 200

// inboxBase is the name of src as a file of the inbox may end in: its base name, with what is not a letter, a digit, a dot, a hyphen or an underscore in ASCII made an underscore, so a file name never carries a path or a character a file system refuses, cut to inboxBaseMax bytes. A name cut keeps its extension, unless the extension is most of it. Every character is one byte once mapped, so the cut never splits one.
func inboxBase(src string) string {
	name := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			return r
		}
		return '_'
	}, filepath.Base(src))
	if len(name) <= inboxBaseMax {
		return name
	}
	ext := filepath.Ext(name)
	if len(ext) > inboxBaseMax/2 {
		ext = ""
	}
	return name[:inboxBaseMax-len(ext)] + ext
}

// Inbox is the files waiting in root's inbox (Drop), oldest first, by when they were dropped; files being dropped (hidden ones) and anything that is not a regular file are left out. An inbox that does not exist holds none.
func Inbox(root string) ([]string, error) {
	dir := filepath.Join(root, InboxName)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	type dropped struct {
		path string
		at   int64
	}
	var files []dropped
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, dropped{filepath.Join(dir, e.Name()), info.ModTime().UnixNano()})
	}
	slices.SortStableFunc(files, func(a, b dropped) int {
		if a.at != b.at {
			return int(min(max(a.at-b.at, -1), 1))
		}
		return strings.Compare(a.path, b.path)
	})
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.path
	}
	return out, nil
}
