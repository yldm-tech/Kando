package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// soundPipeline downloads into a folder of the test, the sound alone or the video as the job's quality asks (formatFor), and writes its subtitles beside what it downloaded, as Tools does. Its download goes into folder, which a site renaming its video changes.
type soundPipeline struct {
	t           *testing.T
	root        string
	folder      string
	downloadErr error
	transcribed int
	asked       []string
}

func (p *soundPipeline) Check() error { return nil }

func (p *soundPipeline) Download(ctx context.Context, _ string) (string, error) {
	p.asked = append(p.asked, formatFor(ctx))
	if p.downloadErr != nil {
		return "", p.downloadErr
	}
	name := "video.mp4"
	if formatFor(ctx) == "ba/b" {
		name = "video.webm"
	}
	path := filepath.Join(p.root, p.folder, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		p.t.Fatal(err)
	}
	return path, nil
}

func (p *soundPipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	p.transcribed++
	return []core.Segment{{End: time.Second, Text: "hello"}}, nil
}

func (p *soundPipeline) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	return segments, nil
}

func (p *soundPipeline) Export(ctx context.Context, video string, original, translated []core.Segment) (string, error) {
	return exportSubtitles(ctx, video, original, translated)
}

// A film at 仅字幕 downloads the sound alone and removes it once its subtitles are written beside it, keeping the path they are named after; any other quality keeps its video.
func TestRunSubtitlesOnlyRemovesTheSound(t *testing.T) {
	p := &soundPipeline{t: t, root: t.TempDir(), folder: "Talk [abc]"}
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	job.Quality = core.QualitySubtitles
	done := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
	if done.Stage != core.StageComplete || done.Err != nil {
		t.Fatalf("job %+v", done)
	}
	want := filepath.Join(p.root, "Talk [abc]", "video.webm")
	if done.VideoPath != want || !done.SubtitlesOnly {
		t.Fatalf("video %q, subtitles only %v", done.VideoPath, done.SubtitlesOnly)
	}
	if _, err := os.Stat(want); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the sound is still there: %v", err)
	}
	if _, err := os.Stat(done.SubtitlePath); err != nil || filepath.Dir(done.SubtitlePath) != filepath.Dir(want) {
		t.Fatalf("subtitles %q: %v", done.SubtitlePath, err)
	}
	if !reflect.DeepEqual(p.asked, []string{"ba/b"}) {
		t.Fatalf("asked for %v", p.asked)
	}

	// Processed again at a quality that keeps the video, as after a download of its video failed, the film has its video again.
	redo := done
	redo.Redo, redo.Quality, redo.Stage = core.RedoAll, core.QualityBest, core.StageQueued
	done = Run(context.Background(), p, redo, "简体中文", func(core.Job) {})
	if done.SubtitlesOnly || done.Stage != core.StageComplete {
		t.Fatalf("job %+v", done)
	}
	if _, err := os.Stat(done.VideoPath); err != nil || filepath.Ext(done.VideoPath) != ".mp4" {
		t.Fatalf("video %q: %v", done.VideoPath, err)
	}

	kept := &soundPipeline{t: t, root: t.TempDir(), folder: "Talk [abc]"}
	job.Quality = core.QualityBest
	done = Run(context.Background(), kept, job, "简体中文", func(core.Job) {})
	if done.SubtitlesOnly {
		t.Fatal("a film at the best picture keeps no video")
	}
	if _, err := os.Stat(done.VideoPath); err != nil {
		t.Fatalf("the video is gone: %v", err)
	}
}

// A local video at 仅字幕, which no download fetched, stays as it is.
func TestRunSubtitlesOnlyKeepsALocalVideo(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(video, []byte("clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &soundPipeline{t: t, root: dir}
	job, _ := core.NewJob(core.FileURL(video))
	job.Quality = core.QualitySubtitles
	done := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
	if done.Stage != core.StageComplete || done.SubtitlesOnly {
		t.Fatalf("job %+v", done)
	}
	if _, err := os.Stat(video); err != nil {
		t.Fatalf("the video is gone: %v", err)
	}
}

// Downloading the video of a film that keeps none (core.RedoVideo) puts it beside the film's subtitles, under the name they go by, even when the site renamed the video since, and neither transcribes nor translates the film again.
func TestRunVideoRedoDownloadsBesideTheSubtitles(t *testing.T) {
	p := &soundPipeline{t: t, root: t.TempDir(), folder: "Talk [abc]"}
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	job.Quality = core.QualitySubtitles
	film := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
	if !film.SubtitlesOnly {
		t.Fatalf("film %+v", film)
	}
	p.folder, p.transcribed = "Talk, renamed [abc]", 0
	redo := film
	redo.Redo, redo.Quality, redo.Stage = core.RedoVideo, core.Quality720, core.StageQueued
	var stages []core.Stage
	done := Run(context.Background(), p, redo, "简体中文", func(j core.Job) { stages = append(stages, j.Stage) })
	if !reflect.DeepEqual(stages, []core.Stage{core.StageDownloading, core.StageComplete}) {
		t.Fatalf("stages %v", stages)
	}
	want := filepath.Join(p.root, "Talk [abc]", "video.mp4")
	if done.VideoPath != want || done.SubtitlesOnly || done.Redo != "" || done.Err != nil {
		t.Fatalf("job %+v", done)
	}
	if data, err := os.ReadFile(want); err != nil || string(data) != "video.mp4" {
		t.Fatalf("video %q: %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(p.root, "Talk, renamed [abc]")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the renamed folder stayed: %v", err)
	}
	if done.SubtitlePath != film.SubtitlePath || !done.CompletedAt.Equal(film.CompletedAt) || p.transcribed != 0 {
		t.Fatalf("job %+v, transcribed %d", done, p.transcribed)
	}
	if !strings.Contains(p.asked[len(p.asked)-1], "720") {
		t.Fatalf("asked for %v", p.asked)
	}
}

// A video download that fails leaves the film keeping none, and the job still says it was downloading its video, so the queue puts the film back as it was.
func TestRunVideoRedoThatFailsKeepsTheFilm(t *testing.T) {
	p := &soundPipeline{t: t, root: t.TempDir(), folder: "Talk [abc]", downloadErr: errors.New("ERROR: [youtube] jNQXAC9IVRw: Video unavailable. This video is private")}
	job := core.Job{ID: "1", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageQueued, Redo: core.RedoVideo, SubtitlesOnly: true,
		VideoPath: filepath.Join(p.root, "Talk [abc]", "video.webm"), Quality: core.QualityBest, CompletedAt: time.Now()}
	done := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
	if done.Stage != core.StageFailed || done.Redo != core.RedoVideo || !done.SubtitlesOnly || done.VideoPath != job.VideoPath {
		t.Fatalf("job %+v", done)
	}
	if done.FailKind != core.FailPrivate {
		t.Fatalf("fail kind %q", done.FailKind)
	}
}

// StreamURL asks yt-dlp for the address of the video's HLS stream, borrowing the browser's login, and refuses while offline mode is on.
func TestStreamURL(t *testing.T) {
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	tool := writeFakeTool(t, dir, "yt-dlp", fakeTool{Args: args, Stdout: "https://manifest.googlevideo.com/api/manifest/hls_playlist/id/1/index.m3u8\n"})
	tools := Tools{YTDLP: tool, Cookies: "firefox"}
	got, err := tools.StreamURL(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err != nil || got != "https://manifest.googlevideo.com/api/manifest/hls_playlist/id/1/index.m3u8" {
		t.Fatalf("url %q, err %v", got, err)
	}
	data, _ := os.ReadFile(args)
	for _, want := range []string{"--get-url\n", "-f\n" + streamFormat + "\n", "--cookies-from-browser\nfirefox\n", "--\nhttps://www.youtube.com/watch?v=jNQXAC9IVRw\n"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("args lack %q: %s", want, data)
		}
	}

	if _, err := (Tools{YTDLP: tool, Offline: true}).StreamURL(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw"); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline: %v", err)
	}
	empty := writeFakeTool(t, filepath.Join(dir, "empty"), "yt-dlp", fakeTool{Stdout: "\n"})
	if _, err := (Tools{YTDLP: empty}).StreamURL(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw"); err == nil {
		t.Fatal("no address is not an error")
	}
}

func TestStreams(t *testing.T) {
	for _, c := range []struct {
		goos, source string
		want         bool
	}{
		{"darwin", "https://www.youtube.com/watch?v=jNQXAC9IVRw", true},
		{"darwin", "https://youtu.be/jNQXAC9IVRw", true},
		{"darwin", "https://www.bilibili.com/video/BV1xx411c7mD", false},
		{"windows", "https://www.youtube.com/watch?v=jNQXAC9IVRw", false},
		{"linux", "https://www.youtube.com/watch?v=jNQXAC9IVRw", false},
	} {
		if got := streamsOn(c.goos, c.source); got != c.want {
			t.Errorf("%s %s: %v", c.goos, c.source, got)
		}
	}
}
