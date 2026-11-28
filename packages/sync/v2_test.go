package sync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	gosync "sync"
	"testing"
	"time"
)

// fakeService is a desktop's Service that queues each link as a job of its own, which a test then publishes in the journal, and counts the jobs it made.
type fakeService struct {
	mu      gosync.Mutex
	receive string
	made    int
	callers []Caller
	// entered, when set, is told of each CreateJob as it starts, which then waits for gate to close.
	entered chan struct{}
	gate    chan struct{}
}

func (f *fakeService) Hello(context.Context, Caller) (Hello, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return Hello{Ops: []string{OpHello, OpJobCreate, OpJobs, OpJob, OpJobCancel, OpEvents}, Receive: f.receive}, nil
}

func (f *fakeService) CreateJob(_ context.Context, caller Caller, req JobCreate) (JobCreated, error) {
	if f.entered != nil {
		f.entered <- struct{}{}
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callers = append(f.callers, caller)
	switch {
	case f.receive == ReceiveOff:
		return JobCreated{}, Errorf(CodeLinksOff, "links from phones are off")
	case req.Options != (JobOptions{}):
		return JobCreated{}, Errorf(CodeUnsupported, "no options")
	case f.receive == ReceiveAsk:
		return JobCreated{Pending: true}, nil
	}
	f.made++
	return JobCreated{Job: &Job{ID: "j" + string(rune('0'+f.made)), Link: req.Link, Device: caller.Name, State: JobWaiting}}, nil
}

func (f *fakeService) ActOnJob(_ context.Context, _ Caller, op, id string) (Job, error) {
	if op != OpJobCancel {
		return Job{}, Errorf(CodeUnsupported, "no %s", op)
	}
	return Job{ID: id, State: JobCanceled}, nil
}

func (f *fakeService) UpdateYTDLP(context.Context, Caller) (ToolUpdate, error) {
	return ToolUpdate{}, errors.New("disk full")
}

// v2Desktop serves desk on loopback over TLS as the desktop does, letting in token "tok" as device d1, "Pixel", and answers a client of it.
func v2Desktop(t *testing.T, desk *Desk) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+PathCall, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(ErrorBody{Code: CodeUnauthorized})
			return
		}
		HandleCall(w, r, desk, Caller{DeviceID: "d1", Name: "Pixel"})
	})
	srv := httptest.NewUnstartedServer(mux)
	srv.StartTLS()
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	c := NewClient(netip.MustParseAddrPort(u.Host), Fingerprint(srv.Certificate().Raw), "tok")
	t.Cleanup(c.Close)
	return c
}

func newDesk(f *fakeService) *Desk {
	return &Desk{Service: f, Journal: NewJournal(), Desktop: "Mac"}
}

func wantError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Status != status || e.Code != code {
		t.Fatalf("error %v, want %d %s", err, status, code)
	}
}

func TestHelloNegotiatesV2(t *testing.T) {
	ctx := context.Background()
	desk := newDesk(&fakeService{receive: ReceiveAsk})
	c := v2Desktop(t, desk)
	h, err := c.Hello(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if h.Desktop != "Mac" || h.Receive != ReceiveAsk || h.Epoch != desk.Journal.Epoch() || len(h.Versions) != 2 || h.Versions[1] != V2 || h.Options == nil {
		t.Fatalf("hello %+v", h)
	}
	// A call in a version the desktop does not speak is refused, and says hello lists the ones it does.
	err = c.Call(ctx, OpHello, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := Call{ID: "x", V: 3, Op: OpHello}
	reply := desk.Answer(ctx, Caller{DeviceID: "d1"}, body)
	if reply.Error == nil || reply.Error.Code != CodeUnsupported || reply.ID != "x" {
		t.Fatalf("reply %+v", reply)
	}
	if reply := desk.Answer(ctx, Caller{}, Call{V: CallVersion, Op: "films.delete"}); reply.Error == nil || reply.Error.Code != CodeUnsupported {
		t.Fatalf("an unknown operation answered %+v", reply)
	}
	// A desktop that does not share its settings says so.
	if reply := desk.Answer(ctx, Caller{}, Call{V: CallVersion, Op: OpSettingsGet}); reply.Error == nil || reply.Error.Code != CodeUnsupported {
		t.Fatalf("settings of a desktop without them answered %+v", reply)
	}
}

func TestHelloOfAV1DesktopSaysSo(t *testing.T) {
	_, addr, fp := desktop(t)
	c := NewClient(addr, fp, "tok")
	defer c.Close()
	if _, err := c.Hello(context.Background()); !errors.Is(err, ErrV1Only) {
		t.Fatalf("hello of a v1 desktop: %v", err)
	}
}

func TestCreateJobFollowsTheDesktopsSetting(t *testing.T) {
	ctx := context.Background()
	f := &fakeService{receive: ReceiveQueue}
	c := v2Desktop(t, newDesk(f))
	got, err := c.CreateJob(ctx, "https://youtu.be/abc", JobOptions{})
	if err != nil || got.Job == nil || got.Job.Link != "https://youtu.be/abc" || got.Job.Device != "Pixel" {
		t.Fatalf("created %+v, %v", got, err)
	}
	_, err = c.CreateJob(ctx, "https://youtu.be/abc", JobOptions{Quality: "720p"})
	wantError(t, err, http.StatusNotImplemented, CodeUnsupported)
	f.receive = ReceiveAsk
	if got, err := c.CreateJob(ctx, "https://youtu.be/def", JobOptions{}); err != nil || !got.Pending || got.Job != nil {
		t.Fatalf("asked: %+v, %v", got, err)
	}
	f.receive = ReceiveOff
	_, err = c.CreateJob(ctx, "https://youtu.be/ghi", JobOptions{})
	wantError(t, err, http.StatusForbidden, CodeLinksOff)
	// An error that is not the protocol's is a failure, its words kept for logs.
	_, err = c.UpdateYTDLP(ctx)
	wantError(t, err, http.StatusInternalServerError, CodeFailed)
}

// A call sent again with the same ID, as a relay delivers twice or a phone sends again after the network dropped, answers as it did the first time without doing its work again; reads are read again.
func TestACallSentAgainDoesItsWorkOnce(t *testing.T) {
	ctx := context.Background()
	f := &fakeService{receive: ReceiveQueue}
	desk := newDesk(f)
	c := v2Desktop(t, desk)
	var first, again JobCreated
	if err := c.CallID(ctx, "call-1", OpJobCreate, JobCreate{Link: "https://youtu.be/abc"}, &first); err != nil {
		t.Fatal(err)
	}
	if err := c.CallID(ctx, "call-1", OpJobCreate, JobCreate{Link: "https://youtu.be/abc"}, &again); err != nil {
		t.Fatal(err)
	}
	if f.made != 1 || first.Job.ID != again.Job.ID {
		t.Fatalf("made %d jobs: %+v then %+v", f.made, first.Job, again.Job)
	}
	// Another device's call with the same ID is its own.
	desk.Answer(ctx, Caller{DeviceID: "d2"}, Call{ID: "call-1", V: CallVersion, Op: OpJobCreate, Args: json.RawMessage(`{"link":"https://youtu.be/x"}`)})
	if f.made != 2 {
		t.Fatalf("made %d jobs after another device's call", f.made)
	}
	// Forgetting a device, as when it is unpaired, forgets its replies.
	desk.Forget("d1")
	if err := c.CallID(ctx, "call-1", OpJobCreate, JobCreate{Link: "https://youtu.be/abc"}, &again); err != nil || f.made != 3 {
		t.Fatalf("made %d jobs after forgetting, %v", f.made, err)
	}
}

// The same call arriving again while the first is still being answered waits for its reply rather than doing the work a second time.
func TestACallSentAgainWhileAnsweredWaitsForTheFirst(t *testing.T) {
	ctx := context.Background()
	f := &fakeService{receive: ReceiveQueue, entered: make(chan struct{}, 2), gate: make(chan struct{})}
	desk := newDesk(f)
	call := Call{ID: "c", V: CallVersion, Op: OpJobCreate, Args: json.RawMessage(`{"link":"https://youtu.be/abc"}`)}
	replies := make(chan Reply, 2)
	go func() { replies <- desk.Answer(ctx, Caller{DeviceID: "d1"}, call) }()
	<-f.entered
	go func() { replies <- desk.Answer(ctx, Caller{DeviceID: "d1"}, call) }()
	select {
	case <-f.entered:
		t.Fatal("the call sent again ran while the first was being answered")
	case <-time.After(200 * time.Millisecond):
	}
	close(f.gate)
	first, again := <-replies, <-replies
	if f.made != 1 || first.Error != nil || string(first.Result) != string(again.Result) {
		t.Fatalf("made %d jobs: %+v then %+v", f.made, first, again)
	}
	// One that gives up waiting answers its own context's end, and the first is still remembered.
	f.gate = make(chan struct{})
	slow := Call{ID: "d", V: CallVersion, Op: OpJobCreate, Args: json.RawMessage(`{"link":"https://youtu.be/def"}`)}
	go func() { replies <- desk.Answer(ctx, Caller{DeviceID: "d1"}, slow) }()
	<-f.entered
	short, cancel := context.WithCancel(ctx)
	cancel()
	if r := desk.Answer(short, Caller{DeviceID: "d1"}, slow); r.Error == nil || r.Error.Code != CodeFailed {
		t.Fatalf("a call that gave up waiting answered %+v", r)
	}
	close(f.gate)
	if r := <-replies; r.Error != nil {
		t.Fatalf("the first call answered %+v", r)
	}
	if r := desk.Answer(ctx, Caller{DeviceID: "d1"}, slow); r.Error != nil || f.made != 2 {
		t.Fatalf("made %d jobs, sent again answered %+v", f.made, r)
	}
}

func TestRepliesAreRememberedForAWhile(t *testing.T) {
	ctx := context.Background()
	f := &fakeService{receive: ReceiveQueue}
	now := time.Unix(1_700_000_000, 0)
	desk := newDesk(f)
	desk.Now = func() time.Time { return now }
	call := Call{ID: "c", V: CallVersion, Op: OpJobCreate, Args: json.RawMessage(`{"link":"https://youtu.be/abc"}`)}
	desk.Answer(ctx, Caller{DeviceID: "d1"}, call)
	now = now.Add(ReplyMemory)
	desk.Answer(ctx, Caller{DeviceID: "d1"}, call)
	if f.made != 2 {
		t.Fatalf("made %d jobs, want the call done again once its reply is forgotten", f.made)
	}
	for i := range RepliesPerDevice {
		desk.Answer(ctx, Caller{DeviceID: "d1"}, Call{ID: string(rune(0x4e00 + i)), V: CallVersion, Op: OpJobCreate})
	}
	desk.Answer(ctx, Caller{DeviceID: "d1"}, call)
	if want := 2 + RepliesPerDevice + 1; f.made != want {
		t.Fatalf("made %d jobs, want %d: a device's oldest replies are forgotten past RepliesPerDevice", f.made, want)
	}
}

func TestJobsAndEventsFollowWhatTheDesktopPublishes(t *testing.T) {
	ctx := context.Background()
	desk := newDesk(&fakeService{})
	c := v2Desktop(t, desk)
	j := desk.Journal
	running := Job{ID: "a", State: JobRunning, Steps: []Step{{Name: StepDownload, State: StepRunning, Progress: 0.1}}}
	waiting := Job{ID: "b", State: JobWaiting}
	if !j.PublishJobs([]Job{running, waiting}) {
		t.Fatal("publishing new jobs changed nothing")
	}
	if j.PublishJobs([]Job{running, waiting}) {
		t.Fatal("publishing the same jobs again changed something")
	}
	list, err := c.Jobs(ctx)
	if err != nil || list.Rev != 2 || len(list.Jobs) != 2 || list.Jobs[0].ID != "a" || list.Jobs[0].Rev != 1 || list.Jobs[1].Rev != 2 || list.Epoch != j.Epoch() {
		t.Fatalf("list %+v, %v", list, err)
	}
	if got, err := c.Job(ctx, "b"); err != nil || got.State != JobWaiting {
		t.Fatalf("job b: %+v, %v", got, err)
	}
	_, err = c.Job(ctx, "zz")
	wantError(t, err, http.StatusNotFound, CodeNotFound)

	// Nothing new: the wait ends empty when its time is up.
	start := time.Now()
	ev, err := c.Events(ctx, list.Epoch, list.Rev, 50*time.Millisecond)
	if err != nil || len(ev.Events) != 0 || ev.Reset || ev.Rev != 2 || time.Since(start) < 50*time.Millisecond {
		t.Fatalf("events %+v, %v after %v", ev, err, time.Since(start))
	}
	// A change wakes the wait; a job's progress moving several times answers once, as it is now, and a job gone says so.
	go func() {
		time.Sleep(30 * time.Millisecond)
		running.Steps[0].Progress = 0.5
		j.PublishJobs([]Job{running, waiting})
	}()
	ev, err = c.Events(ctx, list.Epoch, list.Rev, 5*time.Second)
	if err != nil || len(ev.Events) != 1 || ev.Events[0].Job.Steps[0].Progress != 0.5 || ev.Rev != 3 {
		t.Fatalf("events %+v, %v", ev, err)
	}
	running.Steps[0].Progress = 0.9
	j.PublishJobs([]Job{running, waiting})
	running.Steps[0] = Step{Name: StepDownload, State: StepDone, Progress: 1}
	running.State = JobDone
	j.PublishJobs([]Job{running})
	ev, err = c.Events(ctx, ev.Epoch, ev.Rev, time.Second)
	if err != nil || len(ev.Events) != 2 || ev.Events[0].Kind != EventJob || ev.Events[0].Job.State != JobDone || ev.Events[1].Kind != EventJobGone || ev.Events[1].ID != "b" || ev.Rev != 6 {
		t.Fatalf("events %+v, %v", ev, err)
	}
	// Another epoch, as the desktop's before it restarted, or a revision ahead of it, is told to list again at once.
	for _, w := range []EventsWait{{Epoch: "old", Since: 3}, {Epoch: "", Since: 0}, {Epoch: j.Epoch(), Since: 99}} {
		ev, err := c.Events(ctx, w.Epoch, w.Since, 5*time.Second)
		if err != nil || !ev.Reset || ev.Epoch != j.Epoch() || ev.Rev != 6 {
			t.Fatalf("waiting after %+v: %+v, %v", w, ev, err)
		}
	}
}

func TestAJournalForgetsItsOldestEvents(t *testing.T) {
	j := NewJournal()
	j.keep = 3
	for i := range 5 {
		j.PublishJobs([]Job{{ID: "a", Title: string(rune('a' + i))}})
	}
	ctx := context.Background()
	if ev := j.Wait(ctx, j.Epoch(), 1, 0); !ev.Reset {
		t.Fatalf("waiting after a forgotten revision: %+v", ev)
	}
	if ev := j.Wait(ctx, j.Epoch(), 2, 0); ev.Reset || len(ev.Events) != 1 || ev.Events[0].Job.Title != "e" {
		t.Fatalf("waiting after the oldest kept: %+v", ev)
	}
	// A wait ends with its context.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if ev := j.Wait(cctx, j.Epoch(), 5, time.Minute); ev.Reset || len(ev.Events) != 0 {
		t.Fatalf("waiting with a context ended: %+v", ev)
	}
}

func TestHandleCallRefusesWhatIsNotACall(t *testing.T) {
	desk := newDesk(&fakeService{})
	for _, body := range []string{"not json", `{"id":"1","v":2,"op":"jobs.get","args":"x"}`} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, PathCall, strings.NewReader(body))
		HandleCall(w, r, desk, Caller{})
		var reply Reply
		if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || w.Code != http.StatusBadRequest || reply.Error == nil || reply.Error.Code != CodeInvalid {
			t.Fatalf("%q answered %d %s", body, w.Code, w.Body)
		}
	}
}

// A device the desktop's user made read-only reads everything but changes nothing: every call that changes something, the jobs', the learning state's and the settings', is refused with CodeReadOnly before the desktop's work, and not remembered, so the same call sent once the device may write again does its work. Hello tells the device it is read-only.
func TestAReadOnlyDeviceChangesNothing(t *testing.T) {
	ctx := context.Background()
	f := &fakeService{receive: ReceiveQueue}
	state := &fakeState{}
	desk := newDesk(f)
	desk.State = state
	settings := &fakeSettings{}
	desk.Settings = settings
	reader := Caller{DeviceID: "d1", Name: "Pixel", ReadOnly: true}
	call := func(caller Caller, id, op string, args any) Reply {
		data, _ := json.Marshal(args)
		return desk.Answer(ctx, caller, Call{ID: id, V: CallVersion, Op: op, Args: data})
	}
	if r := call(reader, "h", OpHello, nil); r.Error != nil || !strings.Contains(string(r.Result), `"read_only":true`) {
		t.Fatalf("hello of a read-only device: %s %+v", r.Result, r.Error)
	}
	if r := call(Caller{DeviceID: "d2"}, "h", OpHello, nil); strings.Contains(string(r.Result), "read_only") {
		t.Fatalf("hello of a device that may write says read_only: %s", r.Result)
	}
	writes := map[string]any{
		OpJobCreate:    JobCreate{Link: "https://youtu.be/abc"},
		OpJobCancel:    JobRef{ID: "j1"},
		OpJobRetry:     JobRef{ID: "j1"},
		OpJobPause:     JobRef{ID: "j1"},
		OpJobResume:    JobRef{ID: "j1"},
		OpJobRemove:    JobRef{ID: "j1"},
		OpUpdateYTDLP:  nil,
		OpStatePut:     StatePut{Library: "l", Records: []Record{{Kind: KindPosition, ID: "f", Fields: map[string]Field{FieldPositionMS: {Value: Value(5000)}}}}},
		OpStateResolve: ConflictResolve{Library: "l", ID: "c", Value: Value("x")},
		OpSettingsPut:  SettingsPut{Target: "ja"},
	}
	for op, args := range writes {
		r := call(reader, "w-"+op, op, args)
		if r.Error == nil || r.Error.Code != CodeReadOnly {
			t.Fatalf("%s of a read-only device answered %s %+v", op, r.Result, r.Error)
		}
		if StatusOf(r.Error.Code) != http.StatusForbidden {
			t.Fatalf("read_only answers status %d", StatusOf(r.Error.Code))
		}
	}
	if f.made != 0 || len(f.callers) != 0 || state.puts != 0 || state.resolves != 0 || settings.puts != 0 {
		t.Fatalf("a read-only device's calls reached the desktop: %d jobs, %d puts, %d resolves, %d settings", f.made, state.puts, state.resolves, settings.puts)
	}
	for _, op := range []string{OpJobs, OpStateGet, OpSettingsGet} {
		if r := call(reader, "r-"+op, op, StateGet{Library: "l"}); r.Error != nil {
			t.Fatalf("%s of a read-only device: %+v", op, r.Error)
		}
	}
	// Once it may write again, the same call does its work: the refusal was not remembered.
	writer := reader
	writer.ReadOnly = false
	if r := call(writer, "w-"+OpJobCreate, OpJobCreate, writes[OpJobCreate]); r.Error != nil || f.made != 1 {
		t.Fatalf("the call sent again once the device may write: %+v, %d jobs", r.Error, f.made)
	}
	if r := call(writer, "w-"+OpStatePut, OpStatePut, writes[OpStatePut]); r.Error != nil || state.puts != 1 {
		t.Fatalf("the put sent again once the device may write: %+v, %d puts", r.Error, state.puts)
	}
}

// fakeSettings is a desktop's SettingsService that counts the changes it was asked for.
type fakeSettings struct{ puts int }

func (f *fakeSettings) Settings(context.Context, Caller) (Settings, error) {
	return Settings{Target: "zh-Hans"}, nil
}

func (f *fakeSettings) PutSettings(_ context.Context, _ Caller, req SettingsPut) (Settings, error) {
	f.puts++
	return Settings{Target: req.Target}, nil
}

// fakeState is a desktop's StateService that counts what it was asked to change.
type fakeState struct {
	puts, resolves int
}

func (f *fakeState) GetState(context.Context, Caller, StateGet) (StateChanges, error) {
	return StateChanges{Records: []Record{}, Conflicts: []Conflict{}}, nil
}

func (f *fakeState) PutState(context.Context, Caller, StatePut) (StatePutResult, error) {
	f.puts++
	return StatePutResult{Records: []Record{}, Conflicts: []Conflict{}}, nil
}

func (f *fakeState) ResolveConflict(context.Context, Caller, ConflictResolve) (StatePutResult, error) {
	f.resolves++
	return StatePutResult{Records: []Record{}, Conflicts: []Conflict{}}, nil
}
