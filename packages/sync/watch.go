package sync

import (
	"context"
	"time"
)

// Watching a film while it is made (Film.Making): a phone playing one tells its computer where it is watched, so the computer makes the lines there first, as it does for the film open in its own player, and learns how far the film has come. It is an addition to v2: a desktop that answers it lists it in Hello.Ops, one before it answers it CodeUnsupported; a phone before it never lists films being made. It only reads (not Mutates): it changes nothing the desktop keeps, as opening a film in its player does not, so a read-only device may send it. It goes over the local network alone: over the storage relay it answers CodeUnsupported, as a film being made is not in the storage.
//
// The film first is the one last claimed: opened, played or moved in a player, on the computer or on a phone (FilmWatch.Claim). A phone sends the call when it opens the film, plays it, holds it or moves its playhead, each a claim, and every WatchEvery while the film is open, which keeps its claim and moves where it is watched without taking the film first from a player that claimed since; WatchLease after its last call the computer no longer counts it watching.

// OpFilmWatch takes FilmWatch and answers FilmWatching.
const OpFilmWatch = "films.watch"

// How often a phone watching a film being made says so, and how long the computer counts it watching after it last did.
const (
	WatchEvery = 5 * time.Second
	WatchLease = 15 * time.Second
)

// FilmWatch is where film ID is watched on the phone, AtMS milliseconds from its start, and whether the phone's user just claimed it: opened, played, held or moved it.
type FilmWatch struct {
	ID    string `json:"id"`
	AtMS  int64  `json:"at_ms"`
	Claim bool   `json:"claim,omitempty"`
}

// What PathVideo answers of a film being made (FilmWatching.Plays): its video, or a link's sound, downloaded first, until its video comes; "" while it has neither.
const (
	PlaysVideo = "video"
	PlaysSound = "sound"
)

// FilmWatching is how far a film watched has come: Making while its job is waiting or running, false once it is done, when the phone reads it again as any other film; Updated, when its lines last changed (Film.Updated), so the phone reads them again (PathSubtitles) when it moves; and Plays, what PathVideo answers of it now, so the phone plays the video in place of the sound once it comes. A film no longer listed, as one removed or failed, answers CodeNotFound.
type FilmWatching struct {
	Making  bool      `json:"making"`
	Updated time.Time `json:"updated"`
	Plays   string    `json:"plays"`
}

// WatchService is what a desktop does for OpFilmWatch, over the local network. Its errors are as Service's.
type WatchService interface {
	WatchFilm(ctx context.Context, caller Caller, w FilmWatch) (FilmWatching, error)
}

// answerWatch runs OpFilmWatch.
func (d *Desk) answerWatch(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.Watch == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not make films first for phones")
	}
	if caller.Relay {
		return nil, Errorf(CodeUnsupported, "a film being made is watched on the local network only")
	}
	var w FilmWatch
	if err := args(call, &w); err != nil {
		return nil, err
	}
	if w.ID == "" {
		return nil, Errorf(CodeInvalid, "%s names no film", call.Op)
	}
	if w.AtMS < 0 {
		w.AtMS = 0
	}
	return d.Watch.WatchFilm(ctx, caller, w)
}

// WatchFilm tells the computer film id is watched at at, claimed by the phone's user when claim is set, and answers how far it has come.
func (c *Client) WatchFilm(ctx context.Context, id string, at time.Duration, claim bool) (FilmWatching, error) {
	var out FilmWatching
	err := c.Call(ctx, OpFilmWatch, FilmWatch{ID: id, AtMS: at.Milliseconds(), Claim: claim}, &out)
	return out, err
}
