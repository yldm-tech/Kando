package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Films through the bucket (yldm-tech/Kando#219). Away from the desktop's network, a phone's Library lists the films the desktop put in the user's bucket: the desktop uploads each film once it is done, and lists them in every phone's state.json (RelayState.Shelf), each with links to its files, as the local network's PathLibraries and PathFilms list them.
//
// Layout. Under RelayPrefix, each film has a folder of a random name below RelayFilmsFolder: its video as "video" with the file's extension, its lines as RelaySubtitlesFile (a Subtitles, as PathSubtitles answers them) and its poster as "poster" with the image's extension. A film the desktop shares the lines of alone has no video there: one whose video the user keeps off the bucket to save room, one cleaned up after it was watched, or one that keeps no video at all; a phone shows and studies its lines and says the video is on the computer. The objects are not sealed, so the system player streams the video from its link; their names are random, and only the links in a phone's state.json reach them.
//
// Links. The desktop presigns a GET link for each file, good for RelayURLLifetime (or until the bucket's short-lived key lapses, Relay.LinksUntil), and signs it anew once it is RelayRegrant old or half its life is spent, as it does a grant, so a phone that reads its state.json holds links that are good for days; between two signings a state.json lists the same links and is not written again for them. A link reaches its file whoever holds it, so every phone is given the same.
//
// The phone. RelayFilms reads a film's lines, poster and video by these links as Client reads them on the local network, the video by ranges, so a download that stopped goes on from where it stopped.

// RelayFilmsFolder is the folder of the films below RelayPrefix.
const RelayFilmsFolder = "films/"

// RelaySubtitlesFile is the name of a film's lines in its folder.
const RelaySubtitlesFile = "subtitles.json"

// RelayFilmObject is the name, below RelayPrefix, of file in the film folder named folder.
func RelayFilmObject(folder, file string) string { return RelayFilmsFolder + folder + "/" + file }

// RelayShelf is the films a desktop put in the bucket, as a phone's state.json lists them: the libraries they are in, in the desktop's order, and the films.
type RelayShelf struct {
	Libraries []Library   `json:"libraries"`
	Films     []RelayFilm `json:"films"`
}

// RelayFilm is a film of a RelayShelf: the ID of its library, the film as PathFilms lists it, and the GET links of its files. The file of a film of sound alone (Film.Sound) is linked as Sound, not Video, as every phone reads the one state.json and one from before them would keep it as an MP4 it cannot play: such a phone studies its lines alone and says its video is on the computer. Film.VideoSize is the size of the video or the sound in the bucket, 0 with neither link; Film.Poster is PosterPath for a film with a Poster link, "" for one without.
type RelayFilm struct {
	Library   string `json:"library"`
	Film      Film   `json:"film"`
	Video     string `json:"video,omitempty"`
	Sound     string `json:"sound,omitempty"`
	Subtitles string `json:"subtitles"`
	Poster    string `json:"poster,omitempty"`
}

// RelayFiles is what the desktop put in the bucket for its phones, as Relay.Films gives it: the libraries, and each film with the names of its objects.
type RelayFiles struct {
	Libraries []Library
	Films     []RelayFilmFiles
}

// RelayFilmFiles is a film the desktop put in the bucket: the ID of its library, the film, and the names below RelayPrefix of its video, lines and poster (RelayFilmObject), "" for one not there. A film without lines is not listed.
type RelayFilmFiles struct {
	Library                  string
	Film                     Film
	Video, Subtitles, Poster string
}

// filmLink is a presigned link to a film's file, when it was signed and when it lapses.
type filmLink struct {
	url             string
	minted, expires time.Time
}

// shelf is what Films gives, as the phones' state.json lists it, nil when it gives nothing: the links of the files, each signed anew once it is RelayRegrant old. It is called on Run's goroutine.
func (r *Relay) shelf(ctx context.Context) (*RelayShelf, error) {
	if r.Films == nil {
		return nil, nil
	}
	files := r.Films()
	if files == nil {
		return nil, nil
	}
	now := r.now()
	r.mu.Lock()
	signed := r.filmLinks
	r.mu.Unlock()
	kept := map[string]filmLink{}
	// life is how long a link signed in this turn stays good, asked once, at the first link signed (linkLife).
	var life time.Duration
	link := func(name string) (string, error) {
		if name == "" {
			return "", nil
		}
		if l, ok := kept[name]; ok {
			return l.url, nil
		}
		if l, ok := signed[name]; ok && now.Sub(l.minted) < RelayRegrant && !halfSpent(l.minted, l.expires, now) {
			kept[name] = l
			return l.url, nil
		}
		if life == 0 {
			var err error
			if life, err = r.linkLife(ctx, now); err != nil {
				return "", err
			}
		}
		u, err := r.Bucket.PresignGet(ctx, r.Prefix+name, life)
		if err != nil {
			return "", err
		}
		kept[name] = filmLink{url: u, minted: now, expires: now.Add(life)}
		return u, nil
	}
	out := &RelayShelf{Libraries: append([]Library{}, files.Libraries...), Films: []RelayFilm{}}
	for _, f := range files.Films {
		if f.Subtitles == "" {
			continue
		}
		rf := RelayFilm{Library: f.Library, Film: f.Film}
		var err error
		if rf.Subtitles, err = link(f.Subtitles); err != nil {
			return nil, err
		}
		media := &rf.Video
		if f.Film.Sound {
			media = &rf.Sound
		}
		if *media, err = link(f.Video); err != nil {
			return nil, err
		}
		if rf.Poster, err = link(f.Poster); err != nil {
			return nil, err
		}
		if rf.Media() == "" {
			rf.Film.VideoSize = 0
		}
		rf.Film.Poster = ""
		if rf.Poster != "" {
			rf.Film.Poster = PosterPath(f.Film.ID)
		}
		out.Films = append(out.Films, rf)
	}
	r.mu.Lock()
	r.filmLinks = kept
	r.mu.Unlock()
	return out, nil
}

// Handoff hands the phones over to the bucket to, as the desktop moves its relay there (换一个存储): it answers what waits in the inboxes here, then writes each phone's state.json here once more with a grant of links to to, which the phone takes as its newest (RelayPhone.State), so from then on it reads its state.json and leaves its messages in to without joining again. Once it went through, this relay writes no state.json any more, so a turn of Run does not take the phones back; it still answers what they left here before they moved. The caller then stops this relay and starts one in to under the same Prefix, whose first grants are newer still; the objects left here are the user's to delete. A handoff that fails partway takes the phones back: each is given a newer grant of links here, in its state.json here and in to, wherever it reads it now.
func (r *Relay) Handoff(ctx context.Context, to RelayBucket) error {
	if _, err := r.Poll(ctx); err != nil {
		r.logf("relay: handoff: inbox: %v", err)
	}
	r.publishing.Lock()
	defer r.publishing.Unlock()
	if r.handedOff {
		return errors.New("kando sync: the relay was handed over already")
	}
	next := &Relay{Bucket: to, Prefix: r.Prefix, Desk: r.Desk, Devices: r.Devices, Now: r.Now, Logf: r.Logf}
	shelf, err := r.shelf(ctx)
	if err != nil {
		return err
	}
	devices := r.Devices()
	given := map[string]RelayGrant{}
	var errs []error
	for _, dev := range devices {
		grant, err := next.NewGrant(ctx, dev.ID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		given[dev.ID] = grant
		errs = append(errs, r.writeState(ctx, dev, grant, shelf, true))
	}
	if err := errors.Join(errs...); err != nil {
		back, cancel := context.WithTimeout(context.WithoutCancel(ctx), relayHandBack)
		defer cancel()
		for _, dev := range devices {
			if g, ok := given[dev.ID]; ok {
				// The grant given last is the one to be newer than, as the phone keeps the newest.
				r.mu.Lock()
				if r.grants == nil {
					r.grants = map[string]*relayGrant{}
				}
				r.grants[dev.ID] = &relayGrant{RelayGrant: g, used: map[string]bool{}}
				r.mu.Unlock()
			}
			grant, gerr := r.NewGrant(back, dev.ID)
			if gerr != nil {
				r.logf("relay: handoff: take %s back: %v", dev.ID, gerr)
				continue
			}
			if werr := r.writeState(back, dev, grant, shelf, true); werr != nil {
				r.logf("relay: handoff: take %s back: %v", dev.ID, werr)
			}
			// A phone that took the grant to to reads its state.json there now.
			if werr := next.writeState(back, dev, grant, shelf, true); werr != nil {
				r.logf("relay: handoff: take %s back from the new bucket: %v", dev.ID, werr)
			}
		}
		return err
	}
	r.handedOff = true
	return nil
}

// relayHandBack is how long a handoff that failed has to give the phones their grants here again.
const relayHandBack = 30 * time.Second

// The most a phone reads of a film's lines and of its poster by their links.
const (
	maxRelayLines  = 64 << 20
	maxRelayPoster = 16 << 20
)

// RelayFilms reads the films of a RelayShelf by their links, as Client reads a desktop's on the local network: their lines, their posters and their videos by ranges, so what downloads a film from the desktop downloads it from the bucket alike. A film it does not list, or one without the file asked for, is an *Error of status 404, as the desktop answers one it does not serve; a link the bucket refuses, as one that lapsed, is ErrRelayLapsed.
type RelayFilms struct {
	// Films are the films by ID; HTTP is the client the links are requested with, nil for http.DefaultClient.
	Films map[string]RelayFilm
	HTTP  *http.Client
}

// NewRelayFilms is the films of shelf, requested with client (nil for http.DefaultClient).
func NewRelayFilms(shelf RelayShelf, client *http.Client) RelayFilms {
	films := make(map[string]RelayFilm, len(shelf.Films))
	for _, f := range shelf.Films {
		films[f.Film.ID] = f
	}
	return RelayFilms{Films: films, HTTP: client}
}

func (r RelayFilms) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return http.DefaultClient
}

// notServed is the error of a film or file the shelf does not list.
func notServed(what string) error {
	return &Error{Status: http.StatusNotFound, ErrorBody: ErrorBody{Code: CodeNotFound, Message: what + " is not in the storage"}}
}

// link is the link pick takes of film id, notServed when there is none.
func (r RelayFilms) link(id, what string, pick func(RelayFilm) string) (string, error) {
	f, ok := r.Films[id]
	if !ok || pick(f) == "" {
		return "", notServed(what)
	}
	return pick(f), nil
}

// fetch requests link with header, and turns an answer of 400 or more into the error it stands for.
func (r RelayFilms) fetch(ctx context.Context, link string, header http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 400 {
		return resp, nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, notServed("the file")
	case http.StatusForbidden:
		return nil, ErrRelayLapsed
	}
	return nil, &Error{Status: resp.StatusCode, ErrorBody: ErrorBody{Code: CodeFailed, Message: fmt.Sprintf("the store answered %d", resp.StatusCode)}}
}

// read reads link whole, at most limit bytes, with its content type.
func (r RelayFilms) read(ctx context.Context, link string, limit int64) ([]byte, string, error) {
	resp, err := r.fetch(ctx, link, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", errors.New("the file in the storage is too large")
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// Subtitles is film id's lines.
func (r RelayFilms) Subtitles(ctx context.Context, id string) ([]Cue, error) {
	link, err := r.link(id, "the film's lines", func(f RelayFilm) string { return f.Subtitles })
	if err != nil {
		return nil, err
	}
	data, _, err := r.read(ctx, link, maxRelayLines)
	if err != nil {
		return nil, err
	}
	var subs Subtitles
	if err := json.Unmarshal(data, &subs); err != nil {
		return nil, err
	}
	return subs.Cues, nil
}

// Poster is film id's poster: the image file's bytes and its content type.
func (r RelayFilms) Poster(ctx context.Context, id string) ([]byte, string, error) {
	link, err := r.link(id, "the film's poster", func(f RelayFilm) string { return f.Poster })
	if err != nil {
		return nil, "", err
	}
	return r.read(ctx, link, maxRelayPoster)
}

// Media is the link of the film's file in the bucket, its video's or, for a film of sound alone, its sound's (Sound); "" for a film whose file is not there.
func (f RelayFilm) Media() string {
	if f.Video != "" {
		return f.Video
	}
	return f.Sound
}

// VideoLink is the link that streams film id's video, or its sound for a film of sound alone, "" for a film whose file is not in the storage.
func (r RelayFilms) VideoLink(id string) string { return r.Films[id].Media() }

// Video is the part of film id's video from byte from to byte to, both included, to -1 for the end, as Client.Video answers it. When ifRange, the ETag of the file a part was kept of, is not the file's now, the answer is the file whole from its start, as If-Range has the desktop answer: the store's ETag names the object, which a film changed since is uploaded as anew.
func (r RelayFilms) Video(ctx context.Context, id string, from, to int64, ifRange string) (*Part, error) {
	link, err := r.link(id, "the film's video", RelayFilm.Media)
	if err != nil {
		return nil, err
	}
	part, err := r.video(ctx, link, from, to)
	if err != nil || from == 0 || ifRange == "" || part.ETag == ifRange {
		return part, err
	}
	part.Body.Close()
	return r.video(ctx, link, 0, to)
}

func (r RelayFilms) video(ctx context.Context, link string, from, to int64) (*Part, error) {
	rng := "bytes=" + strconv.FormatInt(from, 10) + "-"
	if to >= 0 {
		rng += strconv.FormatInt(to, 10)
	}
	resp, err := r.fetch(ctx, link, http.Header{"Range": {rng}})
	if err != nil {
		return nil, err
	}
	part := &Part{Body: resp.Body, ETag: resp.Header.Get("ETag"), ContentType: resp.Header.Get("Content-Type")}
	if resp.StatusCode == http.StatusPartialContent {
		if _, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &part.Start, &part.End, &part.Total); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("kando sync: Content-Range %q: %v", resp.Header.Get("Content-Range"), err)
		}
		return part, nil
	}
	part.Total = resp.ContentLength
	part.End = resp.ContentLength - 1
	return part, nil
}
