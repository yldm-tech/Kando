package phonesync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"time"

	"kando/apps/kando/cloud"
	psync "kando/packages/sync"
)

// Films through the bucket (psync's relay_films.go, yldm-tech/Kando#219): while the relay runs with Config.Films set, the server puts every film of the libraries it serves in the user's bucket for the phones away from this computer's network: its lines and poster as soon as it is done, then its video, one film after another, and lists them in each phone's state.json with links to the files. It does so on a goroutine of its own, as the relay does, when the jobs change (a film is done) and every filmsEvery besides. A video goes in parts, and an upload that stopped goes on from the parts the store took, which the server keeps with what it put in the bucket, in FilmsFile. A film no library serves any more, as one deleted or in a library locked since, is deleted from the bucket. With Videos off, films go as their lines alone, and videos already there stay; a video the user cleaned up, or that was watched to its end CleanAfter ago, is deleted and not put back while the film's file is the same.

// FilmsFile is the file, in the config folder, where the server keeps what it put in the bucket for the phones.
const FilmsFile = "phone-sync-relay-films.json"

// filmsEvery is how often the server looks at the libraries for films to put in the bucket when the jobs do not change. Tests shorten it.
var filmsEvery = 5 * time.Minute

// filmsRetry is how long the server waits after a turn that failed before the next. Tests shorten it.
var filmsRetry = time.Minute

// FilmsConfig is how the server shares the films through the bucket.
type FilmsConfig struct {
	// Videos is whether the films' videos go to the bucket; without it a film goes as its lines and poster, which the phones show and study while the video stays on the computer.
	Videos bool
	// CleanAfter is how long after a film was last watched to its end its video is deleted from the bucket, 0 for never.
	CleanAfter time.Duration
	// Store names the bucket (its kind, endpoint, region and name, without the keys), so what was put in one is not taken for what is in another, as after the user moved the relay.
	Store string
	// Status is told, on the server's goroutine, where the films stand after each change.
	Status func(FilmsStatus)
}

// FilmsStatus is where the films shared through the bucket stand, as Settings › Phone Sync shows them.
type FilmsStatus struct {
	// Measured is a listing of the bucket that went through, which Videos, Lines and Mail measure: the bytes of the videos, of the lines and posters, and of the phones' inboxes and outboxes.
	Measured            bool
	Videos, Lines, Mail int64
	Films, WithVideo    int
	// Uploading is the title of the film whose video goes up now, "" for none, with Sent of its Size bytes taken; Waiting is how many films' videos are still to go up, that one among them.
	Uploading  string
	Sent, Size int64
	Waiting    int
	// Watched are the films whose video is in the bucket and which were watched to their end, the longest ago first, which the user may clean up.
	Watched []WatchedFilm
	// Err is why the last turn failed, nil for one that went through. Settled is a status told as a turn ended, until the next measure: one told within a turn, with videos still to go up or a failure not yet known, is not.
	Err     error
	Settled bool
}

// WatchedFilm is a film whose video is in the bucket and was watched to its end: its library's ID and its ID, its title, the size of its video in the bucket, and when it was last watched to its end, zero when not known.
type WatchedFilm struct {
	Library, ID, Title string
	Size               int64
	At                 time.Time
}

// filmsData is FilmsFile: the bucket the films are in (FilmsConfig.Store), and each film put there.
type filmsData struct {
	Store string       `json:"store"`
	Films []filmRecord `json:"films"`
}

// filmRecord is a film the server put in the bucket: its library's ID and its ID, the folder of a random name its files are in, and each file, with what it was put from so a change is put again.
type filmRecord struct {
	Library string     `json:"library"`
	ID      string     `json:"id"`
	Folder  string     `json:"folder"`
	Film    psync.Film `json:"film"`
	// Lines is the lines' object, put for the film as it was at LinesOf (psync.Film.Updated).
	Lines   string    `json:"lines,omitempty"`
	LinesOf time.Time `json:"linesOf,omitzero"`
	// Poster is the poster's object, put from a file of PosterSize bytes changed at PosterAt.
	Poster     string    `json:"poster,omitempty"`
	PosterSize int64     `json:"posterSize,omitempty"`
	PosterAt   time.Time `json:"posterAt,omitzero"`
	// Video is the video's object, "" while none is there, put from a file of VideoSize bytes changed at VideoAt; Upload is how far one under way got. Cleaned is a video deleted after it was watched, which is not put back while the file is the one of VideoSize and VideoAt.
	Video     string        `json:"video,omitempty"`
	VideoSize int64         `json:"videoSize,omitempty"`
	VideoAt   time.Time     `json:"videoAt,omitzero"`
	Upload    *cloud.Resume `json:"upload,omitempty"`
	Cleaned   bool          `json:"cleaned,omitempty"`
	// Watched is a film watched to its end, last at WatchedAt, zero when that is not known; Title is its title, which the cleanup lists it by.
	Watched   bool      `json:"watched,omitempty"`
	WatchedAt time.Time `json:"watchedAt,omitzero"`
	Title     string    `json:"title,omitempty"`
}

func (r filmRecord) key() string { return r.Library + "/" + r.ID }

// films is the server's films through the bucket. Its fields are guarded by mu, but bucket and cfg's Store, which do not change.
type films struct {
	s      *Server
	bucket Bucket
	path   string
	wake   chan struct{}

	mu     gosync.Mutex
	cfg    FilmsConfig
	data   filmsData
	libs   []psync.Library
	status FilmsStatus
}

// startFilms starts putting the films in bucket, until Close. Start calls it after startRelay, whose Films it sets.
func (s *Server) startFilms(bucket Bucket, cfg FilmsConfig) {
	f := &films{s: s, bucket: bucket, cfg: cfg, path: filepath.Join(s.cfg.Dir, FilmsFile), wake: make(chan struct{}, 1)}
	f.load()
	s.shared = f
	s.relay.Films = f.files
	ctx, cancel := context.WithCancel(context.Background())
	s.done.Add(2)
	go func() {
		defer s.done.Done()
		<-s.stop
		cancel()
	}()
	go func() {
		defer s.done.Done()
		f.run(ctx)
	}()
}

// load reads FilmsFile, keeping it only when it is of the bucket the films go to now.
func (f *films) load() {
	data, err := os.ReadFile(f.path)
	if err == nil {
		var saved filmsData
		if json.Unmarshal(data, &saved) == nil && saved.Store == f.cfg.Store {
			f.data = saved
		}
	}
	f.data.Store = f.cfg.Store
}

// save writes FilmsFile, readable by the user alone. f.mu is held.
func (f *films) save() error {
	data, err := json.MarshalIndent(f.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

// files is what the films put in the bucket are, for the relay's state.json: those whose lines are there.
func (f *films) files() *psync.RelayFiles {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &psync.RelayFiles{Libraries: slices.Clone(f.libs)}
	for _, r := range f.data.Films {
		if r.Lines == "" {
			continue
		}
		film := r.Film
		film.VideoSize = 0
		if r.Video != "" {
			film.VideoSize = r.VideoSize
		}
		out.Films = append(out.Films, psync.RelayFilmFiles{Library: r.Library, Film: film, Video: r.Video, Subtitles: r.Lines, Poster: r.Poster})
	}
	return out
}

// Wake has the films looked at again now, as the policy or a library changed.
func (f *films) Wake() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

// run takes turns until ctx ends: one now, then one when the jobs change, when woken, or every filmsEvery, and filmsRetry after one that failed.
func (f *films) run(ctx context.Context) {
	journal := f.s.desk.Journal
	for ctx.Err() == nil {
		err := f.turn(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			f.s.cfg.Logf("phonesync: relay films: %v", err)
		}
		f.tell(func(st *FilmsStatus) { st.Err, st.Settled = err, true })
		wait := filmsEvery
		if err != nil {
			wait = filmsRetry
		}
		list := journal.List()
		waiting, stop := context.WithCancel(ctx)
		go func() {
			select {
			case <-f.wake:
				stop()
			case <-waiting.Done():
			}
		}()
		journal.Wait(waiting, list.Epoch, list.Rev, wait)
		stop()
	}
}

// tell changes the status by change and tells it.
func (f *films) tell(change func(*FilmsStatus)) {
	f.mu.Lock()
	change(&f.status)
	st := f.status
	st.Watched = slices.Clone(st.Watched)
	report := f.cfg.Status
	f.mu.Unlock()
	if report != nil {
		report(st)
	}
}

// listed is a film a library serves now, with the library it is in.
type listed struct {
	lib  Library
	film Film
}

// turn puts what is missing or changed of the films in the bucket, deletes what is there of films no library serves, cleans up the videos watched long enough ago, and measures the bucket.
func (f *films) turn(ctx context.Context) error {
	prefix := f.s.relay.Prefix
	have, err := f.objects(ctx, prefix)
	if err != nil {
		return err
	}
	libs, err := f.s.cfg.Source.Libraries(ctx)
	if err != nil {
		return err
	}
	var all []listed
	var shelf []psync.Library
	for _, lib := range libs {
		list, err := f.s.cfg.Source.Films(ctx, lib)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		shelf = append(shelf, psync.Library{ID: lib.ID, Name: lib.Name, Films: len(list)})
		for _, film := range list {
			all = append(all, listed{lib: lib, film: film})
		}
	}
	f.mu.Lock()
	f.libs = shelf
	policy := f.cfg
	keep := map[string]bool{}
	for _, l := range all {
		keep[l.lib.ID+"/"+l.film.ID] = true
	}
	var gone []filmRecord
	f.data.Films = slices.DeleteFunc(f.data.Films, func(r filmRecord) bool {
		if keep[r.key()] {
			return false
		}
		gone = append(gone, r)
		return true
	})
	f.mu.Unlock()
	var errs []error
	for _, r := range gone {
		for _, name := range []string{r.Lines, r.Poster, r.Video} {
			if name != "" {
				errs = append(errs, f.bucket.Delete(ctx, prefix+name))
			}
		}
	}
	// The lines and posters first, so every film shows on the phones before the first video is up.
	var videos []listed
	for _, l := range all {
		if err := f.small(ctx, prefix, l, have); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			errs = append(errs, err)
			continue
		}
		if f.clean(ctx, prefix, l, policy) {
			continue
		}
		if f.needsVideo(l, have, policy) {
			videos = append(videos, l)
		}
	}
	f.persist(&errs)
	f.s.relay.Wake()
	f.measure(ctx, prefix, len(videos))
	failed := 0
	for i, l := range videos {
		if err := f.video(ctx, prefix, l, len(videos)-i); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			errs = append(errs, err)
			failed++
		}
		f.s.relay.Wake()
	}
	if len(videos) > 0 {
		// A video that failed is still to go up, at the next turn.
		f.measure(ctx, prefix, failed)
	}
	return errors.Join(errs...)
}

// objects are the sizes of the objects of the relay, by name below prefix.
func (f *films) objects(ctx context.Context, prefix string) (map[string]int64, error) {
	list, err := f.bucket.Sizes(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(list))
	for _, o := range list {
		out[strings.TrimPrefix(o.Key, prefix)] = o.Size
	}
	return out, nil
}

// measure lists the bucket for the status, with waiting videos still to go up.
func (f *films) measure(ctx context.Context, prefix string, waiting int) {
	have, err := f.objects(ctx, prefix)
	if err != nil {
		return
	}
	f.mu.Lock()
	st := FilmsStatus{Measured: true, Waiting: waiting}
	for name, size := range have {
		switch {
		case strings.HasPrefix(name, psync.RelayFilmsFolder) && strings.HasPrefix(filepath.Base(name), "video"):
			st.Videos += size
		case strings.HasPrefix(name, psync.RelayFilmsFolder):
			st.Lines += size
		default:
			st.Mail += size
		}
	}
	for _, r := range f.data.Films {
		if r.Lines == "" {
			continue
		}
		st.Films++
		if r.Video == "" {
			continue
		}
		st.WithVideo++
		if r.Watched {
			st.Watched = append(st.Watched, WatchedFilm{Library: r.Library, ID: r.ID, Title: r.Title, Size: r.VideoSize, At: r.WatchedAt})
		}
	}
	slices.SortStableFunc(st.Watched, func(a, b WatchedFilm) int { return a.At.Compare(b.At) })
	f.mu.Unlock()
	f.tell(func(cur *FilmsStatus) {
		// What goes up now and why the last turn failed stay as they are.
		st.Uploading, st.Sent, st.Size, st.Err = cur.Uploading, cur.Sent, cur.Size, cur.Err
		*cur = st
	})
}

// record is the record of l, made with a new folder for a film not put in the bucket yet. f.mu is held; the pointer is good until the list changes.
func (f *films) record(l listed) *filmRecord {
	key := l.lib.ID + "/" + l.film.ID
	if i := slices.IndexFunc(f.data.Films, func(r filmRecord) bool { return r.key() == key }); i >= 0 {
		return &f.data.Films[i]
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	f.data.Films = append(f.data.Films, filmRecord{Library: l.lib.ID, ID: l.film.ID, Folder: hex.EncodeToString(b)})
	return &f.data.Films[len(f.data.Films)-1]
}

// update changes l's record by change, under f.mu.
func (f *films) update(l listed, change func(*filmRecord)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(f.record(l))
}

// persist saves FilmsFile, adding a failure to errs.
func (f *films) persist(errs *[]error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.save(); err != nil {
		*errs = append(*errs, err)
	}
}

// described is l's film as the phones list it, with when its files last changed.
func described(l listed) psync.Film {
	film := l.film
	p := psync.Film{ID: film.ID, Title: film.Title, Source: film.Source, DurationMS: film.Duration.Milliseconds(), Spoken: film.Spoken, Translation: film.Translation, Untranslated: film.Untranslated}
	var updated time.Time
	for _, path := range []string{film.Video, film.Subtitles, film.Transcript, film.Corrections} {
		if info, err := os.Stat(path); path != "" && err == nil && !info.IsDir() && info.ModTime().After(updated) {
			updated = info.ModTime()
		}
	}
	p.Updated = updated.UTC()
	return p
}

// small puts l's lines and poster in the bucket where they are missing or changed. A film without lines is not shared.
func (f *films) small(ctx context.Context, prefix string, l listed, have map[string]int64) error {
	film := described(l)
	f.mu.Lock()
	r := *f.record(l)
	f.mu.Unlock()
	_, linesThere := have[r.Lines]
	if r.Lines == "" || !linesThere || !r.LinesOf.Equal(film.Updated) {
		cues, err := f.s.cfg.Source.Cues(ctx, l.film)
		if errors.Is(err, ErrNotFound) {
			// A film without lines yet is not shared, and is no failure.
			return nil
		}
		if err != nil {
			return err
		}
		body, err := json.Marshal(psync.Subtitles{Cues: append([]psync.Cue{}, cues...)})
		if err != nil {
			return err
		}
		name := psync.RelayFilmObject(r.Folder, psync.RelaySubtitlesFile)
		if err := f.bucket.Put(ctx, prefix+name, body); err != nil {
			return err
		}
		r.Lines, r.LinesOf = name, film.Updated
	}
	if info, err := os.Stat(l.film.Poster); l.film.Poster != "" && err == nil && !info.IsDir() {
		_, posterThere := have[r.Poster]
		if r.Poster == "" || !posterThere || r.PosterSize != info.Size() || !r.PosterAt.Equal(info.ModTime()) {
			name := psync.RelayFilmObject(r.Folder, "poster"+strings.ToLower(filepath.Ext(l.film.Poster)))
			if err := f.bucket.PutFile(ctx, prefix+name, l.film.Poster, "", cloud.Resume{}, func(cloud.Resume) error { return nil }, nil); err != nil {
				return err
			}
			if r.Poster != "" && r.Poster != name {
				_ = f.bucket.Delete(ctx, prefix+r.Poster)
			}
			r.Poster, r.PosterSize, r.PosterAt = name, info.Size(), info.ModTime()
		}
	}
	_, videoThere := have[r.Video]
	f.update(l, func(cur *filmRecord) {
		cur.Lines, cur.LinesOf = r.Lines, r.LinesOf
		cur.Poster, cur.PosterSize, cur.PosterAt = r.Poster, r.PosterSize, r.PosterAt
		cur.Film, cur.Title = film, film.Title
		cur.Watched, cur.WatchedAt = l.film.Watched, l.film.WatchedAt
		if cur.Video == r.Video && r.Video != "" && !videoThere {
			// Deleted from the bucket by other hands: it goes up again, unless it was cleaned up.
			cur.Video = ""
		}
	})
	return nil
}

// fileOf is the size and time of l's video file, false when it has none on disk.
func fileOf(l listed) (int64, time.Time, bool) {
	info, err := os.Stat(l.film.Video)
	if l.film.Video == "" || err != nil || info.IsDir() {
		return 0, time.Time{}, false
	}
	return info.Size(), info.ModTime(), true
}

// needsVideo reports whether l's video is to go up: videos go, its file is on disk, and the bucket holds none of it as it is now, nor was it cleaned up as it is now.
func (f *films) needsVideo(l listed, have map[string]int64, policy FilmsConfig) bool {
	size, at, ok := fileOf(l)
	if !policy.Videos || !ok {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.record(l)
	same := r.VideoSize == size && r.VideoAt.Equal(at)
	if r.Lines == "" || (r.Cleaned && same) {
		return false
	}
	return r.Video == "" || !same
}

// clean deletes l's video from the bucket once it was watched to its end CleanAfter ago, and reports whether it did.
func (f *films) clean(ctx context.Context, prefix string, l listed, policy FilmsConfig) bool {
	f.mu.Lock()
	r := *f.record(l)
	f.mu.Unlock()
	if policy.CleanAfter <= 0 || r.Video == "" || !l.film.Watched || l.film.WatchedAt.IsZero() || f.s.cfg.Now().Sub(l.film.WatchedAt) < policy.CleanAfter {
		return false
	}
	return f.cleanVideo(ctx, prefix, r.key()) == nil
}

// cleanVideo deletes the video of the film of key from the bucket, and keeps that it was cleaned up.
func (f *films) cleanVideo(ctx context.Context, prefix, key string) error {
	f.mu.Lock()
	i := slices.IndexFunc(f.data.Films, func(r filmRecord) bool { return r.key() == key })
	if i < 0 || f.data.Films[i].Video == "" {
		f.mu.Unlock()
		return nil
	}
	name := f.data.Films[i].Video
	f.mu.Unlock()
	if err := f.bucket.Delete(ctx, prefix+name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := slices.IndexFunc(f.data.Films, func(r filmRecord) bool { return r.key() == key }); i >= 0 {
		f.data.Films[i].Video, f.data.Films[i].Cleaned, f.data.Films[i].Upload = "", true, nil
	}
	return f.save()
}

// video puts l's video in the bucket, going on from an upload that stopped, with left videos still to go, this one among them.
func (f *films) video(ctx context.Context, prefix string, l listed, left int) error {
	size, at, ok := fileOf(l)
	if !ok {
		return nil
	}
	f.mu.Lock()
	r := *f.record(l)
	f.mu.Unlock()
	name := psync.RelayFilmObject(r.Folder, "video"+strings.ToLower(filepath.Ext(l.film.Video)))
	var resume cloud.Resume
	if r.Upload != nil {
		resume = *r.Upload
	}
	f.tell(func(st *FilmsStatus) {
		st.Uploading, st.Sent, st.Size, st.Waiting = l.film.Title, 0, size, left
	})
	defer f.tell(func(st *FilmsStatus) { st.Uploading, st.Sent, st.Size = "", 0, 0 })
	var shown int64 = -1
	err := f.bucket.PutFile(ctx, prefix+name, l.film.Video, "", resume, func(r cloud.Resume) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(l).Upload = &r
		return f.save()
	}, func(done, total int64) {
		// The status goes out at each whole percent, as the account's upload does.
		percent := done * 100 / max(total, 1)
		if percent == shown {
			return
		}
		shown = percent
		f.tell(func(st *FilmsStatus) { st.Sent, st.Size = done, total })
	})
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rec := f.record(l)
	if rec.Video != "" && rec.Video != name {
		_ = f.bucket.Delete(ctx, prefix+rec.Video)
	}
	rec.Video, rec.VideoSize, rec.VideoAt, rec.Upload, rec.Cleaned = name, size, at, nil, false
	return f.save()
}

// FilmRef names a film of a library.
type FilmRef struct{ Library, ID string }

// CleanVideos deletes the videos of films from the bucket, as the user cleans them up from Settings › Phone Sync, on a goroutine of its own: their lines stay, and they are not put back while their files are the same. done is told, on that goroutine, how it went.
func (s *Server) CleanVideos(films []FilmRef, done func(error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.shared
	if f == nil || s.closed {
		return
	}
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		ctx, cancel := context.WithTimeout(context.Background(), forgetTimeout)
		defer cancel()
		var errs []error
		for _, film := range films {
			errs = append(errs, f.cleanVideo(ctx, s.relay.Prefix, film.Library+"/"+film.ID))
		}
		err := errors.Join(errs...)
		f.measure(ctx, s.relay.Prefix, f.waiting())
		s.relay.Wake()
		if done != nil {
			done(err)
		}
	}()
}

// waiting is how many videos the status says are still to go up.
func (f *films) waiting() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status.Waiting
}

// SetFilms changes whether videos go to the bucket and when watched ones are cleaned up, and looks at the films again with it.
func (s *Server) SetFilms(videos bool, cleanAfter time.Duration) {
	f := s.shared
	if f == nil {
		return
	}
	f.mu.Lock()
	f.cfg.Videos, f.cfg.CleanAfter = videos, cleanAfter
	f.mu.Unlock()
	f.Wake()
}

// Handoff hands the paired phones over to the bucket to as the relay moves there (psync.Relay.Handoff): their state.json here carries links to it from then on. The caller then starts the server again with the relay in to, which puts the films there anew.
func (s *Server) Handoff(ctx context.Context, to Bucket) error {
	if s.relay == nil {
		return errors.New("the relay is off")
	}
	return s.relay.Handoff(ctx, to)
}
