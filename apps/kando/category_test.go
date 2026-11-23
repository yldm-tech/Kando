package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/category"
	"kando/apps/kando/films"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// categoryLibrary is a library of two done films, 山间小站 and 第01话, each in a folder of its own with its bilingual subtitles, open on the library page.
func categoryLibrary(t *testing.T) (a *app, tt *nsgotest.App, station, episode string) {
	t.Helper()
	lib := filepath.Join(t.TempDir(), "日语学习.kando")
	a, tt, _ = newTestApp(t, &fakePipeline{})
	var jobs []core.Job
	for i, name := range []string{"山间小站", "第01话"} {
		video := filepath.Join(lib, name, "video.mp4")
		srt := filepath.Join(lib, name, "video.bilingual.srt")
		if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
			t.Fatal(err)
		}
		for path, body := range map[string]string{video: "v", srt: "1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n"} {
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		jobs = append(jobs, core.Job{ID: []string{"station", "episode"}[i], Title: name, Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: srt, LibraryDir: lib})
	}
	apply(tt, a, func() {
		a.library.Jobs = jobs
		a.switchLibrary(lib)
	})
	return a, tt, jobs[0].VideoPath, jobs[1].VideoPath
}

// shownFilms is the keys of the done films the library page shows, in its order.
func shownFilms(a *app) []string {
	var keys []string
	for _, item := range a.InCategories(films.Shown(a.films(), a, a.side.filter, a.SpokenFilter(), a.side.watch, a.side.query)) {
		keys = append(keys, item.Key)
	}
	slices.Sort(keys)
	return keys
}

// TestCategoriesSortFilmsWithoutMovingThem makes a category from the sidebar's +, puts a film in it from its menu, and shows the category's films, then 未分类's, and 全部 again: the films' files stay where they are, and the categories are kept in the library's folder.
func TestCategoriesSortFilmsWithoutMovingThem(t *testing.T) {
	a, tt, station, episode := categoryLibrary(t)
	if !tt.Exists("id:sidebar-categories") || !tt.Exists("id:category-empty") {
		t.Fatal("the library with done films shows no categories section, or no line saying what it is for")
	}

	tt.Press("id:category-add")
	refresh(tt, a)
	if !a.CategoryNaming().Open || !a.modalOpen() {
		t.Fatal("the + did not open the naming dialog holding the window")
	}
	typeFocused(t, tt, "动漫")
	tt.Submit("id:category-name-field")
	tt.Advance(time.Second)
	if a.CategoryNaming().Open || a.modalOpen() {
		t.Fatal("the dialog stays open after Enter")
	}
	cats := a.Categories()
	if len(cats) != 1 || cats[0].Name != "动漫" {
		t.Fatalf("categories = %+v", cats)
	}
	anime := cats[0].ID

	// A film's menu puts it in the category.
	tt.RightClick("id:film-episode")
	tt.ChoosePopup("分类", "动漫")
	refresh(tt, a)
	if !a.InCategory(episode, anime) || a.InCategory(station, anime) {
		t.Fatalf("after 分类 ▸ 动漫: episode %v, station %v", a.InCategory(episode, anime), a.InCategory(station, anime))
	}
	if got := tt.Find("id:category-count-" + anime).Text; got != "1" {
		t.Fatalf("动漫's row counts %q", got)
	}

	tt.Press("id:category-" + anime)
	refresh(tt, a)
	if got := shownFilms(a); !slices.Equal(got, []string{"episode"}) {
		t.Fatalf("动漫 shows %v", got)
	}
	if tt.Exists("id:film-station") || !tt.Exists("id:film-episode") {
		t.Fatal("the page does not show 动漫's film alone")
	}
	tt.Press("id:category-" + category.None)
	refresh(tt, a)
	if got := shownFilms(a); !slices.Equal(got, []string{"station"}) {
		t.Fatalf("未分类 shows %v", got)
	}
	tt.Press("id:category-" + category.All)
	refresh(tt, a)
	if got := shownFilms(a); len(got) != 2 {
		t.Fatalf("全部 shows %v", got)
	}

	for _, path := range []string{station, episode} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s moved: %v", path, err)
		}
	}
	book, err := category.Load(a.snap().LibraryDir)
	if err != nil || !book.In(episode, anime) {
		t.Fatalf("categories.json: %+v, %v", book, err)
	}
}

// TestCategoryNamesRenameMoveAndDelete refuses a name a category has, moves a film from one category into another while the page shows the first, renames a category, and deletes one after asking, its films staying in the library and the page showing every film again.
func TestCategoryNamesRenameMoveAndDelete(t *testing.T) {
	a, tt, station, episode := categoryLibrary(t)
	var films, anime string
	for _, name := range []string{"电影", "动漫"} {
		apply(tt, a, func() {
			a.AskNewCategory(station, episode)
			a.CategoryNaming().Name = name
			a.SubmitCategoryName()
		})
	}
	for _, c := range a.Categories() {
		switch c.Name {
		case "电影":
			films = c.ID
		case "动漫":
			anime = c.ID
		}
	}
	if films == "" || anime == "" || !a.InCategory(station, films) || !a.InCategory(episode, anime) {
		t.Fatalf("categories %+v", a.Categories())
	}

	// A name another category has is refused, the dialog staying open and saying why.
	apply(tt, a, func() {
		a.AskNewCategory()
		a.CategoryNaming().Name = "动漫"
		a.SubmitCategoryName()
	})
	if !a.CategoryNaming().Open || !tt.Exists("id:category-name-problem") {
		t.Fatal("a name taken was not refused")
	}
	apply(tt, a, a.CloseCategoryName)

	// While the page shows 电影, 移到 ▸ 动漫 takes the film out of it into 动漫.
	tt.Press("id:category-" + films)
	refresh(tt, a)
	tt.RightClick("id:film-station")
	tt.ChoosePopup("移到", "动漫")
	refresh(tt, a)
	if a.InCategory(station, films) || !a.InCategory(station, anime) {
		t.Fatalf("after 移到 ▸ 动漫: %v", a.cats.book.Of(station))
	}

	apply(tt, a, func() {
		a.AskRenameCategory(anime)
		a.CategoryNaming().Name = "アニメ"
		a.SubmitCategoryName()
	})
	if a.CategoryName(anime) != "アニメ" || !a.InCategory(episode, anime) {
		t.Fatalf("renamed: %q, its films %v", a.CategoryName(anime), a.InCategory(episode, anime))
	}

	tt.Press("id:category-" + anime)
	refresh(tt, a)
	apply(tt, a, func() { a.AskDeleteCategory(anime) })
	if !tt.Exists("id:category-delete") || !a.modalOpen() {
		t.Fatal("deleting a category does not ask first")
	}
	tt.Press("id:category-delete-confirm")
	refresh(tt, a)
	if _, ok := a.cats.book.Find(anime); ok || a.modalOpen() {
		t.Fatal("the category was not deleted, or the confirmation still holds the window")
	}
	if a.cats.filter != category.All || len(shownFilms(a)) != 2 || len(a.films()) != 2 {
		t.Fatalf("after deleting: filter %q, shown %v", a.cats.filter, shownFilms(a))
	}
}

// TestCategoriesFollowGatheredVideos puts a video dropped loose into the library in a category, then lets the refresh gather it into a folder of its own: it stays in the category by its new path. Another library starts with every category shown.
func TestCategoriesFollowGatheredVideos(t *testing.T) {
	a, tt, station, _ := categoryLibrary(t)
	lib := a.snap().LibraryDir
	loose := filepath.Join(lib, "雨夜.mp4")
	if err := os.WriteFile(loose, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	var anime string
	apply(tt, a, func() {
		a.AskNewCategory(loose, station)
		a.CategoryNaming().Name = "动漫"
		a.SubmitCategoryName()
		anime = a.Categories()[0].ID
		a.SetCategoryFilter(anime)
		a.refreshLibrary()
	})
	gathered := filepath.Join(lib, "雨夜", "video.mp4")
	if !a.InCategory(gathered, anime) || a.InCategory(loose, anime) {
		t.Fatalf("after gathering: %v", a.cats.book.Of(gathered))
	}
	if book, _ := category.Load(lib); !book.In(gathered, anime) {
		t.Fatal("categories.json does not follow the gathered video")
	}

	other := filepath.Join(t.TempDir(), "英语听力.kando")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.switchLibrary(other) })
	if a.cats.filter != category.All || len(a.Categories()) != 0 {
		t.Fatalf("another library: filter %q, categories %+v", a.cats.filter, a.Categories())
	}
}
