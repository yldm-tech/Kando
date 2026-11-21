package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
