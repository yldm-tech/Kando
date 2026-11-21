package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// writeFiles writes each file of files, by path, with its body, making its folder.
func writeFiles(t *testing.T, files map[string]string) {
	t.Helper()
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// newSeriesFixture is newMoveFixture with a series in 我的影片 too: Course, a folder of two episodes no job processed, each with its subtitles, and a notes file beside them.
func newSeriesFixture(t *testing.T) (f *moveFixture, ep1, ep2 string) {
	t.Helper()
	f = newMoveFixture(t)
	ep1, ep2 = filepath.Join(f.from, "Course", "ep1.mp4"), filepath.Join(f.from, "Course", "ep2.mp4")
	writeFiles(t, map[string]string{
		ep1: "one", ep2: "two",
		filepath.Join(f.from, "Course", "ep1.bilingual.srt"): "1\n00:00:01,000 --> 00:00:02,000\n一。\nOne.\n\n",
		filepath.Join(f.from, "Course", "notes.txt"):         "notes",
	})
	f.a.refreshLibrary()
	refresh(f.tt, f.a)
	return f, ep1, ep2
}

// TestMoveSeveralFilmsAtOnce checks 移到资料库 for several films selected, within one volume: a series whose every episode is selected moves with its folder, the notes in it too, rather than an episode a folder; the film beside it moves too; the message bar says once how many moved and offers Undo, which brings them all back.
func TestMoveSeveralFilmsAtOnce(t *testing.T) {
	f, ep1, ep2 := newSeriesFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	items := []films.Film{must(films.Find(a.films(), ep1)), must(films.Find(a.films(), ep2)), must(films.Find(a.films(), "film"))}
	a.MoveFilms(items, f.to)
	refresh(tt, a)
	for _, path := range []string{filepath.Join(f.to, "Course", "ep1.mp4"), filepath.Join(f.to, "Course", "ep2.mp4"), filepath.Join(f.to, "Course", "notes.txt"), filepath.Join(f.to, "Talk", "video.mp4")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("not moved: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(f.from, "Course")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the series' folder stayed: %v", err)
	}
	if n := a.lib.notice; n.Text != m.LibraryMovedMany(3, "英语听力") || !n.Undo || n.Library != f.to {
		t.Fatalf("the bar says %+v", n)
	}
	if len(a.films()) != 0 {
		t.Fatalf("the moved films are still on the shelf: %+v", a.films())
	}
	f.wantKept(t, filepath.Join(f.to, "Talk", "video.mp4"))
	a.UndoMove()
	refresh(tt, a)
	for _, path := range []string{ep1, ep2, filepath.Join(f.from, "Course", "notes.txt"), f.video} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Undo did not bring it back: %v", err)
		}
	}
	if len(a.films()) != 3 {
		t.Fatalf("after Undo the shelf holds %d films", len(a.films()))
	}
	f.wantKept(t, f.video)
	wantLines(t, f.from, f.video, f.to)

	// Of a series, an episode moves on its own and its folder stays, with the other.
	a.MoveFilms([]films.Film{must(films.Find(a.films(), ep2))}, f.to)
	// The folder keeps the other episode, which the library then files as the folder's video (media.GatherVideos).
	if entries, _ := os.ReadDir(filepath.Join(f.from, "Course")); len(entries) != 3 || len(a.films()) != 2 {
		t.Fatalf("the episode left behind went too: the folder holds %v and the shelf %d films", entries, len(a.films()))
	}
	if _, err := os.Stat(filepath.Join(f.to, "ep2", "ep2.mp4")); err != nil {
		t.Fatalf("the episode is not in a folder of its own in the other library: %v", err)
	}
}

// TestMoveSeveralFilmsOneAfterAnotherAcrossVolumes checks several films copied to another volume: one at a time, the message bar saying which of how many; 取消 stops the copy and the films after it; a film being processed stays where it is while the others move, and the bar says how many moved and why it did not.
func TestMoveSeveralFilmsOneAfterAnotherAcrossVolumes(t *testing.T) {
	prev, prevCopy := libraryMoveRename, libraryMoveCopyFiles
	libraryMoveRename = func(media.FilmMove) error { return errors.New("cross-device link") }
	t.Cleanup(func() { libraryMoveRename, libraryMoveCopyFiles = prev, prevCopy })
	f, ep1, _ := newSeriesFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	talk, episode := must(films.Find(a.films(), "film")), must(films.Find(a.films(), ep1))

	copies := 0
	libraryMoveCopyFiles = func(_ media.FilmMove, ctx context.Context, _ func(done, total int64)) error {
		copies++
		<-ctx.Done()
		return ctx.Err()
	}
	a.MoveFilms([]films.Film{talk, episode}, f.to)
	refresh(tt, a)
	wantText(t, tt, m.LibraryMovingMany(1, 2, "英语听力", 0))
	if menu := a.libraryMoveItems(a.cx, []films.Film{talk}, func(fn func()) func() { return fn }); len(menu) == 0 || !menu[0].Disabled {
		t.Fatalf("another library can be chosen while films move: %+v", menu)
	}
	tt.Press("id:notice-stop")
	waitUntil(t, tt, a, f.updates, func() bool { return a.lib.move.run == nil })
	if copies != 1 {
		t.Fatalf("%d copies began, want the stopped one alone", copies)
	}
	if n := a.lib.notice; n.Text != m.LibraryMoveStoppedMany(0, "英语听力") || n.Undo {
		t.Fatalf("after 取消 the bar says %+v", n)
	}
	for _, path := range []string{f.video, ep1} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("a stopped move took %s: %v", path, err)
		}
	}

	// The talk is being processed: the episode moves on its own.
	libraryMoveCopyFiles = prevCopy
	job, _ := a.library.Find("film")
	job.Stage = core.StageTranslating
	a.library.Upsert(job)
	a.MoveFilms([]films.Film{talk, episode}, f.to)
	waitUntil(t, tt, a, f.updates, func() bool { return a.lib.move.run == nil })
	if _, err := os.Stat(filepath.Join(f.to, "ep1", "ep1.mp4")); err != nil {
		t.Fatalf("the episode did not move: %v", err)
	}
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("the film being processed moved: %v", err)
	}
	if n := a.lib.notice; n.Kind != "warning" || !n.Undo || n.Text != m.LibraryMovedSome(1, 2, "英语听力", m.LibraryMoveProcessing(moveName)) {
		t.Fatalf("after a move of which one film stayed the bar says %+v", n)
	}
}

// TestTrashSeveralFilms checks 从资料库移除 for several films selected: the confirmation counts them, a series whose every episode is selected goes to the Trash with its folder, a film that cannot go stays while the others go, and the message bar says once how many went and why the first that stayed did.
func TestTrashSeveralFilms(t *testing.T) {
	f, ep1, ep2 := newSeriesFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	var trashed []string
	prev := trashItem
	trashItem = func(_ *ns.Scope, path string, done func(error)) {
		if strings.Contains(path, "Talk") {
			done(errors.New("not trashed"))
			return
		}
		trashed = append(trashed, path)
		done(os.RemoveAll(path))
	}
	t.Cleanup(func() { trashItem = prev })
	items := []films.Film{must(films.Find(a.films(), ep1)), must(films.Find(a.films(), ep2)), must(films.Find(a.films(), "film"))}
	a.AskRemove(items...)
	refresh(tt, a)
	wantText(t, tt, m.FilmsRemoveConfirmMany(3))
	tt.Press("id:remove-confirm-trash")
	if want := []string{filepath.Join(f.from, "Course")}; !slices.Equal(trashed, want) {
		t.Fatalf("moved %q to the Trash, want the series' folder alone", trashed)
	}
	if n := a.lib.notice; n.Kind != "warning" || n.Text != m.LibraryTrashedSome(2, 3, m.LibraryTrashFailed(moveName)) {
		t.Fatalf("the bar says %+v", n)
	}
	if got := films.Keys(a.films()); !slices.Equal(got, []string{"film"}) {
		t.Fatalf("the shelf holds %q, want the film that stayed", got)
	}
}

// TestReprocessAndTranslateSeveralFilms checks 重新处理 and 重新翻译为 for several films selected: they are queued one after another without the window leaving the page it shows, the message bar saying once how many; translating them into another language than theirs asks once, for them all; and without a language to translate into the bar says why once.
func TestReprocessAndTranslateSeveralFilms(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	m := testlocale.Messages()
	dir := filepath.Join(t.TempDir(), "我的影片.kando")
	a.edit(func(s *session) { s.LibraryDir = dir })
	for _, id := range []string{"one", "two"} {
		video := filepath.Join(dir, id, "video.mp4")
		writeFiles(t, map[string]string{video: id, filepath.Join(dir, id, "video.bilingual.srt"): ""})
		a.library.Upsert(core.Job{ID: id, Source: "https://youtu.be/" + id, Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: filepath.Join(dir, id, "video.bilingual.srt"), LibraryDir: dir})
	}
	a.refreshLibrary()
	a.queue.stopped = true
	a.page = pageYouTube
	refresh(tt, a)
	items := []films.Film{must(a.JobFilm("one")), must(a.JobFilm("two"))}

	a.TranslateFilmsAgain(items, "日本語")
	if open, asked, language := a.Retranslating(); !*open || len(asked) != 2 || language != "日本語" {
		t.Fatalf("translating into another language asks %v about %d films in %q", *open, len(asked), language)
	}
	refresh(tt, a)
	wantText(t, tt, m.FilmsRetranslateConfirmMany(2, m.SettingsLanguageJapanese()))
	tt.Press("id:retranslate-confirm-run")
	for _, id := range []string{"one", "two"} {
		if job, _ := a.library.Find(id); job.Stage != core.StageQueued || job.Redo != core.RedoTranslation || job.RedoLanguage != "日本語" {
			t.Fatalf("%s is %s %s %s, want queued to be translated into 日本語", id, job.Stage, job.Redo, job.RedoLanguage)
		}
	}
	if a.lib.notice.Text != m.LibraryRetranslatingMany(2, m.SettingsLanguageJapanese()) || a.page != pageYouTube {
		t.Fatalf("the bar says %q on page %q", a.lib.notice.Text, a.page)
	}

	for _, id := range []string{"one", "two"} {
		job, _ := a.library.Find(id)
		job.Stage, job.Redo, job.RedoLanguage = core.StageComplete, "", ""
		a.library.Upsert(job)
	}
	a.queue.order = nil
	a.ReprocessFilms(items...)
	for _, id := range []string{"one", "two"} {
		if job, _ := a.library.Find(id); job.Stage != core.StageQueued || job.Redo != core.RedoAll {
			t.Fatalf("%s is %s %s, want queued to be processed again", id, job.Stage, job.Redo)
		}
	}
	if a.lib.notice.Text != m.LibraryReprocessingMany(2) || a.page != pageYouTube {
		t.Fatalf("the bar says %q on page %q", a.lib.notice.Text, a.page)
	}

	// Without a language to translate into, the bar says why once.
	for _, id := range []string{"one", "two"} {
		job, _ := a.library.Find(id)
		job.Stage, job.Redo, job.Language = core.StageComplete, "", ""
		a.library.Upsert(job)
	}
	a.prefs.language = ""
	a.ReprocessFilms(items...)
	if n := a.lib.notice; n.Kind != "warning" || n.Text != m.QueueNoLanguage() {
		t.Fatalf("without a language the bar says %+v", n)
	}
}

// TestSelectionFollowsThePage checks that the selection keeps to the films the library page shows: a search that hides one lets go of it, another library opening lets go of them all, and the File menu acts on what is selected.
func TestSelectionFollowsThePage(t *testing.T) {
	f, ep1, ep2 := newSeriesFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	a.lib.selection.All([]string{ep1, ep2, "film"})
	refresh(tt, a)
	if row := menuRow(t, a, m.FilmsRemoveFromLibrary()); row.When == nil || !row.When() {
		t.Fatal("File › 从资料库移除 is off with films selected")
	}
	if got := a.librarySelectionKeys(a.cx); len(got) == 0 {
		t.Fatal("no File rows")
	}
	menuRow(t, a, m.FilmsRemoveFromLibrary()).Run()
	if open, items, _ := a.Removing(); !*open || len(items) != 3 {
		t.Fatalf("File › 从资料库移除 asks %v about %d films", *open, len(items))
	}
	a.CloseRemove()
	a.side.query = "ep"
	refresh(tt, a)
	if got := a.lib.selection.Keys; len(got) != 2 || slices.Contains(got, "film") {
		t.Fatalf("a search that hides a film kept %q selected", got)
	}
	a.side.query = ""
	a.Choose(f.to)
	refresh(tt, a)
	if a.lib.selection.Len() != 0 {
		t.Fatalf("another library kept %q selected", a.lib.selection.Keys)
	}
}

// menuRow is the row of the shortcut table labelled label.
func menuRow(t *testing.T, a *app, label string) shortcut {
	t.Helper()
	for _, s := range a.shortcuts(a.cx) {
		if s.Label == label && s.Group == groupFile {
			return s
		}
	}
	t.Fatalf("no File row %q", label)
	return shortcut{}
}
