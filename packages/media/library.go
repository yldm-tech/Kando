package media

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Video struct {
	Path    string
	Name    string
	ModTime int64
}

func ListVideos(directory string) ([]Video, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	videos := make([]Video, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !isVideoFile(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		videos = append(videos, Video{Path: filepath.Join(directory, entry.Name()), Name: entry.Name(), ModTime: info.ModTime().UnixNano()})
	}
	sort.SliceStable(videos, func(i, j int) bool {
		if videos[i].ModTime == videos[j].ModTime {
			return videos[i].Name < videos[j].Name
		}
		return videos[i].ModTime > videos[j].ModTime
	})
	return videos, nil
}

// IsVideoFile reports whether name has an extension the library lists.
func IsVideoFile(name string) bool { return isVideoFile(name) }

func isVideoFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".flv":
		return true
	default:
		return false
	}
}
