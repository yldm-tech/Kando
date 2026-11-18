package phonesync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"kando/packages/core"
	"kando/packages/media"
	psync "kando/packages/sync"
)

func steps(job psync.Job) string {
	out := ""
	for _, s := range job.Steps {
		out += fmt.Sprintf("%s:%s:%g ", s.Name, s.State, s.Progress)
	}
	return out
}

func TestJobOfTellsEachStep(t *testing.T) {
	link := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	facts := JobFacts{Title: "Me at the zoo", Library: "lib1"}
	for _, tc := range []struct {
		name  string
		job   core.Job
		state string
		steps string
	}{
		{"waiting", core.Job{Stage: core.StageQueued, Source: link}, psync.JobWaiting, "download:waiting:0 transcribe:waiting:0 translate:waiting:0 "},
		{"downloading, its tool silent", core.Job{Stage: core.StageDownloading, Source: link}, psync.JobRunning, "download:running:-1 transcribe:waiting:0 translate:waiting:0 "},
		{"transcribing", core.Job{Stage: core.StageTranscribing, Source: link, Progress: core.Progress{Known: true, Fraction: 0.4}}, psync.JobRunning, "download:done:1 transcribe:running:0.4 translate:waiting:0 "},
		{"exporting", core.Job{Stage: core.StageExporting, Source: link}, psync.JobRunning, "download:done:1 transcribe:done:1 translate:running:-1 "},
		{"a local video", core.Job{Stage: core.StageTranscribing, Source: "file:///films/talk.mp4"}, psync.JobRunning, "download:skipped:0 transcribe:running:-1 translate:waiting:0 "},
		{"translating a done film again", core.Job{Stage: core.StageTranslating, Source: link, Redo: core.RedoMissing}, psync.JobRunning, "download:skipped:0 transcribe:skipped:0 translate:running:-1 "},
		{"downloading a film's video", core.Job{Stage: core.StageDownloading, Source: link, Redo: core.RedoVideo, SubtitlesOnly: true}, psync.JobRunning, "download:running:-1 transcribe:skipped:0 translate:skipped:0 "},
		{"done", core.Job{Stage: core.StageComplete, Source: link, VideoPath: "/v.mp4"}, psync.JobDone, "download:done:1 transcribe:done:1 translate:done:1 "},
		{"done in its own language", core.Job{Stage: core.StageComplete, Source: link, SameLanguage: true}, psync.JobDone, "download:done:1 transcribe:done:1 translate:skipped:0 "},
		{"done untranslated", core.Job{Stage: core.StageComplete, Source: link, TranslationErr: "no AI service"}, psync.JobDone, "download:done:1 transcribe:done:1 translate:failed:0 "},
		{"failed to transcribe", core.Job{Stage: core.StageFailed, Source: link, Err: fmt.Errorf("transcribe: %w", media.ErrNoSpeech)}, psync.JobFailed, "download:done:1 transcribe:failed:0 translate:waiting:0 "},
		{"a local video's check failed", core.Job{Stage: core.StageFailed, Source: "file:///films/talk.mp4", Err: errors.New("check: whisper-cli is not installed")}, psync.JobFailed, "download:skipped:0 transcribe:failed:0 translate:waiting:0 "},
		{"canceled downloading", core.Job{Stage: core.StageFailed, Source: link, Err: context.Canceled}, psync.JobCanceled, "download:waiting:0 transcribe:waiting:0 translate:waiting:0 "},
	} {
		got := JobOf(tc.job, facts)
		if got.State != tc.state || steps(got) != tc.steps || got.Title != facts.Title || got.Library != "lib1" {
			t.Errorf("%s: %s %s", tc.name, got.State, steps(got))
		}
		if (got.Failure != nil) != (tc.state == psync.JobFailed) {
			t.Errorf("%s: failure %+v", tc.name, got.Failure)
		}
	}
	done := JobOf(core.Job{ID: "j1", Stage: core.StageComplete, Source: link, VideoPath: "/v.mp4", Device: "Evan's iPhone"}, facts)
	if done.Film != "j1" || done.Device != "Evan's iPhone" || done.Link != link {
		t.Fatalf("done %+v", done)
	}
	// A film that keeps no video is not among the films a phone is served.
	if streamed := JobOf(core.Job{ID: "j2", Stage: core.StageComplete, Source: link, VideoPath: "/v.webm", SubtitlesOnly: true}, facts); streamed.Film != "" {
		t.Fatalf("a film that keeps no video is served as %q", streamed.Film)
	}
	// A local video's path is the desktop's own, which a phone is not told.
	if local := JobOf(core.Job{Stage: core.StageQueued, Source: "file:///Users/evan/talk.mp4"}, facts); local.Link != "" {
		t.Fatalf("a local video's link %q", local.Link)
	}
}

func TestJobOfTypesTheFailure(t *testing.T) {
	link := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	for _, tc := range []struct {
		name  string
		job   core.Job
		facts JobFacts
		want  psync.Failure
	}{
		{"yt-dlp too old", core.Job{Err: errors.New("download: ERROR: unable to extract"), FailKind: core.FailOutdated}, JobFacts{Reason: "yt-dlp is too old"},
			psync.Failure{Step: psync.StepDownload, Code: psync.FailOutdated, Message: "yt-dlp is too old", Remedy: psync.RemedyUpdateYTDLP}},
		{"the network", core.Job{Err: errors.New("download: timed out"), FailKind: core.FailNetwork}, JobFacts{},
			psync.Failure{Step: psync.StepDownload, Code: psync.FailNetwork, Message: "download: timed out", Remedy: psync.RemedyRetry}},
		{"members only", core.Job{Err: errors.New("download: members"), FailKind: core.FailMembers}, JobFacts{Reason: "members only"},
			psync.Failure{Step: psync.StepDownload, Code: psync.FailMembers, Message: "members only", Remedy: psync.RemedyDesktop}},
		{"no speech model", core.Job{Err: errors.New("check: no whisper model")}, JobFacts{Reason: "Choose a speech model", Setup: true},
			psync.Failure{Step: psync.StepDownload, Code: psync.FailDesktopSetup, Message: "Choose a speech model", Remedy: psync.RemedyDesktop}},
		{"no speech", core.Job{Err: fmt.Errorf("transcribe: %w", media.ErrNoSpeech)}, JobFacts{Reason: "No speech"},
			psync.Failure{Step: psync.StepTranscribe, Code: psync.FailNoSpeech, Message: "No speech"}},
		{"a redo's translation", core.Job{Err: errors.New("translate: 500"), Redo: core.RedoTranslation}, JobFacts{Reason: "The AI service failed"},
			psync.Failure{Step: psync.StepTranslate, Code: psync.FailTranslation, Message: "The AI service failed", Remedy: psync.RemedyRetry}},
	} {
		tc.job.Stage, tc.job.Source = core.StageFailed, link
		got := JobOf(tc.job, tc.facts)
		if got.Failure == nil || *got.Failure != tc.want {
			t.Errorf("%s: failure %+v, want %+v", tc.name, got.Failure, tc.want)
		}
	}
}

// fakeJobs is the app's queue as a test gives it: it queues every link it is sent as a job of the device's.
type fakeJobs struct{ srv *Server }

func (f *fakeJobs) Hello(context.Context, psync.Caller) (psync.Hello, error) {
	return psync.Hello{Ops: []string{psync.OpHello, psync.OpJobCreate, psync.OpJobs, psync.OpJob, psync.OpEvents}, Receive: psync.ReceiveQueue}, nil
}

func (f *fakeJobs) CreateJob(_ context.Context, caller psync.Caller, req psync.JobCreate) (psync.JobCreated, error) {
	job := JobOf(core.Job{ID: "j1", Source: req.Link, Stage: core.StageQueued, Device: caller.Name}, JobFacts{Title: req.Link})
	f.srv.PublishJobs([]psync.Job{job})
	return psync.JobCreated{Job: &job}, nil
}

func (f *fakeJobs) ActOnJob(context.Context, psync.Caller, string, string) (psync.Job, error) {
	return psync.Job{}, psync.Errorf(psync.CodeUnsupported, "no")
}

func (f *fakeJobs) UpdateYTDLP(context.Context, psync.Caller) (psync.ToolUpdate, error) {
	return psync.ToolUpdate{Started: true}, nil
}

func TestCallsNeedAPairedDeviceAndNameIt(t *testing.T) {
	jobs := &fakeJobs{}
	srv, err := Start(Config{Dir: t.TempDir(), Name: "Test Mac", Source: fakeSource{dir: t.TempDir()}, Jobs: jobs, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	jobs.srv = srv
	ctx := context.Background()
	_, err = client(t, srv, "made-up").Hello(ctx)
	wantCode(t, err, http.StatusUnauthorized, psync.CodeUnauthorized)

	c := client(t, srv, "")
	res, err := pair(t, c, srv.NewOffer().Code)
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.Hello(ctx)
	if err != nil || h.Desktop != "Test Mac" || !reflect.DeepEqual(h.Versions, []string{psync.V1, psync.V2}) || h.Receive != psync.ReceiveQueue {
		t.Fatalf("hello %+v, %v", h, err)
	}
	list, err := c.Jobs(ctx)
	if err != nil || len(list.Jobs) != 0 {
		t.Fatalf("jobs %+v, %v", list, err)
	}
	// A phone waiting for the queue's changes hears the job it sent.
	type waited struct {
		ev  psync.Events
		err error
	}
	got := make(chan waited, 1)
	go func() {
		ev, err := c.Events(ctx, list.Epoch, list.Rev, 10*time.Second)
		got <- waited{ev, err}
	}()
	var created psync.JobCreated
	err = c.CallID(ctx, "send-1", psync.OpJobCreate, psync.JobCreate{Link: "https://youtu.be/abc"}, &created)
	if err != nil || created.Job == nil || created.Job.Device != "Evan's iPhone" {
		t.Fatalf("created %+v, %v", created, err)
	}
	w := <-got
	if w.err != nil || len(w.ev.Events) != 1 || w.ev.Events[0].Job.Link != "https://youtu.be/abc" || w.ev.Events[0].Job.Device != "Evan's iPhone" {
		t.Fatalf("events %+v, %v", w.ev, w.err)
	}
	if err := srv.Revoke(res.DeviceID); err != nil {
		t.Fatal(err)
	}
	_, err = c.Jobs(ctx)
	wantCode(t, err, http.StatusUnauthorized, psync.CodeUnauthorized)
}

// A server the app gives no queue says hello and lists no jobs, and takes no link.
func TestWithoutJobsTheServerTakesNoLinks(t *testing.T) {
	srv, _, _ := startTest(t)
	c := client(t, srv, "")
	if _, err := pair(t, c, srv.NewOffer().Code); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if h, err := c.Hello(ctx); err != nil || h.Receive != psync.ReceiveOff {
		t.Fatalf("hello %+v, %v", h, err)
	}
	_, err := c.CreateJob(ctx, "https://youtu.be/abc", psync.JobOptions{})
	wantCode(t, err, http.StatusForbidden, psync.CodeLinksOff)
}
