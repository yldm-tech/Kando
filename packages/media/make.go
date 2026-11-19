package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"kando/packages/core"
	"kando/packages/track"
)

// Length is how long the video at videoPath is, as ffmpeg reads it, with or without a picture, as the sound a link is made from first has none; 0 when it cannot tell. A file whose header does not say, as a webm a browser recorded, is decoded once through its sound, which takes seconds for a film, and is as long as the last time the decoding reached.
func (t Tools) Length(ctx context.Context, videoPath string) time.Duration {
	if d := parseDuration(ffmpegStderr(ctx, t.FFmpeg, "-hide_banner", "-nostdin", "-i", videoPath)); d > 0 {
		return d
	}
	return lastDecoded(ffmpegStderr(ctx, t.FFmpeg, "-hide_banner", "-nostdin", "-vn", "-i", videoPath, "-f", "null", "-"))
}

// ffmpegStderr is what ffmpeg run with args writes to stderr, where it tells what a file holds and how far it has decoded, whether it succeeds or not.
func ffmpegStderr(ctx context.Context, ffmpeg string, args ...string) string {
	cmd := Command(ctx, ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = io.Discard, &stderr
	_ = cmd.Run()
	return stderr.String()
}

// decodedPattern is how far ffmpeg says it has decoded, as "time=00:01:05.00".
var decodedPattern = regexp.MustCompile(`time=(\d+):(\d+):(\d+(?:\.\d+)?)`)

// lastDecoded is the last time ffmpeg's stderr says it decoded to, 0 when it says none.
func lastDecoded(text string) time.Duration {
	all := decodedPattern.FindAllStringSubmatch(text, -1)
	if len(all) == 0 {
		return 0
	}
	m := all[len(all)-1]
	return parseDuration("Duration: " + m[1] + ":" + m[2] + ":" + m[3])
}

// Watcher says where a film is watched while it is made (Make).
type Watcher interface {
	// Playhead is where the film is watched now, or will be once it is opened: the lines nearest it after it are made first.
	Playhead() time.Duration
	// Jumped receives when the playhead jumped, so the work far from it gives way to the work near it.
	Jumped() <-chan struct{}
}

// From is a Watcher of a film not watched yet, which will be watched from at.
type From time.Duration

func (f From) Playhead() time.Duration { return time.Duration(f) }
func (From) Jumped() <-chan struct{}   { return nil }

// Making a film a part at a time: how much of it one run of the speech recognition hears, and how much sound before and after that run hears too, so a line cut by the run's ends is heard whole by one of the runs on either side, which keeps it where its middle falls (track.Cell).
const (
	runPad = 15 * time.Second
	// draftContext is how many lines said before a cell go with its draft, as the translation sends the lines before a batch (contextLines).
	draftContext = 80
	// projectEvery is how often the subtitle files beside the video are written out while the film is made, at most.
	projectEvery = 3 * time.Second
)

// runCellsMax is how many cells one run of the speech recognition hears at most; tests shorten it.
var runCellsMax = 10

// minRun is how long a run of the speech recognition goes before a jump may stop it, so jumping back and forth does not stop every run before it hears anything; tests shorten it.
var minRun = 10 * time.Second

// errGivenUp is why a film made again failed that gave up a part of it, tried as many times as a part is (track.Attempts).
var errGivenUp = errors.New("a part of the film failed every time it was tried")

// ErrNoLength is why a film failed whose length cannot be told, as a file that is not a video, so its parts cannot be told either.
var ErrNoLength = errors.New("the video's length cannot be told")

// Make makes the subtitles of a film a part at a time, as its job asks, into the film's subtitle document (track), device's log, and writes them out beside the video as they come (the transcript and "<video>.bilingual.srt"), so the film can be watched while it is made. The film's cells are heard and drafted nearest the playhead watch says first (track.Doc.Plan): the speech recognition hears a run of cells at a time, and the translation drafts each cell once heard, the two at once. A job run again, after it was stopped, failed or the app quit, goes on with what its document lacks.
//
// A link that keeps no video (core.QualitySubtitles) is made from its sound alone, which goes once its subtitles are written (core.Job.SubtitlesOnly); a job downloading the video of such a film (core.RedoVideo) does only that (downloadVideo).
//
// A film made again entirely (core.RedoAll) is made in a generation of its own, which the film shows once it is done; one translated again (core.RedoTranslation) copies the lines the film shows into one (translateAgain); one whose missing lines are translated is filled in where it is (fillMissing). A link that takes its site's subtitles (core.SubtitlesPlatform) takes them where they match the speech (making.siteHeard). A film whose document a newer Kando wrote is not written to, and fails with track.ErrNewer.
func Make(ctx context.Context, p Pipeline, job core.Job, language, device string, watch Watcher, report func(core.Job)) core.Job {
	if watch == nil {
		watch = From(job.ResumeAt)
	}
	if device == "" && job.Redo != core.RedoVideo {
		job.Err, job.Stage = fmt.Errorf("check: %w", track.ErrDevice), core.StageFailed
		report(job)
		return job
	}
	enter := func(stage core.Stage) {
		job.Stage = stage
		job.Progress = core.Progress{}
		report(job)
	}
	fail := func(step string, err error) core.Job {
		job.Err = fmt.Errorf("%s: %w", step, err)
		enter(core.StageFailed)
		return job
	}
	var chapters []core.Chapter
	ctx = WithTrace(ctx, &Trace{
		// How far a download has gone, while the job downloads: once the film is made, the making tells how far it is (making.progress).
		Progress: func(progress core.Progress) {
			if job.Stage != core.StageDownloading {
				return
			}
			progress.Started, progress.From = job.Progress.Started, job.Progress.From
			if progress.Known && progress.Started.IsZero() {
				progress.Started, progress.From = time.Now(), progress.Fraction
			}
			job.Progress = progress
			report(job)
		},
		Info: func(info Info) {
			if info.Title == "" || (info.Title == job.Title && info.Thumbnail == job.Thumbnail) {
				return
			}
			job.Title, job.Thumbnail = info.Title, info.Thumbnail
			report(job)
		},
		Chapters: func(list []core.Chapter) { chapters = list },
	})
	if code := strings.TrimSpace(job.SpeechLanguage); code != "" {
		ctx = withSpeechLanguage(ctx, code)
	}
	ctx = withQuality(ctx, job.Quality)
	if job.Redo == core.RedoVideo {
		return downloadVideo(ctx, p, &job, report)
	}
	if job.Redo == core.RedoTranslation {
		return translateAgain(ctx, p, &job, language, device, watch, report)
	}
	if job.Redo == core.RedoMissing {
		return fillMissing(ctx, p, &job, language, device, report)
	}
	// A redo needs the translator as well as the tools that hear the film, as it would otherwise replace the film's translated subtitles with fewer.
	check := p.Check
	if tc, ok := p.(TranslateChecker); ok && job.Redo != "" {
		check = func() error { return errors.Join(p.Check(), tc.CheckTranslate()) }
	}
	if err := check(); err != nil {
		return fail("check", err)
	}
	_, local := localVideoPath(job.Source)
	// soundOnly is a link at core.QualitySubtitles, which keeps no video: it is made from its sound alone, which goes once its subtitles are written.
	soundOnly := !local && job.Quality == core.QualitySubtitles
	video := job.VideoPath
	// A link not downloaded yet is heard from its sound, downloaded alone first, which is soon there, while its video downloads beside the making (coming); the film is shown once its video is.
	var coming chan downloaded
	stopVideo := func() {}
	// hearFrom is what the speech is heard from: the video, or a link's sound until its video comes.
	hearFrom := job.VideoPath
	switch {
	case local || Downloaded(video):
		if local {
			var err error
			if video, err = p.Download(ctx, job.Source); err != nil {
				job.FailKind = downloadFailKind(job, err)
				return fail("download", err)
			}
		}
		job.VideoPath, hearFrom = video, video
	default:
		enter(core.StageDownloading)
		sound, err := p.Download(withQuality(ctx, core.QualitySubtitles), job.Source)
		if err != nil {
			job.FailKind = downloadFailKind(job, err)
			return fail("download", err)
		}
		// The sound is kept hidden beside where the video comes (soundPath), so the library does not take a sound saved as .webm for a video of its own; the film's document is the video's from the start, as the video is named as the sound was but for its extension.
		video, hearFrom = sound, soundPath(sound)
		if os.Rename(sound, hearFrom) != nil {
			hearFrom = sound
		}
		if soundOnly {
			break
		}
		coming = make(chan downloaded, 1)
		videoCtx, cancelVideo := context.WithCancel(WithTrace(ctx, &Trace{}))
		videoDone := make(chan struct{})
		// Stopping the video's download waits for it to end, so nothing of the job still runs once Make returns.
		stopVideo = func() {
			cancelVideo()
			<-videoDone
		}
		go func() {
			defer close(videoDone)
			path, err := p.Download(videoCtx, job.Source)
			coming <- downloaded{path: path, err: err}
		}()
	}
	if len(chapters) > 0 {
		keepChapters(video, chapters, job.ChapterUse)
	}
	length := p.Length(ctx, hearFrom)
	doc, err := track.Read(video)
	switch {
	case err == nil && doc.Newer:
		err = track.ErrNewer
	case err == nil && length <= 0:
		err = ErrNoLength
	}
	if err != nil {
		// The video is downloaded all the same, as a job that fails after its download keeps it, so one run again does not download it again; a job stopped stops it at once.
		if coming != nil {
			if d := <-coming; d.err == nil {
				job.VideoPath = d.path
			}
		}
		stopVideo()
		if hearFrom != video {
			_ = os.Remove(hearFrom)
		}
		return fail("transcribe", err)
	}
	// A redo keeps showing the film as its files hold it until it is done: what they hold that the document does not is taken in first, as the film it redoes.
	if job.Redo != "" {
		if events, err := trackFiles(doc, job.SubtitlePath, TranscriptOf(video)); err == nil && len(events) > 0 {
			written, _ := track.Append(video, device, events...)
			doc.Events = append(doc.Events, written...)
		}
	}
	m := &making{ctx: ctx, p: p, job: &job, video: video, hearFrom: hearFrom, soundOnly: soundOnly, redo: job.Redo != "", device: device, doc: doc, report: report, coming: coming}
	if _, fetches := p.(PlatformSubtitler); !fetches || local || job.SubtitleSource != core.SubtitlesPlatform {
		m.siteState = siteNone
	}
	if _, fetches := p.(AutoCaptioner); !fetches || local || job.SubtitleSource != core.SubtitlesAuto {
		m.reference = referenceKept
	}
	defer stopVideo()
	return m.run(length, language, watch)
}

// soundPath is where the sound of the link whose video is to be at video is kept while it is made from it: a hidden file beside it, as ".video.sound", which moves with the video (Companions) and goes once the video is there.
func soundPath(video string) string { return hiddenBeside(video, ".sound") }

// MakesAPartAtATime reports whether Make makes job a part at a time, so stopping it loses no more than the work it was doing: any job but one downloading the video of a film that keeps none (core.RedoVideo), which stopping sets back to the start of its download.
func MakesAPartAtATime(job core.Job) bool {
	return job.Redo != core.RedoVideo
}

// downloadVideo downloads the video of a film that keeps none (core.RedoVideo) at job.Quality, puts it under the name its subtitles go by (placeVideo) and completes, neither hearing nor translating the film; one that fails leaves the film as it was.
func downloadVideo(ctx context.Context, p Pipeline, job *core.Job, report func(core.Job)) core.Job {
	job.Stage, job.Progress = core.StageDownloading, core.Progress{}
	report(*job)
	video, err := p.Download(ctx, job.Source)
	if err == nil {
		video, err = placeVideo(job.VideoPath, video)
	}
	if err != nil {
		job.FailKind = downloadFailKind(*job, err)
		job.Err, job.Stage = fmt.Errorf("download: %w", err), core.StageFailed
	} else {
		job.VideoPath, job.SubtitlesOnly = video, false
		job.Redo, job.RedoLanguage = "", ""
		job.Stage = core.StageComplete
	}
	job.Progress = core.Progress{}
	report(*job)
	return *job
}

// downloaded is how the download of a film's video beside its making ended: where the video is, or why it failed.
type downloaded struct {
	path string
	err  error
}

// making is a film being made by Make.
type making struct {
	ctx context.Context
	// workers is the work run beside the making, a run of the speech recognition, a draft, a fetch, which it waits for before it returns (spawn).
	workers sync.WaitGroup
	p       Pipeline
	job     *core.Job
	video   string
	// hearFrom is the file the speech is heard from: the video, or the sound of a link whose video is still coming, or that keeps none (soundOnly).
	hearFrom  string
	soundOnly bool
	// redo is a film made again (core.RedoAll, core.RedoTranslation) into a generation of its own, which the film shows only once it is done: until then nothing of it is written out, and the job keeps describing the film as it was.
	redo bool
	// siteCh brings the subtitles the link's site carries (core.SubtitlesPlatform), fetched beside the making once the language is known (fetchSite); site holds them, and siteState says what is known of them.
	siteCh    chan []core.Segment
	site      []core.Segment
	siteState siteState
	// referenceCh brings the captions the link's site made itself (core.SubtitlesAuto), fetched beside the making once the language is known (fetchReference), which are kept beside the video as the film's reference layer; reference says how far that is.
	referenceCh chan struct{}
	reference   referenceState
	// copied is a film translated again (core.RedoTranslation), whose generation holds the lines the film shows as they read, heard already (copyGeneration): they are translated again, and not proofread again.
	copied bool
	device string
	doc    track.Doc
	report func(core.Job)
	// transcript and bilingual are the files the film's lines are written out to (project), and projected when they last were.
	transcript, bilingual string
	projected             time.Time
	// redrafts are lines near the playhead drafted before a term was found in them, made again with it before the next draft (commitDraft).
	redrafts []draftInput
	// coming is the video of a link downloading beside the making, which is made from its sound (video) until it is there; nil once it is, or for a film whose video was there from the start.
	coming <-chan downloaded
	// videoErr is why the video failed to download, which fails the job once its subtitles are made.
	videoErr error
	// writeErr is why the film's log could not be written, which fails the job at once: what is not written is not made, and would be made again and again.
	writeErr error
}

// heardCell is a cell a run of the speech recognition heard, with its lines, and edges, the lines the run heard outside its cells, which it hands on with its first and last cell for the making to keep where no run has them (commitHeard).
type heardCell struct {
	cell  int
	lines []core.Segment
	edges []core.Segment
}

// hearDone is how a run of the speech recognition ended: the language it heard, and why it failed, with whether it had heard any line when it did.
type hearDone struct {
	language string
	err      error
	heard    bool
}

// drafted is how a draft or a final proofread ended: what it was given, its lines proofread and their translations line for line, the corrections the proofreading made, the changes the final proofread suggested, and why it failed.
type drafted struct {
	in         draftInput
	original   []core.Segment
	translated []core.Segment
	fixes      []Fix
	proofs     []core.Proof
	err        error
}

func (m *making) run(length time.Duration, language string, watch Watcher) core.Job {
	job := m.job
	// Whatever is still running when the making ends, a run of the speech recognition, a draft or a fetch, stops with it, and the making returns once it has: nothing of a job stopped or failed still asks a service or writes beside the video after it.
	var stop context.CancelFunc
	m.ctx, stop = context.WithCancel(m.ctx)
	defer func() {
		stop()
		m.workers.Wait()
	}()
	model := ""
	if h, ok := m.p.(hearer); ok {
		model, _ = h.hearing(m.ctx)
	}
	var gen string
	var err error
	if m.copied {
		gen, err = m.copyGeneration(length)
	} else {
		gen, err = m.generation(model)
	}
	if err != nil {
		return m.fail("transcribe", err)
	}
	m.transcript = TranscriptOf(m.video)
	if _, local := localVideoPath(job.Source); local && !job.Transcribed && foreignTranscript(m.video) {
		m.transcript = asideTranscriptPath(m.video)
	}
	m.bilingual = strings.TrimSuffix(m.video, filepath.Ext(m.video)) + ".bilingual.srt"

	// spoken is the language the film is heard in, which a redo keeps to itself until it is done, so a redo that fails leaves the film's as it was.
	spoken := job.SpokenLanguage
	if m.redo && !m.copied {
		spoken = ""
	}
	setSpoken := func(language string) {
		spoken = language
		if !m.redo {
			job.SpokenLanguage = language
		}
	}
	// The language the speech is heard in: the one the user named for the film, else the one Tools names, else the one an earlier run heard: a first making's, or a redo's own, as a redo hears the film anew. Without one, the first run hears a cell alone to find it, and every cell after is heard in it.
	hearing := strings.TrimSpace(job.SpeechLanguage)
	if h, ok := m.p.(hearer); ok && hearing == "" {
		_, hearing = h.hearing(m.ctx)
	}
	switch {
	case m.copied:
		hearing = copyHearing
	case hearing != "":
	case m.redo:
		hearing = m.heardIn(gen)
	default:
		hearing = cmpOr(job.SpokenLanguage, m.heardIn(gen))
	}
	if hearing != "" && hearing != "auto" && !m.copied {
		m.ctx = withSpeechLanguage(m.ctx, hearing)
		setSpoken(cmpOr(spoken, hearing))
	}
	plan := track.Making{Gen: gen, Length: length, Playhead: watch.Playhead(), Hearing: cmpOr(hearing, "auto")}

	// notTranslated is why the film is not translated, once it is not: no translator set up, the service gone, or failing cell after cell.
	var notTranslated string
	if tc, ok := m.p.(TranslateChecker); ok {
		if err := tc.CheckTranslate(); err != nil {
			notTranslated = err.Error()
		}
	}
	// translates is the language the film's lines are drafted in, "" while they are not: a film heard in the language its subtitles are for is not translated, which for Chinese is told by the script of the lines heard so far.
	translates := func() string {
		if notTranslated != "" || hearing == "" || core.SameLanguage(spoken, language, track.Originals(m.doc.Lines(gen))) {
			return ""
		}
		return language
	}
	// The final proofread, when the job asks for it and the pipeline can: each block of the film proofread as a whole once drafted, its inputs the film's terms found in it.
	auto, reviewAsked := reviewOf(m.ctx)
	_, canReview := m.p.(Reviewer)
	finals := reviewAsked && canReview
	relanguage := func() {
		plan.Language, plan.Final = translates(), nil
		if finals && plan.Language != "" {
			plan.Final = m.finalKey(gen)
		}
	}
	relanguage()

	heardCh := make(chan heardCell)
	doneCh := make(chan hearDone, 1)
	draftCh := make(chan drafted, 1)
	var hearStop context.CancelFunc
	var hearCells []int
	var hearStarted time.Time
	// pending are the cells the first run heard while the language was not known yet, kept until it is, so they are kept as heard in it.
	var pending []heardCell
	drafting := false
	draftFailures := 0
	// last is the work left when the making last looked, which once it is done says which blocks' final proofread holds (keepProofs).
	var last track.Plan
	defer func() {
		if hearStop != nil {
			hearStop()
		}
	}()
	for {
		// A making stopped starts nothing more, though what it waited for came at once with the stop and was taken first.
		if err := m.ctx.Err(); err != nil {
			return m.fail("transcribe", err)
		}
		plan.Playhead = watch.Playhead()
		if m.reference == referenceUnasked && hearing != "" && hearing != "auto" {
			m.fetchReference(hearing)
		}
		switch {
		case m.siteState == siteUnasked && hearing != "" && hearing != "auto":
			m.fetchSite(hearing)
		case m.siteState == siteUndecided:
			// A run that ended, as one that failed, may have left nothing more to hear.
			m.siteHeard(gen, plan)
		}
		p := m.doc.Plan(plan)
		last = p
		m.progress(p)
		siteWaits := m.siteState == siteFetching || m.siteState == siteUndecided
		if p.Done() && hearStop == nil && !drafting && !siteWaits && m.reference != referenceFetching && (len(m.redrafts) == 0 || plan.Language == "") {
			break
		}
		// A run hearing far from where the film is now watched gives way, once it has gone long enough to have heard something; what it heard is kept.
		if hearStop != nil && len(p.Hear) > 0 && !containsCell(hearCells, p.Hear[0].Cell) && time.Since(hearStarted) >= minRun {
			hearStop()
		}
		if hearStop == nil && len(p.Hear) > 0 {
			hearCells = runOf(p.Hear, hearing != "")
			var runCtx context.Context
			runCtx, hearStop = context.WithCancel(m.ctx)
			hearStarted = time.Now()
			cells := hearCells
			m.spawn(func() { m.hear(runCtx, cells, heardCh, doneCh) })
		}
		// The translation's one line of work: lines to draft again for a new term first, then drafts, then, while no draft is ready, the final proofread. While the site's subtitles are fetched, or in hand and not yet found to match the speech or not, nothing is drafted, as the lines they replace would be drafted for nothing; the fetch gives up after platformFetchTimeout.
		if !drafting && plan.Language != "" && !siteWaits {
			switch {
			case len(m.redrafts) > 0:
				drafting = true
				in := m.redrafts[0]
				m.redrafts = m.redrafts[1:]
				ctx, language := m.ctx, plan.Language
				m.spawn(func() { m.draft(ctx, in, language, draftCh) })
			case len(p.Draft) > 0:
				if group, ready := m.draftGroup(gen, p, plan.Playhead); ready {
					drafting = true
					ctx, in, language := m.ctx, m.draftOf(gen, group), plan.Language
					m.spawn(func() { m.draft(ctx, in, language, draftCh) })
				}
			}
			if !drafting && len(p.Final) > 0 {
				drafting = true
				ctx, in, language := m.ctx, m.finalOf(gen, p.Final[0]), plan.Language
				m.spawn(func() { m.final(ctx, in, language, auto, draftCh) })
			}
		}
		if hearStop == nil && !drafting && m.siteState != siteFetching && m.reference != referenceFetching {
			// Nothing runs and nothing can start, yet the film is not done. Plan says no such state exists, and a draft waits for more cells only while a run hears them; stop rather than wait for ever.
			return m.fail("transcribe", errors.New("subtitles: work is left that nothing can do"))
		}
		select {
		case <-m.ctx.Done():
			return m.fail("transcribe", m.ctx.Err())
		case d := <-m.coming:
			m.videoCame(d)
		case subtitles := <-m.siteCh:
			m.siteCame(gen, plan, subtitles)
		case <-m.referenceCh:
			m.referenceCh, m.reference = nil, referenceKept
		case <-watch.Jumped():
		case c := <-heardCh:
			if hearing == "" {
				pending = append(pending, c)
				continue
			}
			m.commitHeard(gen, plan.Hearing, c)
			m.siteHeard(gen, plan)
		case d := <-doneCh:
			hearStop()
			hearStop = nil
			switch {
			case d.err != nil && m.ctx.Err() != nil:
			case errors.Is(d.err, context.Canceled):
				// Stopped for a jump: the cells it did not hear are heard again.
			case d.err != nil && !d.heard:
				// A run that failed before it heard anything, as one whose model cannot load, fails every run: the job fails, as the transcription failing does.
				return m.fail("transcribe", d.err)
			case d.err != nil:
				m.append(track.Unit{Work: track.WorkHear, Gen: gen, Cell: m.firstUnheard(gen, plan.Hearing, hearCells), Key: plan.Hearing}.Failed(d.err.Error()))
			case hearing == "":
				// The first run heard the language: the cells it heard are kept as heard in it, and every cell after is heard in it.
				hearing = cmpOr(d.language, "auto")
				setSpoken(d.language)
				if d.language != "" {
					m.ctx = withSpeechLanguage(m.ctx, d.language)
				}
				plan.Hearing = hearing
				for _, c := range pending {
					m.commitHeard(gen, hearing, c)
				}
				pending = nil
				m.siteHeard(gen, plan)
				relanguage()
			}
			if hearing == "" {
				pending = nil
			}
		case d := <-draftCh:
			drafting = false
			switch {
			case d.err == nil && d.in.final:
				draftFailures = 0
				m.commitFinal(d)
			case d.err == nil:
				draftFailures = 0
				m.commitDraft(gen, d, plan.Playhead)
			case d.in.redraft:
				// A line made again for a term keeps its draft when that fails; the final proofread makes it with the term.
			case m.ctx.Err() != nil:
			case d.in.final && (errors.Is(d.err, ErrOffline) || draftFailures+1 >= maxFailedBatches):
				// The service is gone, or fails block after block: the film stays a draft where it is not proofread yet, as one done with blocks the final proofread did not get to.
				finals = false
				relanguage()
			case (errors.Is(d.err, ErrOffline) || draftFailures+1 >= maxFailedBatches) && m.redo:
				// A redo without its translation would replace the film's translated subtitles with fewer: it fails, leaving the film as it was.
				return m.fail("translate", d.err)
			case errors.Is(d.err, ErrOffline) || draftFailures+1 >= maxFailedBatches:
				// The service is not there, or fails cell after cell: the film is made without the translation of the cells left, as it is without a translator.
				notTranslated = d.err.Error()
				relanguage()
			default:
				draftFailures++
				for _, u := range d.in.units {
					m.append(u.Failed(d.err.Error()))
				}
			}
		}
		m.project(gen)
		if m.writeErr != nil {
			return m.fail("export", m.writeErr)
		}
	}

	// A making stopped as its last work ended, as the fetch of the site's captions, is stopped, not done.
	if err := m.ctx.Err(); err != nil {
		return m.fail("transcribe", err)
	}
	// A redo that gave up hearing or drafting a part of the film would replace the film with one that lacks it: it fails, leaving the film as it was.
	if m.redo {
		for _, u := range last.GivenUp {
			switch u.Work {
			case track.WorkHear:
				return m.fail("transcribe", errGivenUp)
			case track.WorkDraft:
				return m.fail("translate", errGivenUp)
			}
		}
	}
	lines := m.doc.Lines(gen)
	if len(lines) == 0 {
		return m.fail("transcribe", ErrNoSpeech)
	}
	same := core.SameLanguage(cmpOr(spoken, job.SpokenLanguage), language, track.Originals(lines))
	// Terms are suggested for the film's glossary once it is translated, when the job asks for them (WithTermSuggestions), and kept before its subtitles are written whole, whose change has the window read them again (state_glossary_suggest.go), so it reads these with them. A film not translated, or whose missing lines alone are translated (fillMissing), keeps the suggestions it had, and so does one whose request failed; a making stopped while it asks stops.
	if notTranslated == "" && !same {
		if terms, ok := suggest(m.ctx, m.p, track.Originals(lines), track.Translations(lines), filmTitle(*job), language); ok {
			_ = KeepSuggestions(m.video, terms)
		}
		if err := m.ctx.Err(); err != nil {
			return m.fail("translate", err)
		}
	}
	// A link made from its sound waits for its video, then keeps it in the sound's place.
	if m.coming != nil {
		job.Stage, job.Progress = core.StageDownloading, core.Progress{}
		m.report(*job)
		select {
		case d := <-m.coming:
			m.videoCame(d)
		case <-m.ctx.Done():
			return m.fail("download", m.ctx.Err())
		}
	}
	if m.videoErr != nil {
		job.FailKind = downloadFailKind(*job, m.videoErr)
		return m.fail("download", m.videoErr)
	}
	if err := m.finish(gen); err != nil {
		return m.fail("export", err)
	}
	if m.redo {
		m.keepProofs(gen, last)
	}
	// The sound a film was made from goes once its subtitles are written: a film that keeps no video is named by where its sound was, which its subtitles are named after.
	subtitlesOnly := false
	if m.soundOnly {
		job.VideoPath, subtitlesOnly = m.video, removeSound(m.hearFrom)
	} else if m.hearFrom != job.VideoPath {
		_ = os.Remove(m.hearFrom)
	}
	if spoken != "" {
		job.SpokenLanguage = spoken
	}
	untranslated := 0
	if !same {
		for _, l := range lines {
			if l.Translation.Text == "" {
				untranslated++
			}
		}
	}
	// The chapters' titles are translated as the lines are; a film not translated has their earlier translations cleared (translateChapters).
	if notTranslated == "" {
		if err := translateChapters(m.ctx, m.p, m.video, language, same); err != nil {
			return m.fail("translate", err)
		}
	}
	job.SameLanguage = same
	// A film translated again keeps the lines it had, and what they were heard from.
	if !m.copied {
		job.Transcribed, job.Proofread = true, false
		job.PlatformSubtitles, job.LocalLines = m.siteCounts(gen)
		job.SubtitlesOnly = subtitlesOnly
	}
	job.SubtitlePath = m.bilingual
	job.CompletedAt = time.Now()
	job.Language, job.Untranslated, job.TranslationErr = language, untranslated, notTranslated
	job.Redo, job.RedoLanguage = "", ""
	job.Progress = core.Progress{}
	job.Stage = core.StageComplete
	m.report(*job)
	return *job
}

func (m *making) fail(step string, err error) core.Job {
	// A link made from its sound keeps its video once it comes, as a job that fails after its download does, so one run again does not download it again; a job stopped stops the download at once.
	if m.coming != nil {
		m.videoCame(<-m.coming)
		if m.videoErr == nil && m.hearFrom != m.job.VideoPath {
			_ = os.Remove(m.hearFrom)
		}
	}
	m.job.Err = fmt.Errorf("%s: %w", step, err)
	m.job.Stage = core.StageFailed
	m.job.Progress = core.Progress{}
	m.report(*m.job)
	return *m.job
}

// generation is the generation the making makes, which names the speech model it is heard by (Key): the one an earlier run began and did not finish, or a new one. A redo goes on with one only when it was heard by the same model, as a redo is the film heard anew by the model set now; a first making goes on with whatever its earlier run heard, but never with the copy a film translated again made of its lines (copyGeneration), whose cells were not heard at all. Neither goes on with one another device began while that device may still be making it (mayResume).
func (m *making) generation(model string) (string, error) {
	var open track.Event
	for _, e := range m.doc.Events {
		if e.Kind != track.KindGen || e.Source != makeSource || m.doc.Done(e.Gen) || strings.HasPrefix(e.Key, copyHearing+" ") || !m.mayResume(e) {
			continue
		}
		// A redo that gave up a part of the film failed for it (run), and is begun again rather than gone on with, which would fail at once.
		if !m.redo || e.Key == model && !m.doc.GivenUp(e.Gen, track.WorkHear, track.WorkDraft) {
			open = e
		}
	}
	if open.Gen != "" {
		return open.Gen, nil
	}
	gen := track.NewID()
	return gen, m.append(track.Event{Kind: track.KindGen, Gen: gen, Source: makeSource, Key: model})
}

// referenceState is how far the making is with the captions the link's site made itself (core.SubtitlesAuto).
type referenceState int

const (
	// referenceUnasked: not fetched yet, as the language they are to be in is not known yet.
	referenceUnasked referenceState = iota
	referenceFetching
	// referenceKept: kept beside the video, or none to keep, as a site without them or a job that does not keep them.
	referenceKept
)

// fetchReference fetches the captions the link's site made itself in language, the code of the language the speech is heard in, beside the making, and keeps them beside the video as the film's reference layer (keepReference); referenceCh says when that is done. The film's lines stay those heard. A job stopped meanwhile stops the fetch with it.
func (m *making) fetchReference(language string) {
	m.reference = referenceFetching
	done := make(chan struct{})
	m.referenceCh = done
	ctx, p, job := m.ctx, m.p, *m.job
	job.VideoPath = m.video
	m.spawn(func() {
		_ = keepReference(ctx, p, job, language)
		close(done)
	})
}

// heardOnly are the lines of lines that are not the uploader's (site), copied, and where each is among lines.
func heardOnly(lines []core.Segment, site []bool) ([]core.Segment, []int) {
	var out []core.Segment
	var at []int
	for i, l := range lines {
		if i >= len(site) || !site[i] {
			out, at = append(out, l), append(at, i)
		}
	}
	return out, at
}

// heardOnlyOf are the lines of lines at at.
func heardOnlyOf(lines []core.Segment, at []int) []core.Segment {
	out := make([]core.Segment, len(at))
	for k, i := range at {
		out[k] = lines[i]
	}
	return out
}

// siteState is what the making knows of the subtitles a link's site carries (core.SubtitlesPlatform).
type siteState int

const (
	// siteUnasked: not fetched yet, as the language they are to be in is not known yet.
	siteUnasked siteState = iota
	siteFetching
	// siteUndecided: in hand, with too few of their lines heard yet to tell whether they say what is said (siteCheck).
	siteUndecided
	// siteTaken: they say what is said, and each cell heard takes them where they match it (siteCell).
	siteTaken
	// siteNone: the film is heard alone, as a job that does not take them, a site that has none, or subtitles that do not match the speech.
	siteNone
)

// siteSource is the Source of the revisions that hold the uploader's lines, and the Work of the record that a cell took them (siteCell).
const siteSource = "site"

// siteChecked is how many of the site's lines with words are checked against the speech before the film decides whether it takes them: enough that a few short words that happen to match do not decide it, as matchPlatform decides by every line of the film.
const siteChecked = 20

// fetchSite fetches the site's subtitles in language, the code of the language the speech is heard in, beside the making (siteCh).
func (m *making) fetchSite(language string) {
	m.siteState = siteFetching
	m.siteCh = make(chan []core.Segment, 1)
	fetcher, source, ctx := m.p.(PlatformSubtitler), m.job.Source, m.ctx
	siteCh := m.siteCh
	m.spawn(func() {
		subtitles, err := fetcher.PlatformSubtitles(ctx, source, language)
		if err != nil {
			subtitles = nil
		}
		siteCh <- subtitles
	})
}

// siteCame takes in the site's subtitles once fetched: none, and the film is heard alone; else the cells heard so far decide whether it takes them, if they can yet.
func (m *making) siteCame(gen string, plan track.Making, subtitles []core.Segment) {
	m.siteCh = nil
	if len(subtitles) == 0 {
		m.siteState = siteNone
		return
	}
	m.site, m.siteState = subtitles, siteUndecided
	// A film that took them before, as one stopped and run again, takes them still: the lines they replaced are gone.
	for _, e := range m.doc.Events {
		if e.Kind == track.KindUnit && e.Gen == gen && e.Source == siteSource {
			m.siteState = siteTaken
		}
	}
	m.siteHeard(gen, plan)
}

// siteHeard decides, once enough of the site's lines are heard (siteChecked) or the whole film is, whether the film takes them: when at least half of those checked match the speech, as matchPlatform decides; and each cell heard of a film that takes them takes them where they match (siteCell).
func (m *making) siteHeard(gen string, plan track.Making) {
	if m.siteState == siteUndecided {
		heard := m.heardCells(gen, plan)
		var subtitles []core.Segment
		for _, s := range m.site {
			if heard[int((s.Start+s.End)/2/track.Cell)] {
				subtitles = append(subtitles, s)
			}
		}
		_, _, checked, hits := platformCheck(subtitles, m.heardLines(gen, heard))
		// Every cell heard, or given up, is as much as the film will tell.
		if p := m.doc.Plan(plan); checked < siteChecked && len(p.Hear) > 0 {
			return
		}
		m.siteState = siteNone
		if checked > 0 && hits*2 >= checked {
			m.siteState = siteTaken
		}
	}
	if m.siteState != siteTaken {
		return
	}
	taken := map[int]bool{}
	for _, e := range m.doc.Events {
		if e.Kind == track.KindUnit && e.Gen == gen && e.Source == siteSource {
			taken[int(e.Start/track.Cell.Milliseconds())] = true
		}
	}
	for c := range m.heardCells(gen, plan) {
		if !taken[c] {
			m.siteCell(gen, plan, c)
		}
	}
}

// heardCells are the cells of gen heard as plan hears them.
func (m *making) heardCells(gen string, plan track.Making) map[int]bool {
	out := map[int]bool{}
	for _, e := range m.doc.Events {
		if e.Kind == track.KindUnit && e.Gen == gen && track.Work(e.Source) == track.WorkHear && e.Key == plan.Hearing {
			out[int(e.Start/track.Cell.Milliseconds())] = true
		}
	}
	return out
}

// heardLines are the lines of gen in cells, as they were heard.
func (m *making) heardLines(gen string, cells map[int]bool) []core.Segment {
	var out []core.Segment
	for _, l := range m.siteHeardLines(gen, cells) {
		out = append(out, l.seg)
	}
	return out
}

type heardLine struct {
	id  string
	seg core.Segment
}

// siteHeardLines are the lines of gen in cells that were heard, not the uploader's, with their text as heard.
func (m *making) siteHeardLines(gen string, cells map[int]bool) []heardLine {
	heard := map[string]string{}
	for _, e := range m.doc.Events {
		if e.Kind == track.KindRev && e.Side == track.Original && e.Layer == track.LayerHeard {
			heard[e.Line] = e.Text
		}
	}
	var out []heardLine
	for _, l := range m.doc.Lines(gen) {
		if cells[int((l.Start+l.End)/2/track.Cell)] && l.Original.Source != siteSource {
			out = append(out, heardLine{id: l.ID, seg: core.Segment{Start: l.Start, End: l.End, Text: heard[l.ID]}})
		}
	}
	return out
}

// siteCell makes cell c of gen take the site's lines where they match what was heard there (platformMerge), the cells on either side looked at too, as a line heard often spans two of the site's: the lines heard there are replaced by the lines the merge keeps whose middle is in the cell, the uploader's revisions marked siteSource, and a record that the cell took them counts the site's lines with words that lines heard replaced (Count), which the job's LocalLines adds up (siteCounts).
func (m *making) siteCell(gen string, plan track.Making, c int) {
	in := func(s core.Segment, cells ...int) bool {
		cell := int((s.Start + s.End) / 2 / track.Cell)
		return slices.Contains(cells, cell)
	}
	var subtitles []core.Segment
	for _, s := range m.site {
		if in(s, c-1, c, c+1) {
			subtitles = append(subtitles, s)
		}
	}
	around := map[int]bool{}
	heardCells := m.heardCells(gen, plan)
	for _, n := range []int{c - 1, c, c + 1} {
		around[n] = heardCells[n]
	}
	near := m.siteHeardLines(gen, around)
	var heard []core.Segment
	for _, l := range near {
		heard = append(heard, l.seg)
	}
	matched, words, _, _ := platformCheck(subtitles, heard)
	merged, site, _ := platformMerge(subtitles, heard, matched, words)
	var events []track.Event
	for _, l := range m.doc.Lines(gen) {
		if in(core.Segment{Start: l.Start, End: l.End}, c) {
			events = append(events, track.Event{Kind: track.KindGone, Line: l.ID})
		}
	}
	local := 0
	for i, s := range subtitles {
		if in(s, c) && words[i] && !slices.ContainsFunc(merged, func(m core.Segment) bool { return m == s }) {
			local++
		}
	}
	for i, s := range merged {
		if !in(s, c) {
			continue
		}
		source := makeSource
		if site[i] {
			source = siteSource
		}
		id := track.NewID()
		events = append(events,
			track.Event{Kind: track.KindLine, Gen: gen, Line: id, Start: s.Start.Milliseconds(), End: s.End.Milliseconds()},
			track.Event{Kind: track.KindRev, Line: id, Side: track.Original, Layer: track.LayerHeard, Text: s.Text, Source: source},
		)
	}
	made := track.Unit{Work: siteSource, Gen: gen, Cell: c}.Made()
	made.Count = local
	_ = m.append(append(events, made)...)
}

// siteCounts are whether gen holds the uploader's lines and how many of the site's lines with words the speech replaced, as the job keeps them (core.Job.PlatformSubtitles, core.Job.LocalLines).
func (m *making) siteCounts(gen string) (bool, int) {
	taken, local := false, 0
	for _, e := range m.doc.Events {
		if e.Kind == track.KindUnit && e.Gen == gen && e.Source == siteSource {
			local += e.Count
		}
	}
	for _, l := range m.doc.Lines(gen) {
		taken = taken || l.Original.Source == siteSource
	}
	return taken, local
}

// translateAgain translates job's film again (core.RedoTranslation), every line of it, into language, a part at a time nearest where it is watched first, as Make makes a film: its lines as the film shows them, its files taken in first, are copied into a generation of their own (copyGeneration), which the film shows once it is done, so one that fails or is stopped leaves the film as it was. It neither downloads nor hears the film, so it checks only what translating needs, and a film whose lines are gone fails with ErrNoTranscript.
func translateAgain(ctx context.Context, p Pipeline, job *core.Job, language, device string, watch Watcher, report func(core.Job)) core.Job {
	failed := func(step string, err error) core.Job {
		job.Err, job.Stage, job.Progress = fmt.Errorf("%s: %w", step, err), core.StageFailed, core.Progress{}
		report(*job)
		return *job
	}
	if tc, ok := p.(TranslateChecker); ok {
		if err := tc.CheckTranslate(); err != nil {
			return failed("check", err)
		}
	}
	if device == "" {
		return failed("check", track.ErrDevice)
	}
	doc, err := track.Read(job.VideoPath)
	if err == nil && doc.Newer {
		err = track.ErrNewer
	}
	if err != nil {
		return failed("translate", err)
	}
	// A film whose document holds no line is read from its files, which need the transcript: the bilingual subtitles alone cannot tell a translation from an original line of two rows.
	transcript := TranscriptOf(job.VideoPath)
	if len(doc.View()) == 0 && !fileExists(transcript) {
		return failed("translate", ErrNoTranscript)
	}
	if events, err := trackFiles(doc, job.SubtitlePath, transcript); err != nil {
		return failed("translate", err)
	} else if len(events) > 0 {
		written, err := track.Append(job.VideoPath, device, events...)
		doc.Events = append(doc.Events, written...)
		if err != nil {
			return failed("export", err)
		}
	}
	lines := doc.View()
	if len(lines) == 0 {
		return failed("translate", ErrNoTranscript)
	}
	if watch == nil {
		watch = From(job.ResumeAt)
	}
	m := &making{ctx: ctx, p: p, job: job, video: job.VideoPath, hearFrom: job.VideoPath, redo: true, copied: true, siteState: siteNone, reference: referenceKept, device: device, doc: doc, report: report}
	return m.run(lines[len(lines)-1].End, language, watch)
}

// fillMissing translates the lines of job's film that the film shows without a translation (core.RedoMissing) into language, a batch at a time, with the lines said before each as context, and keeps the rest as they are. It writes each batch into the lines the film shows and out beside the video as it is translated, as filling in what is missing cannot make the film worse: one that fails or is stopped keeps the lines it filled, and job.Untranslated says how many are left. It neither downloads nor hears the film. A film whose subtitles were edited outside Kando so they no longer match its transcript line for line fails with ErrSubtitlesChanged, sending nothing, as which lines are missing cannot be told; one whose bilingual subtitles are gone has every line its document does not translate translated.
func fillMissing(ctx context.Context, p Pipeline, job *core.Job, language, device string, report func(core.Job)) core.Job {
	failed := func(step string, err error) core.Job {
		job.Err, job.Stage, job.Progress = fmt.Errorf("%s: %w", step, err), core.StageFailed, core.Progress{}
		report(*job)
		return *job
	}
	if tc, ok := p.(TranslateChecker); ok {
		if err := tc.CheckTranslate(); err != nil {
			return failed("check", err)
		}
	}
	if device == "" {
		return failed("check", track.ErrDevice)
	}
	video := job.VideoPath
	doc, err := track.Read(video)
	if err == nil && doc.Newer {
		err = track.ErrNewer
	}
	if err != nil {
		return failed("translate", err)
	}
	transcript := TranscriptOf(video)
	subtitles := job.SubtitlePath
	if !fileExists(subtitles) {
		// The bilingual subtitles are gone: the film is its transcript, untranslated, unless its document has its lines.
		subtitles = transcript
	}
	events, err := trackFiles(doc, subtitles, transcript)
	if err != nil {
		return failed("translate", err)
	}
	if len(events) > 0 {
		// The files hold what the document did not write: they are taken in only when they still match the transcript line for line.
		if _, _, err := readExported(video, subtitles != transcript); err != nil {
			return failed("translate", err)
		}
		written, err := track.Append(video, device, events...)
		doc.Events = append(doc.Events, written...)
		if err != nil {
			return failed("export", err)
		}
	}
	gen := doc.Shown()
	lines := doc.Lines(gen)
	if len(lines) == 0 {
		return failed("translate", ErrNoTranscript)
	}
	job.Stage, job.Progress = core.StageTranslating, core.Progress{}
	report(*job)
	same := core.SameLanguage(job.SpokenLanguage, language, track.Originals(lines))
	var missing []int
	for i, l := range lines {
		if !same && strings.TrimSpace(l.Translation.Text) == "" {
			missing = append(missing, i)
		}
	}
	bilingual := strings.TrimSuffix(video, filepath.Ext(video)) + ".bilingual.srt"
	project := func() error {
		files, err := projectTrack(doc, doc.Lines(gen), transcript, bilingual)
		if err != nil {
			return err
		}
		written, err := track.Append(video, device, files...)
		doc.Events = append(doc.Events, written...)
		return err
	}
	left := len(missing)
	started := time.Now()
	for at := 0; at < len(missing); at += DefaultBatchSize {
		batch := missing[at:min(at+DefaultBatchSize, len(missing))]
		segments := make([]core.Segment, len(batch))
		for k, i := range batch {
			segments[k] = core.Segment{Start: lines[i].Start, End: lines[i].End, Text: lines[i].Original.Text}
		}
		var earlier []string
		for _, l := range lines[max(batch[0]-draftContext, 0):batch[0]] {
			earlier = append(earlier, l.Original.Text)
		}
		translated, err := p.Translate(WithEarlier(ctx, earlier), segments, language)
		var partial *UntranslatedError
		if errors.As(err, &partial) && len(translated) == len(segments) {
			err = nil
		}
		if err == nil && len(translated) != len(segments) {
			err = fmt.Errorf("%d translated lines for %d untranslated lines", len(translated), len(segments))
		}
		if err != nil {
			return failed("translate", err)
		}
		var revs []track.Event
		for k, i := range batch {
			text := strings.TrimSpace(translated[k].Text)
			if text == "" {
				continue
			}
			// A line taken in from a file may hold an empty translation at the final layer, which a draft would not show over.
			revs = append(revs, track.Event{Kind: track.KindRev, Line: lines[i].ID, Side: track.Translation, Layer: max(lines[i].Translation.Layer, track.LayerDraft), Text: text, Source: makeSource})
			left--
		}
		written, err := track.Append(video, device, revs...)
		doc.Events = append(doc.Events, written...)
		if err == nil {
			err = project()
		}
		if err != nil {
			return failed("export", err)
		}
		job.Untranslated = left
		progress := core.Fraction(float64(at+len(batch)) / float64(len(missing)))
		progress.Started = started
		job.Progress = progress
		report(*job)
	}
	if len(missing) == 0 {
		if err := project(); err != nil {
			return failed("export", err)
		}
	}
	if err := translateChapters(ctx, p, video, language, same); err != nil {
		return failed("translate", err)
	}
	job.SubtitlePath, job.SameLanguage = bilingual, same
	job.CompletedAt = time.Now()
	job.Language, job.Untranslated, job.TranslationErr = language, left, ""
	job.Redo, job.RedoLanguage = "", ""
	job.Stage, job.Progress = core.StageComplete, core.Progress{}
	report(*job)
	return *job
}

// copyHearing is how the cells of a film translated again are heard (track.Making.Hearing): not at all, their lines copied from the film as it shows them.
const copyHearing = "copy"

// copyGeneration is the generation a film translated again makes, of length: the one an earlier run began from the lines the film shows now and did not finish, or a new one holding a copy of each of them, as it reads, heard already in every cell, so only their translation and the final proofread are left to make.
func (m *making) copyGeneration(length time.Duration) (string, error) {
	from := m.doc.Shown()
	key := copyHearing + " " + from
	for _, e := range m.doc.Events {
		if e.Kind == track.KindGen && e.Source == makeSource && e.Key == key && !m.doc.Done(e.Gen) && m.mayResume(e) {
			return e.Gen, nil
		}
	}
	gen := track.NewID()
	events := []track.Event{{Kind: track.KindGen, Gen: gen, Source: makeSource, Key: key}}
	for _, l := range m.doc.Lines(from) {
		id := track.NewID()
		events = append(events,
			track.Event{Kind: track.KindLine, Gen: gen, Line: id, Start: l.Start.Milliseconds(), End: l.End.Milliseconds()},
			track.Event{Kind: track.KindRev, Line: id, Side: track.Original, Layer: track.LayerHeard, Text: l.Original.Text, Source: copyHearing},
		)
	}
	for c := 0; time.Duration(c)*track.Cell < length; c++ {
		events = append(events, track.Unit{Work: track.WorkHear, Gen: gen, Cell: c, Key: copyHearing}.Made())
	}
	return gen, m.append(events...)
}

// mayResume reports whether the making may go on with gen, a generation begun and not done: one this device began, or one another device began whose log has had no record for track.Lease, so two computers sharing a library do not make one generation at once, each writing its own lines into it.
func (m *making) mayResume(gen track.Event) bool {
	return gen.Device == m.device || m.doc.Idle(gen.Device, track.Now())
}

// heardIn is how generation gen's cells were heard, as their records name it (track.Making.Hearing): the language, or "auto" for a film whose language the speech recognition did not say; "" when no cell is heard yet.
func (m *making) heardIn(gen string) string {
	hearing := ""
	for _, e := range m.doc.Events {
		if e.Kind == track.KindUnit && e.Gen == gen && track.Work(e.Source) == track.WorkHear {
			hearing = e.Key
		}
	}
	return hearing
}

// makeSource is the Source of what Make writes.
const makeSource = "make"

// append writes events to the device's log and keeps them in the document.
func (m *making) append(events ...track.Event) error {
	written, err := track.Append(m.video, m.device, events...)
	m.doc.Events = append(m.doc.Events, written...)
	if err != nil && m.writeErr == nil {
		m.writeErr = err
	}
	return err
}

// hear hears cells, a run of consecutive cells, handing each on with its lines once a line past it is heard, or the run ends, then how the run ended.
func (m *making) hear(ctx context.Context, cells []int, heardCh chan<- heardCell, doneCh chan<- hearDone) {
	first, last := cells[0], cells[len(cells)-1]
	from := time.Duration(first) * track.Cell
	to := time.Duration(last+1) * track.Cell
	buffers := map[int][]core.Segment{}
	// before and after are the lines heard in the sound before and after the run's cells.
	var before, after []core.Segment
	next := first
	heard := false
	send := func(upTo int) {
		for ; next <= last && next < upTo; next++ {
			c := heardCell{cell: next, lines: buffers[next]}
			if next == first {
				c.edges = append(c.edges, before...)
			}
			if next == last {
				c.edges = append(c.edges, after...)
			}
			select {
			case heardCh <- c:
			case <-ctx.Done():
				return
			}
		}
	}
	language, err := m.p.Hear(ctx, m.hearFrom, max(from-runPad, 0), to+runPad, func(s core.Segment) {
		// A line heard, wherever it is, says the speech recognition runs: a run that fails after it fails a cell, not every run.
		heard = true
		mid := (s.Start + s.End) / 2
		switch {
		case mid < from:
			before = append(before, s)
			return
		case mid >= to:
			after = append(after, s)
			return
		}
		cell := int(mid / track.Cell)
		buffers[cell] = append(buffers[cell], s)
		// Lines come in the order they are said: a line in this cell means every cell before it is heard whole.
		send(cell)
	})
	if err == nil {
		send(last + 1)
	}
	doneCh <- hearDone{language: language, err: err, heard: heard}
}

// commitHeard writes a cell's lines, then that it is heard, at once, so a run stopped never leaves part of a cell. With them go the lines the run heard beside its cells (heardCell.edges) that no run has: a line said across two cells can be placed in one by one run and in the other by the next, as each hears it cut differently, and kept by neither. One whose cell is heard and that shows over no line heard there for at least half its time is kept; one whose cell is not heard yet is left to the run that hears it.
func (m *making) commitHeard(gen, hearing string, c heardCell) {
	var events []track.Event
	// A cell heard again another way, as in another language, holds the lines it is heard as now in place of those it was heard as before.
	for _, l := range m.doc.Lines(gen) {
		if int((l.Start+l.End)/2/track.Cell) == c.cell {
			events = append(events, track.Event{Kind: track.KindGone, Line: l.ID})
		}
	}
	lines := c.lines
	if len(c.edges) > 0 {
		have := m.doc.Lines(gen)
		for _, s := range c.edges {
			if m.heard(gen, hearing, int((s.Start+s.End)/2/track.Cell)) && !overlapsAny(s, have) {
				lines = append(lines, s)
			}
		}
	}
	for _, s := range lines {
		id := track.NewID()
		events = append(events,
			track.Event{Kind: track.KindLine, Gen: gen, Line: id, Start: s.Start.Milliseconds(), End: s.End.Milliseconds()},
			track.Event{Kind: track.KindRev, Line: id, Side: track.Original, Layer: track.LayerHeard, Text: s.Text, Source: makeSource},
		)
	}
	events = append(events, track.Unit{Work: track.WorkHear, Gen: gen, Cell: c.cell, Key: hearing}.Made())
	_ = m.append(events...)
}

// heard reports whether cell of generation gen is heard, as hearing hears it.
func (m *making) heard(gen, hearing string, cell int) bool {
	want := track.Unit{Work: track.WorkHear, Gen: gen, Cell: cell, Key: hearing}.Made()
	for _, e := range m.doc.Events {
		if e.Kind == track.KindUnit && e.Source == want.Source && e.Gen == gen && e.Start == want.Start && e.Key == hearing {
			return true
		}
	}
	return false
}

// overlapsAny reports whether s shows with one of lines for at least half of the shorter of the two: the same line, heard by another run.
func overlapsAny(s core.Segment, lines []track.Line) bool {
	for _, l := range lines {
		overlap := min(s.End, l.End) - max(s.Start, l.Start)
		if overlap > 0 && 2*overlap >= min(s.End-s.Start, l.End-l.Start) {
			return true
		}
	}
	return false
}

// firstUnheard is the first of cells not heard yet, the one a run that failed was hearing.
func (m *making) firstUnheard(gen, hearing string, cells []int) int {
	p := m.doc.Plan(track.Making{Gen: gen, Length: time.Duration(cells[len(cells)-1]+1) * track.Cell, Hearing: hearing})
	for _, c := range cells {
		for _, u := range p.Hear {
			if u.Cell == c {
				return c
			}
		}
	}
	return cells[0]
}

// draftInput is what a draft makes: the cells it drafts (none for a redraft), its lines by id as they read now, the lines said before them, and the film's terms.
type draftInput struct {
	units   []track.Unit
	ids     []string
	lines   []core.Segment
	earlier []string
	terms   []Fix
	// site says, line for line, which lines are the uploader's (siteSource), which neither terms nor the proofreading change.
	site []bool
	// redraft is a draft of lines drafted before a term was found in them: they take the term and are translated again, not proofread again.
	redraft bool
	// title names the film for the proofreading, read before the draft starts, as the making goes on changing the job beside it.
	title string
	// final is the final proofread of a block (its one unit), its lines' translations as they are now in translations.
	final        bool
	translations []core.Segment
}

// draftGroup is the cells the next draft makes, from the plan's cells needing one, and whether to make it now: the first alone while it is near the playhead, so its lines come at once; past that, as many of the cells after it as fit in a batch, so a draft far from where the film is watched sends the lines before it once for a batch rather than for every cell. A batch that is not full waits while the cell after it is still to be heard, as the run hearing it hands it on soon.
func (m *making) draftGroup(gen string, p track.Plan, playhead time.Duration) ([]track.Unit, bool) {
	needed := p.Draft
	group := needed[:1]
	start, end := needed[0].Span()
	if end > playhead-track.Behind && start < playhead+track.Ahead {
		return group, true
	}
	count := map[int]int{}
	for _, l := range m.doc.Lines(gen) {
		count[int((l.Start+l.End)/2/track.Cell)]++
	}
	lines := count[needed[0].Cell]
	for _, u := range needed[1:] {
		if u.Cell != group[len(group)-1].Cell+1 {
			break
		}
		if lines+count[u.Cell] > DefaultBatchSize {
			return group, true
		}
		lines += count[u.Cell]
		group = needed[:len(group)+1]
	}
	next := group[len(group)-1].Cell + 1
	for _, u := range p.Hear {
		if u.Cell == next {
			return group, false
		}
	}
	return group, true
}

// draftOf is what the draft of units makes, read from the document by the making's own goroutine, so the draft running beside it never reads the document while the making writes it.
func (m *making) draftOf(gen string, units []track.Unit) draftInput {
	start, _ := units[0].Span()
	_, end := units[len(units)-1].Span()
	in := draftInput{units: units, terms: m.terms(gen), title: filmTitle(*m.job)}
	for _, l := range m.doc.Lines(gen) {
		mid := (l.Start + l.End) / 2
		switch {
		case mid < start:
			in.earlier = append(in.earlier, l.Original.Text)
		case mid < end:
			in.ids = append(in.ids, l.ID)
			in.lines = append(in.lines, core.Segment{Start: l.Start, End: l.End, Text: l.Original.Text})
			in.site = append(in.site, l.Original.Source == siteSource)
		}
	}
	in.earlier = in.earlier[max(len(in.earlier)-draftContext, 0):]
	return in
}

// terms are the film's terms as the proofreading makes them.
func (m *making) terms(gen string) []Fix {
	var out []Fix
	for _, t := range m.doc.Terms(gen) {
		out = append(out, Fix{From: t.From, To: t.To})
	}
	return out
}

// draft makes a draft of in's lines in language: the film's terms put in them, then, for lines not drafted before, what the speech recognition misheard corrected by the proofreading (a proofreading that fails leaves them as they are), then each translated with the lines said before as context.
func (m *making) draft(ctx context.Context, in draftInput, language string, out chan<- drafted) {
	lines := slices.Clone(in.lines)
	// The uploader's lines (in.site) are taken as they are written: only the lines heard take the terms and are proofread.
	heard, at := heardOnly(lines, in.site)
	ApplyRecurring(heard, in.terms)
	var fixes []Fix
	if pr, ok := m.p.(Proofreader); ok && !in.redraft && !m.copied && len(heard) > 0 {
		if fixed, err := pr.Proofread(ctx, heard, in.title); err == nil && len(fixed) == len(heard) {
			heard = fixed
		} else if ctx.Err() != nil {
			out <- drafted{in: in, err: ctx.Err()}
			return
		}
		fixes = ObservedFixes(heardOnlyOf(in.lines, at), heard)
	}
	for k, i := range at {
		lines[i] = heard[k]
	}
	translated, err := m.p.Translate(WithEarlier(ctx, in.earlier), lines, language)
	var partial *UntranslatedError
	if errors.As(err, &partial) && len(translated) == len(lines) {
		err = nil
	}
	if err == nil && len(translated) != len(lines) {
		err = fmt.Errorf("%d translated lines for %d lines", len(translated), len(lines))
	}
	out <- drafted{in: in, original: lines, translated: translated, fixes: fixes, err: err}
}

// commitDraft writes a draft: each line it corrected and each translation, the corrections it made, then that its cells are drafted. A correction that becomes a term with it is put at once in the lines near the playhead drafted before (redrafts); the rest of the film takes it in the final proofread.
func (m *making) commitDraft(gen string, d drafted, playhead time.Duration) {
	before := map[track.Term]bool{}
	for _, t := range m.doc.Terms(gen) {
		before[t] = true
	}
	// A line drafted again for a term may be of a block proofread already: its new texts are written at the layer they replace, so the final proofread's do not hide them, and a text the user wrote is left as it is.
	layer := func(id string, side track.Side) (track.Layer, bool) {
		if !d.in.redraft {
			return track.LayerDraft, true
		}
		for _, l := range m.doc.Lines(gen) {
			if l.ID != id {
				continue
			}
			was := l.Original.Layer
			if side == track.Translation {
				was = l.Translation.Layer
			}
			return max(was, track.LayerDraft), was != track.LayerUser
		}
		return track.LayerDraft, true
	}
	var events []track.Event
	for i, id := range d.in.ids {
		if text := d.original[i].Text; text != d.in.lines[i].Text {
			if at, ok := layer(id, track.Original); ok {
				events = append(events, track.Event{Kind: track.KindRev, Line: id, Side: track.Original, Layer: at, Text: text, Source: makeSource})
			}
		}
		if text := strings.TrimSpace(d.translated[i].Text); text != "" {
			if at, ok := layer(id, track.Translation); ok {
				events = append(events, track.Event{Kind: track.KindRev, Line: id, Side: track.Translation, Layer: at, Text: text, Source: makeSource})
			}
		}
	}
	for _, f := range d.fixes {
		events = append(events, track.Event{Kind: track.KindFix, Gen: gen, Text: f.From, To: f.To, Count: f.Lines})
	}
	for _, u := range d.in.units {
		events = append(events, u.Made())
	}
	_ = m.append(events...)
	// The lines near the playhead already drafted that the new terms change are drafted again with them.
	var fresh []Fix
	for _, t := range m.doc.Terms(gen) {
		if !before[t] {
			fresh = append(fresh, Fix{From: t.From, To: t.To})
		}
	}
	if len(fresh) == 0 || d.in.redraft {
		return
	}
	in := draftInput{redraft: true, terms: fresh}
	drafted := map[string]bool{}
	for _, id := range d.in.ids {
		drafted[id] = true
	}
	var earlier []string
	for _, l := range m.doc.Lines(gen) {
		mid := (l.Start + l.End) / 2
		near := mid >= playhead-track.Behind && mid < playhead+track.Ahead
		if near && !drafted[l.ID] && l.Translation.Text != "" && l.Original.Source != siteSource {
			line := []core.Segment{{Start: l.Start, End: l.End, Text: l.Original.Text}}
			if ApplyRecurring(line, fresh)[0] {
				in.ids = append(in.ids, l.ID)
				in.lines = append(in.lines, core.Segment{Start: l.Start, End: l.End, Text: l.Original.Text})
				if in.earlier == nil {
					in.earlier = slices.Clone(earlier[max(len(earlier)-draftContext, 0):])
				}
			}
		}
		earlier = append(earlier, l.Original.Text)
	}
	if len(in.ids) > 0 {
		m.redrafts = append(m.redrafts, in)
	}
}

// finalKey is what a block's final proofread is made from besides its lines as heard (track.Making.Final): the film's terms found in them, so a term found later proofreads again only the blocks it is in.
func (m *making) finalKey(gen string) func([]string) string {
	return finalKeyOf(func() []Fix { return m.terms(gen) })
}

// finalKeyOf is finalKey with terms giving the film's terms as they are now. What it finds for a block's lines is kept until the film has more terms.
func finalKeyOf(terms func() []Fix) func([]string) string {
	type memo struct {
		terms int
		key   string
	}
	seen := map[string]memo{}
	return func(heard []string) string {
		terms := terms()
		id := strings.Join(heard, "\x00")
		if was, ok := seen[id]; ok && was.terms == len(terms) {
			return was.key
		}
		lines := make([]core.Segment, len(heard))
		var found []string
		for _, t := range terms {
			for i, h := range heard {
				lines[i].Text = h
			}
			if slices.Contains(ApplyRecurring(lines, []Fix{t}), true) {
				found = append(found, t.From+"\x01"+t.To)
			}
		}
		key := strings.Join(found, "\x02")
		seen[id] = memo{terms: len(terms), key: key}
		return key
	}
}

// DraftSpans are the spans of the lines doc shows that are drafts (track.Doc.Final), of a film made a part at a time (Make) into language: not proofread as a whole yet, or proofread before what their block holds now. A film not made so, as one whose subtitles were taken in from its files, has none.
func DraftSpans(doc track.Doc, language string) map[core.Span]bool {
	gen := doc.Shown()
	made := slices.ContainsFunc(doc.Events, func(e track.Event) bool { return e.Kind == track.KindGen && e.Gen == gen && e.Source == makeSource })
	if !made {
		return nil
	}
	var terms []Fix
	for _, t := range doc.Terms(gen) {
		terms = append(terms, Fix{From: t.From, To: t.To})
	}
	final := doc.Final(gen, language, finalKeyOf(func() []Fix { return terms }))
	out := map[core.Span]bool{}
	for _, l := range doc.View() {
		if !final[l.ID] {
			out[core.Span{Start: l.Start, End: l.End}] = true
		}
	}
	return out
}

// finalOf is what the final proofread of unit's block reads: its lines as they read now, their translations, the lines said before them and the film's terms.
func (m *making) finalOf(gen string, unit track.Unit) draftInput {
	start, _ := unit.Span()
	end := start + track.Block*track.Cell
	in := draftInput{units: []track.Unit{unit}, terms: m.terms(gen), title: filmTitle(*m.job), final: true}
	for _, l := range m.doc.Lines(gen) {
		mid := (l.Start + l.End) / 2
		switch {
		case mid < start:
			in.earlier = append(in.earlier, l.Original.Text)
		case mid < end:
			in.ids = append(in.ids, l.ID)
			in.lines = append(in.lines, core.Segment{Start: l.Start, End: l.End, Text: l.Original.Text})
			in.translations = append(in.translations, core.Segment{Start: l.Start, End: l.End, Text: l.Translation.Text})
			in.site = append(in.site, l.Original.Source == siteSource)
		}
	}
	in.earlier = in.earlier[max(len(in.earlier)-draftContext, 0):]
	return in
}

// final proofreads a block as a whole: the film's terms found since it was drafted are put in its lines, those they change are translated again, then the final proofread reads the block, its changes of the kinds auto names made at once and the rest kept for the user to review.
func (m *making) final(ctx context.Context, in draftInput, language string, auto core.ProofAuto, out chan<- drafted) {
	lines := slices.Clone(in.lines)
	translated := slices.Clone(in.translations)
	heard, at := heardOnly(lines, in.site)
	changed := make([]bool, len(lines))
	for k, c := range ApplyRecurring(heard, in.terms) {
		lines[at[k]], changed[at[k]] = heard[k], c
	}
	var again []int
	for i, c := range changed {
		if c && strings.TrimSpace(translated[i].Text) != "" {
			again = append(again, i)
		}
	}
	if len(again) > 0 {
		segments := make([]core.Segment, len(again))
		for k, i := range again {
			segments[k] = lines[i]
		}
		redone, err := m.p.Translate(WithEarlier(ctx, in.earlier), segments, language)
		var partial *UntranslatedError
		if errors.As(err, &partial) && len(redone) == len(segments) {
			err = nil
		}
		if err == nil && len(redone) != len(segments) {
			err = fmt.Errorf("%d translated lines for %d lines", len(redone), len(segments))
		}
		if err != nil {
			out <- drafted{in: in, err: err}
			return
		}
		for k, i := range again {
			if text := strings.TrimSpace(redone[k].Text); text != "" {
				translated[i].Text = text
			}
		}
	}
	found, err := m.p.(Reviewer).Review(ctx, lines, translated, in.title, language)
	if err != nil {
		out <- drafted{in: in, err: err}
		return
	}
	var proofs []core.Proof
	for _, f := range found {
		f.Status = core.ProofPending
		if auto.Applies(f.Kind) {
			for i := range translated {
				if translated[i].Start == f.Span.Start && translated[i].End == f.Span.End && strings.TrimSpace(translated[i].Text) == f.Old {
					translated[i].Text, f.Status = f.New, core.ProofApplied
					break
				}
			}
		}
		proofs = append(proofs, f)
	}
	out <- drafted{in: in, original: lines, translated: translated, proofs: proofs}
}

// commitFinal writes a block's final proofread: each text it changed, as final, then that the block is proofread, which makes its lines final; and its changes, in place of those an earlier proofread of the block suggested, beside the video for the user to review (ProofsPath).
func (m *making) commitFinal(d drafted) {
	made := d.in.units[0].Made()
	// A redo's changes are kept with its block's record until it is done, as the film shows what it redoes until then (keepProofs).
	if m.redo {
		data, _ := json.Marshal(d.proofs)
		made.Text = string(data)
	}
	var events []track.Event
	for i, id := range d.in.ids {
		if text := d.original[i].Text; text != d.in.lines[i].Text {
			events = append(events, track.Event{Kind: track.KindRev, Line: id, Side: track.Original, Layer: track.LayerFinal, Text: text, Source: makeSource})
		}
		if text := strings.TrimSpace(d.translated[i].Text); text != "" && text != d.in.translations[i].Text {
			events = append(events, track.Event{Kind: track.KindRev, Line: id, Side: track.Translation, Layer: track.LayerFinal, Text: text, Source: makeSource})
		}
	}
	_ = m.append(append(events, made)...)
	if m.redo {
		return
	}
	start, _ := d.in.units[0].Span()
	end := start + track.Block*track.Cell
	path := ProofsPath(m.video)
	kept, err := LoadProofs(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	proofs := slices.DeleteFunc(kept, func(p core.Proof) bool {
		mid := (p.Span.Start + p.Span.End) / 2
		return mid >= start && mid < end
	})
	_ = SaveProofs(path, append(proofs, d.proofs...))
}

// keepProofs writes the changes the final proofread of a redo suggested beside the video for the user to review (ProofsPath), once the redo is done and its lines are the film's: those of each block whose final proofread holds for what it has now (track.Plan.Finished), the latest made, in place of every earlier change. A redo whose final proofread was not made keeps none.
func (m *making) keepProofs(gen string, last track.Plan) {
	latest := map[int]string{}
	for _, e := range m.doc.Events {
		if e.Kind == track.KindUnit && e.Gen == gen && track.Work(e.Source) == track.WorkFinal {
			latest[int(e.Start/(track.Cell*track.Block).Milliseconds())] = e.Text
		}
	}
	var proofs []core.Proof
	for block, data := range latest {
		var found []core.Proof
		if last.Finished[block] && json.Unmarshal([]byte(data), &found) == nil {
			proofs = append(proofs, found...)
		}
	}
	slices.SortFunc(proofs, func(a, b core.Proof) int { return int(a.Span.Start - b.Span.Start) })
	_ = SaveProofs(ProofsPath(m.video), proofs)
}

// progress reports how far the film is made: hearing while cells are left to hear, then translating.
func (m *making) progress(p track.Plan) {
	stage := core.StageTranscribing
	if len(p.Hear) == 0 {
		stage = core.StageTranslating
	}
	done := float64(p.Heard+p.Drafted) / float64(max(2*p.Cells, 1))
	if m.job.Stage != stage {
		m.job.Stage, m.job.Progress = stage, core.Progress{}
	}
	progress := core.Fraction(done)
	progress.Started, progress.From = m.job.Progress.Started, m.job.Progress.From
	if progress.Started.IsZero() {
		progress.Started, progress.From = time.Now(), done
	}
	m.job.Progress = progress
	m.report(*m.job)
}

// videoCame takes in how the download of the film's video ended: the film is its video from now on, or the job fails for it once its subtitles are made.
func (m *making) videoCame(d downloaded) {
	m.coming = nil
	if d.err != nil {
		m.videoErr = d.err
		return
	}
	m.job.VideoPath = d.path
	m.report(*m.job)
}

// project writes the lines of gen out beside the video, as they are now, at most every projectEvery (projectTrack): a redo, whose lines show once it is done, writes nothing until then (finish). It reads the film's logs again first (reread), and takes in what was edited in the film's subtitles outside Kando (takeEdits). The film shows its bilingual subtitles from the first lines written out.
func (m *making) project(gen string) {
	if time.Since(m.projected) < projectEvery {
		return
	}
	if m.redo {
		return
	}
	m.reread()
	m.takeEdits(gen)
	lines := m.doc.Lines(gen)
	events, err := projectTrack(m.doc, lines, m.transcript, m.bilingual)
	if err != nil || len(lines) == 0 {
		return
	}
	m.projected = time.Now()
	_ = m.append(events...)
	// A film is shown with its subtitles once there is a video to show them on.
	if m.job.VideoPath != "" && m.job.SubtitlePath != m.bilingual {
		m.job.SubtitlePath = m.bilingual
		m.report(*m.job)
	}
}

// spawn runs work beside the making, which waits for it before it returns (workers).
func (m *making) spawn(work func()) {
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		work()
	}()
}

// reread reads the film's logs again, so what was written beside the making, as the user's decision on a change of the final proofread (DecideProofs), is written out too and not lost.
func (m *making) reread() {
	if doc, err := track.Read(m.video); err == nil && len(doc.Events) >= len(m.doc.Events) {
		m.doc = doc
	}
}

// takeEdits takes into gen, as the final layer, the texts the film's subtitles hold that the document did not write out, as edited in another editor while the film is made, which writing its lines out again would otherwise put back. A film made again writes nothing out until it is done, and took its files in as it began (Make). Only texts are taken in, on the lines of gen shown at the times the file shows them: a file whose lines show at other times, as one with lines added, taken out or moved, is written over as before, as which of the lines still to come it stands for cannot be told.
func (m *making) takeEdits(gen string) {
	if m.redo {
		return
	}
	shown := m.bilingual
	if !fileExists(shown) {
		shown = m.transcript
	}
	paired, file, err := trackFileLines(m.doc, shown, m.transcript)
	if err != nil || file.Kind == "" {
		return
	}
	at := map[core.Span][]track.Line{}
	for _, l := range m.doc.Lines(gen) {
		span := core.Span{Start: l.Start, End: l.End}
		at[span] = append(at[span], l)
	}
	matched := make([]track.Line, len(paired))
	for i, p := range paired {
		span := core.Span{Start: p.Start, End: p.End}
		if len(at[span]) == 0 {
			return
		}
		matched[i], at[span] = at[span][0], at[span][1:]
	}
	_ = m.append(append(trackTakeIn(matched, paired), file)...)
}

// finish writes the lines of gen out beside the video whole, then records that gen holds every line of the film, which makes it the one the film shows: a film whose subtitles cannot be written stays as it was, a redo the film it redoes.
func (m *making) finish(gen string) error {
	m.reread()
	m.takeEdits(gen)
	if m.writeErr != nil {
		return m.writeErr
	}
	files, err := projectTrack(m.doc, m.doc.Lines(gen), m.transcript, m.bilingual)
	if err != nil {
		return err
	}
	return m.append(append([]track.Event{{Kind: track.KindDone, Gen: gen}}, files...)...)
}

// projectTrack writes lines out to transcript, the film's .srt of the speech alone, and bilingual, its bilingual subtitles, each written whole or not at all, and returns the records of what it wrote for the caller to keep in the film's log, so a read does not take the files back in as written by something else (ReadTrack). No lines write nothing, and neither do lines doc last wrote out as they read, to files that still hold them: a film made a part at a time is written out every few seconds, and its log and every reader of its files would otherwise take each write in again.
func projectTrack(doc track.Doc, lines []track.Line, transcript, bilingual string) ([]track.Event, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	speech := []byte(formatSRT(track.Originals(lines)))
	both := []byte(formatSRT(track.Bilingual(lines)))
	files := []track.Event{
		{Kind: track.KindFile, Source: filepath.Base(transcript), Key: trackFileHash(speech, nil)},
		{Kind: track.KindFile, Source: filepath.Base(bilingual), Key: trackFileHash(both, speech)},
	}
	if projected(doc, transcript, speech, files[0]) && projected(doc, bilingual, both, files[1]) {
		return nil, nil
	}
	if err := writeFileAtomic(transcript, speech); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(bilingual, both); err != nil {
		return nil, err
	}
	return files, nil
}

// projected reports whether the file at path holds data and doc's last record of it is file, the record of data: it was written out as it would be now, and is still as written.
func projected(doc track.Doc, path string, data []byte, file track.Event) bool {
	last, ok := doc.File(file.Source)
	if !ok || last.Key != file.Key {
		return false
	}
	held, err := os.ReadFile(path)
	return err == nil && bytes.Equal(held, data)
}

// runOf is the cells the next run of the speech recognition hears: from the first cell needed, those after it that are needed too, as many as a run hears, or the first alone when the language is not known yet, so it is found before the rest are heard.
func runOf(needed []track.Unit, known bool) []int {
	cells := []int{needed[0].Cell}
	if !known {
		return cells
	}
	for _, u := range needed[1:] {
		if u.Cell != cells[len(cells)-1]+1 || len(cells) == runCellsMax {
			break
		}
		cells = append(cells, u.Cell)
	}
	return cells
}

func containsCell(cells []int, cell int) bool {
	for _, c := range cells {
		if c == cell {
			return true
		}
	}
	return false
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
