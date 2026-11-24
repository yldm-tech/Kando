package mediasource

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/kando/storage"
	"kando/packages/core"
	"kando/packages/media"
)

// SubtitleExts are the extensions of the subtitle files that go with a video on a source: a file beside it whose name is the video's without its extension, and maybe a language after a dot (Heat.srt, Heat.en.srt), is its subtitles, which the browse page marks on the video's row rather than listing apart, and which Copier copies with it.
var SubtitleExts = []string{".srt", ".ass", ".vtt"}

// IsSubtitles reports whether name is a subtitle file's (SubtitleExts), in any case.
func IsSubtitles(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	for _, s := range SubtitleExts {
		if ext == s {
			return true
		}
	}
	return false
}

// SubtitlesOf are the names among names that are subtitles of the video named video (SubtitleExts).
func SubtitlesOf(video string, names []string) []string {
	stem := strings.TrimSuffix(video, path.Ext(video))
	var out []string
	for _, name := range names {
		if !IsSubtitles(name) {
			continue
		}
		rest := strings.TrimSuffix(name, path.Ext(name))
		if rest == stem || (strings.HasPrefix(rest, stem+".") && !strings.Contains(rest[len(stem)+1:], ".")) {
			out = append(out, name)
		}
	}
	return out
}

// Host is the server s reaches, as offline mode judges it (Local) and a source without a name is named: an SMB or SFTP source's host, a WebDAV source's address's, a plugin's source's endpoint's.
func Host(s Source) string {
	if h := strings.TrimSpace(s.Config[SMBHost]); h != "" {
		return h
	}
	for _, key := range []string{WebDAVURL, EndpointKey} {
		if u, err := url.Parse(strings.TrimSpace(s.Config[key])); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	return ""
}

// copyProgressEvery is how often a copy reports how far it got.
const copyProgressEvery = 250 * time.Millisecond

// Copier copies the videos of the sources Pool holds into the library: it is the app's media.SourceReader. Offline reports whether offline mode is on, in which only a source on the local network is read (Local; LocalHost, when set, judges in its place, as a test's does).
type Copier struct {
	Pool      *Pool
	Offline   func() bool
	LocalHost func(ctx context.Context, host string) (bool, error)
}

// Copy copies the video at source, a source's address (Ref), into dir, the library's folder, where the source's films go (Placeholder: <dir>/<source name>/<path on the source>, numbered when that is taken, and a video at the root of the source in a folder of its own, ownFolder), with the subtitle files beside it on the source (SubtitlesOf) named after the copy, and returns its path. It reports how far it got to ctx's Trace (media.ReportProgress) with how fast. A source on the internet is refused in offline mode with media.ErrOffline. A copy that fails leaves nothing in the library (storage.Get).
func (c Copier) Copy(ctx context.Context, source, dir string) (string, error) {
	ref, err := ParseRef(source)
	if err != nil {
		return "", err
	}
	s, ok := c.Pool.Source(ref.Source)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSourceGone, ref.Source)
	}
	if c.Offline != nil && c.Offline() {
		local := c.LocalHost
		if local == nil {
			local = Local
		}
		if near, err := local(ctx, Host(s)); err != nil || !near {
			return "", media.ErrOffline
		}
	}
	d, err := c.Pool.Driver(ctx, ref.Source)
	if err != nil {
		return "", err
	}
	placeholder, err := Placeholder(dir, s.Name, ref.Path, func(p string) bool {
		if own := ownFolder(dir, p); own != p {
			_, err := os.Lstat(filepath.Dir(own))
			return err == nil
		}
		_, err := os.Lstat(p)
		return err == nil
	})
	if err != nil {
		return "", err
	}
	dest := ownFolder(dir, placeholder)
	start, last := time.Now(), time.Time{}
	progress := func(done, total int64) {
		now := time.Now()
		if total <= 0 || (now.Sub(last) < copyProgressEvery && done < total) {
			return
		}
		last = now
		p := core.Progress{Fraction: float64(done) / float64(total), Known: true}
		if secs := now.Sub(start).Seconds(); secs > 0 {
			p.Speed = float64(done) / secs
		}
		media.ReportProgress(ctx, p)
	}
	if err := storage.Get(ctx, StorageReader(d), ref.Path, dest, progress); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrNotExist) {
			// The connection may be what failed; the next copy makes a new one.
			c.Pool.Forget(ref.Source)
		}
		return "", err
	}
	c.copySubtitles(ctx, d, ref.Path, dest)
	return dest, nil
}

// ownFolder is where Copy puts the video Placeholder places at p in the library dir: a video at the root of its source, which Placeholder places loose in the source's folder, goes in a folder of its own named as the video, as video.<ext>, as the library keeps a film. Loose there it may be the folder's one video, which the library's watcher renames to video.<ext> (media.GatherVideos) while the job that copied it is still making the film from the name it was copied to. A video deeper on the source is where Placeholder places it, in a folder the watcher does not look into.
func ownFolder(dir, p string) string {
	folder := filepath.Dir(p)
	if filepath.Dir(folder) != filepath.Clean(dir) {
		return p
	}
	name := filepath.Base(p)
	ext := filepath.Ext(name)
	if ext == name {
		ext = ""
	}
	return filepath.Join(folder, strings.TrimSuffix(name, ext), "video"+ext)
}

// copySubtitles copies the subtitle files beside p on the source next to the copy at dest, each named as the copy is, with its own language and extension after it. One that cannot be read or is already there is left out: the film is made all the same.
func (c Copier) copySubtitles(ctx context.Context, d Driver, p, dest string) {
	dir := path.Dir(p)
	if dir == "." {
		dir = ""
	}
	entries, err := d.List(ctx, dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.Dir {
			names = append(names, e.Name)
		}
	}
	video := path.Base(p)
	stem := strings.TrimSuffix(video, path.Ext(video))
	destStem := strings.TrimSuffix(dest, filepath.Ext(dest))
	for _, name := range SubtitlesOf(video, names) {
		local := destStem + strings.TrimPrefix(name, stem)
		_ = storage.Get(ctx, StorageReader(d), path.Join(dir, name), local, nil)
	}
}
