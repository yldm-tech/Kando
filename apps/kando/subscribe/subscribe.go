// Package subscribe is following a channel, a bilibili uploader's space, a collection or a playlist, so its new videos are added to a library by themselves or after asking. A Sub is one subscription and Store keeps them in subscriptions.json beside settings.json. A check lists the videos with the pipeline's yt-dlp (media.Lister, as package playlist does) and Take folds what it found into the subscription: the videos it had not seen, which pass its Rules or are skipped, and a new name the site gives the list. The app schedules the checks while it runs, queues or keeps the new videos and draws the sidebar's list (List) and the subscription's page (Page) (state_subscribe.go).
package subscribe

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"kando/packages/core"
	"kando/packages/media"
)

// Mode is what a check does with the new videos that pass the rules: ModeAuto queues them, ModeAsk lists them on the subscription's page until each is added or skipped.
type Mode string

const (
	ModeAuto Mode = "auto"
	ModeAsk  Mode = "ask"
)

// Filter is which new videos a subscription takes: all of them, those that play longer than LongEnough, or those whose titles hold one of its keywords.
type Filter string

const (
	FilterAll      Filter = "all"
	FilterLong     Filter = "long"
	FilterKeywords Filter = "keywords"
)

// LongEnough is how long a video plays at least to pass FilterLong.
const LongEnough = 5 * time.Minute

// Cadence is how often a subscription is checked while the app runs: each time it opens, every hour or once a day. Nothing checks while the app is not running.
type Cadence string

const (
	CadenceOpen   Cadence = "open"
	CadenceHourly Cadence = "hourly"
	CadenceDaily  Cadence = "daily"
)

// Rules are what a subscription does with its new videos and how often it looks for them.
type Rules struct {
	Mode     Mode    `json:"mode"`
	Filter   Filter  `json:"filter"`
	Keywords string  `json:"keywords,omitempty"`
	Cadence  Cadence `json:"cadence"`
	// Series makes the videos it adds a series, which share a glossary (core.Job.Series).
	Series bool `json:"series,omitempty"`
	// Quality is the picture its jobs download (core.Job.Quality), and Subtitles where they take their original lines from (core.Job.SubtitleSource); "" leaves each to Settings › 通用 › 在线视频, as a subscription made before it had them does.
	Quality   core.Quality        `json:"quality,omitempty"`
	Subtitles core.SubtitleSource `json:"subtitles,omitempty"`
}

// DefaultRules are a new subscription's: every new video is queued, and the list is checked each time the app opens.
func DefaultRules() Rules {
	return Rules{Mode: ModeAuto, Filter: FilterAll, Cadence: CadenceOpen}
}

// clean puts a value Rules does not know, as one a later version wrote, back to the default's.
func (r Rules) clean() Rules {
	d := DefaultRules()
	if r.Mode != ModeAuto && r.Mode != ModeAsk {
		r.Mode = d.Mode
	}
	if r.Filter != FilterAll && r.Filter != FilterLong && r.Filter != FilterKeywords {
		r.Filter = d.Filter
	}
	if r.Cadence != CadenceOpen && r.Cadence != CadenceHourly && r.Cadence != CadenceDaily {
		r.Cadence = d.Cadence
	}
	if r.Quality != "" && !r.Quality.Known() {
		r.Quality = ""
	}
	if r.Subtitles != "" && !r.Subtitles.Known() {
		r.Subtitles = ""
	}
	return r
}

// Words are the keywords of FilterKeywords, separated by commas (ASCII or Chinese), semicolons or white space.
func (r Rules) Words() []string {
	return strings.FieldsFunc(r.Keywords, func(c rune) bool {
		return c == ',' || c == '，' || c == ';' || c == '；' || c == '、' || unicode.IsSpace(c)
	})
}

// Fits reports whether v passes the filter. A video whose length the site did not give passes FilterLong, as it may be long; FilterKeywords without a keyword passes every video, and a keyword matches a title whatever its case.
func (r Rules) Fits(v Video) bool {
	switch r.Filter {
	case FilterLong:
		return v.Duration == 0 || v.Duration >= LongEnough
	case FilterKeywords:
		words := r.Words()
		if len(words) == 0 {
			return true
		}
		title := strings.ToLower(v.Title)
		return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(title, strings.ToLower(w)) })
	}
	return true
}

// Video is one video a check found: the link a job downloads it from, its title and how long it plays, 0 when the site did not say.
type Video struct {
	URL      string        `json:"url"`
	Title    string        `json:"title,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
}

// Sub is one subscription.
type Sub struct {
	ID string `json:"id"`
	// Source is the link of the channel, space, collection or playlist, as it was pasted.
	Source string `json:"source"`
	// Library is the library its videos are added to: the one open when it was subscribed.
	Library string `json:"library"`
	// Name is what the app calls it; Title is the name the site gave it at the last check, which tells when the site renames it.
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
	Rules
	// Paused stops the checks until it is resumed.
	Paused bool `json:"paused,omitempty"`
	// Renamed is the new name the site gives it, until the user keeps one of the two (KeepName).
	Renamed string `json:"renamed,omitempty"`
	// Missing is how many checks in a row the site said the list is not there (IsGone); Gone is set once GoneAfter did, so one passing 404 does not stop the subscription. Stopped is set once the user chose to keep it in the sidebar without checking it any more. A check that lists it again clears Missing and Gone.
	Missing int  `json:"missing,omitempty"`
	Gone    bool `json:"gone,omitempty"`
	Stopped bool `json:"stopped,omitempty"`
	// Checked is when it was last checked, whether or not that worked. Why the last check failed is kept as data, not as words, so the page says it in the language it shows: TimedOut when yt-dlp took too long, otherwise Err, what yt-dlp said, "" when it worked.
	Checked  time.Time `json:"checked,omitzero"`
	TimedOut bool      `json:"timedOut,omitempty"`
	Err      string    `json:"error,omitempty"`
	// Seen is the links of every video a check has listed, so each is new only once. Pending is the new videos waiting to be added or skipped, oldest first, and Added those added to the queue, oldest first.
	Seen    []string `json:"seen,omitempty"`
	Pending []Video  `json:"pending,omitempty"`
	Added   []Video  `json:"added,omitempty"`
	// Skipped is the links of the waiting videos the user skipped, so the check the page's 列出来 asks for does not offer them again (Earlier).
	Skipped []string `json:"skipped,omitempty"`
	// Offered is set once the videos the list held when it was subscribed were queued or offered to the user (Start). A subscription made before Kando did that counted them as seen and added none; for one that follows a collection or a playlist, the check its page's 列出来 asks for offers them (Take).
	Offered bool `json:"offered,omitempty"`
}

// Source is the link a subscription follows for one typed or sent to subscribe to: a playlist or a channel (core.Link.IsList) as it is, and a bilibili video's page as its whole upload (core.Link.Parts), which may hold several parts; reading it says whether it does, as an upload of one part lists as one video (media.ErrOneVideo). It reports false for any other link.
func Source(typed string) (string, bool) {
	typed = strings.TrimSpace(typed)
	link, ok := core.ParseLink(typed)
	switch {
	case !ok:
		return "", false
	case link.IsList():
		return typed, true
	case link.Parts() != "":
		return link.Parts(), true
	}
	return "", false
}

// New is a subscription to source for the library at library, which listing, what a first check found, seeds: its name is the list's, and the videos the list holds now count as seen, so only the ones that come after are new. Which of them are added now is the caller's: Start says which.
func New(source, library string, listing media.Listing, now time.Time) Sub {
	s := Sub{ID: newID(), Source: strings.TrimSpace(source), Library: library, Name: NameOf(source, listing), Title: listing.Title, Rules: DefaultRules(), Checked: now, Offered: true}
	for _, v := range listing.Videos {
		s.see(v.URL)
	}
	return s
}

// Start is the videos listing holds that a new subscription adds now, in the list's order: for a collection or a playlist every one that fits the rules, as the user subscribed for the episodes it has; for a channel or an uploader's space the recent most recent ones (listed first) that fit, as the subscribe dialog asks how many.
func (s Sub) Start(listing media.Listing, recent int) []Video {
	listed := listing.Videos
	if s.Channel() {
		listed = listed[:min(max(recent, 0), len(listed))]
	}
	var out []Video
	for _, lv := range listed {
		v := Video{URL: strings.TrimSpace(lv.URL), Title: lv.Title, Duration: lv.Duration}
		if v.URL != "" && s.Fits(v) && !slices.ContainsFunc(out, func(o Video) bool { return o.URL == v.URL }) {
			out = append(out, v)
		}
	}
	return out
}

// newID is a random ID, which names a subscription in the sidebar and in subscriptions.json.
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NameOf is what a subscription to the list at source that listing was read of is called: the list's name, or its link without the scheme when the site gives none.
func NameOf(source string, listing media.Listing) string {
	if listing.Title != "" {
		return listing.Title
	}
	return fallbackName(strings.TrimSpace(source))
}

// fallbackName is the name of a list whose site gives none: its link without the scheme.
func fallbackName(source string) string {
	link, ok := core.ParseLink(source)
	if !ok {
		return source
	}
	name := link.URL
	for _, prefix := range []string{"https://", "http://", "www."} {
		name = strings.TrimPrefix(name, prefix)
	}
	return name
}

// Link is what core.ParseLink reads from the subscription's link: its platform and whether it is a channel.
func (s Sub) Link() core.Link {
	link, _ := core.ParseLink(s.Source)
	return link
}

// Channel reports whether the subscription follows a channel or an uploader's space rather than a playlist or a collection.
func (s Sub) Channel() bool { return s.Link().Kind == core.LinkChannel }

// Checks reports whether the subscription is checked at all: not paused, and not kept after the site deleted it (Stopped).
func (s Sub) Checks() bool { return !s.Paused && !s.Stopped }

// GoneAfter is how many checks in a row must find the list not there before the subscription is marked Gone.
const GoneAfter = 2

// Due reports whether the subscription is to be checked now, opened being when the app started: it checks (Checks), and for CadenceOpen it was not checked since the app started, so one passed over at launch, as while its library was locked, is still due later; for the others an hour or a day has passed since it was last checked. One the site deleted (Gone) is still checked once a day, so a list that comes back is followed again. A clock set back, which makes the last check later than now, makes it due too.
func (s Sub) Due(now, opened time.Time) bool {
	if !s.Checks() {
		return false
	}
	since := now.Sub(s.Checked)
	switch {
	case since < 0:
		return true
	case s.Gone || s.Cadence == CadenceDaily:
		return since >= 24*time.Hour
	case s.Cadence == CadenceHourly:
		return since >= time.Hour
	}
	return s.Checked.Before(opened)
}

// Found is what Take found: the new videos that fit the rules, in the list's order, and how many new ones did not and were skipped. Earlier is, for a collection or a playlist subscribed before its videos were offered (Sub.Offered), the ones it held then that fit the rules and were neither added nor waiting, in the list's order, which the caller offers to the user.
type Found struct {
	Fresh   []Video
	Skipped int
	Earlier []Video
}

// AddPerCheck is how many new videos a check of a subscription that adds them by itself (ModeAuto) queues at most, so a channel that uploads a season at once, or a first check after a long time, does not fill the queue; the others wait for the user as ModeAsk's do (Cap).
const AddPerCheck = 3

// Cap splits the new videos a check of a ModeAuto subscription found, in the list's order, into the first AddPerCheck, which it queues, and the rest, which it holds for the user.
func Cap(fresh []Video) (queue, hold []Video) {
	if len(fresh) <= AddPerCheck {
		return fresh, nil
	}
	return fresh[:AddPerCheck:AddPerCheck], fresh[AddPerCheck:]
}

// Take folds a check's listing into the subscription at now: every video not seen yet is new once, kept in Fresh when it fits the rules and skipped otherwise; a name the site gives the list that is not the one it gave before is kept in Renamed, and the site going back to the old one forgets it. A check that lists the videos also shows that the list is still there, so Gone and Err are cleared. With offer, the check the user asked for to list the videos a collection or a playlist subscribed before its videos were offered left out (the page's 列出来), it finds them in Earlier, and the subscription is then Offered; any other check leaves them, so a scheduled one never fills the page with a list's back catalogue unasked. What to do with Fresh and Earlier is the caller's: Queue them, or Hold them for the user.
func (s *Sub) Take(listing media.Listing, now time.Time, offer bool) Found {
	s.Checked, s.Err, s.TimedOut, s.Missing, s.Gone = now, "", false, 0, false
	switch title := strings.TrimSpace(listing.Title); {
	case title == "":
	case s.Title == "":
		s.Title = title
	case title == s.Title:
		s.Renamed = ""
	default:
		s.Renamed = title
	}
	var found Found
	earlier := offer && !s.Offered && !s.Channel()
	if earlier {
		s.Offered = true
	}
	for _, lv := range listing.Videos {
		if !s.see(lv.URL) {
			if v := (Video{URL: strings.TrimSpace(lv.URL), Title: lv.Title, Duration: lv.Duration}); earlier && v.URL != "" && s.Fits(v) && !s.has(v.URL) && !slices.ContainsFunc(found.Earlier, func(e Video) bool { return e.URL == v.URL }) {
				found.Earlier = append(found.Earlier, v)
			}
			continue
		}
		v := Video{URL: lv.URL, Title: lv.Title, Duration: lv.Duration}
		if s.Fits(v) {
			found.Fresh = append(found.Fresh, v)
		} else {
			found.Skipped++
		}
	}
	return found
}

// has reports whether the video at link was added, waits for the user or was skipped by them.
func (s Sub) has(link string) bool {
	same := func(v Video) bool { return v.URL == link }
	return slices.ContainsFunc(s.Added, same) || slices.ContainsFunc(s.Pending, same) || slices.Contains(s.Skipped, link)
}

// Earlier is the links of the videos a check the user asks for would offer as Found.Earlier, as far as the subscription knows: those it counted as seen when it was subscribed that were neither added, waiting nor skipped, for a collection or a playlist whose videos were not offered yet (Offered); none otherwise. It cannot apply the rules, as Seen keeps no title or length, nor know which the list still holds, so it is at most what the check offers; the caller leaves out those its library has.
func (s Sub) Earlier() []string {
	if s.Offered || s.Channel() {
		return nil
	}
	var out []string
	for _, link := range s.Seen {
		if !s.has(link) {
			out = append(out, link)
		}
	}
	return out
}

// see marks the video at link as seen and reports whether it was not seen before.
func (s *Sub) see(link string) bool {
	link = strings.TrimSpace(link)
	if link == "" || slices.Contains(s.Seen, link) {
		return false
	}
	s.Seen = append(s.Seen, link)
	return true
}

// Fail records that the check at now failed with err. A list the site says is not there (IsGone) is marked Gone once it said so GoneAfter times in a row; until then it is a failure like any other. yt-dlp taking too long sets TimedOut; any other failure, as the network being down, is kept in Err, what yt-dlp said. The page says why, and the next check tries again.
func (s *Sub) Fail(err error, now time.Time) {
	s.Checked, s.Err, s.TimedOut = now, "", false
	if IsGone(err) {
		s.Missing++
		if s.Missing >= GoneAfter {
			s.Gone = true
			return
		}
	} else {
		s.Missing = 0
	}
	if errors.Is(err, media.ErrListTimeout) {
		s.TimedOut = true
		return
	}
	s.Err = err.Error()
}

// IsGone reports whether err is yt-dlp saying that the list does not exist or was removed, as it says for a deleted playlist, channel or collection (media.ClassifyDownload's core.FailPrivate), rather than that it could not reach it.
func IsGone(err error) bool {
	return err != nil && media.ClassifyDownload(err.Error()) == core.FailPrivate
}

// Hold adds videos to the ones waiting for the user, each once.
func (s *Sub) Hold(videos []Video) {
	for _, v := range videos {
		if !slices.ContainsFunc(s.Pending, func(p Video) bool { return p.URL == v.URL }) {
			s.Pending = append(s.Pending, v)
		}
	}
}

// Queued records that videos were added to the queue: they leave the waiting ones and join Added, each once.
func (s *Sub) Queued(videos []Video) {
	s.unwait(urls(videos))
	for _, v := range videos {
		if !slices.ContainsFunc(s.Added, func(a Video) bool { return a.URL == v.URL }) {
			s.Added = append(s.Added, v)
		}
	}
}

// Skip drops the waiting videos at links, which stay seen and are not added, and records them in Skipped, so they are not offered again.
func (s *Sub) Skip(links []string) {
	for _, v := range s.Pending {
		if slices.Contains(links, v.URL) && !slices.Contains(s.Skipped, v.URL) {
			s.Skipped = append(s.Skipped, v.URL)
		}
	}
	s.unwait(links)
}

// unwait drops the waiting videos at links.
func (s *Sub) unwait(links []string) {
	s.Pending = slices.DeleteFunc(s.Pending, func(v Video) bool { return slices.Contains(links, v.URL) })
}

// Waiting is the waiting videos at links, all of them when links is nil, in their order.
func (s Sub) Waiting(links []string) []Video {
	if links == nil {
		return slices.Clone(s.Pending)
	}
	var out []Video
	for _, v := range s.Pending {
		if slices.Contains(links, v.URL) {
			out = append(out, v)
		}
	}
	return out
}

// KeepName answers a rename: useNew takes the site's new name as the subscription's, otherwise the old name stays; either way the new name becomes the one later checks compare with.
func (s *Sub) KeepName(useNew bool) {
	if s.Renamed == "" {
		return
	}
	if useNew {
		s.Name = s.Renamed
	}
	s.Title, s.Renamed = s.Renamed, ""
}

func urls(videos []Video) []string {
	out := make([]string, len(videos))
	for i, v := range videos {
		out[i] = v.URL
	}
	return out
}

// Store is the subscriptions, in the order they were made, and the file they are kept in.
type Store struct {
	path string
	Subs []Sub
}

// stored is subscriptions.json.
type stored struct {
	Subscriptions []Sub `json:"subscriptions"`
}

// Load reads the subscriptions at path. A missing file has none; a file that cannot be read or is not JSON has none either, and the error says why.
func Load(path string) (Store, error) {
	s := Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var file stored
	if err := json.Unmarshal(data, &file); err != nil {
		return s, err
	}
	for _, sub := range file.Subscriptions {
		if sub.ID == "" || strings.TrimSpace(sub.Source) == "" {
			continue
		}
		sub.Rules = sub.Rules.clean()
		s.Subs = append(s.Subs, sub)
	}
	return s, nil
}

// Path is the file the store is kept in, "" for one that is not kept.
func (s Store) Path() string { return s.path }

// Save writes the subscriptions to the store's file, readable by the user alone, replacing it whole so a crash leaves the old one; a store without a file writes nothing.
func (s Store) Save() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(stored{Subscriptions: s.Subs}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Find is the subscription with id.
func (s *Store) Find(id string) (*Sub, bool) {
	for i := range s.Subs {
		if s.Subs[i].ID == id {
			return &s.Subs[i], true
		}
	}
	return nil, false
}

// Following is the subscription to source in the library at library, which a link pasted again finds rather than subscribing twice: the same list, as core.ParseLink writes it.
func (s *Store) Following(source, library string) (*Sub, bool) {
	key := sameList(source)
	for i := range s.Subs {
		if filepath.Clean(s.Subs[i].Library) == filepath.Clean(library) && sameList(s.Subs[i].Source) == key {
			return &s.Subs[i], true
		}
	}
	return nil, false
}

// sameList is what two links to the same list have in common: the host core.ParseLink reads, without www. or m., and the path in lower case without a slash at its end, as a channel's handle is the same in any case, then the query as it is, as a playlist's ID is not.
func sameList(source string) string {
	link, ok := core.ParseLink(source)
	if !ok {
		return strings.TrimSpace(source)
	}
	u, err := url.Parse(link.URL)
	if err != nil {
		return link.URL
	}
	return link.Host + strings.TrimSuffix(strings.ToLower(u.EscapedPath()), "/") + "?" + u.RawQuery
}

// Remove drops the subscription with id.
func (s *Store) Remove(id string) {
	s.Subs = slices.DeleteFunc(s.Subs, func(sub Sub) bool { return sub.ID == id })
}

// In is the subscriptions of the library at library, in their order.
func (s Store) In(library string) []Sub {
	var out []Sub
	for _, sub := range s.Subs {
		if library != "" && filepath.Clean(sub.Library) == filepath.Clean(library) {
			out = append(out, sub)
		}
	}
	return out
}

// MoveLibrary points the subscriptions of the library at old at its new path, after its folder was renamed, and reports whether any was.
func (s *Store) MoveLibrary(old, next string) bool {
	moved := false
	for i := range s.Subs {
		if filepath.Clean(s.Subs[i].Library) == filepath.Clean(old) {
			s.Subs[i].Library = next
			moved = true
		}
	}
	return moved
}
