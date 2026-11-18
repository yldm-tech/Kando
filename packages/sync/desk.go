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

// Caller is who sent a Call, as its transport tells: the paired device's ID and name.
type Caller struct {
	DeviceID string
	Name     string
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
	Journal *Journal
	// Desktop is the desktop's name; Now the clock, nil for time.Now.
	Desktop string
	Now     func() time.Time

	mu      gosync.Mutex
	replies map[string][]remembered
}

// remembered is a reply a device's call got, and when.
type remembered struct {
	id    string
	reply Reply
	at    time.Time
}

// Answer answers call from caller. A call the device sent before answers the reply it got then, unless it only read (OpHello, OpJobs, OpJob, OpEvents), which reads again.
func (d *Desk) Answer(ctx context.Context, caller Caller, call Call) Reply {
	mutates := call.Op == OpJobCreate || call.Op == OpJobCancel || call.Op == OpJobRetry || call.Op == OpJobPause || call.Op == OpJobResume || call.Op == OpUpdateYTDLP
	if mutates && call.ID != "" {
		if r, ok := d.recall(caller.DeviceID, call.ID); ok {
			return r
		}
	}
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
	// A call that did not reach the desktop's work, as one cut short by its context, is not remembered, so sending it again tries again.
	if mutates && call.ID != "" && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		d.remember(caller.DeviceID, call.ID, reply)
	}
	return reply
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
		h.Desktop, h.Versions, h.Epoch = d.Desktop, []string{V1, V2}, d.Journal.Epoch()
		if h.Ops == nil {
			h.Ops = []string{}
		}
		if h.Options == nil {
			h.Options = []string{}
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
	case OpEvents:
		var w EventsWait
		if err := args(call, &w); err != nil {
			return nil, err
		}
		return d.Journal.Wait(ctx, w.Epoch, w.Since, time.Duration(w.WaitMS)*time.Millisecond), nil
	}
	return nil, Errorf(CodeUnsupported, "no operation %q", call.Op)
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

// recall is the reply device's call id got, while it is remembered.
func (d *Desk) recall(device, id string) (Reply, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	for _, r := range d.replies[device] {
		if r.id == id && now.Sub(r.at) < ReplyMemory {
			return r.reply, true
		}
	}
	return Reply{}, false
}

// remember keeps the reply device's call id got, forgetting the device's oldest past RepliesPerDevice and those older than ReplyMemory.
func (d *Desk) remember(device, id string, reply Reply) {
	d.mu.Lock()
	defer d.mu.Unlock()
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

// maxCall is the most bytes a Call may take.
const maxCall = 64 << 10

// HandleCall answers a Call POSTed to PathCall on the local network from caller, a device the transport has let in: the Reply, with status 200 for a result and StatusOf its code for an error.
func HandleCall(w http.ResponseWriter, r *http.Request, d *Desk, caller Caller) {
	var call Call
	data, err := io.ReadAll(io.LimitReader(r.Body, maxCall+1))
	var reply Reply
	switch {
	case err != nil:
		reply = Reply{Error: &ErrorBody{Code: CodeInvalid, Message: "the call could not be read"}}
	case len(data) > maxCall:
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
