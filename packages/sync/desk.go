package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	gosync "sync"
	"time"
)

// Caller is who sent a Call, as its transport tells: the paired device's ID and name, whether the desktop's user made it read-only (ReadOnly), which the transport reads from the device's record at every call, and whether the call came through the storage relay (relay.go) rather than the local network.
type Caller struct {
	DeviceID string
	Name     string
	// ReadOnly is a device that may read but not change anything: Desk refuses its calls that change something (Mutates) with CodeReadOnly.
	ReadOnly bool
	Relay    bool
}

// Service is what a desktop does for protocol v2's calls, whichever transport carries them; Desk answers the jobs' list and the events itself, from its Journal. Its errors are *CallError for the protocol's codes, and any other error answers CodeFailed.
type Service interface {
	// Hello is what the desktop says of itself, without Versions and Epoch, which Desk fills in.
	Hello(ctx context.Context, caller Caller) (Hello, error)
	CreateJob(ctx context.Context, caller Caller, req JobCreate) (JobCreated, error)
	// ActOnJob runs op, one of OpJobCancel, OpJobRetry, OpJobPause and OpJobResume, on job id.
	ActOnJob(ctx context.Context, caller Caller, op, id string) (Job, error)
	UpdateYTDLP(ctx context.Context, caller Caller) (ToolUpdate, error)
}

// StateService is what a desktop does for the learning state's calls (state.go), whichever transport carries them. Desk has stamped what PutState is given with the caller, at no later than its own clock, before it gets it. Its errors are as Service's.
type StateService interface {
	GetState(ctx context.Context, caller Caller, req StateGet) (StateChanges, error)
	PutState(ctx context.Context, caller Caller, req StatePut) (StatePutResult, error)
	ResolveConflict(ctx context.Context, caller Caller, req ConflictResolve) (StatePutResult, error)
}

// RelayJoiner hands a phone its key and first grant of the storage relay (OpRelayJoin).
type RelayJoiner interface {
	Join(ctx context.Context, caller Caller) (RelayJoin, error)
}

// CallError is an error with a protocol code, which a Service returns and a client reads from the Reply.
type CallError struct {
	Code    string
	Message string
}

func (e *CallError) Error() string { return e.Code + ": " + e.Message }

// Errorf is a *CallError with code and a message in English.
func Errorf(code, format string, args ...any) error {
	return &CallError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ReplyMemory is how long, and for how many calls of each device, a desktop remembers the replies of its calls, so a call sent again answers as it did the first time.
const (
	ReplyMemory      = 24 * time.Hour
	RepliesPerDevice = 256
)

// Desk answers protocol v2's calls for a desktop: the jobs' list and the events from Journal, everything else from Service. It is safe for concurrent use.
type Desk struct {
	Service Service
	// State answers the learning state's calls, nil for a desktop that does not serve it, which answers them CodeUnsupported.
	State StateService
	// Study answers lookups and explanations (study.go), nil for a desktop that does not, which answers them CodeUnsupported.
	Study StudyService
	// Relay answers OpRelayJoin, nil while the desktop's storage relay is off, which answers it CodeUnsupported.
	Relay   RelayJoiner
	Journal *Journal
	// Desktop is the desktop's name; Now the clock, nil for time.Now.
	Desktop string
	Now     func() time.Time

	mu      gosync.Mutex
	replies map[string][]remembered
	// running is the calls that change something being answered now, by device and call ID, so the same call sent again meanwhile waits for their reply rather than doing the work a second time.
	running map[callKey]*inFlight
}

// callKey is a device's call.
type callKey struct{ device, id string }

// inFlight is a call being answered: done closes once reply is set, and kept says whether the reply was remembered, which a call cut short by its context is not.
type inFlight struct {
	done  chan struct{}
	reply Reply
	kept  bool
}

// remembered is a reply a device's call got, and when.
type remembered struct {
	id    string
	reply Reply
	at    time.Time
}

// Answer answers call from caller. A call the device sent before answers the reply it got then, unless it only read (OpHello, OpJobs, OpJob, OpEvents, OpStateGet), which reads again. The same call arriving while the first is still being answered, as a relay delivers twice or a phone sends again after the network dropped, waits for the first one's reply and answers it.
func (d *Desk) Answer(ctx context.Context, caller Caller, call Call) Reply {
	mutates := Mutates(call.Op)
	if mutates && caller.ReadOnly {
		// Refused before the desktop's work and not remembered, so the same call sent once the device may write again does its work.
		return Reply{ID: call.ID, Op: call.Op, Error: &ErrorBody{Code: CodeReadOnly, Message: "this device is read-only; the desktop's user can let it make changes in Settings › Phone Sync"}}
	}
	if !mutates || call.ID == "" {
		reply, _ := d.reply(ctx, caller, call)
		return reply
	}
	key := callKey{caller.DeviceID, call.ID}
	for {
		r, ok, first := d.claim(key)
		if ok {
			return r
		}
		if first == nil {
			break
		}
		select {
		case <-first.done:
			if first.kept {
				return first.reply
			}
			// The first one was cut short before it reached the desktop's work, so this one tries again.
		case <-ctx.Done():
			return Reply{ID: call.ID, Op: call.Op, Error: &ErrorBody{Code: CodeFailed, Message: ctx.Err().Error()}}
		}
	}
	reply, err := d.reply(ctx, caller, call)
	// A call that did not reach the desktop's work, as one cut short by its context, is not remembered, so sending it again tries again.
	d.settle(key, reply, !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded))
	return reply
}

// Mutates reports whether op changes something on the desktop: a read-only device's calls of it are refused (Caller.ReadOnly), and a call of it sent again answers the reply it got the first time.
func Mutates(op string) bool {
	switch op {
	case OpJobCreate, OpJobCancel, OpJobRetry, OpJobPause, OpJobResume, OpUpdateYTDLP, OpStatePut, OpStateResolve:
		return true
	}
	return false
}

// reply runs call and answers its Reply, with the error it ended with.
func (d *Desk) reply(ctx context.Context, caller Caller, call Call) (Reply, error) {
	result, err := d.answer(ctx, caller, call)
	reply := Reply{ID: call.ID, Op: call.Op}
	if err == nil {
		reply.Result, err = json.Marshal(result)
	}
	if err != nil {
		var e *CallError
		if !errors.As(err, &e) {
			e = &CallError{Code: CodeFailed, Message: err.Error()}
		}
		reply.Result = nil
		reply.Error = &ErrorBody{Code: e.Code, Message: e.Message}
	}
	return reply, err
}

// claim is the reply key got, while it is remembered; else the call answering key now, which the caller waits for; else neither, and key is marked as being answered by the caller, who must settle it.
func (d *Desk) claim(key callKey) (Reply, bool, *inFlight) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r, ok := d.recallLocked(key.device, key.id); ok {
		return r, true, nil
	}
	if f, ok := d.running[key]; ok {
		return Reply{}, false, f
	}
	if d.running == nil {
		d.running = map[callKey]*inFlight{}
	}
	d.running[key] = &inFlight{done: make(chan struct{})}
	return Reply{}, false, nil
}

// settle ends the call answering key with reply, remembering it when keep is set, and lets the same call sent meanwhile answer it.
func (d *Desk) settle(key callKey, reply Reply, keep bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if keep {
		d.rememberLocked(key.device, key.id, reply)
	}
	f := d.running[key]
	delete(d.running, key)
	f.reply, f.kept = reply, keep
	close(f.done)
}

func (d *Desk) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// answer runs call.
func (d *Desk) answer(ctx context.Context, caller Caller, call Call) (any, error) {
	if call.V != CallVersion {
		return nil, Errorf(CodeUnsupported, "version %d is not spoken here; hello lists the versions", call.V)
	}
	switch call.Op {
	case OpHello:
		h, err := d.Service.Hello(ctx, caller)
		if err != nil {
			return nil, err
		}
		h.Desktop, h.Versions, h.Epoch, h.ReadOnly = d.Desktop, []string{V1, V2}, d.Journal.Epoch(), caller.ReadOnly
		if h.Ops == nil {
			h.Ops = []string{}
		}
		if h.Options == nil {
			h.Options = []string{}
		}
		if h.State == nil {
			h.State = []string{}
		}
		if d.Relay != nil && !slices.Contains(h.Ops, OpRelayJoin) {
			h.Ops = append(slices.Clone(h.Ops), OpRelayJoin)
		}
		return h, nil
	case OpJobCreate:
		var req JobCreate
		if err := args(call, &req); err != nil {
			return nil, err
		}
		return d.Service.CreateJob(ctx, caller, req)
	case OpJobs:
		return d.Journal.List(), nil
	case OpJob:
		var ref JobRef
		if err := args(call, &ref); err != nil {
			return nil, err
		}
		job, ok := d.Journal.Job(ref.ID)
		if !ok {
			return nil, Errorf(CodeNotFound, "no job %q", ref.ID)
		}
		return job, nil
	case OpJobCancel, OpJobRetry, OpJobPause, OpJobResume:
		var ref JobRef
		if err := args(call, &ref); err != nil {
			return nil, err
		}
		return d.Service.ActOnJob(ctx, caller, call.Op, ref.ID)
	case OpUpdateYTDLP:
		return d.Service.UpdateYTDLP(ctx, caller)
	case OpStateGet, OpStatePut, OpStateResolve:
		return d.answerState(ctx, caller, call)
	case OpLookUp, OpExplain:
		return d.answerStudy(ctx, caller, call)
	case OpEvents:
		var w EventsWait
		if err := args(call, &w); err != nil {
			return nil, err
		}
		if caller.Relay {
			// Over the relay a phone follows the changes in its state.json; a call waiting here would hold up the calls behind it.
			w.WaitMS = 0
		}
		return d.Journal.Wait(ctx, w.Epoch, w.Since, time.Duration(w.WaitMS)*time.Millisecond), nil
	case OpRelayJoin:
		if d.Relay == nil {
			return nil, Errorf(CodeUnsupported, "the storage relay is off")
		}
		if caller.Relay {
			return nil, Errorf(CodeUnsupported, "a phone joins the relay on the local network only")
		}
		return d.Relay.Join(ctx, caller)
	}
	return nil, Errorf(CodeUnsupported, "no operation %q", call.Op)
}

// answerState runs a call of the learning state (state.go).
func (d *Desk) answerState(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.State == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not serve the learning state")
	}
	switch call.Op {
	case OpStateGet:
		var req StateGet
		if err := args(call, &req); err != nil {
			return nil, err
		}
		return d.State.GetState(ctx, caller, req)
	case OpStatePut:
		var req StatePut
		if err := args(call, &req); err != nil {
			return nil, err
		}
		if err := d.stampPut(caller, &req); err != nil {
			return nil, err
		}
		return d.State.PutState(ctx, caller, req)
	}
	var req ConflictResolve
	if err := args(call, &req); err != nil {
		return nil, err
	}
	if req.ID == "" || len(req.Value) == 0 || !json.Valid(req.Value) {
		return nil, Errorf(CodeInvalid, "a resolution names a conflict and the value to keep")
	}
	return d.State.ResolveConflict(ctx, caller, req)
}

// stampPut checks req and stamps every change in it with the calling device, whatever device it names, and its time with the desk's clock's when it is later, or none, so a device's clock running ahead cannot have its changes win over every later one.
func (d *Desk) stampPut(caller Caller, req *StatePut) error {
	now := d.now()
	stamp := func(s Stamp) Stamp {
		if s.At.IsZero() || s.At.After(now) {
			s.At = now
		}
		s.Device = caller.DeviceID
		return s
	}
	for i := range req.Records {
		r := &req.Records[i]
		if r.Kind == "" || r.ID == "" {
			return Errorf(CodeInvalid, "a record names its kind and ID")
		}
		r.Rev = 0
		if r.Removed != nil {
			if !Tombstoned(r.Kind) {
				return Errorf(CodeInvalid, "a %s is not removed but set to null", r.Kind)
			}
			s := stamp(*r.Removed)
			r.Removed, r.Fields = &s, nil
			continue
		}
		for name, f := range r.Fields {
			if len(f.Value) == 0 || !json.Valid(f.Value) {
				return Errorf(CodeInvalid, "field %s of %s %s holds no value", name, r.Kind, r.ID)
			}
			f.Stamp = stamp(f.Stamp)
			r.Fields[name] = f
		}
	}
	return nil
}

// args reads call's arguments into v; none read as v's zero value.
func args(call Call, v any) error {
	if len(call.Args) == 0 {
		return nil
	}
	if err := json.Unmarshal(call.Args, v); err != nil {
		return Errorf(CodeInvalid, "the arguments of %s: %v", call.Op, err)
	}
	return nil
}

// recallLocked is the reply device's call id got, while it is remembered. d.mu is held.
func (d *Desk) recallLocked(device, id string) (Reply, bool) {
	now := d.now()
	for _, r := range d.replies[device] {
		if r.id == id && now.Sub(r.at) < ReplyMemory {
			return r.reply, true
		}
	}
	return Reply{}, false
}

// rememberLocked keeps the reply device's call id got, forgetting the device's oldest past RepliesPerDevice and those older than ReplyMemory. d.mu is held.
func (d *Desk) rememberLocked(device, id string, reply Reply) {
	if d.replies == nil {
		d.replies = map[string][]remembered{}
	}
	now := d.now()
	kept := slices.DeleteFunc(d.replies[device], func(r remembered) bool { return now.Sub(r.at) >= ReplyMemory })
	kept = append(kept, remembered{id: id, reply: reply, at: now})
	if len(kept) > RepliesPerDevice {
		kept = slices.Delete(kept, 0, len(kept)-RepliesPerDevice)
	}
	d.replies[device] = kept
}

// Forget forgets the replies of device, as when it is unpaired.
func (d *Desk) Forget(device string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.replies, device)
}

// MaxCall is the most bytes a Call may take; a device sends what is more in several calls.
const MaxCall = 64 << 10

// HandleCall answers a Call POSTed to PathCall on the local network from caller, a device the transport has let in: the Reply, with status 200 for a result and StatusOf its code for an error.
func HandleCall(w http.ResponseWriter, r *http.Request, d *Desk, caller Caller) {
	var call Call
	data, err := io.ReadAll(io.LimitReader(r.Body, MaxCall+1))
	var reply Reply
	switch {
	case err != nil:
		reply = Reply{Error: &ErrorBody{Code: CodeInvalid, Message: "the call could not be read"}}
	case len(data) > MaxCall:
		reply = Reply{Error: &ErrorBody{Code: CodeInvalid, Message: "the call is too long"}}
	case json.Unmarshal(data, &call) != nil:
		reply = Reply{Error: &ErrorBody{Code: CodeInvalid, Message: "the request is not a call"}}
	default:
		reply = d.Answer(r.Context(), caller, call)
	}
	status := http.StatusOK
	if reply.Error != nil {
		status = StatusOf(reply.Error.Code)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(reply)
}

// NewCallID is a new Call ID: 16 random bytes in hex.
func NewCallID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
