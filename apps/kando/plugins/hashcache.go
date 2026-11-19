package plugins

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// hashCacheName is the file in the plugins folder where Scan keeps the hash of each pack it validated, with what its files looked like (hashCache). Its name starts with a dot, so nothing takes it for a pack, a staging folder or a pack moved out of the way.
const hashCacheName = ".hashes.json"

// hashCacheFormat is the format of the cache. It goes up whenever what Validate reads a pack's files for changes (the hash, the limits, the checks of a dictionary's zips), so a pack a Kando before passed is read again in full rather than taken on trust.
const hashCacheFormat = 1

// hashCacheMax is the most bytes the cache file may have, far more than the packs of a plugins folder need; a larger one is ignored.
const hashCacheMax = 4 << 20

// racyWindow is how long after a file last changed Scan still reads it again: a file written in the same tick of the clock as it was hashed may change again without its time of change moving on, as the times of some file systems are only to the second or two, so its hash is kept only once it is older.
const racyWindow = 3 * time.Second

// hashCache is what Scan knows of the hashes of the packs of a plugins folder from the scan before (old), and what it learns this time (next), by the pack's folder relative to the plugins folder (<kind>/<id>, with slashes). Validating the packs every time Kando starts would read every file of every pack, a dictionary's up to a GiB unpacked, so a pack whose files are just as they were, by name, size and time of change (fileStamp), keeps the hash it had and its dictionary's zips are not read again; its manifest and the shape of its folder are still checked every time (validate). A pack changed in place in a way that keeps the size and the time of change of every file (as a program of the user's may, setting the times back) keeps its old hash until one of them changes; such a program can change state.json as well, so the cache makes nothing easier for it (docs/plugin-system.md 6.7).
type hashCache struct {
	root      string
	old, next map[string]cachedPack
	now       time.Time
}

// cachedPack is the hash of a pack's files as they were when it was taken.
type cachedPack struct {
	Hash  string      `json:"hash"`
	Files []fileStamp `json:"files"`
}

// fileStamp is what a file of a pack looked like: its name, size and time of change, in nanoseconds since 1970.
type fileStamp struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

// hashCacheFile is the cache's file: its format and the packs.
type hashCacheFile struct {
	Format int                   `json:"format"`
	Packs  map[string]cachedPack `json:"packs"`
}

// loadHashCache reads the cache of the plugins folder root. A cache that is missing, cannot be read or is of another format is empty, and every pack is then read in full.
func loadHashCache(root string, now time.Time) *hashCache {
	c := &hashCache{root: root, old: map[string]cachedPack{}, next: map[string]cachedPack{}, now: now}
	data, err := readSmall(filepath.Join(root, hashCacheName), hashCacheMax)
	if err != nil {
		return c
	}
	var f hashCacheFile
	if json.Unmarshal(data, &f) != nil || f.Format != hashCacheFormat {
		return c
	}
	for key, p := range f.Packs {
		if p.Hash != "" {
			c.old[key] = p
		}
	}
	return c
}

// key is the cache's key of the pack in dir, false for a folder outside the plugins folder.
func (c *hashCache) key(dir string) (string, bool) {
	rel, err := filepath.Rel(c.root, dir)
	if err != nil || !filepath.IsLocal(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// get is the hash of the pack in dir whose files are stamps, when the scan before took it of the same files, false otherwise. A hash it gives is kept for the next scan too.
func (c *hashCache) get(dir string, stamps []fileStamp) (string, bool) {
	key, ok := c.key(dir)
	if !ok {
		return "", false
	}
	p, ok := c.old[key]
	if !ok || !slices.Equal(p.Files, stamps) {
		return "", false
	}
	c.next[key] = p
	return p.Hash, true
}

// put keeps hash as that of the pack in dir whose files were stamps before they were read, unless a file changed too lately to be sure the next change will show in its stamp (racyWindow).
func (c *hashCache) put(dir string, stamps []fileStamp, hash string) {
	key, ok := c.key(dir)
	if !ok {
		return
	}
	for _, s := range stamps {
		if c.now.Sub(time.Unix(0, s.Modified)) < racyWindow {
			return
		}
	}
	c.next[key] = cachedPack{Hash: hash, Files: stamps}
}

// save writes the hashes this scan took or kept over the cache, when they are not those it holds: the packs gone, or no longer valid, are left out. It writes through a file beside it renamed over it, so a scan running at the same time reads either cache whole. A cache that cannot be written is left as it is: the next scan reads the packs in full again.
func (c *hashCache) save() {
	same := maps.EqualFunc(c.old, c.next, func(a, b cachedPack) bool { return a.Hash == b.Hash && slices.Equal(a.Files, b.Files) })
	if same {
		return
	}
	data, err := json.Marshal(hashCacheFile{Format: hashCacheFormat, Packs: c.next})
	if err != nil {
		return
	}
	if _, err := os.Stat(c.root); err != nil {
		return
	}
	writeFileAtomic(filepath.Join(c.root, hashCacheName), data)
}

// stampFiles is what the files names of the pack in dir look like now.
func stampFiles(dir string, names []string) ([]fileStamp, error) {
	stamps := make([]fileStamp, 0, len(names))
	for _, name := range names {
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		stamps = append(stamps, fileStamp{Name: name, Size: info.Size(), Modified: info.ModTime().UnixNano()})
	}
	return stamps, nil
}
