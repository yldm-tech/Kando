package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// comingPipeline makes films a part at a time as preferPipeline does, and downloads a link's sound at once, as its first download, and its video, its second, once video is closed.
type comingPipeline struct {
	preferPipeline
	dir   string
	video chan struct{}
	mu    sync.Mutex
	asked map[string]int
}

func (p *comingPipeline) useFolder(dir string) { p.dir = dir }

func (p *comingPipeline) Download(ctx context.Context, source string) (string, error) {
	if !strings.HasPrefix(source, "https://") {
		return p.preferPipeline.Download(ctx, source)
	}
	p.mu.Lock()
	if p.asked == nil {
		p.asked = map[string]int{}
	}
	p.asked[source]++
	first := p.asked[source] == 1
	p.mu.Unlock()
	folder := filepath.Join(p.dir, "Talk [x]")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}
	if first {
		sound := filepath.Join(folder, "Talk [x].m4a")
		return sound, os.WriteFile(sound, []byte("sound"), 0o644)
	}
	select {
	case <-p.video:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	video := filepath.Join(folder, "Talk [x].mp4")
	return video, os.WriteFile(video, []byte("video"), 0o644)
}

// TestALinkWaitingPlaysItsSoundUntilItsVideoComes opens a link waiting behind a film being made: it goes first, plays its sound, downloaded first, once it is in, the stage saying the picture follows, and plays its video from where the sound was once that is in, its place kept by the video's path.
func TestALinkWaitingPlaysItsSoundUntilItsVideoComes(t *testing.T) {
	p := &comingPipeline{preferPipeline: preferPipeline{makerPipeline: makerPipeline{length: 90 * time.Second, released: make(chan struct{})}}, video: make(chan struct{})}
	a, tt, updates := newTestApp(t, p)
	first := filepath.Join(t.TempDir(), "first", "first.mp4")
	writeFakeClip(t, first, 90, "202020", "101010")
	link := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	if err := a.Enqueue(core.FileURL(first), link); err != nil {
		t.Fatal(err)
	}
	refresh(tt, a)
	ids := jobIDs(a)
	firstID, linkID := ids[core.FileURL(first)], ids[link]
	queueUntilState(t, updates, func() bool { return stageOf(t, a, firstID).SubtitlePath != "" })
	// Nothing of the link is in, yet its card opens it, which puts it first.
	item, ok := films.Find(a.films(), linkID)
	if !ok || films.Watchable(item) || !films.Openable(item) {
		t.Fatalf("the waiting link %+v: watchable %v, openable %v", item, films.Watchable(item), films.Openable(item))
	}
	apply(tt, a, func() { a.Open(item) })
	queueUntilState(t, updates, func() bool { return a.queue.active == linkID && stageOf(t, a, linkID).SoundPath != "" })
	refresh(tt, a)
	job := stageOf(t, a, linkID)
	goesBy := media.SoundVideo(job.SoundPath)
	if job.VideoPath != "" || goesBy != filepath.Join(filepath.Dir(job.SoundPath), "Talk [x].m4a") {
		t.Fatalf("the link plays %q by %q, its video %q", job.SoundPath, goesBy, job.VideoPath)
	}
	play := a.player.play
	if play == nil || play.standIn != job.SoundPath || play.source != job.SoundPath || play.path != goesBy {
		t.Fatalf("the player plays %+v, want the sound %q by %q", play, job.SoundPath, goesBy)
	}
	tt.Play("id:film-video", ns.VideoInfo{Duration: 90 * time.Second})
	refresh(tt, a)
	if got := tt.Find("id:film-stage-note").Text; got != testlocale.Messages().FilmPictureComing() {
		t.Fatalf("the stage says %q while the sound plays", got)
	}
	// It has no poster of its own yet, so the stage shows the site's thumbnail over the sound.
	if !tt.Exists("id:film-sound-cover") {
		t.Fatal("the stage shows no cover while the sound plays")
	}
	// It plays on to 0:30, a subtitle delay set meanwhile.
	apply(tt, a, func() {
		a.player.play.paused, a.player.play.played = false, true
		a.player.play.head, a.player.play.headAt = 30*time.Second, a.playerNow()
		a.setSubtitleDelays(map[string]time.Duration{goesBy: 400 * time.Millisecond})
	})
	close(p.video)
	queueUntilState(t, updates, func() bool { return stageOf(t, a, linkID).VideoPath != "" })
	refresh(tt, a)
	job = stageOf(t, a, linkID)
	play = a.player.play
	if job.SoundPath != "" || play == nil || play.standIn != "" || play.path != job.VideoPath || play.source != job.VideoPath || play.paused {
		t.Fatalf("once the video came the player plays %+v, job %+v", play, job)
	}
	if play.head < 30*time.Second || play.head > 31*time.Second {
		t.Fatalf("the video opens at %v, want where the sound was", play.head)
	}
	if a.player.delays[job.VideoPath] != 400*time.Millisecond || a.player.delays[goesBy] != 0 {
		t.Fatalf("delays %v", a.player.delays)
	}
	if a.PictureComing() {
		t.Fatal("the stage still says the picture follows")
	}
	close(p.released)
	settle(t, a, tt, updates)
	if job := stageOf(t, a, linkID); job.Stage != core.StageComplete || job.SoundPath != "" {
		t.Fatalf("the link ended %+v", job)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(job.VideoPath), ".Talk [x].sound.m4a")); !os.IsNotExist(err) {
		t.Fatalf("the sound is left beside the video: %v", err)
	}
}
