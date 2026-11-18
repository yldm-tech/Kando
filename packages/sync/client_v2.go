package sync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrV1Only is Client.Hello's error for a desktop that speaks protocol v1 alone: the phone keeps to v1's paths.
var ErrV1Only = errors.New("the desktop speaks protocol v1 only")

// Call sends op with args, nil for none, as a Call with a new ID, and decodes its Result into out, nil to drop it. An Error answers an *Error with the status the desktop answered.
func (c *Client) Call(ctx context.Context, op string, args, out any) error {
	return c.CallID(ctx, NewCallID(), op, args, out)
}

// CallID is Call with the call's ID given, for sending a call again after the network dropped: the desktop answers it as it did the first time, without doing its work again.
func (c *Client) CallID(ctx context.Context, id, op string, args, out any) error {
	call := Call{ID: id, V: CallVersion, Op: op}
	if args != nil {
		data, err := json.Marshal(args)
		if err != nil {
			return err
		}
		call.Args = data
	}
	body, err := json.Marshal(call)
	if err != nil {
		return err
	}
	resp, err := c.send(ctx, http.MethodPost, PathCall, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	var reply Reply
	if json.Unmarshal(data, &reply) != nil {
		reply = Reply{}
	}
	if resp.StatusCode >= 400 || reply.Error != nil {
		e := &Error{Status: resp.StatusCode}
		switch {
		case reply.Error != nil:
			e.ErrorBody = *reply.Error
		case json.Unmarshal(data, &e.ErrorBody) != nil:
			e.Message = strings.TrimSpace(string(data))
		}
		return e
	}
	if out == nil || len(reply.Result) == 0 {
		return nil
	}
	return json.Unmarshal(reply.Result, out)
}

// Hello is what the desktop says of itself, or ErrV1Only for a desktop that speaks v1 alone, which has no PathCall.
func (c *Client) Hello(ctx context.Context) (Hello, error) {
	var out Hello
	err := c.Call(ctx, OpHello, nil, &out)
	var e *Error
	if errors.As(err, &e) && e.Status == http.StatusNotFound {
		return Hello{}, ErrV1Only
	}
	return out, err
}

// CreateJob sends link to the desktop to process with opts.
func (c *Client) CreateJob(ctx context.Context, link string, opts JobOptions) (JobCreated, error) {
	var out JobCreated
	err := c.Call(ctx, OpJobCreate, JobCreate{Link: link, Options: opts}, &out)
	return out, err
}

// Jobs are the jobs the phone follows, with the revision to wait for events after.
func (c *Client) Jobs(ctx context.Context) (JobList, error) {
	var out JobList
	err := c.Call(ctx, OpJobs, nil, &out)
	return out, err
}

// Job is job id.
func (c *Client) Job(ctx context.Context, id string) (Job, error) {
	var out Job
	err := c.Call(ctx, OpJob, JobRef{ID: id}, &out)
	return out, err
}

// ActOnJob runs op, one of OpJobCancel, OpJobRetry, OpJobPause and OpJobResume, on job id, and answers the job as it is after.
func (c *Client) ActOnJob(ctx context.Context, op, id string) (Job, error) {
	var out Job
	err := c.Call(ctx, op, JobRef{ID: id}, &out)
	return out, err
}

// UpdateYTDLP has the desktop update its downloader.
func (c *Client) UpdateYTDLP(ctx context.Context) (ToolUpdate, error) {
	var out ToolUpdate
	err := c.Call(ctx, OpUpdateYTDLP, nil, &out)
	return out, err
}

// Events waits up to wait for the events after revision since of epoch (Events).
func (c *Client) Events(ctx context.Context, epoch string, since int64, wait time.Duration) (Events, error) {
	var out Events
	err := c.Call(ctx, OpEvents, EventsWait{Epoch: epoch, Since: since, WaitMS: wait.Milliseconds()}, &out)
	return out, err
}

// send sends a request with the token and answers the response whatever its status, which the caller closes.
func (c *Client) send(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	return c.request(ctx, method, path, body, header)
}

// State is the changes of library's learning state after revision since of epoch, all of it with Reset for an epoch of "" (StateGet).
func (c *Client) State(ctx context.Context, library, epoch string, since int64) (StateChanges, error) {
	var out StateChanges
	err := c.Call(ctx, OpStateGet, StateGet{Library: library, Epoch: epoch, Since: since}, &out)
	return out, err
}

// PutState sends the device's changes of library's records (StatePut), and answers them as the desktop merged them.
func (c *Client) PutState(ctx context.Context, library string, records []Record) (StatePutResult, error) {
	var out StatePutResult
	err := c.Call(ctx, OpStatePut, StatePut{Library: library, Records: records}, &out)
	return out, err
}

// ResolveConflict resolves conflict id of library with value, the value to keep.
func (c *Client) ResolveConflict(ctx context.Context, library, id string, value []byte) (StatePutResult, error) {
	var out StatePutResult
	err := c.Call(ctx, OpStateResolve, ConflictResolve{Library: library, ID: id, Value: value}, &out)
	return out, err
}
