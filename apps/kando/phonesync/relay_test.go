package phonesync

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	psync "kando/packages/sync"
)

// memBucket is a bucket in memory whose presigned links only name what they reach: the server's tests read and write it directly, as the store would for a phone.
type memBucket struct {
	mu      gosync.Mutex
	objects map[string][]byte
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
