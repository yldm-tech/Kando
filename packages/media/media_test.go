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

type fakePipeline struct {
	checkErr     error
	transcribe   []core.Segment
	translateErr error
	language     string
}

func (f *fakePipeline) Check() error { return f.checkErr }

func (f *fakePipeline) Download(context.Context, string) (string, error) {
	return "/videos/talk.mp4", nil
}

func (f *fakePipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	return f.transcribe, nil
}

func (f *fakePipeline) Translate(_ context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	f.language = language
	return segments, f.translateErr
}

func (f *fakePipeline) Export(context.Context, string, []core.Segment, []core.Segment) (string, error) {
	return "/videos/talk.bilingual.srt", nil
}

func runStages(p Pipeline) ([]core.Stage, core.Job) {
	job, _ := core.NewJob("https://example.com/video")
	var stages []core.Stage
	final := Run(context.Background(), p, job, "简体中文", func(j core.Job) { stages = append(stages, j.Stage) })
	return stages, final
}

func TestRunReportsEveryStage(t *testing.T) {
	p := &fakePipeline{transcribe: []core.Segment{{End: time.Second, Text: "hello"}}}
	stages, job := runStages(p)

	want := []core.Stage{core.StageDownloading, core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}
	if !reflect.DeepEqual(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	if job.VideoPath != "/videos/talk.mp4" || job.SubtitlePath != "/videos/talk.bilingual.srt" || job.Err != nil {
		t.Fatalf("unexpected job: %#v", job)
	}
	if p.language != "简体中文" {
		t.Fatalf("translated into %q", p.language)
	}
}

func TestRunStopsAtFailedCheck(t *testing.T) {
	stages, job := runStages(&fakePipeline{checkErr: errors.New("ffmpeg is not installed")})
	if !reflect.DeepEqual(stages, []core.Stage{core.StageFailed}) {
		t.Fatalf("stages = %v", stages)
	}
	if job.Err == nil || job.Err.Error() != "check: ffmpeg is not installed" {
		t.Fatalf("err = %v", job.Err)
	}
}

func TestRunFailsWithoutSpeech(t *testing.T) {
	_, job := runStages(&fakePipeline{})
	if job.Stage != core.StageFailed || !errors.Is(job.Err, ErrNoSpeech) {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestRunKeepsVideoPathWhenLaterStageFails(t *testing.T) {
	p := &fakePipeline{transcribe: []core.Segment{{Text: "hello"}}, translateErr: context.Canceled}
	_, job := runStages(p)
	if job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) || job.VideoPath != "/videos/talk.mp4" {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestRunSkipsDownloadingForALocalVideo(t *testing.T) {
	p := &fakePipeline{transcribe: []core.Segment{{End: time.Second, Text: "hello"}}}
	job, _ := core.NewJob("file:///Users/test/interview.mp4")
	var stages []core.Stage
	before := time.Now()
	final := Run(context.Background(), p, job, "简体中文", func(j core.Job) { stages = append(stages, j.Stage) })
	want := []core.Stage{core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}
	if !reflect.DeepEqual(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	if final.VideoPath != "/videos/talk.mp4" {
		t.Fatalf("video = %q; Download should still give the path", final.VideoPath)
	}
	if final.CompletedAt.Before(before) || final.CompletedAt.After(time.Now()) {
		t.Fatalf("completed at %v", final.CompletedAt)
	}
}

func TestRunLeavesCompletedAtZeroOnFailure(t *testing.T) {
	_, job := runStages(&fakePipeline{})
	if job.Stage != core.StageFailed || !job.CompletedAt.IsZero() {
		t.Fatalf("unexpected job: %#v", job)
	}
}

// localSteps is Tools with the check passed and the steps after Download stubbed, so Run reaches the real Download of a local file.
type localSteps struct{ Tools }

func (localSteps) Check() error { return nil }

func (localSteps) Transcribe(context.Context, string) ([]core.Segment, error) {
	return []core.Segment{{End: time.Second, Text: "hello"}}, nil
}

func (localSteps) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	return segments, nil
}

func (localSteps) Export(context.Context, string, []core.Segment, []core.Segment) (string, error) {
	return "/videos/interview.bilingual.srt", nil
}

// TestRunReadsALocalVideoInPlace checks that Tools does not download a file:// source: the job never shows downloading and its video is the file itself, and a missing file still fails the job.
func TestRunReadsALocalVideoInPlace(t *testing.T) {
	video := filepath.Join(t.TempDir(), "interview.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	job, err := core.NewJob("file://" + video)
	if err != nil {
		t.Fatal(err)
	}
	var stages []core.Stage
	final := Run(context.Background(), localSteps{}, job, "简体中文", func(j core.Job) { stages = append(stages, j.Stage) })
	want := []core.Stage{core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}
	if !reflect.DeepEqual(stages, want) || final.VideoPath != video {
		t.Fatalf("stages %v video %q", stages, final.VideoPath)
	}

	gone, err := core.NewJob("file://" + filepath.Join(t.TempDir(), "gone.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if failed := Run(context.Background(), localSteps{}, gone, "简体中文", func(core.Job) {}); failed.Stage != core.StageFailed || !errors.Is(failed.Err, os.ErrNotExist) {
		t.Fatalf("missing file %+v", failed)
	}
}
