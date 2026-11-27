package sync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// putFilm puts a film's files in the rig's store under folder, its video when video is not nil, and answers its files as the desktop lists them.
func (g *relayRig) putFilm(t *testing.T, folder string, f Film, cues []Cue, video []byte) RelayFilmFiles {
	t.Helper()
	ctx := context.Background()
	ff := RelayFilmFiles{Library: "lib", Film: f, Subtitles: RelayFilmObject(folder, RelaySubtitlesFile)}
	lines, _ := json.Marshal(Subtitles{Cues: cues})
	if err := g.store.Put(ctx, g.relay.Prefix+ff.Subtitles, lines); err != nil {
		t.Fatal(err)
	}
	if video != nil {
		ff.Video = RelayFilmObject(folder, "video.mp4")
		if err := g.store.Put(ctx, g.relay.Prefix+ff.Video, video); err != nil {
			t.Fatal(err)
		}
	}
	return ff
}

// The desktop lists the films it put in the bucket in every phone's state.json, with links the phone reads their lines and video by: a film shared as lines alone has no video link. The links stay the same from one state.json to the next, so the state is not written again for them, until they are a day old, when they are signed anew.
func TestRelayListsTheFilmsWithLinks(t *testing.T) {
	g := newRelayRig(t)
	ctx := context.Background()
	video := []byte("0123456789abcdef")
	files := &RelayFiles{Libraries: []Library{{ID: "lib", Name: "Japanese Class", Films: 2}}, Films: []RelayFilmFiles{
		g.putFilm(t, "a1", Film{ID: "f1", Title: "Morning Talk", VideoSize: 16, Poster: PosterPath("f1")}, []Cue{{StartMS: 1000, EndMS: 2000, Original: "おはよう", Translation: "早上好"}}, video),
		g.putFilm(t, "b2", Film{ID: "f2", Title: "Evening Talk", VideoSize: 99}, []Cue{{StartMS: 0, EndMS: 500, Original: "こんばんは"}}, nil),
	}}
	g.relay.Films = func() *RelayFiles { return files }
	if err := g.relay.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := g.phone.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Shelf == nil || len(state.Shelf.Films) != 2 || state.Shelf.Libraries[0].Name != "Japanese Class" {
		t.Fatalf("shelf %+v", state.Shelf)
	}
	f1, f2 := state.Shelf.Films[0], state.Shelf.Films[1]
	if f1.Video == "" || f1.Film.VideoSize != 16 || f1.Film.Poster != "" || f1.Poster != "" || f1.Library != "lib" {
		t.Fatalf("a film with its video: %+v", f1)
	}
	if f2.Video != "" || f2.Film.VideoSize != 0 || f2.Subtitles == "" {
		t.Fatalf("a film shared as lines alone: %+v", f2)
	}
	films := NewRelayFilms(*state.Shelf, nil)
	cues, err := films.Subtitles(ctx, "f1")
	if err != nil || len(cues) != 1 || cues[0].Translation != "早上好" {
		t.Fatalf("lines %+v, %v", cues, err)
	}
	part, err := films.Video(ctx, "f1", 0, -1, "")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(part.Body)
	part.Body.Close()
	if string(got) != string(video) || part.Total != 16 || part.ETag == "" {
		t.Fatalf("video %q of %d, etag %q", got, part.Total, part.ETag)
	}
	var e *Error
	if _, err := films.Video(ctx, "f2", 0, -1, ""); !errors.As(err, &e) || e.Status != http.StatusNotFound {
		t.Fatalf("the video of a film shared as lines alone: %v", err)
	}
	if _, _, err := films.Poster(ctx, "f2"); !errors.As(err, &e) || e.Status != http.StatusNotFound {
		t.Fatalf("a poster not there: %v", err)
	}

	puts := g.store.puts
	g.clock = g.clock.Add(time.Minute)
	g.relay.Publish(ctx)
	if g.store.puts != puts {
		t.Fatal("the same films were written again")
	}
	g.clock = g.clock.Add(RelayRegrant)
	g.relay.Publish(ctx)
	again, err := g.phone.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.Shelf.Films[0].Video == f1.Video {
		t.Fatal("a day on, the video's link was not signed anew")
	}
	// The link signed first lapses a week after it was signed; the new one reads on.
	g.clock = g.clock.Add(RelayURLLifetime - RelayRegrant)
	if _, err := films.Video(ctx, "f1", 0, -1, ""); !errors.Is(err, ErrRelayLapsed) {
		t.Fatalf("a lapsed link: %v", err)
	}
	if _, err := NewRelayFilms(*again.Shelf, nil).Subtitles(ctx, "f1"); err != nil {
		t.Fatalf("the new link: %v", err)
	}
}

// A video is read by ranges from where a download stopped; a part kept of a file uploaded anew since, whose ETag is another, comes whole from the start.
func TestRelayFilmsReadTheVideoByRanges(t *testing.T) {
	g := newRelayRig(t)
	ctx := context.Background()
	files := &RelayFiles{Films: []RelayFilmFiles{g.putFilm(t, "a1", Film{ID: "f1"}, nil, []byte("0123456789"))}}
	g.relay.Films = func() *RelayFiles { return files }
	g.relay.Publish(ctx)
	state, err := g.phone.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	films := NewRelayFilms(*state.Shelf, nil)
	whole, err := films.Video(ctx, "f1", 0, -1, "")
	if err != nil {
		t.Fatal(err)
	}
	whole.Body.Close()
	part, err := films.Video(ctx, "f1", 4, -1, whole.ETag)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(part.Body)
	part.Body.Close()
	if string(got) != "456789" || part.Start != 4 || part.End != 9 || part.Total != 10 {
		t.Fatalf("from byte 4: %q, %d-%d/%d", got, part.Start, part.End, part.Total)
	}
	part, err = films.Video(ctx, "f1", 4, -1, `"another file"`)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = io.ReadAll(part.Body)
	part.Body.Close()
	if string(got) != "0123456789" || part.Start != 0 {
		t.Fatalf("a part of another file: %q from %d", got, part.Start)
	}
}

// Moving the relay to another bucket hands the phones over: the state.json in the old bucket carries a grant of links to the new one, which the phone takes, and from then on it reads its state.json and leaves its calls there, which a relay in the new bucket answers.
func TestRelayHandoffMovesThePhonesToAnotherBucket(t *testing.T) {
	g := newRelayRig(t)
	ctx := context.Background()
	g.relay.Publish(ctx)
	if _, err := g.phone.State(ctx); err != nil {
		t.Fatal(err)
	}
	slot := g.send(t, "c1", OpJobs, nil)
	moved := newMemRelay(t, func() time.Time { return g.clock })
	g.clock = g.clock.Add(time.Minute)
	if err := g.relay.Handoff(ctx, moved, nil); err != nil {
		t.Fatal(err)
	}
	// What waited in the old bucket was answered there before the move.
	if r := g.reply(t, slot); r.ID != "c1" {
		t.Fatalf("reply %+v", r)
	}
	// A turn of the old relay before it stops does not write over the state.json that hands the phone over.
	g.clock = g.clock.Add(time.Minute)
	if err := g.relay.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := g.phone.State(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := g.phone.State(ctx); !errors.Is(err, ErrNoObject) {
		t.Fatalf("the new bucket's state.json before its relay wrote one: %v", err)
	}
	next := &Relay{Bucket: moved, Prefix: g.relay.Prefix, Desk: g.desk, Devices: g.relay.Devices, Now: g.relay.Now}
	g.clock = g.clock.Add(time.Minute)
	if err := next.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := g.phone.State(ctx); err != nil {
		t.Fatalf("the new bucket's state.json: %v", err)
	}
	slot = g.send(t, "c2", OpJobs, nil)
	if n, err := next.Poll(ctx); err != nil || n != 1 {
		t.Fatalf("the new relay answered %d, %v", n, err)
	}
	if r := g.reply(t, slot); r.ID != "c2" {
		t.Fatalf("reply %+v", r)
	}
	if keys := g.store.keys(g.relay.Prefix + "inbox/"); len(keys) != 0 {
		t.Fatalf("the old bucket's inbox holds %q", keys)
	}
}

// failingPuts is a bucket whose links to PUT a key holding fail are not signed, as a store that refuses them; before refusing it calls first, once.
type failingPuts struct {
	*memRelay
	fail  string
	first *func()
}

func (f failingPuts) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if strings.Contains(key, f.fail) {
		if *f.first != nil {
			(*f.first)()
			*f.first = nil
		}
		return "", errors.New("refused")
	}
	return f.memRelay.PresignPut(ctx, key, ttl)
}

// A handoff that fails for one phone takes the others back: the phone that read its state.json between the handoff's writes, and took the grant to the new bucket, finds there a newer grant of links to the old one, and goes on with the relay that still runs there.
func TestAHandoffThatFailsTakesThePhonesBack(t *testing.T) {
	g := newRelayRig(t)
	ctx := context.Background()
	g.devs = append(g.devs, RelayDevice{ID: "D2", Name: "iPad", Key: NewRelayKey()})
	g.relay.Publish(ctx)
	if _, err := g.phone.State(ctx); err != nil {
		t.Fatal(err)
	}
	var tookNew error
	first := func() { _, tookNew = g.phone.State(ctx) }
	moved := newMemRelay(t, func() time.Time { return g.clock })
	if err := g.relay.Handoff(ctx, failingPuts{moved, "/D2/", &first}, nil); err == nil {
		t.Fatal("a handoff that failed for a phone went through")
	}
	if tookNew != nil {
		t.Fatalf("the state.json the handoff wrote: %v", tookNew)
	}
	// The phone reads its state.json in the new bucket now, which takes it back.
	if _, err := g.phone.State(ctx); err != nil {
		t.Fatalf("the new bucket's state.json after the handoff failed: %v", err)
	}
	if _, err := g.phone.State(ctx); err != nil {
		t.Fatalf("the old bucket's state.json: %v", err)
	}
	slot := g.send(t, "c1", OpJobs, nil)
	g.poll(t, 1)
	if r := g.reply(t, slot); r.ID != "c1" {
		t.Fatalf("reply %+v", r)
	}
	// The old relay goes on writing the phones' state.json.
	g.clock = g.clock.Add(time.Minute)
	if err := g.relay.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := g.phone.State(ctx); err != nil {
		t.Fatal(err)
	}
	if keys := moved.keys(g.relay.Prefix + "inbox/"); len(keys) != 0 {
		t.Fatalf("the new bucket's inbox holds %q", keys)
	}
}

// A film of sound alone is linked as its sound, not its video, so a phone from before them studies its lines alone rather than keep it as an MP4; a phone that knows them reads it by its link as it reads a video.
func TestRelayLinksAFilmOfSoundAsItsSound(t *testing.T) {
	g := newRelayRig(t)
	ctx := context.Background()
	sound := []byte("ID3 sound")
	ff := g.putFilm(t, "c3", Film{ID: "f3", Title: "Episode", VideoSize: 9, Sound: true}, []Cue{{StartMS: 0, EndMS: 500, Original: "こんにちは"}}, sound)
	g.relay.Films = func() *RelayFiles {
		return &RelayFiles{Libraries: []Library{{ID: "lib", Name: "Podcasts", Films: 1}}, Films: []RelayFilmFiles{ff}}
	}
	if err := g.relay.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := g.phone.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rf := state.Shelf.Films[0]
	if rf.Video != "" || rf.Sound == "" || rf.Media() != rf.Sound || rf.Film.VideoSize != 9 || !rf.Film.Sound {
		t.Fatalf("a film of sound: %+v", rf)
	}
	films := NewRelayFilms(*state.Shelf, nil)
	if films.VideoLink("f3") != rf.Sound {
		t.Fatalf("the sound streams from %q", films.VideoLink("f3"))
	}
	part, err := films.Video(ctx, "f3", 0, -1, "")
	if err != nil {
		t.Fatal(err)
	}
	defer part.Body.Close()
	if got, _ := io.ReadAll(part.Body); string(got) != string(sound) {
		t.Fatalf("read %q", got)
	}
}
