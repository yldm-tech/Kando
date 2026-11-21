package sync

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

type stubWatch struct{ got []FilmWatch }

func (s *stubWatch) WatchFilm(_ context.Context, _ Caller, w FilmWatch) (FilmWatching, error) {
	if w.ID == "gone" {
		return FilmWatching{}, Errorf(CodeNotFound, "no film %q", w.ID)
	}
	s.got = append(s.got, w)
	return FilmWatching{Making: true, Updated: time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC), Plays: PlaysSound}, nil
}

func TestDeskAnswersFilmWatch(t *testing.T) {
	ctx := context.Background()
	watch := &stubWatch{}
	d := &Desk{Service: stubService{}, Watch: watch, Journal: NewJournal()}
	// Watching changes nothing the desktop keeps, so a read-only phone may say where it watches, and the same call sent again is answered again rather than from memory.
	for range 2 {
		reply := d.Answer(ctx, Caller{DeviceID: "p", ReadOnly: true}, Call{ID: "1", V: CallVersion, Op: OpFilmWatch, Args: json.RawMessage(`{"id":"f1","at_ms":-5,"claim":true}`)})
		if reply.Error != nil || string(reply.Result) != `{"making":true,"updated":"2026-11-22T08:00:00Z","plays":"sound"}` {
			t.Fatalf("a read-only phone's call answered %s, %+v", reply.Result, reply.Error)
		}
	}
	if !slices.Equal(watch.got, []FilmWatch{{ID: "f1", Claim: true}, {ID: "f1", Claim: true}}) {
		t.Fatalf("the desktop was told %+v", watch.got)
	}
	if reply := d.Answer(ctx, Caller{DeviceID: "p"}, Call{ID: "2", V: CallVersion, Op: OpFilmWatch, Args: json.RawMessage(`{"at_ms":1}`)}); reply.Error == nil || reply.Error.Code != CodeInvalid {
		t.Fatalf("a call naming no film answered %+v", reply)
	}
	if reply := d.Answer(ctx, Caller{DeviceID: "p"}, Call{ID: "3", V: CallVersion, Op: OpFilmWatch, Args: json.RawMessage(`{"id":"gone"}`)}); reply.Error == nil || reply.Error.Code != CodeNotFound {
		t.Fatalf("a film not listed answered %+v", reply)
	}
	// A film being made is not in the storage, so the relay does not carry the call.
	if reply := d.Answer(ctx, Caller{DeviceID: "p", Relay: true}, Call{ID: "4", V: CallVersion, Op: OpFilmWatch, Args: json.RawMessage(`{"id":"f1"}`)}); reply.Error == nil || reply.Error.Code != CodeUnsupported || len(watch.got) != 2 {
		t.Fatalf("a call through the relay answered %+v", reply)
	}

	older := &Desk{Service: stubService{}, Journal: NewJournal()}
	if reply := older.Answer(ctx, Caller{DeviceID: "p"}, Call{ID: "5", V: CallVersion, Op: OpFilmWatch, Args: json.RawMessage(`{"id":"f1"}`)}); reply.Error == nil || reply.Error.Code != CodeUnsupported {
		t.Fatalf("a desktop from before watching answered %+v", reply)
	}
}

func TestFilmMakingIsAnAddition(t *testing.T) {
	data, err := json.Marshal(Film{ID: "f1"})
	if err != nil || strings.Contains(string(data), "making") {
		t.Fatalf("a done film reads %s, %v", data, err)
	}
	if !AsksWith("subtitles_only,making", WithMaking) || AsksWith("sound", WithMaking) {
		t.Fatal("asking for the films being made reads wrong")
	}
	if Mutates(OpFilmWatch) {
		t.Fatal("watching a film counts as a change")
	}
}
