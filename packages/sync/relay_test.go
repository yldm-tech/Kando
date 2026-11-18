package sync

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"
)

// memRelay is a store in memory with presigned links as S3 makes them: a link names one method on one key until it lapses, by the test's clock, and needs no credentials; the store refuses it (403) after.
type memRelay struct {
	srv *httptest.Server
	now func() time.Time

	mu      gosync.Mutex
	objects map[string]memObject
	links   map[string]memLink
	puts    int
}

type memObject struct {
	body     []byte
	modified time.Time
}

type memLink struct {
	method, key string
	expires     time.Time
}

func newMemRelay(t *testing.T, now func() time.Time) *memRelay {
	m := &memRelay{now: now, objects: map[string]memObject{}, links: map[string]memLink{}}
	m.srv = httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *memRelay) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	defer m.mu.Unlock()
	link, ok := m.links[r.URL.Query().Get("sig")]
	if !ok || link.method != r.Method || "/o/"+link.key != r.URL.Path || !m.now().Before(link.expires) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		o, ok := m.objects[link.key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write(o.body)
	case http.MethodPut:
		m.objects[link.key] = memObject{body: body, modified: m.now()}
	}
}

func (m *memRelay) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[key]
	if !ok {
		return nil, ErrNoObject
	}
	return o.body, nil
}

func (m *memRelay) Put(_ context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.puts++
	m.objects[key] = memObject{body: slices.Clone(body), modified: m.now()}
	return nil
}

func (m *memRelay) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[key]
	return ok, nil
}

func (m *memRelay) List(_ context.Context, prefix string) ([]RelayObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []RelayObject
	for k, o := range m.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, RelayObject{Key: k, Modified: o.modified})
		}
	}
	slices.SortFunc(out, func(a, b RelayObject) int { return strings.Compare(a.Key, b.Key) })
	return out, nil
}

func (m *memRelay) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *memRelay) presign(method, key string, ttl time.Duration) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	sig := rand.Text()
	m.links[sig] = memLink{method: method, key: key, expires: m.now().Add(ttl)}
	return m.srv.URL + "/o/" + key + "?sig=" + sig
}

func (m *memRelay) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return m.presign(http.MethodGet, key, ttl), nil
}

func (m *memRelay) PresignPut(_ context.Context, key string, ttl time.Duration) (string, error) {
	return m.presign(http.MethodPut, key, ttl), nil
}

func (m *memRelay) keys(prefix string) []string {
	objects, _ := m.List(context.Background(), prefix)
	var out []string
	for _, o := range objects {
		out = append(out, o.Key)
	}
	return out
}

// relayRig is a desktop with a relay in a memRelay, device d1 "Pixel" joined to it, and that phone's side.
type relayRig struct {
	clock time.Time
	store *memRelay
	svc   *fakeService
	desk  *Desk
	relay *Relay
	phone *RelayPhone
	devs  []RelayDevice
}

func newRelayRig(t *testing.T) *relayRig {
	t.Helper()
	g := &relayRig{clock: time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)}
	now := func() time.Time { return g.clock }
	g.store = newMemRelay(t, now)
	g.svc = &fakeService{receive: ReceiveQueue}
	g.desk = &Desk{Service: g.svc, Journal: NewJournal(), Desktop: "Studio", Now: now}
	g.devs = []RelayDevice{{ID: "D1", Name: "Pixel", Key: NewRelayKey()}}
	g.relay = &Relay{Bucket: g.store, Prefix: RelayPrefix([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}), Desk: g.desk, Devices: func() []RelayDevice { return g.devs }, Now: now}
	grant, err := g.relay.Grant(context.Background(), "D1")
	if err != nil {
		t.Fatal(err)
	}
	g.phone = &RelayPhone{Device: "D1", Key: g.devs[0].Key, Grant: grant, Now: now}
	return g
}

func (g *relayRig) send(t *testing.T, id, op string, args any) string {
	t.Helper()
	call := Call{ID: id, V: CallVersion, Op: op}
	if args != nil {
		call.Args, _ = json.Marshal(args)
	}
	slot, err := g.phone.Send(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	return slot
}

func (g *relayRig) poll(t *testing.T, want int) {
	t.Helper()
	n, err := g.relay.Poll(context.Background())
	if err != nil || n != want {
		t.Fatalf("poll answered %d, %v; want %d", n, err, want)
	}
}

func (g *relayRig) reply(t *testing.T, slot string) Reply {
	t.Helper()
	reply, ok, err := g.phone.Reply(context.Background(), slot)
	if err != nil || !ok {
		t.Fatalf("reply of %s: %v %v", slot, ok, err)
	}
	return reply
}

func TestSealRelayOpensOnlyWithItsKeyAndName(t *testing.T) {
	key := NewRelayKey()
	sealed, err := SealRelay(key, "inbox/D1/a", []byte(`{"seq":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("seq")) || !bytes.HasPrefix(sealed, []byte("KRL1")) {
		t.Fatalf("sealed %q", sealed)
	}
	if plain, err := OpenRelay(key, "inbox/D1/a", sealed); err != nil || string(plain) != `{"seq":1}` {
		t.Fatalf("opened %q, %v", plain, err)
	}
	changed := slices.Clone(sealed)
	changed[len(changed)-1] ^= 1
	for name, tc := range map[string]struct {
		key    []byte
		name   string
		sealed []byte
	}{
		"another key":    {NewRelayKey(), "inbox/D1/a", sealed},
		"another slot":   {key, "inbox/D1/b", sealed},
		"another device": {key, "inbox/D2/a", sealed},
		"changed":        {key, "inbox/D1/a", changed},
		"cut short":      {key, "inbox/D1/a", sealed[:20]},
		"not sealed":     {key, "inbox/D1/a", []byte(`{"seq":1}`)},
	} {
		if _, err := OpenRelay(tc.key, tc.name, tc.sealed); !errors.Is(err, ErrRelaySealed) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := SealRelay(key[:16], "x", nil); err == nil {
		t.Fatal("a short key sealed")
	}
}

func TestRelayCarriesACallAndItsReply(t *testing.T) {
	g := newRelayRig(t)
	slot := g.send(t, "c1", OpJobCreate, JobCreate{Link: "https://youtu.be/a"})
	if _, ok, err := g.phone.Reply(context.Background(), slot); ok || err != nil {
		t.Fatalf("a reply before the desktop polled: %v %v", ok, err)
	}
	// What the store holds is sealed: neither the link nor the operation shows.
	for _, k := range g.store.keys("") {
		body, _ := g.store.Get(context.Background(), k)
		if bytes.Contains(body, []byte("youtu.be")) || bytes.Contains(body, []byte(OpJobCreate)) {
			t.Fatalf("%s holds the call in the clear", k)
		}
	}
	g.poll(t, 1)
	reply := g.reply(t, slot)
	var created JobCreated
	if reply.ID != "c1" || reply.Error != nil || json.Unmarshal(reply.Result, &created) != nil || created.Job == nil || created.Job.Link != "https://youtu.be/a" {
		t.Fatalf("reply %+v", reply)
	}
	if got := g.svc.callers; len(got) != 1 || got[0] != (Caller{DeviceID: "D1", Name: "Pixel", Relay: true}) {
		t.Fatalf("callers %+v", got)
	}
	if left := g.store.keys(g.relay.Prefix + "inbox/"); len(left) != 0 {
		t.Fatalf("inbox after the poll %v", left)
	}
	if !strings.HasPrefix(g.relay.Prefix, "kando-relay/0102030405060708/") {
		t.Fatalf("prefix %s", g.relay.Prefix)
	}
}

// TestRelayDoesTheWorkOnceWhenAMessageComesTwice has the phone put the same message again after the desktop answered it, as a PUT whose answer was lost is sent again, and send the same call again in another slot, as a phone that heard no reply does.
func TestRelayDoesTheWorkOnceWhenAMessageComesTwice(t *testing.T) {
	g := newRelayRig(t)
	slot := g.send(t, "c1", OpJobCreate, JobCreate{Link: "https://youtu.be/a"})
	key := g.relay.Prefix + "inbox/D1/" + slot
	sealed, _ := g.store.Get(context.Background(), key)
	g.poll(t, 1)
	first := g.reply(t, slot)
	g.store.Put(context.Background(), key, sealed)
	g.poll(t, 0)
	if left := g.store.keys(g.relay.Prefix + "inbox/"); len(left) != 0 {
		t.Fatalf("the message answered already was kept: %v", left)
	}
	again := g.send(t, "c1", OpJobCreate, JobCreate{Link: "https://youtu.be/a"})
	g.poll(t, 1)
	if second := g.reply(t, again); string(second.Result) != string(first.Result) {
		t.Fatalf("the call sent again answered %s, then %s", first.Result, second.Result)
	}
	if g.svc.made != 1 {
		t.Fatalf("%d jobs made", g.svc.made)
	}
}

func TestRelayAnswersAPhonesMessagesInTheirOrder(t *testing.T) {
	g := newRelayRig(t)
	var links []string
	for i := range 5 {
		link := "https://youtu.be/" + string(rune('a'+i))
		links = append(links, link)
		g.send(t, NewCallID(), OpJobCreate, JobCreate{Link: link})
	}
	g.poll(t, 5)
	var order []string
	for _, slot := range g.phone.Spent {
		var created JobCreated
		json.Unmarshal(g.reply(t, slot).Result, &created)
		order = append(order, created.Job.ID+" "+created.Job.Link)
	}
	// The jobs were made in the order the phone sent their links, whatever order the slots' names list in.
	want := []string{"j1 " + links[0], "j2 " + links[1], "j3 " + links[2], "j4 " + links[3], "j5 " + links[4]}
	if !slices.Equal(order, want) {
		t.Fatalf("made %v", order)
	}
}

func TestRelayDeletesWhatItCannotRead(t *testing.T) {
	g := newRelayRig(t)
	ctx := context.Background()
	slot := g.send(t, "c1", OpJobCreate, JobCreate{Link: "https://youtu.be/a"})
	sealed, _ := g.store.Get(ctx, g.relay.Prefix+RelayInbox("D1", slot))
	// The same sealed message moved to another slot, a stranger's message, junk, and a name that is not a slot's.
	g.store.Put(ctx, g.relay.Prefix+RelayInbox("D1", newSlotID()), sealed)
	stranger, _ := SealRelay(NewRelayKey(), RelayInbox("D9", "x"), []byte(`{}`))
	g.store.Put(ctx, g.relay.Prefix+RelayInbox("D9", newSlotID()), stranger)
	g.store.Put(ctx, g.relay.Prefix+RelayInbox("D1", newSlotID()), []byte("junk"))
	g.store.Put(ctx, g.relay.Prefix+"inbox/D1/state.json", sealed)
	g.poll(t, 1)
	if left := g.store.keys(g.relay.Prefix + "inbox/"); len(left) != 0 {
		t.Fatalf("left in the inboxes: %v", left)
	}
	if replies := g.store.keys(g.relay.Prefix + "outbox/"); len(replies) != 1 || g.svc.made != 1 {
		t.Fatalf("replies %v, %d jobs", replies, g.svc.made)
	}
}

func TestRelayStateFollowsTheDesktopAndRenewsTheGrant(t *testing.T) {
	g := newRelayRig(t)
	ctx := context.Background()
	if _, err := g.phone.State(ctx); !errors.Is(err, ErrNoObject) {
		t.Fatalf("state before any was written: %v", err)
	}
	g.desk.Journal.PublishJobs([]Job{{ID: "j1", Link: "https://youtu.be/a", State: JobRunning}})
	g.desk.Journal.PublishState("lib1")
	if err := g.relay.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := g.phone.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.V != RelayVersion || state.Desktop != "Studio" || !state.Written.Equal(g.clock) || len(state.Jobs.Jobs) != 1 || state.Jobs.Jobs[0].ID != "j1" || state.Learning["lib1"] != 2 || state.Hello.Receive != ReceiveQueue || !slices.Contains(state.Hello.Versions, V2) {
		t.Fatalf("state %+v", state)
	}
	// Nothing changed: no write until the heartbeat is due.
	puts := g.store.puts
	g.clock = g.clock.Add(time.Minute)
	g.relay.Publish(ctx)
	if g.store.puts != puts {
		t.Fatal("an unchanged state was written again")
	}
	g.desk.Journal.PublishJobs([]Job{{ID: "j1", Link: "https://youtu.be/a", State: JobDone}})
	g.relay.Publish(ctx)
	if state, _ := g.phone.State(ctx); g.store.puts != puts+1 || state.Jobs.Jobs[0].State != JobDone {
		t.Fatalf("a changed job: %d writes, %+v", g.store.puts-puts, state.Jobs)
	}
	g.clock = g.clock.Add(RelayHeartbeat)
	g.relay.Publish(ctx)
	if state, _ := g.phone.State(ctx); !state.Written.Equal(g.clock) {
		t.Fatalf("no heartbeat: written %v", state.Written)
	}
	// Once half the slots are used, the next state carries a new grant, which the phone takes.
	old := g.phone.Grant
	for range RelaySlots / 2 {
		g.send(t, NewCallID(), OpJobs, nil)
	}
	g.poll(t, RelaySlots/2)
	g.relay.Publish(ctx)
	if _, err := g.phone.State(ctx); err != nil {
		t.Fatal(err)
	}
	if g.phone.Grant.Slots[0].ID == old.Slots[0].ID || len(g.phone.Spent) != 0 {
		t.Fatalf("the phone kept its grant: %d spent", len(g.phone.Spent))
	}
	// A day on, the grant is renewed however few slots were used, well before its links lapse.
	renewed := g.phone.Grant
	g.clock = g.clock.Add(RelayRegrant)
	g.relay.Publish(ctx)
	g.phone.State(ctx)
	if !g.phone.Grant.Minted.After(renewed.Minted) || !g.phone.Grant.Expires.Equal(g.clock.Add(RelayURLLifetime)) {
		t.Fatalf("grant minted %v, expires %v", g.phone.Grant.Minted, g.phone.Grant.Expires)
	}
}

func TestRelayLinksLapse(t *testing.T) {
	g := newRelayRig(t)
	g.relay.Publish(context.Background())
	g.clock = g.clock.Add(RelayURLLifetime)
	if _, err := g.phone.State(context.Background()); !errors.Is(err, ErrRelayLapsed) {
		t.Fatalf("a lapsed link: %v", err)
	}
}

func TestRelayForgetCutsAPhoneOff(t *testing.T) {
	g := newRelayRig(t)
	ctx := context.Background()
	slot := g.send(t, "c1", OpJobs, nil)
	g.poll(t, 1)
	g.relay.Publish(ctx)
	pending := g.send(t, "c2", OpJobCreate, JobCreate{Link: "https://youtu.be/a"})
	if err := g.relay.Forget(ctx, "D1"); err != nil {
		t.Fatal(err)
	}
	g.devs = nil
	if left := g.store.keys(g.relay.Prefix); len(left) != 0 {
		t.Fatalf("left after forgetting: %v", left)
	}
	if _, err := g.phone.State(ctx); !errors.Is(err, ErrNoObject) {
		t.Fatalf("state after forgetting: %v", err)
	}
	if _, ok, _ := g.phone.Reply(ctx, slot); ok {
		t.Fatal("a reply read after forgetting")
	}
	// A message it still puts through a link that has not lapsed is deleted unread.
	g.phone.Spent = slices.DeleteFunc(g.phone.Spent, func(s string) bool { return s == pending })
	g.send(t, "c3", OpJobCreate, JobCreate{Link: "https://youtu.be/b"})
	g.poll(t, 0)
	if left := g.store.keys(g.relay.Prefix); len(left) != 0 || g.svc.made != 0 {
		t.Fatalf("left %v, %d jobs", left, g.svc.made)
	}
}

func TestRelaySweepsOldRepliesAndStrangers(t *testing.T) {
	g := newRelayRig(t)
	ctx := context.Background()
	slot := g.send(t, "c1", OpJobs, nil)
	g.poll(t, 1)
	g.relay.Publish(ctx)
	g.store.Put(ctx, g.relay.Prefix+RelayStateName("GONE"), []byte("x"))
	g.relay.Sweep(ctx)
	want := []string{g.relay.Prefix + RelayReply("D1", slot), g.relay.Prefix + RelayStateName("D1")}
	if got := g.store.keys(g.relay.Prefix); !slices.Equal(got, want) {
		t.Fatalf("after a sweep %v", got)
	}
	g.clock = g.clock.Add(ReplyMemory)
	g.relay.Sweep(ctx)
	if got := g.store.keys(g.relay.Prefix); !slices.Equal(got, want[1:]) {
		t.Fatalf("after a day %v", got)
	}
}

type fakeJoiner struct{ key []byte }

func (f fakeJoiner) Join(_ context.Context, caller Caller) (RelayJoin, error) {
	return RelayJoin{Key: f.key, Grant: RelayGrant{State: "state of " + caller.DeviceID}}, nil
}

func TestRelayJoinIsOnTheLocalNetworkOnly(t *testing.T) {
	ctx := context.Background()
	desk := &Desk{Service: &fakeService{receive: ReceiveQueue}, Journal: NewJournal()}
	join := Call{ID: "j", V: CallVersion, Op: OpRelayJoin}
	if r := desk.Answer(ctx, Caller{DeviceID: "D1"}, join); r.Error == nil || r.Error.Code != CodeUnsupported {
		t.Fatalf("join with the relay off: %+v", r)
	}
	var h Hello
	json.Unmarshal(desk.Answer(ctx, Caller{DeviceID: "D1"}, Call{V: CallVersion, Op: OpHello}).Result, &h)
	if slices.Contains(h.Ops, OpRelayJoin) {
		t.Fatal("hello lists the join with the relay off")
	}
	key := NewRelayKey()
	desk.Relay = fakeJoiner{key: key}
	json.Unmarshal(desk.Answer(ctx, Caller{DeviceID: "D1"}, Call{V: CallVersion, Op: OpHello}).Result, &h)
	if !slices.Contains(h.Ops, OpRelayJoin) {
		t.Fatalf("hello lists %v", h.Ops)
	}
	r := desk.Answer(ctx, Caller{DeviceID: "D1"}, join)
	var joined RelayJoin
	if r.Error != nil || json.Unmarshal(r.Result, &joined) != nil || !bytes.Equal(joined.Key, key) || joined.Grant.State != "state of D1" {
		t.Fatalf("join %+v", r)
	}
	if r := desk.Answer(ctx, Caller{DeviceID: "D1", Relay: true}, join); r.Error == nil || r.Error.Code != CodeUnsupported {
		t.Fatalf("join over the relay: %+v", r)
	}
}

func TestEventsOverTheRelayDoNotWait(t *testing.T) {
	desk := &Desk{Service: &fakeService{}, Journal: NewJournal()}
	list := desk.Journal.List()
	args, _ := json.Marshal(EventsWait{Epoch: list.Epoch, Since: list.Rev, WaitMS: 5000})
	began := time.Now()
	r := desk.Answer(context.Background(), Caller{DeviceID: "D1", Relay: true}, Call{V: CallVersion, Op: OpEvents, Args: args})
	if r.Error != nil || time.Since(began) > time.Second {
		t.Fatalf("events over the relay took %v: %+v", time.Since(began), r)
	}
}
