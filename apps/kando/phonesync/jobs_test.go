package phonesync

import (
	"context"
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
		{"failed to transcribe", core.Job{Stage: core.StageFailed, Source: link, Err: &media.StageError{Stage: media.StageTranscribe, Err: media.ErrNoSpeech}}, psync.JobFailed, "download:done:1 transcribe:failed:0 translate:waiting:0 "},
		{"a local video's check failed", core.Job{Stage: core.StageFailed, Source: "file:///films/talk.mp4", Err: media.ReadStageError("check: whisper-cli is not installed")}, psync.JobFailed, "download:skipped:0 transcribe:failed:0 translate:waiting:0 "},
		{"canceled downloading", core.Job{Stage: core.StageFailed, Source: link, Err: context.Canceled}, psync.JobCanceled, "download:waiting:0 transcribe:waiting:0 translate:waiting:0 "},
		{"paused by a phone", core.Job{Stage: core.StageQueued, Source: link, PausedBy: "Evan's iPhone", VideoPath: "/v.mp4", Transcribed: true}, psync.JobPaused, "download:waiting:0 transcribe:waiting:0 translate:waiting:0 "},
		{"paused while transcribing, not stopped yet", core.Job{Stage: core.StageTranscribing, Source: link, PausedBy: "Evan's iPhone"}, psync.JobPaused, "download:done:1 transcribe:waiting:0 translate:waiting:0 "},
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
	// A film that keeps no video is served too, by its subtitles.
	if streamed := JobOf(core.Job{ID: "j2", Stage: core.StageComplete, Source: link, VideoPath: "/v.webm", SubtitlesOnly: true}, facts); streamed.Film != "j2" {
		t.Fatalf("a film that keeps no video is served as %q, want j2", streamed.Film)
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
		{"yt-dlp too old", core.Job{Err: media.ReadStageError("download: ERROR: unable to extract"), FailKind: core.FailOutdated}, JobFacts{Reason: "yt-dlp is too old"},
			psync.Failure{Step: psync.StepDownload, Code: psync.FailOutdated, Message: "yt-dlp is too old", Remedy: psync.RemedyUpdateYTDLP}},
		// The phone cannot download the desktop's Deno, so it says the desktop has something to set up rather than to try again later.
		{"no JavaScript runtime", core.Job{Err: media.ReadStageError("download: ERROR: [youtube] jNQXAC9IVRw: The page needs to be reloaded. (yt-dlp found no JavaScript runtime)"), FailKind: core.FailNoJSRuntime}, JobFacts{Reason: "YouTube needs a JavaScript runtime to download"},
			psync.Failure{Step: psync.StepDownload, Code: psync.FailDesktopSetup, Message: "YouTube needs a JavaScript runtime to download", Remedy: psync.RemedyDesktop}},
		{"the network", core.Job{Err: media.ReadStageError("download: timed out"), FailKind: core.FailNetwork}, JobFacts{},
			psync.Failure{Step: psync.StepDownload, Code: psync.FailNetwork, Message: "download: timed out", Remedy: psync.RemedyRetry}},
		{"members only", core.Job{Err: media.ReadStageError("download: members"), FailKind: core.FailMembers}, JobFacts{Reason: "members only"},
			psync.Failure{Step: psync.StepDownload, Code: psync.FailMembers, Message: "members only", Remedy: psync.RemedyDesktop}},
		{"no speech model", core.Job{Err: media.ReadStageError("check: no whisper model")}, JobFacts{Reason: "Choose a speech model", Setup: true},
			psync.Failure{Step: psync.StepDownload, Code: psync.FailDesktopSetup, Message: "Choose a speech model", Remedy: psync.RemedyDesktop}},
		{"no speech", core.Job{Err: &media.StageError{Stage: media.StageTranscribe, Err: media.ErrNoSpeech}}, JobFacts{Reason: "No speech"},
			psync.Failure{Step: psync.StepTranscribe, Code: psync.FailNoSpeech, Message: "No speech"}},
		{"a redo's translation", core.Job{Err: media.ReadStageError("translate: 500"), Redo: core.RedoTranslation}, JobFacts{Reason: "The AI service failed"},
			psync.Failure{Step: psync.StepTranslate, Code: psync.FailTranslation, Message: "The AI service failed", Remedy: psync.RemedyRetry}},
		// The AI service's words, and the desktop's of them, stay on the desktop: the code alone says it.
		{"an account out of credit", core.Job{Err: media.ReadStageError("translate: translation API returned 402 Payment Required: top up at https://pay.example.com")}, JobFacts{Reason: "Top it up, then retry.", Setup: true, AI: true, AIUnavailable: true},
			psync.Failure{Step: psync.StepTranslate, Code: psync.FailAIUnavailable, Remedy: psync.RemedyDesktop}},
		{"a key the AI service rejects", core.Job{Err: media.ReadStageError("translate: translation API returned 401 Unauthorized: bad key")}, JobFacts{Reason: "Check the key.", Setup: true, AI: true},
			psync.Failure{Step: psync.StepTranslate, Code: psync.FailDesktopSetup, Remedy: psync.RemedyDesktop}},
		{"the AI service's own words", core.Job{Err: media.ReadStageError("translate: translation API returned 400 Bad Request: see https://example.com")}, JobFacts{AI: true},
			psync.Failure{Step: psync.StepTranslate, Code: psync.FailTranslation, Remedy: psync.RemedyRetry}},
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

// fakeState is the app's learning state as a test gives it: it counts the puts it takes.
type fakeState struct{ puts int }

func (f *fakeState) GetState(context.Context, psync.Caller, psync.StateGet) (psync.StateChanges, error) {
	return psync.StateChanges{Records: []psync.Record{}, Conflicts: []psync.Conflict{}}, nil
}

func (f *fakeState) PutState(context.Context, psync.Caller, psync.StatePut) (psync.StatePutResult, error) {
	f.puts++
	return psync.StatePutResult{Records: []psync.Record{}, Conflicts: []psync.Conflict{}}, nil
}

func (f *fakeState) ResolveConflict(context.Context, psync.Caller, psync.ConflictResolve) (psync.StatePutResult, error) {
	return psync.StatePutResult{}, psync.Errorf(psync.CodeNotFound, "no conflict")
}

// A paired phone may change everything until the user makes it read-only, which the server checks at each of its calls: from then on it reads the libraries, the jobs and the learning state, and every call that changes something, the jobs' and the learning state's, is refused, until it may write again. The switch is kept with the device across starts.
func TestAReadOnlyPhoneIsRefusedEveryWrite(t *testing.T) {
	jobs, state := &fakeJobs{}, &fakeState{}
	dir := t.TempDir()
	start := func() *Server {
		srv, err := Start(Config{Dir: dir, Name: "Test Mac", Source: fakeSource{dir: t.TempDir()}, Jobs: jobs, State: state, Addrs: loopback, Logf: testLogf(t)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { srv.Close() })
		jobs.srv = srv
		return srv
	}
	srv := start()
	ctx := context.Background()
	c := client(t, srv, "")
	res, err := pair(t, c, srv.NewOffer().Code)
	if err != nil {
		t.Fatal(err)
	}
	c = client(t, srv, res.Token)
	if devs := srv.Devices(); len(devs) != 1 || devs[0].ReadOnly {
		t.Fatalf("a phone just paired: %+v", devs)
	}
	if err := srv.SetReadOnly(res.DeviceID, true); err != nil {
		t.Fatal(err)
	}
	if h, err := c.Hello(ctx); err != nil || !h.ReadOnly {
		t.Fatalf("hello of a read-only phone %+v, %v", h, err)
	}
	_, err = c.CreateJob(ctx, "https://youtu.be/abc", psync.JobOptions{})
	wantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	_, err = c.ActOnJob(ctx, psync.OpJobCancel, "j1")
	wantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	_, err = c.UpdateYTDLP(ctx)
	wantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	put := []psync.Record{{Kind: psync.KindPosition, ID: "talk", Film: "talk", Fields: map[string]psync.Field{psync.FieldPositionMS: {Value: psync.Value(5000)}}}}
	_, err = c.PutState(ctx, "lib1", put)
	wantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	_, err = c.ResolveConflict(ctx, "lib1", "c1", psync.Value("x"))
	wantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	if state.puts != 0 {
		t.Fatalf("a read-only phone's put reached the app")
	}
	// Reads go on.
	if _, err := c.Jobs(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.State(ctx, "lib1", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Libraries(ctx); err != nil {
		t.Fatal(err)
	}
	// The switch is kept: a server started again still refuses the phone's writes.
	srv.Close()
	srv = start()
	if devs := srv.Devices(); len(devs) != 1 || !devs[0].ReadOnly {
		t.Fatalf("after a restart: %+v", devs)
	}
	c = client(t, srv, res.Token)
	_, err = c.PutState(ctx, "lib1", put)
	wantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	if err := srv.SetReadOnly(res.DeviceID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PutState(ctx, "lib1", put); err != nil || state.puts != 1 {
		t.Fatalf("a put once the phone may write again: %v, %d puts", err, state.puts)
	}
	if created, err := c.CreateJob(ctx, "https://youtu.be/abc", psync.JobOptions{}); err != nil || created.Job == nil {
		t.Fatalf("a link once the phone may write again: %+v, %v", created, err)
	}
	if err := srv.SetReadOnly("nobody", true); err == nil {
		t.Fatal("a device that is not paired was made read-only")
	}
}

// A phone is online while it was seen within OnlineWithin.
func TestADeviceIsOnlineWhileSeenLately(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	d := Device{LastSeen: now.Add(-OnlineWithin + time.Second)}
	if !d.Online(now) {
		t.Fatal("a phone seen just now is offline")
	}
	if d.Online(now.Add(time.Minute)) {
		t.Fatal("a phone not seen for a while is online")
	}
}
