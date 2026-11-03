package media

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"lingocut/packages/core"
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
