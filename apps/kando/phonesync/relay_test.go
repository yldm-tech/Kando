package phonesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"kando/apps/kando/cloud"
	psync "kando/packages/sync"
)

// memBucket is a bucket in memory whose presigned links only name what they reach: the server's tests read and write it directly, as the store would for a phone.
type memBucket struct {
	mu      gosync.Mutex
	objects map[string][]byte
	// files are the keys PutFile was asked to store, resumed the upload each went on from ("" for none), and fail the names of the files whose uploads fail.
	files   []string
	resumed []string
	fail    map[string]bool
}

func (m *memBucket) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.objects[key]
	if !ok {
		return nil, psync.ErrNoObject
	}
	return body, nil
}

func (m *memBucket) Put(_ context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.objects == nil {
		m.objects = map[string][]byte{}
	}
	m.objects[key] = slices.Clone(body)
	return nil
}

func (m *memBucket) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[key]
	return ok, nil
}

func (m *memBucket) List(_ context.Context, prefix string) ([]psync.RelayObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []psync.RelayObject
	for k := range m.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, psync.RelayObject{Key: k})
		}
	}
	return out, nil
}

func (m *memBucket) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

// PutFile stores the file at path, as the store does; a file whose name is in m.fail fails, as a network gone, after a first part kept in the upload's resume.
func (m *memBucket) PutFile(_ context.Context, key, path, _ string, resume cloud.Resume, save func(cloud.Resume) error, progress func(done, total int64)) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.files = append(m.files, key)
	failing := m.fail[filepath.Base(path)]
	m.mu.Unlock()
	if failing {
		if err := save(cloud.Resume{UploadID: "u1", PartSize: 1, Size: int64(len(body)), Parts: []cloud.UploadedPart{{N: 1, ETag: "e1"}}}); err != nil {
			return err
		}
		return errors.New("the network went away")
	}
	if progress != nil {
		progress(int64(len(body)), int64(len(body)))
	}
	m.mu.Lock()
	m.resumed = append(m.resumed, resume.UploadID)
	m.mu.Unlock()
	return m.Put(context.Background(), key, body)
}

// putFiles are the keys PutFile was asked to store, in their order.
func (m *memBucket) putFiles() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.files)
}

func (m *memBucket) Sizes(_ context.Context, prefix string) ([]cloud.Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []cloud.Object
	for k, body := range m.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, cloud.Object{Key: k, Size: int64(len(body))})
		}
	}
	return out, nil
}

func (m *memBucket) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "get:" + key, nil
}

func (m *memBucket) PresignPut(_ context.Context, key string, _ time.Duration) (string, error) {
	return "put:" + key, nil
}

func (m *memBucket) keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for k := range m.objects {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// waitFor polls cond until it holds, failing the test after a while.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited in vain for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestAPhoneJoinsTheRelayAndIsAnsweredThroughIt pairs a phone, has it join the relay on the local network, leaves a call in its inbox as it would away from home, and reads the reply; unpairing it then empties its folders.
func TestAPhoneJoinsTheRelayAndIsAnsweredThroughIt(t *testing.T) {
	every, minWrite := relayEvery, relayMinWrite
	relayEvery, relayMinWrite = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { relayEvery, relayMinWrite = every, minWrite })
	bucket := &memBucket{}
	jobs := &fakeJobs{}
	dir := t.TempDir()
	srv, err := Start(Config{Dir: dir, Name: "Test Mac", Source: fakeSource{dir: t.TempDir()}, Jobs: jobs, Relay: bucket, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	jobs.srv = srv
	ctx := context.Background()
	c := client(t, srv, "")
	res, err := pair(t, c, srv.NewOffer().Code)
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.Hello(ctx)
	if err != nil || !slices.Contains(h.Ops, psync.OpRelayJoin) {
		t.Fatalf("hello %+v, %v", h, err)
	}
	var joined psync.RelayJoin
	if err := c.Call(ctx, psync.OpRelayJoin, nil, &joined); err != nil {
		t.Fatal(err)
	}
	prefix := psync.RelayPrefix(srv.Fingerprint())
	if len(joined.Key) != psync.RelayKeySize || len(joined.Grant.Slots) != psync.RelaySlots || joined.Grant.State != "get:"+prefix+psync.RelayStateName(res.DeviceID) {
		t.Fatalf("joined %+v", joined.Grant)
	}
	var again psync.RelayJoin
	if err := c.Call(ctx, psync.OpRelayJoin, nil, &again); err != nil || string(again.Key) != string(joined.Key) {
		t.Fatalf("joining again changed the key: %v", err)
	}
	// Joining again hands out a new grant, as the phone may have spent the old one's slots.
	if again.Grant.Slots[0].ID == joined.Grant.Slots[0].ID || !again.Grant.Minted.After(joined.Grant.Minted) {
		t.Fatalf("joining again gave back the grant: %v, then %v", joined.Grant.Minted, again.Grant.Minted)
	}
	// The key is kept with the device, readable by the user alone, and not given out with the list of devices.
	data, _ := os.ReadFile(filepath.Join(dir, DevicesFile))
	if !strings.Contains(string(data), `"relay_key"`) {
		t.Fatalf("devices file %s", data)
	}
	if devs := srv.Devices(); len(devs) != 1 || devs[0].RelayKey != nil {
		t.Fatalf("devices %+v", devs)
	}

	// Away from home, the phone leaves a call in a slot.
	slot := joined.Grant.Slots[0].ID
	args, _ := json.Marshal(psync.JobCreate{Link: "https://youtu.be/abc"})
	plain, _ := json.Marshal(psync.RelayMessage{Seq: 1, Call: psync.Call{ID: "c1", V: psync.CallVersion, Op: psync.OpJobCreate, Args: args}})
	sealed, err := psync.SealRelay(joined.Key, psync.RelayInbox(res.DeviceID, slot), plain)
	if err != nil {
		t.Fatal(err)
	}
	bucket.Put(ctx, prefix+psync.RelayInbox(res.DeviceID, slot), sealed)
	var reply psync.Reply
	waitFor(t, "the reply", func() bool {
		body, err := bucket.Get(ctx, prefix+psync.RelayReply(res.DeviceID, slot))
		if err != nil {
			return false
		}
		plain, err := psync.OpenRelay(joined.Key, psync.RelayReply(res.DeviceID, slot), body)
		return err == nil && json.Unmarshal(plain, &reply) == nil
	})
	var created psync.JobCreated
	if reply.Error != nil || json.Unmarshal(reply.Result, &created) != nil || created.Job == nil || created.Job.Device != "Evan's iPhone" {
		t.Fatalf("reply %+v", reply)
	}
	var state psync.RelayState
	waitFor(t, "the state", func() bool {
		body, err := bucket.Get(ctx, prefix+psync.RelayStateName(res.DeviceID))
		if err != nil {
			return false
		}
		plain, err := psync.OpenRelay(joined.Key, psync.RelayStateName(res.DeviceID), body)
		return err == nil && json.Unmarshal(plain, &state) == nil && len(state.Jobs.Jobs) == 1
	})
	if state.Desktop != "Test Mac" || state.Jobs.Jobs[0].Link != "https://youtu.be/abc" {
		t.Fatalf("state %+v", state)
	}

	if err := srv.Revoke(res.DeviceID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the phone's folders to go", func() bool { return len(bucket.keys()) == 0 })
	if _, err := c.Hello(ctx); err == nil {
		t.Fatal("the unpaired phone was answered")
	}
}

// TestWithoutTheRelayAPhoneCannotJoin has a server whose relay is off answer the join unsupported, and not list it.
func TestWithoutTheRelayAPhoneCannotJoin(t *testing.T) {
	srv, _, _ := startTest(t)
	c := client(t, srv, "")
	if _, err := pair(t, c, srv.NewOffer().Code); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if h, err := c.Hello(ctx); err != nil || slices.Contains(h.Ops, psync.OpRelayJoin) {
		t.Fatalf("hello %+v, %v", h, err)
	}
	err := c.Call(ctx, psync.OpRelayJoin, nil, nil)
	wantCode(t, err, http.StatusNotImplemented, psync.CodeUnsupported)
}

// stuckBucket is a store that does not answer listings: each waits until its context ends.
type stuckBucket struct{ memBucket }

func (b *stuckBucket) List(ctx context.Context, _ string) ([]psync.RelayObject, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestClosingDoesNotWaitForAStuckStore unpairs a phone while the store does not answer, then turns the server off: the deletion of the phone's folders is given up, not waited out.
func TestClosingDoesNotWaitForAStuckStore(t *testing.T) {
	dir := t.TempDir()
	srv, err := Start(Config{Dir: dir, Name: "Test Mac", Source: fakeSource{dir: t.TempDir()}, Jobs: &fakeJobs{}, Relay: &stuckBucket{}, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	c := client(t, srv, "")
	res, err := pair(t, c, srv.NewOffer().Code)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Call(context.Background(), psync.OpRelayJoin, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := srv.Revoke(res.DeviceID); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() {
		srv.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close waited for the store")
	}
}

// TestARelayedReadOnlyPhoneChangesNothing makes a phone that joined the relay read-only, then leaves in its inbox, as it would away from home, a change of the learning state, of the settings and of a subscription: the server refuses each with the code it answers on the local network, before any of its work, and still answers the reads; let write again, the phone's change goes through.
func TestARelayedReadOnlyPhoneChangesNothing(t *testing.T) {
	every, minWrite := relayEvery, relayMinWrite
	relayEvery, relayMinWrite = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { relayEvery, relayMinWrite = every, minWrite })
	bucket := &memBucket{}
	jobs := &fakeJobs{}
	dir := t.TempDir()
	srv, err := Start(Config{Dir: dir, Name: "Test Mac", Source: fakeSource{dir: t.TempDir()}, Jobs: jobs, Relay: bucket, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	jobs.srv = srv
	ctx := context.Background()
	c := client(t, srv, "")
	res, err := pair(t, c, srv.NewOffer().Code)
	if err != nil {
		t.Fatal(err)
	}
	var joined psync.RelayJoin
	if err := c.Call(ctx, psync.OpRelayJoin, nil, &joined); err != nil {
		t.Fatal(err)
	}
	if err := srv.SetReadOnly(res.DeviceID, true); err != nil {
		t.Fatal(err)
	}
	prefix := psync.RelayPrefix(srv.Fingerprint())
	seq := int64(0)
	// leave puts a call of op with args in the next slot, as the phone does, and answers the slot.
	leave := func(op string, args any) string {
		t.Helper()
		seq++
		slot := joined.Grant.Slots[seq-1].ID
		call := psync.Call{ID: fmt.Sprintf("c%d", seq), V: psync.CallVersion, Op: op}
		if args != nil {
			call.Args, _ = json.Marshal(args)
		}
		plain, _ := json.Marshal(psync.RelayMessage{Seq: seq, Call: call})
		sealed, err := psync.SealRelay(joined.Key, psync.RelayInbox(res.DeviceID, slot), plain)
		if err != nil {
			t.Fatal(err)
		}
		bucket.Put(ctx, prefix+psync.RelayInbox(res.DeviceID, slot), sealed)
		return slot
	}
	// replyOf waits for the reply in slot.
	replyOf := func(slot string) psync.Reply {
		t.Helper()
		var reply psync.Reply
		waitFor(t, "the reply in "+slot, func() bool {
			body, err := bucket.Get(ctx, prefix+psync.RelayReply(res.DeviceID, slot))
			if err != nil {
				return false
			}
			plain, err := psync.OpenRelay(joined.Key, psync.RelayReply(res.DeviceID, slot), body)
			return err == nil && json.Unmarshal(plain, &reply) == nil
		})
		return reply
	}
	note := psync.Field{Value: psync.Value("a note"), Stamp: psync.Stamp{At: time.Now()}}
	changes := map[string]any{
		psync.OpStatePut:         psync.StatePut{Library: "lib", Records: []psync.Record{{Kind: psync.KindLine, ID: "l1", Film: "f1", Fields: map[string]psync.Field{psync.FieldNote: note}}}},
		psync.OpStateResolve:     psync.ConflictResolve{Library: "lib", ID: "x", Value: psync.Value("a")},
		psync.OpSettingsPut:      psync.SettingsPut{Target: "ja"},
		psync.OpSubscribe:        psync.SubscriptionAdd{Link: "https://www.youtube.com/@earthscience"},
		psync.OpSubscriptionSet:  psync.SubscriptionSet{ID: "s1"},
		psync.OpSubscriptionTake: psync.SubscriptionTake{ID: "s1"},
		psync.OpUnsubscribe:      psync.SubscriptionRef{ID: "s1"},
		psync.OpJobCreate:        psync.JobCreate{Link: "https://youtu.be/abc"},
	}
	slots := map[string]string{}
	for _, op := range slices.Sorted(maps.Keys(changes)) {
		slots[op] = leave(op, changes[op])
	}
	read := leave(psync.OpSettingsGet, nil)
	for op, slot := range slots {
		if reply := replyOf(slot); reply.Error == nil || reply.Error.Code != psync.CodeReadOnly {
			t.Errorf("a read-only phone's %s over the relay was answered %+v", op, reply)
		}
	}
	if reply := replyOf(read); reply.Error != nil && reply.Error.Code == psync.CodeReadOnly {
		t.Errorf("a read-only phone's read of the settings was refused: %+v", reply.Error)
	}

	if err := srv.SetReadOnly(res.DeviceID, false); err != nil {
		t.Fatal(err)
	}
	if reply := replyOf(leave(psync.OpJobCreate, psync.JobCreate{Link: "https://youtu.be/abc"})); reply.Error != nil {
		t.Fatalf("once it may write again: %+v", reply.Error)
	}
}
