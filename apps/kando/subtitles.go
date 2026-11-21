package main

import (
	"errors"
	"os"
	"slices"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/track"
)

// The open film's subtitle file, read off the engine's goroutine.

// What went wrong opening the film, as the player's state keeps it (subtitleView.err, playErr): an ID, which ErrText puts in the interface's language when the page shows it.
const (
	playerErrSubsMissing    = "subtitle-missing"
	playerErrSubsUnreadable = "subtitle-unreadable"
	playerErrUnplayable     = "video-unplayable"
	playerErrPlugins        = film.ErrMissingPlugins
	// A video moved to a storage location (playerStored): the location is no longer in the settings, could not be reached, no longer has the video, or the system's player cannot play it from there.
	playerErrStoredGone        = film.ErrStoredGone
	playerErrStoredUnreachable = film.ErrStoredUnreachable
	playerErrStoredMissing     = film.ErrStoredMissing
	playerErrStoredUnplayable  = film.ErrStoredUnplayable
)

// playerErrTextIn is what the page says for one of the errors above in the language of m, as the page says it (ErrText) and the feedback issue says it in the language it is written in; any other text shows as it is.
func playerErrTextIn(m locales.Messages, id string) string {
	switch id {
	case playerErrSubsMissing:
		return m.PlayerSubsMissing()
	case playerErrSubsUnreadable:
		return m.PlayerSubsUnreadable()
	case playerErrUnplayable:
		return m.PlayerUnplayable()
	case playerErrPlugins:
		return m.PlayerMissingPlugins()
	case playerErrStoredGone:
		return m.PlayerStoredGone()
	case playerErrStoredUnreachable:
		return m.PlayerStoredUnreachable()
	case playerErrStoredMissing:
		return m.PlayerStoredMissing()
	case playerErrStoredUnplayable:
		return m.PlayerStoredUnplayable()
	}
	return id
}

// subtitleView is the subtitle file open on the film page. err is one of the player's error IDs, or "".
type subtitleView struct {
	key, video string
	path       string
	// machine is the file's cues as the pipeline wrote them, and edits the user's corrections of them, kept in the film's corrections file (media.CorrectionsPath); editsErr is set when that file could not be read, which is then not written over. cues are the lines as they show, the corrections laid over the machine's and those marked as no speech left out (core.CorrectSegments), and from the corrected line each of them comes from, index for index.
	machine []core.Segment
	// machineDrafts are the spans of the machine's lines that are drafts, not proofread as a whole yet (media.DraftSpans); drafts says, cue for cue, which of cues come from one.
	machineDrafts map[core.Span]bool
	drafts        []bool
	// showDrafts is whether the lines are marked drafts as the film shows them, told after each change (playerDraftsFollow), so Subs, read for every line on screen, does not look the open film up each time.
	showDrafts bool
	edits      []core.Correction
	editsErr   bool
	cues       []core.Segment
	from       []core.Corrected
	err        string
	ready      bool
	mod        time.Time
	size       int64
	seen       bool
	// held is the line on screen when the machine rewrote it, shown as it was until the playhead leaves it (subtitlesLetGo), so a line is not changed while it is read; machine holds its new text all along, which export and the corrections work from. immediate has the next read of the file show what it holds at once, as after the user's own decision on a change of the final proofread.
	held      *heldCue
	immediate bool
	// reading is a read of the same film's lines again under way (reloadOpenSubtitles), as after a job writes them out: the lines read before go on showing, with the line selected and a correction being typed, until it lands and they are swapped in place. Nothing writes the corrections meanwhile (correctLoaded), as the read would bring back what they held before. readGen tells the read that lands from one begun before it.
	reading bool
	readGen int
	// heard and givenUp are the cells of the film's lines the speech recognition heard and gave up on (track.Doc.Heard), as read with them; gaps are the parts of a film being made with no lines yet that they leave (film.Gaps), and making whether its job works on it now, told after each change (playerGapsFollow).
	heard, givenUp map[int]bool
	gaps           []film.Gap
	making         bool
}

// loadSubtitles reads the lines of the film whose video is at video off the engine's goroutine: its subtitle document, with path, the subtitle file it shows, taken in where it changed (media.ReadTrack), and the user's corrections of the film (media.LoadCorrections), and the frames of the 片段 its lines cut the film into once read (loadClipFrames). An empty path means this film has no subtitle yet; a file that is gone or cannot be read says so, as the film's lines come from that file until its subtitles are made into the document itself.
func (a *app) loadSubtitles(key, video, path string) {
	if a.player.subs.key == key && a.player.subs.path == path {
		return
	}
	a.player.subs = subtitleView{key: key, video: video, path: path}
	if path == "" {
		a.player.subs.ready = true
		return
	}
	if a.update == nil {
		a.player.subs.err = playerErrSubsUnreadable
		a.player.subs.ready = true
		return
	}
	a.subtitlesRead()
}

// subtitlesRead reads the open subtitle file's lines off the engine's goroutine, and shows them once read. A read again of lines already shown (subtitleView.reading) keeps the line on screen as it was (subtitlesHold).
func (a *app) subtitlesRead() {
	subs := &a.player.subs
	key, video, path, update := subs.key, subs.video, subs.path, a.update
	// A read of the open film's subtitles again knows what showed before it, so the line on screen is kept as it was.
	var was []core.Segment
	if subs.ready && subs.err == "" {
		was = subtitlesShown(subs.machine, subs.held)
	}
	immediate := subs.immediate
	subs.immediate = false
	subs.reading = subs.ready
	subs.readGen++
	gen := subs.readGen
	// Which lines are drafts is told by the language the film is translated into, which its final proofread is made for.
	language := ""
	if item, ok := a.openFilm(); ok && item.Key == key {
		language = item.Language
	}
	go func() {
		info, statErr := os.Stat(path)
		edits, editsErr := media.LoadCorrections(media.CorrectionsPath(path))
		var cues []core.Segment
		var drafts map[core.Span]bool
		var heard, givenUp map[int]bool
		msg := ""
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				msg = playerErrSubsMissing
			} else {
				msg = playerErrSubsUnreadable
			}
		} else if doc, err := media.ReadTrack(video, path, subtitlesTranscript(video)); err != nil {
			msg = playerErrSubsUnreadable
		} else {
			cues = track.Bilingual(doc.View())
			drafts = media.DraftSpans(doc, language)
			heard, givenUp = doc.Heard(doc.Shown())
		}
		update(func() {
			subs := &a.player.subs
			if subs.key != key || subs.path != path || subs.readGen != gen {
				return
			}
			subs.held = nil
			if !immediate {
				subs.held = subtitlesHold(was, cues, a.playerLineAt())
			}
			subs.machine, subs.machineDrafts = cues, drafts
			subs.heard, subs.givenUp = heard, givenUp
			subs.edits, subs.editsErr = edits, editsErr != nil
			a.playerCorrectSubs()
			subs.err = msg
			subs.ready, subs.reading = true, false
			if statErr == nil {
				subs.mod = info.ModTime()
				subs.size = info.Size()
				subs.seen = true
			}
			a.loadClipFrames()
		})
	}()
}

// playerGapsFollow tells which parts of the open film have no lines yet as it is made (film.Gaps), from the cells its lines were read with and the length the player found, and whether its job works on it now, and reports whether that changed. Only a film being made for the first time has them: a film done is heard whole, and one made again shows its own lines where the redo has not.
func (a *app) playerGapsFollow() bool {
	subs := &a.player.subs
	var gaps []film.Gap
	making := false
	if item, ok := a.openFilm(); ok && item.Key == subs.key && subs.ready && subs.err == "" && item.Stage != "" && !item.Stage.Done() && !a.filmRedoing(item.JobID) {
		gaps = film.Gaps(subs.heard, subs.givenUp, subs.machine, a.PlayLength())
		making = item.Stage != core.StageQueued
	}
	if slices.Equal(gaps, subs.gaps) && making == subs.making {
		return false
	}
	subs.gaps, subs.making = gaps, making
	return true
}

// heldCue is a line kept as it showed while it is on screen: its span, and the text it showed, which it shows until the playhead leaves it.
type heldCue struct {
	span core.Span
	text string
}

// subtitlesHold is the line of cues, a film's lines read again, to keep on screen at at as it was in was, the lines read before, when the machine rewrote it there, or nil: a line rewritten while it is read is distracting, so it shows its new text once it is left (subtitlesLetGo). A line that had no translation takes the one that came at once, as a gap filled is not a line rewritten.
func subtitlesHold(was, cues []core.Segment, at time.Duration) *heldCue {
	_, cur, _ := film.CueNear(was, at)
	if cur < 0 || at < was[cur].Start || at >= was[cur].End {
		return nil
	}
	shown := was[cur]
	if translation, _ := core.SplitCue(shown.Text); translation == "" {
		return nil
	}
	for _, c := range cues {
		if c.Start == shown.Start && c.End == shown.End {
			if c.Text == shown.Text {
				return nil
			}
			return &heldCue{span: core.Span{Start: c.Start, End: c.End}, text: shown.Text}
		}
	}
	return nil
}

// subtitlesShown is machine, the machine's lines, as they show: the line held on screen (held) with the text it shows.
func subtitlesShown(machine []core.Segment, held *heldCue) []core.Segment {
	if held == nil {
		return machine
	}
	out := slices.Clone(machine)
	for i, c := range out {
		if c.Start == held.span.Start && c.End == held.span.End {
			out[i].Text = held.text
		}
	}
	return out
}

// subtitlesLetGo shows the line held on screen with its new text once the playhead is off it (subtitlesHold), and reports whether that changed the lines.
func (a *app) subtitlesLetGo() bool {
	subs := &a.player.subs
	if subs.held == nil {
		return false
	}
	if at := a.playerLineAt(); at >= subs.held.span.Start && at < subs.held.span.End {
		return false
	}
	subs.held = nil
	a.playerCorrectSubs()
	return true
}

// subtitlesTranscript is the .srt of the speech alone beside the video at video (media.TranscriptOf), by which a film's bilingual subtitles are split into each line's original and translation; "" for a film without a video.
func subtitlesTranscript(video string) string {
	if video == "" {
		return ""
	}
	return media.TranscriptOf(video)
}

// reloadOpenSubtitles reads the open subtitle again when the file on disk was rewritten or removed, the lines read before showing until it lands (subtitleView.reading).
func (a *app) reloadOpenSubtitles() {
	if a.player.subs.key == "" || a.player.subs.path == "" {
		return
	}
	info, err := os.Stat(a.player.subs.path)
	if err != nil {
		if !a.player.subs.ready || a.player.subs.err != "" {
			return
		}
	} else if a.player.subs.seen && a.player.subs.size == info.Size() && a.player.subs.mod.Equal(info.ModTime()) {
		return
	}
	// One read again at a time: a change made while it runs is seen once it lands, as it reads what the file holds then. A read asked for whatever the file looks like (seen cleared, as after the user's decision on a change of the final proofread) begins anew, the read under way dropped.
	if a.player.subs.reading && a.player.subs.seen {
		return
	}
	a.subtitlesRead()
}

// playerCorrectSubs lays the user's corrections of the open film over the lines the machine wrote, as the picture, the subtitle list and the keys take them (subtitleView.cues), the line held on screen as it showed (subtitleView.held) unless the user corrected it.
func (a *app) playerCorrectSubs() {
	subs := &a.player.subs
	subs.cues, subs.from = core.CorrectSegments(subtitlesShown(subs.machine, subs.held), subs.edits)
	if len(subs.machine) == 0 {
		subs.cues, subs.from = nil, nil
	}
	// A line the user corrected is theirs, final whatever the machine's is.
	subs.drafts = nil
	if len(subs.machineDrafts) > 0 {
		subs.drafts = make([]bool, len(subs.from))
		for i, l := range subs.from {
			subs.drafts[i] = l.Edit < 0 && subs.machineDrafts[l.Machine]
		}
	}
}
