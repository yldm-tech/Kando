package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"kando/packages/core"
)

// heatRef is the address of a video on a media source, as apps/kando/mediasource writes it.
const heatRef = "kando-source://0123456789abcdef/Movies/Heat%20(1995).mkv"

// fakeSourceReader copies a media source's video by writing a file of its own into the folder it is given, reporting half of it done on the way, and counts its copies.
type fakeSourceReader struct {
	copies int
	err    error
}

func (r *fakeSourceReader) Copy(ctx context.Context, source, dir string) (string, error) {
	r.copies++
	if r.err != nil {
		return "", r.err
	}
	ReportProgress(ctx, core.Progress{Fraction: 0.5, Known: true})
	path := filepath.Join(dir, "Living Room NAS", "Movies", "Heat (1995).mkv")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte("heat"), 0o644)
}

// A media source's video is copied by the Tools' SourceReader into the library, not downloaded, and the job is named after its file meanwhile; Tools without one fail it.
func TestDownloadCopiesAMediaSourcesVideo(t *testing.T) {
	if !IsSourceRef(heatRef) || IsSourceRef("https://example.com/v") || IsSourceRef("file:///tmp/a.mp4") {
		t.Fatal("IsSourceRef")
	}
	dir := t.TempDir()
	reader := &fakeSourceReader{}
	var info Info
	var progress []core.Progress
	ctx := WithTrace(context.Background(), &Trace{Info: func(i Info) { info = i }, Progress: func(p core.Progress) { progress = append(progress, p) }})
	path, err := Tools{OutputDir: dir, Sources: reader}.Download(ctx, heatRef)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "Living Room NAS", "Movies", "Heat (1995).mkv"); path != want || reader.copies != 1 {
		t.Fatalf("copied to %q (%d copies), want %q", path, reader.copies, want)
	}
	if info.Title != "Heat (1995)" || len(progress) != 1 || progress[0].Fraction != 0.5 {
		t.Fatalf("info %+v progress %+v", info, progress)
	}
	if _, err := (Tools{OutputDir: dir}).Download(ctx, heatRef); !errors.Is(err, ErrNoSourceReader) {
		t.Fatalf("without a reader: %v", err)
	}
	// Offline mode is the reader's to judge, as a NAS on the local network stays readable.
	offline := Tools{OutputDir: dir, Sources: &fakeSourceReader{err: ErrOffline}, Offline: true}
	if _, err := offline.Download(ctx, heatRef); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline: %v", err)
	}
}

// sourcePipeline is fakePipeline whose download copies a media source's video as Tools does, through a SourceReader.
type sourcePipeline struct {
	fakePipeline
	reader *fakeSourceReader
}

func (p *sourcePipeline) Download(ctx context.Context, source string) (string, error) {
	return Tools{OutputDir: p.dir, Sources: p.reader}.Download(ctx, source)
}

// A media source's video is copied whole first, as a download the card shows, and is then made as a local film: heard from the copy, with no site's subtitles asked for. A job whose copy is already in the library, as one tried again, does not copy it again.
func TestMakeCopiesAMediaSourcesVideoOnce(t *testing.T) {
	p := &sourcePipeline{fakePipeline: fakePipeline{dir: t.TempDir(), transcribe: []core.Segment{{End: time.Second, Text: "hello"}}}, reader: &fakeSourceReader{}}
	job, err := core.NewJob(heatRef)
	if err != nil {
		t.Fatal(err)
	}
	var stages []core.Stage
	final := Make(context.Background(), p, job, "简体中文", "mac", nil, func(j core.Job) { stages = enteredStage(stages, j) })
	want := []core.Stage{core.StageDownloading, core.StageTranscribing, core.StageTranslating, core.StageComplete}
	if !reflect.DeepEqual(stages, want) || final.Err != nil {
		t.Fatalf("stages = %v, want %v; err %v", stages, want, final.Err)
	}
	if final.Source != heatRef || filepath.Base(final.VideoPath) != "Heat (1995).mkv" || final.Title != "Heat (1995)" || p.reader.copies != 1 {
		t.Fatalf("job %+v, %d copies", final, p.reader.copies)
	}

	again := core.Job{ID: "again", Source: heatRef, VideoPath: final.VideoPath, Stage: core.StageQueued}
	stages = nil
	if done := Make(context.Background(), p, again, "简体中文", "mac", nil, func(j core.Job) { stages = enteredStage(stages, j) }); done.Err != nil || p.reader.copies != 1 {
		t.Fatalf("again: err %v, %d copies", done.Err, p.reader.copies)
	}
	if stages[0] == core.StageDownloading {
		t.Fatalf("the copy was made again: %v", stages)
	}

	p.reader = &fakeSourceReader{err: errors.New("the NAS is off")}
	failed := Make(context.Background(), p, core.Job{ID: "off", Source: heatRef, Stage: core.StageQueued}, "简体中文", "mac", nil, func(core.Job) {})
	if failed.Stage != core.StageFailed || failed.Err == nil {
		t.Fatalf("a source that cannot be read: %+v", failed)
	}
}

// URL serves nothing in the tests: the address it gives is only checked to be the one heard from.
func (r *fakeSourceReader) URL(ctx context.Context, source string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return "http://127.0.0.1:1/token/Heat%20(1995).mkv", nil
}

// streamPipeline is fakePipeline that reads a media source's video where it is (SourceStreamer), as Tools does through a SourceReader, recording what it was asked to hear from and whether its ffmpeg reads http.
type streamPipeline struct {
	fakePipeline
	reader  *fakeSourceReader
	noHTTP  bool
	heardMu sync.Mutex
	heard   []string
}

func (p *streamPipeline) StreamSource(ctx context.Context, source string) (string, error) {
	return Tools{Sources: p.reader}.StreamSource(ctx, source)
}

func (p *streamPipeline) CheckHTTP(context.Context) error {
	if p.noHTTP {
		return ErrNoHTTP
	}
	return nil
}

func (p *streamPipeline) Hear(ctx context.Context, path string, from, to time.Duration, heard func(core.Segment)) (string, error) {
	p.heardMu.Lock()
	p.heard = append(p.heard, path)
	p.heardMu.Unlock()
	return p.fakePipeline.Hear(ctx, path, from, to, heard)
}

// A film indexed from a media source is not copied: it is heard from the address the app serves its video at, its lines are written beside its placeholder path, which stays its video's, and nothing is removed at the address. An ffmpeg that reads no http fails it at the check, and a source that cannot be read at the download, as a copy's would.
func TestMakeHearsAnIndexedFilmFromItsSource(t *testing.T) {
	dir := t.TempDir()
	placeholder := filepath.Join(dir, "Living Room NAS", "Movies", "Heat (1995).mkv")
	if err := os.MkdirAll(filepath.Dir(placeholder), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &streamPipeline{fakePipeline: fakePipeline{dir: dir, transcribe: []core.Segment{{End: time.Second, Text: "hello"}}}, reader: &fakeSourceReader{}}
	job := core.Job{ID: "heat", Source: heatRef, VideoPath: placeholder, Linked: true, Stage: core.StageQueued}
	var stages []core.Stage
	final := Make(context.Background(), p, job, "简体中文", "mac", nil, func(j core.Job) { stages = enteredStage(stages, j) })
	if final.Err != nil || final.Stage != core.StageComplete {
		t.Fatalf("made %+v", final)
	}
	if final.VideoPath != placeholder || !final.Linked || p.reader.copies != 0 || slices.Contains(stages, core.StageDownloading) {
		t.Fatalf("job %+v, %d copies, stages %v", final, p.reader.copies, stages)
	}
	if len(p.heard) == 0 || p.heard[0] != "http://127.0.0.1:1/token/Heat%20(1995).mkv" {
		t.Fatalf("heard from %q", p.heard)
	}
	if _, err := os.Stat(final.SubtitlePath); err != nil || filepath.Dir(final.SubtitlePath) != filepath.Dir(placeholder) {
		t.Fatalf("subtitles %q: %v", final.SubtitlePath, err)
	}
	if _, err := os.Stat(placeholder); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a video at the placeholder: %v", err)
	}

	p.noHTTP = true
	if failed := Make(context.Background(), p, core.Job{ID: "nohttp", Source: heatRef, VideoPath: placeholder, Linked: true, Stage: core.StageQueued}, "简体中文", "mac", nil, func(core.Job) {}); !errors.Is(failed.Err, ErrNoHTTP) {
		t.Fatalf("an ffmpeg without http: %+v", failed.Err)
	}
	p.noHTTP = false
	p.reader = &fakeSourceReader{err: errors.New("the NAS is off")}
	if failed := Make(context.Background(), p, core.Job{ID: "off", Source: heatRef, VideoPath: placeholder, Linked: true, Stage: core.StageQueued}, "简体中文", "mac", nil, func(core.Job) {}); failed.Stage != core.StageFailed {
		t.Fatalf("a source that cannot be read: %+v", failed)
	}
}

// readsHTTP finds http among the protocols ffmpeg reads, and not among those it only writes.
func TestReadsHTTP(t *testing.T) {
	if !readsHTTP("Supported file protocols:\nInput:\n  file\n  http\n  https\nOutput:\n  file\n") {
		t.Fatal("http to read was not found")
	}
	if readsHTTP("Supported file protocols:\nInput:\n  file\nOutput:\n  file\n  http\n") {
		t.Fatal("http to write was taken for http to read")
	}
}
