package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// dropFixture is an empty library, open, and beside it, outside it, a folder of the user's named 动漫 that holds n episodes, a season folder of them, with a program named as an episode is among them.
func dropFixture(t *testing.T, n int) (a *app, tt *nsgotest.App, updates chan func(), show string, episodes []string) {
	t.Helper()
	root := t.TempDir()
	lib := filepath.Join(root, "日语学习.kando")
	if err := os.Mkdir(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	show = filepath.Join(root, "动漫")
	season := filepath.Join(show, "第一季")
	if err := os.MkdirAll(season, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		path := filepath.Join(season, fmt.Sprintf("第%02d话.mp4", i))
		if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
		episodes = append(episodes, path)
	}
	if err := os.WriteFile(filepath.Join(season, "第99话.mp4.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates = newTestApp(t, &fakePipeline{block: true})
	apply(tt, a, func() { a.switchLibrary(lib) })
	return a, tt, updates, show, episodes
}

// addedFilms is the open library's films that were added without a job, by their videos' paths, in the order the shelf lists them.
func addedFilms(a *app) []string {
	var paths []string
	for _, item := range a.films() {
		if !item.Added.IsZero() && item.JobID == "" {
			paths = append(paths, item.VideoPath)
		}
	}
	return paths
}

// TestALargeFolderIsOnlyAddedUnlessAsked drops a folder of 25 episodes, more than the window queues without asking: the sheet says how many it found and names the program among them, and its default adds the films to the library where they are, in a category named after the folder, queueing nothing and touching no file. Dropped again, it asks nothing, as nothing in it is new.
func TestALargeFolderIsOnlyAddedUnlessAsked(t *testing.T) {
	a, tt, updates, show, episodes := dropFixture(t, 25)
	tt.DropFiles("id:main-column", fileURLs(show)...)
	dropRead(t, tt, a, updates)
	if !a.drop.open || !tt.Exists("id:drop-sheet") || !a.modalOpen() {
		t.Fatal("25 videos dropped did not ask first")
	}
	l := a.dropLook()
	if l.Folder != "动漫" || l.Videos != 25 || l.New != 25 || l.Disguised != 1 || l.Named[0] != "第99话.mp4.exe" || !l.Categories {
		t.Fatalf("look %+v", l)
	}
	if !tt.Exists("id:drop-disguised") || !tt.Exists("id:drop-category") {
		t.Fatal("the sheet does not name the program or offer the category")
	}
	if len(a.library.Jobs) != 0 {
		t.Fatalf("jobs queued before asking: %d", len(a.library.Jobs))
	}

	tt.Press("id:drop-add")
	refresh(tt, a)
	if a.drop.open || a.modalOpen() {
		t.Fatal("the sheet stays open")
	}
	if len(a.library.Jobs) != 0 || a.running() {
		t.Fatalf("only adding queued %d jobs", len(a.library.Jobs))
	}
	added := addedFilms(a)
	if len(added) != 25 || !slices.Contains(added, episodes[0]) {
		t.Fatalf("added %d films: %q", len(added), added)
	}
	for _, path := range episodes {
		if body, err := os.ReadFile(path); err != nil || string(body) != "video" {
			t.Fatalf("%s changed: %v", path, err)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(episodes[0]))
	if len(entries) != 26 {
		t.Fatalf("the folder now holds %d files", len(entries))
	}
	var anime string
	for _, c := range a.Categories() {
		if c.Name == "动漫" {
			anime = c.ID
		}
	}
	if anime == "" || !a.InCategory(episodes[7], anime) {
		t.Fatalf("categories %+v", a.Categories())
	}
	m := testlocale.Messages()
	if want := m.LibraryDropDisguised(m.LibraryAddedOnly(25), "第99话.mp4.exe", 1); a.lib.notice.Text != want {
		t.Fatalf("notice %q, want %q", a.lib.notice.Text, want)
	}
	// library.json keeps them, so they are there after a restart.
	if got := len(a.library.AddedIn(a.snap().LibraryDir)); got != 25 {
		t.Fatalf("library.json keeps %d", got)
	}

	// Dropped again, the folder holds nothing new and nothing without subtitles that it would make: it asks nothing and queues nothing it has.
	a.lib.notice = films.Notice{}
	tt.DropFiles("id:main-column", fileURLs(show)...)
	dropRead(t, tt, a, updates)
	if !a.drop.open {
		t.Fatal("dropped again, the 25 films without subtitles would be made without asking")
	}
	if l := a.dropLook(); l.New != 0 || l.Unmade != 25 || l.Adds(false) != 0 || l.Adds(true) != 25 {
		t.Fatalf("look again %+v", l)
	}
	apply(tt, a, a.dropClose)
}

// TestMakingAllOfALargeFolderGoesBackWhenStopped chooses 全部生成字幕 for a large folder: each film is added and then queued as its 生成字幕 queues it, so one stopped goes back to being a film without subtitles rather than a failed card; once a film's job completes, the job holds the video and it is no longer among those added.
func TestMakingAllOfALargeFolderGoesBackWhenStopped(t *testing.T) {
	a, tt, updates, show, episodes := dropFixture(t, 22)
	a.dropFiles([]string{show})
	dropRead(t, tt, a, updates)
	tt.Press("id:drop-generate")
	refresh(tt, a)
	if !a.drop.generate {
		t.Fatal("全部生成字幕 is not chosen")
	}
	tt.Press("id:drop-add")
	refresh(tt, a)
	if len(a.library.Jobs) != 22 {
		t.Fatalf("queued %d jobs", len(a.library.Jobs))
	}
	for _, job := range a.library.Jobs {
		if !job.FromFolder || !slices.Contains(episodes, job.VideoPath) {
			t.Fatalf("job %+v is not the film's 生成字幕", job)
		}
	}
	var waiting core.Job
	for _, job := range a.library.Jobs {
		if job.ID != a.queue.active && job.Stage == core.StageQueued {
			waiting = job
			break
		}
	}
	apply(tt, a, func() { a.Cancel(waiting.ID) })
	if _, ok := a.library.Find(waiting.ID); ok {
		t.Fatal("the stopped film stays a failed card")
	}
	item, ok := films.Find(a.films(), waiting.VideoPath)
	if !ok || item.JobID != "" || item.Added.IsZero() {
		t.Fatalf("the stopped film is not back as it was: %+v %v", item, ok)
	}

	done := a.library.Jobs[0]
	done.Stage, done.SubtitlePath = core.StageComplete, done.VideoPath+".bilingual.srt"
	apply(tt, a, func() { a.show(done) })
	if _, ok := a.library.AddedIn(a.snap().LibraryDir)[done.VideoPath]; ok {
		t.Fatal("a film whose job completed is still among those added")
	}
}

// TestAFilmAddedWithoutAJobLeavesItsFileWhenRemoved removes a film added without a job: its file stays where it is and it does not come back once the library is read again. One whose file is gone stays on the shelf, marked; one named video.mp4 beside the user's own cover.jpg gets its poster elsewhere.
func TestAFilmAddedWithoutAJobLeavesItsFileWhenRemoved(t *testing.T) {
	a, tt, updates, show, episodes := dropFixture(t, 21)
	a.dropFiles([]string{show})
	dropRead(t, tt, a, updates)
	apply(tt, a, a.dropAdd)
	item, ok := films.Find(a.films(), episodes[0])
	if !ok {
		t.Fatal("the film was not added")
	}
	apply(tt, a, func() { a.TrashFilms(item) })
	apply(tt, a, a.refreshLibrary)
	if _, ok := films.Find(a.films(), episodes[0]); ok {
		t.Fatal("the film removed came back")
	}
	if _, err := os.Stat(episodes[0]); err != nil {
		t.Fatalf("its file went: %v", err)
	}

	if err := os.Remove(episodes[1]); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, a.refreshLibrary)
	if gone, ok := films.Find(a.films(), episodes[1]); !ok || !gone.Missing {
		t.Fatalf("a film whose file is gone: %+v %v", gone, ok)
	}

	own := filepath.Join(show, "Extra", "video.mp4")
	if got := posterDest(a.lib.posterDir, own); got == filepath.Join(filepath.Dir(own), "cover.jpg") {
		t.Fatal("the poster of a video.mp4 of the user's would be written over the cover.jpg beside it")
	}
}

// TestAFewVideosDroppedStillQueueAtOnce drops a folder of 3 episodes and a program named as one: they are queued at once as they always were, and the message bar names the program it left out.
func TestAFewVideosDroppedStillQueueAtOnce(t *testing.T) {
	a, tt, updates, show, _ := dropFixture(t, 3)
	a.dropFiles([]string{show})
	dropRead(t, tt, a, updates)
	if a.drop.open || len(a.library.Jobs) != 3 {
		t.Fatalf("sheet %v, %d jobs", a.drop.open, len(a.library.Jobs))
	}
	m := testlocale.Messages()
	if want := m.LibraryDropDisguised(m.LibraryAddedStarting(3), "第99话.mp4.exe", 1); a.lib.notice.Text != want {
		t.Fatalf("notice %q, want %q", a.lib.notice.Text, want)
	}
}

// TestAFolderReadAsAnotherLibraryOpensAddsNothing switches to another library while the folder dropped is read: nothing is added to either.
func TestAFolderReadAsAnotherLibraryOpensAddsNothing(t *testing.T) {
	a, tt, updates, show, _ := dropFixture(t, 30)
	a.dropFiles([]string{show})
	other := filepath.Join(t.TempDir(), "英语听力.kando")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	a.switchLibrary(other)
	dropRead(t, tt, a, updates)
	if a.drop.open || len(a.library.Added) != 0 || len(a.library.Jobs) != 0 {
		t.Fatalf("sheet %v, added %v", a.drop.open, a.library.Added)
	}
}
