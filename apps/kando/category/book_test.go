package category

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestBookKeepsCategoriesInTheLibrary makes categories, puts films in them, inside the library and kept outside it, and reads them back from the library's folder.
func TestBookKeepsCategoriesInTheLibrary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "日语学习.kando")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := Load(dir)
	if err != nil || len(b.Categories) != 0 {
		t.Fatalf("a library with no file: %+v, %v", b, err)
	}
	films, err := b.Add(" 电影 ")
	if err != nil || films.Name != "电影" {
		t.Fatalf("add: %+v, %v", films, err)
	}
	anime, _ := b.Add("动漫")
	inside := filepath.Join(dir, "山间小站", "video.mp4")
	outside := filepath.Join(t.TempDir(), "番剧", "第01话.mkv")
	if !b.Put(films.ID, true, inside) || !b.Put(anime.ID, true, outside, inside) {
		t.Fatal("putting films in categories changed nothing")
	}
	if b.Put(films.ID, true, inside) {
		t.Fatal("putting a film in a category it is in changed something")
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	read, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(read.Categories, b.Categories) {
		t.Fatalf("categories = %+v, want %+v", read.Categories, b.Categories)
	}
	if got := read.Of(inside); !slices.Equal(got, []string{films.ID, anime.ID}) {
		t.Fatalf("inside is in %v", got)
	}
	if !read.In(outside, anime.ID) || read.In(outside, films.ID) {
		t.Fatalf("outside is in %v", read.Of(outside))
	}
	if read.Key(inside) != "山间小站/video.mp4" || read.Key(outside) != outside {
		t.Fatalf("keys %q %q", read.Key(inside), read.Key(outside))
	}
}

// TestBookNamesAreOneEach refuses an empty name and a name another category has, whatever its case, and lets a category keep its own name as it is renamed.
func TestBookNamesAreOneEach(t *testing.T) {
	b, _ := Load(t.TempDir())
	anime, _ := b.Add("Anime")
	if _, err := b.Add("  "); !errors.Is(err, ErrNameEmpty) {
		t.Fatalf("an empty name: %v", err)
	}
	if _, err := b.Add("anime"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("a name taken: %v", err)
	}
	films, _ := b.Add("Films")
	if err := b.Rename(films.ID, "ANIME"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("renamed to a name taken: %v", err)
	}
	if err := b.Rename(anime.ID, "anime"); err != nil {
		t.Fatalf("renamed to its own name in another case: %v", err)
	}
	if c, _ := b.Find(anime.ID); c.Name != "anime" {
		t.Fatalf("renamed: %+v", c)
	}
}

// TestBookDeleteAndMove deletes a category, its films staying in the others, and moves films from one category to another, their other categories staying.
func TestBookDeleteAndMove(t *testing.T) {
	dir := t.TempDir()
	b, _ := Load(dir)
	films, _ := b.Add("电影")
	anime, _ := b.Add("动漫")
	docs, _ := b.Add("纪录片")
	a, c := filepath.Join(dir, "a", "video.mp4"), filepath.Join(dir, "c", "video.mp4")
	b.Put(films.ID, true, a, c)
	b.Put(docs.ID, true, a)
	if !b.Move(films.ID, anime.ID, a) {
		t.Fatal("moving changed nothing")
	}
	if got := b.Of(a); !slices.Equal(got, []string{docs.ID, anime.ID}) {
		t.Fatalf("a is in %v", got)
	}
	b.Delete(films.ID)
	if _, ok := b.Find(films.ID); ok || len(b.Of(c)) != 0 {
		t.Fatalf("deleted: c is in %v", b.Of(c))
	}
	if b.Put(films.ID, true, c) {
		t.Fatal("a film was put in a deleted category")
	}
}

// TestBookFollowsMovedVideos keys a film by its video's new path when the shelf follows it, as after the library refresh gathered it into a folder of its own, and leaves a film whose video did not move.
func TestBookFollowsMovedVideos(t *testing.T) {
	dir := t.TempDir()
	b, _ := Load(dir)
	anime, _ := b.Add("动漫")
	loose, stays := filepath.Join(dir, "第01话.mkv"), filepath.Join(dir, "第02话", "video.mkv")
	b.Put(anime.ID, true, loose, stays)
	gathered := filepath.Join(dir, "第01话", "video.mkv")
	moved := b.MoveVideos(func(path string) (string, bool) {
		if path == loose {
			return gathered, true
		}
		return "", false
	})
	if !moved || !b.In(gathered, anime.ID) || b.In(loose, anime.ID) || !b.In(stays, anime.ID) {
		t.Fatalf("moved %v: gathered %v, loose %v, stays %v", moved, b.Of(gathered), b.Of(loose), b.Of(stays))
	}
}

// TestBookLeavesAFileItCannotReadAlone reads a file that is not a book as an error, and then writes nothing over it.
func TestBookLeavesAFileItCannotReadAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(dir)
	if err == nil || b.Unread() == nil {
		t.Fatal("a damaged file read as a book")
	}
	b.Add("电影")
	if err := b.Save(); !errors.Is(err, ErrUnread) {
		t.Fatalf("save: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{not json" {
		t.Fatalf("the file was written over: %q", data)
	}
}
