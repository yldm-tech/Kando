// Package cache removes what Kando keeps on this computer that it can make again, as Settings › 隐私 › 清除缓存 does: the site thumbnails and cover frames of the posters folder and the copies of videos made for playback. It never touches a library: subtitles, videos and what the user changed stay.
package cache

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Clear removes every file under each of dirs, except the paths keep names, as a copy the player is playing, and the files whose paths begin with one of them, as the part of that copy ffmpeg is still writing, and the folders left empty, and returns how many bytes it removed. A folder that is not there is skipped. It goes on past a file it cannot remove and returns the first such error with the bytes it did remove.
func Clear(dirs []string, keep ...string) (freed int64, err error) {
	var kept []string
	for _, path := range keep {
		if path != "" {
			kept = append(kept, filepath.Clean(path))
		}
	}
	keeps := func(path string) bool {
		path = filepath.Clean(path)
		for _, k := range kept {
			if strings.HasPrefix(path, k) {
				return true
			}
		}
		return false
	}
	var first error
	fail := func(e error) {
		if first == nil {
			first = e
		}
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		var folders []string
		walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					fail(err)
				}
				return nil
			}
			if d.IsDir() {
				if path != dir {
					folders = append(folders, path)
				}
				return nil
			}
			if keeps(path) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				fail(err)
				return nil
			}
			if err := os.Remove(path); err != nil {
				fail(err)
				return nil
			}
			freed += info.Size()
			return nil
		})
		if walkErr != nil {
			fail(walkErr)
		}
		// The deepest folders go first, so a folder emptied of folders goes too; one that still holds a kept file stays.
		for i := len(folders) - 1; i >= 0; i-- {
			_ = os.Remove(folders[i])
		}
	}
	return freed, first
}
