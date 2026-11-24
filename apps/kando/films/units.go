package films

import (
	"os"
	"path/filepath"
	"slices"

	"kando/packages/media"
)

// Unit is what moving films to another library or to the Trash takes at once: a film, or every film of a folder of the library when they all go and it holds no folder (Folder), which then goes whole, as a film alone in its folder does (AloneIn); otherwise each film's files go on their own, and the folder stays.
type Unit struct {
	Items  []Film
	Folder string
}

// Units is items, films of the library at root, as the units they move or go to the Trash in, in the order of items: the films of a folder of root whose every video is among items, and that holds no folder, go together with it, each other film alone (AloneAmong, away being the films whose videos are not on disk here). A film whose video is outside root goes alone and without its files.
func Units(root string, items []Film, away []string) []Unit {
	byDir := map[string][]string{}
	for _, item := range items {
		if item.VideoPath != "" && InDir(root, item.VideoPath) {
			dir := filepath.Dir(item.VideoPath)
			byDir[dir] = append(byDir[dir], item.VideoPath)
		}
	}
	whole := map[string]bool{}
	for dir, videos := range byDir {
		whole[dir] = InDir(root, dir) && AloneAmong(dir, away, videos...)
	}
	var out []Unit
	at := map[string]int{}
	for _, item := range items {
		dir := filepath.Dir(item.VideoPath)
		if item.VideoPath == "" || !InDir(root, item.VideoPath) || !whole[dir] {
			out = append(out, Unit{Items: []Film{item}})
			continue
		}
		if i, ok := at[dir]; ok {
			out[i].Items = append(out[i].Items, item)
			continue
		}
		at[dir] = len(out)
		out = append(out, Unit{Items: []Film{item}, Folder: dir})
	}
	return out
}

// AloneIn reports whether videos are the only films in dir and dir has no folder inside it; a sound of the name of one of them is its own (media.FilmOf). The shelf lists only the videos directly in a film's folder, so a folder inside may hold videos, or anything else, the user never saw there; the folder then stays, and only the films' files go.
func AloneIn(dir string, videos ...string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return false
		}
		if film, ok := media.FilmOf(filepath.Join(dir, entry.Name())); ok && !slices.Contains(videos, film) {
			return false
		}
	}
	return true
}

// AloneAmong is AloneIn, and also that dir holds none of away other than videos: the paths of the films whose videos are not on disk here, moved to a storage location or indexed from a media source, whose subtitles and the files beside them are in dir all the same, though no video of theirs is there for AloneIn to see.
func AloneAmong(dir string, away []string, videos ...string) bool {
	for _, p := range away {
		if filepath.Dir(p) == filepath.Clean(dir) && !slices.Contains(videos, p) {
			return false
		}
	}
	return AloneIn(dir, videos...)
}

// Name is the whole name of a film, as the page and the message bar call it: the title given to it, else the one it is listed by.
func Name(item Film) string { return filmName(item) }
