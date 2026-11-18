package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/packages/core"
)

var ErrNoSpeech = errors.New("no speech found in the video")

// ErrNoAudio is why transcribing failed for a video without a sound track, as many screen recordings posted on X are: there is nothing to hear, so retrying cannot help.
var ErrNoAudio = errors.New("the video has no sound")

// ErrNoTranscript is why a job translating a done film again failed when the transcript its earlier run exported beside the video is gone or unreadable.
var ErrNoTranscript = errors.New("the film's transcript is no longer beside its video")

// ErrSubtitlesChanged is why a job translating only the missing lines of a done film again failed when the film's bilingual subtitles no longer match its transcript cue for cue, as after they were edited by hand: which lines are missing cannot be told, and translating every line would overwrite the edits.
var ErrSubtitlesChanged = errors.New("the film's bilingual subtitles no longer match its transcript")

// TranslateChecker is a Pipeline that checks what translating needs apart from Check: a job translating a done film again (core.Job.Redo) checks only that, so it does not fail for a download or speech tool it does not run, and a job making a film for the first time checks it before translating, so one without a translator still completes with the original lines (core.Job.TranslationErr).
type TranslateChecker interface {
	CheckTranslate() error
}

// hearer is a Pipeline that says how its Transcribe hears the speech of a video under ctx, as Tools does: by the speech model and in the language it is told, "" for one it detects. Speech that a redo kept aside is read back by a run that hears it the same way (pendingHeard); a pipeline that is not a hearer always hears the same way.
type hearer interface {
	hearing(ctx context.Context) (model, language string)
}

// Pipeline is the cross-platform boundary for media processing. Implementations keep OS-specific details out of the core package.
type Pipeline interface {
	// Check reports missing tools or settings before a job spends time downloading.
	Check() error
	Download(ctx context.Context, source string) (string, error)
	Transcribe(ctx context.Context, videoPath string) ([]core.Segment, error)
	// Translate returns the segments translated into language. When it left some lines untranslated, with an empty Text, it returns every segment with an *UntranslatedError, and the job completes with them.
	Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error)
	// Export writes the subtitles next to the video and returns the bilingual subtitle path.
	Export(ctx context.Context, videoPath string, original, translated []core.Segment) (string, error)
}

// Run drives job through every stage of p. report receives the job each time its stage changes, ending with StageComplete or StageFailed, and each time a stage reports through the context's Trace how far it has gone (job.Progress) or what the video is called (job.Title, job.Thumbnail); the final job is returned. Progress starts again at each stage and is cleared once the job is done; Untranslated counts the lines this run's translation left untranslated. A local video (a file:// source) is not downloaded, so its job goes from StageQueued straight to StageTranscribing; p.Download still runs to check the file and give its path. A job whose SpeechLanguage is set has p.Transcribe hear that language (Tools.Transcribe reads it from the context) rather than the one Tools names for every video.
//
// Once transcribed, a job making a film for the first time reports its transcript as job.SubtitlePath with StageTranslating, so the film can be watched with its original lines while it is translated. Its transcript is "<video>.srt", unless a local video already has a "<video>.srt" beside it that Kando did not write (foreignTranscript), as subtitles the user brought with it: that file is left as it is, and Kando keeps its transcript in "<video>.kando.srt" (TranscriptOf), which Export writes too.
//
// A job run again resumes after the stages an earlier run finished. A link whose video is still at job.VideoPath is not downloaded again, so the retry works offline or after the site took the video down. Once transcribed, the speech is saved beside the video before the translation starts and job.Transcribed set; a job that comes back with it set reads that transcript back instead of calling p.Transcribe, and goes straight to StageTranslating. A job whose Transcribed is false, as a done film processed again, or whose video had to be downloaded again, is transcribed anew.
//
// A job whose Redo is RedoTranslation or RedoMissing translates a done film again from the subtitles its earlier run exported beside job.VideoPath (exportSubtitles): it neither downloads nor transcribes, so it goes from StageQueued straight to StageTranslating, and a pipeline that is a TranslateChecker checks only what translating needs. RedoTranslation translates every line of the transcript again; RedoMissing sends p.Translate only the lines the bilingual subtitles left untranslated and keeps the rest. A film whose transcript is gone fails with ErrNoTranscript, and one whose bilingual subtitles no longer match it, as ones edited by hand, fails a RedoMissing with ErrSubtitlesChanged rather than translating every line over them. A job whose Redo is RedoAll runs every stage as a job run again does, but keeps the speech it transcribed in a hidden file beside the video (pendingTranscriptPath) rather than over the film's transcript, which Export replaces, with how it was heard (pendingHeard): a RedoAll run again after it failed or was stopped reads that speech back when the pipeline hears the video the same way, and transcribes it again when the speech model or the language it is told changed. Until the new subtitles are exported a redo keeps the film's Language, SpokenLanguage, Untranslated and CompletedAt, and its subtitles on disk are untouched, so a redo that fails still describes them; once it is complete its Language is language, its SpokenLanguage the one its transcription heard, and Redo and RedoLanguage are cleared.
//
// A job making a film for the first time (Redo empty) whose translation fails as a whole, other than by being stopped, or whose pipeline has no translator (TranslateChecker), still completes: Export writes the transcript and a bilingual file of the original lines alone, every line counts as untranslated, and job.TranslationErr says why. Translating its missing lines (RedoMissing) translates it later. A redo whose translation fails fails, leaving the film as it was.
//
// Before it translates, a job whose pipeline is a Proofreader has the transcript proofread, unless it was already (job.Proofread, or what a RedoAll or a translating redo kept aside): the words whisper misheard are corrected in the original lines, which every part of Kando shows and the translation is made from. A job making a film for the first time saves the proofread transcript over the one it heard and sets job.Proofread at once; a RedoAll writes it over the speech it keeps aside; a translating redo keeps it in a hidden file beside the video (proofreadTranscriptPath) until it exports, so a redo that fails leaves the film's transcript as it was and one run again does not pay for the proofreading twice. Proofreading that fails, or a translator that is not set up, leaves the lines as they were heard. A film heard in the language its subtitles are for (core.SameLanguage) is not translated: its subtitles are its own lines alone, no line counts as untranslated, and job.SameLanguage says so once it is complete.
//
// A link's job whose SubtitleSource is core.SubtitlesPlatform, on a pipeline that is a PlatformSubtitler, fetches the subtitles the uploader put on the site in the language the speech was heard in once the speech it heard is kept and proofread, and takes them as the film's original lines where they match the speech (matchPlatform): where they do not, and where they have none, the lines are the ones heard, proofread. job.PlatformSubtitles and job.LocalLines say so, and the transcript kept beside the video holds those lines, so a job run again reads them back; one stopped while it fetches them keeps the speech it heard and fetches them again. The uploader's lines are not proofread. A job's Quality is the picture its download asks for (Tools.Download).
//
// A link's download that finds chapters the site marks in the video, the timestamps of a YouTube video's description or a bilibili video's 视频看点 (core.Chapter), keeps them beside the video (ChaptersPath), and each job that translates the film translates their titles into the language of its subtitles, after its lines and apart from them: a failure leaves them as the site wrote them, and the job goes on.
//
// The translation keeps each batch it has done in a hidden file beside the video (withCheckpoint) until the job exports, so a job whose translation stopped partway, cancelled, quit or failed, does not pay for those batches again when it runs again with the same lines and language.
//
// A link's job at core.QualitySubtitles downloads the sound alone and removes it once the subtitles are exported, keeping job.VideoPath, which the subtitles are named after, and setting job.SubtitlesOnly. A job whose Redo is core.RedoVideo downloads the video of such a film at job.Quality, puts it under the name its subtitles go by (placeVideo) and completes, neither transcribing nor translating; one that fails leaves the film as it was.
func Run(ctx context.Context, p Pipeline, job core.Job, language string, report func(core.Job)) core.Job {
	// heard is the language a RedoAll job's transcription heard, which becomes the film's SpokenLanguage only once the job is complete, so a redo that fails or is stopped leaves the film's as it was.
	var heard string
	// chapters are the chapters the site marks in the video, as the download found them, which are kept beside the video once it is downloaded.
	var chapters []core.Chapter
	enter := func(stage core.Stage) {
		job.Stage = stage
		job.Progress = core.Progress{}
		report(job)
	}
	ctx = WithTrace(ctx, &Trace{
		Progress: func(progress core.Progress) {
			// The stage is timed from the first report that says how far it has gone, so the minutes before it, as a model loading, and the batches a translation kept from an earlier run, which it does not report, do not make it look faster than it goes.
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
		// The language heard goes out with the job's next report, as the transcription ends. A redo keeps the film's until it is complete (heard).
		Spoken: func(language string) {
			if job.Redo == core.RedoAll {
				heard = language
				return
			}
			job.SpokenLanguage = language
		},
		Chapters: func(list []core.Chapter) { chapters = list },
	})
	// A film the user said is spoken in another language than the settings name is heard in its own (core.Job.SpeechLanguage).
	if code := strings.TrimSpace(job.SpeechLanguage); code != "" {
		ctx = withSpeechLanguage(ctx, code)
	}
	// A link downloads at the quality the user chose for it (core.Job.Quality).
	ctx = withQuality(ctx, job.Quality)
	if job.Redo == "" {
		job.Untranslated = 0
	}
	fail := func(step string, err error) core.Job {
		job.Err = fmt.Errorf("%s: %w", step, err)
		enter(core.StageFailed)
		return job
	}

	// A film that keeps no video downloads it beside its subtitles, and nothing more (core.RedoVideo).
	if job.Redo == core.RedoVideo {
		enter(core.StageDownloading)
		video, err := p.Download(ctx, job.Source)
		if err == nil {
			video, err = placeVideo(job.VideoPath, video)
		}
		if err != nil {
			job.FailKind = downloadFailKind(job, err)
			return fail("download", err)
		}
		job.VideoPath, job.SubtitlesOnly = video, false
		job.Redo, job.RedoLanguage = "", ""
		enter(core.StageComplete)
		return job
	}

	// A job that has not completed yet needs only its download and speech tools to start: without a translator it still makes the film's subtitles, the original lines alone (core.Job.TranslationErr). A redo needs the translator, as it would otherwise replace the film's translated subtitles with fewer.
	tc, translateChecked := p.(TranslateChecker)
	check := p.Check
	switch {
	case translateChecked && job.Redo.TranslatesOnly():
		check = tc.CheckTranslate
	case translateChecked && job.Redo != "":
		check = func() error { return errors.Join(p.Check(), tc.CheckTranslate()) }
	}
	if err := check(); err != nil {
		return fail("check", err)
	}

	var original, earlier []core.Segment
	// platform is whether original is the site's subtitles (core.Job.PlatformSubtitles), localLines how many of their lines the speech replaced, which a job making a film for the first time keeps in the job as soon as it saves its transcript, and a RedoAll only once it completes. The uploader's own lines are not proofread.
	platform, localLines := job.PlatformSubtitles, job.LocalLines
	// proofread is whether original has been proofread, which a job making a film for the first time keeps in job.Proofread as soon as it saves it, and a redo only once it completes, as the film's says what its transcript on disk is until then.
	proofread := false
	// model and language are how p hears the video (hearer), which a RedoAll writes beside the speech it keeps aside.
	var model, hearLanguage string
	// transcript is where the job keeps the original speech, which Export writes too: the film's transcript (TranscriptOf), or for a video whose "<video>.srt" is not Kando's, a file beside it of Kando's own (asideTranscriptPath).
	var transcript string
	// sound is whether the file at job.VideoPath is the sound alone, downloaded to make the subtitles of a film that keeps no video (core.QualitySubtitles), which goes once they are exported.
	sound := false
	if !job.Redo.TranslatesOnly() {
		_, local := localVideoPath(job.Source)
		sound = !local && job.Quality == core.QualitySubtitles
		video := job.VideoPath
		// same is false once the video is not the file an earlier run heard, as one downloaded again.
		same := true
		if local || !Downloaded(video) {
			if !local {
				enter(core.StageDownloading)
			}
			var err error
			video, err = p.Download(ctx, job.Source)
			if err != nil {
				job.FailKind = downloadFailKind(job, err)
				return fail("download", err)
			}
			// A link downloaded again lands on the same path as before, so the transcript beside it may have been made from the file this download replaced, such as a silent track that never merged.
			if !local {
				job.Transcribed, same = false, false
			}
		}
		// A transcript saved beside another file is not this video's.
		if video != job.VideoPath {
			job.Transcribed, same = false, false
		}
		job.VideoPath = video
		if len(chapters) > 0 {
			keepChapters(video, chapters, job.ChapterUse)
		}

		transcript = TranscriptOf(video)
		if h, ok := p.(hearer); ok {
			model, hearLanguage = h.hearing(ctx)
		}
		switch {
		case job.Redo == core.RedoAll:
			transcript = pendingTranscriptPath(video)
			// A redo that failed or was stopped after it transcribed kept the speech it heard aside; run again, it reads that back when this run would hear the speech the same way, with the same model in the same language, rather than transcribing the film again.
			// What the film's own job found is not what the redo kept aside.
			platform, localLines = false, 0
			was, ok := readPendingHeard(video)
			if ok && !job.Transcribed && same && was.Model == model && was.Language == hearLanguage {
				job.Transcribed = true
			}
			if ok && job.Transcribed {
				heard, proofread = was.Spoken, was.Proofread
				platform, localLines = was.Platform, was.Local
			}
		case job.Redo == "" && !job.Transcribed && local && foreignTranscript(video):
			// A "<video>.srt" Kando did not write, as subtitles the user brought with a video, stays as it is.
			transcript = asideTranscriptPath(video)
		}
		if job.Transcribed {
			// A transcript that is gone or unreadable is made again, as if it had never been saved.
			original, _ = loadTranscript(transcript)
			if job.Redo == "" {
				proofread = job.Proofread
			}
		}
		if len(original) == 0 {
			job.Transcribed, proofread, platform, localLines = false, false, false, 0
			if job.Redo == "" {
				job.Proofread, job.PlatformSubtitles, job.LocalLines = false, false, 0
			}
			enter(core.StageTranscribing)
			var err error
			original, err = p.Transcribe(ctx, video)
			if err != nil {
				return fail("transcribe", err)
			}
			if len(original) == 0 {
				return fail("transcribe", ErrNoSpeech)
			}
			// The transcript is kept before the translation starts, so a translation that fails does not cost the minutes of transcribing again. A transcript that cannot be saved only means a retry transcribes again; Export, which writes beside the video too, says why the folder cannot be written. A redo's says how it was heard, written once the speech it describes is.
			if job.Redo == core.RedoAll {
				_ = os.Remove(pendingHeardPath(video))
			}
			job.Transcribed = saveTranscript(transcript, original) == nil
			if job.Redo == core.RedoAll && job.Transcribed {
				savePendingHeard(video, pendingHeard{Model: model, Language: hearLanguage, Spoken: heard})
			}
		}
		// Until its subtitles are exported, a film being made for the first time shows its transcript, the original lines, so it can be watched while it is translated; Export puts the bilingual subtitles in its place. A redo keeps the film's own subtitles until then.
		if job.Redo == "" && job.Transcribed {
			job.SubtitlePath = transcript
		}
	} else {
		var err error
		original, earlier, err = readExported(job.VideoPath, job.Redo == core.RedoMissing)
		if err != nil {
			return fail("translate", err)
		}
		proofread = job.Proofread
		// A redo stopped or failed after it proofread the film reads the lines it proofread back.
		if lines, ok := readProofread(job.VideoPath, original); !proofread && ok {
			original, proofread = lines, true
		}
	}

	enter(core.StageTranslating)
	var translated []core.Segment
	var untranslated int
	var err error
	if translateChecked && job.Redo == "" {
		err = tc.CheckTranslate()
	}
	// The transcript is proofread before it is translated, by the translator, so what whisper misheard is corrected in the original lines every part of Kando shows and in what is translated from them. Proofreading that fails, or a pipeline that cannot, leaves the lines as they were heard; a job stopped while it proofreads stops. Lines that hold the site's subtitles (platform) are not proofread, as the uploader's are not to be changed and which lines among them were heard is not kept.
	if pr, ok := p.(Proofreader); ok && !proofread && !platform && err == nil {
		fixed, prErr := pr.Proofread(ctx, original, filmTitle(job))
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fail("translate", ctxErr)
		}
		if prErr == nil {
			original, proofread = fixed, true
			switch {
			case job.Redo == "":
				job.Proofread = job.Transcribed && saveTranscript(transcript, original) == nil
			case job.Redo == core.RedoAll:
				if job.Transcribed && saveTranscript(transcript, original) == nil {
					savePendingHeard(job.VideoPath, pendingHeard{Model: model, Language: hearLanguage, Spoken: heard, Proofread: true, Platform: platform, Local: localLines})
				}
			default:
				_ = saveTranscript(proofreadTranscriptPath(job.VideoPath), original)
			}
		}
		// The translation's time is told from its own batches, not from the proofreading's.
		job.Progress = core.Progress{}
		report(job)
	}
	// The site's own subtitles, in the language the speech was heard in, become the film's lines where they match it (core.SubtitlesPlatform), once the speech is heard, kept and proofread: the lines heard that stay among them are proofread like any, and the uploader's are not, as they are proofread once only, before. A job stopped while it fetches them keeps the speech it heard, and one run again fetches them again.
	if !job.Redo.TranslatesOnly() && !platform {
		spokenNow := job.SpokenLanguage
		if job.Redo == core.RedoAll {
			spokenNow = heard
		}
		if spokenNow == "" {
			spokenNow = hearLanguage
		}
		lines, n, ok := platformLines(ctx, p, job, spokenNow, original)
		// Only a job that fetched its site's subtitles can have been stopped in the fetch, which leaves it without them though the site has them; one that fetched none goes on as before, as a job stopped meanwhile is stopped by the steps after.
		if _, fetches := p.(PlatformSubtitler); fetches && job.SubtitleSource == core.SubtitlesPlatform {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fail("translate", ctxErr)
			}
		}
		if ok {
			original, platform, localLines = lines, true, n
			saved := job.Transcribed && saveTranscript(transcript, original) == nil
			switch {
			case job.Redo == "" && saved:
				job.PlatformSubtitles, job.LocalLines = true, n
			case job.Redo == core.RedoAll && saved:
				savePendingHeard(job.VideoPath, pendingHeard{Model: model, Language: hearLanguage, Spoken: heard, Proofread: proofread, Platform: true, Local: n})
			}
		}
	}
	// A film heard in the language its subtitles are for is not translated: its subtitles are its own lines (core.SameLanguage). A RedoAll decides by what it heard; any other job by what the film was heard in when it was transcribed.
	spoken := job.SpokenLanguage
	if job.Redo == core.RedoAll && heard != "" {
		spoken = heard
	}
	same := core.SameLanguage(spoken, language, original)
	kept := translationCheckpointPath(job.VideoPath)
	translateCtx := withCheckpoint(ctx, kept)
	// notTranslated is why this run translated no line, "" once it translated.
	var notTranslated string
	switch {
	case same:
		translated, err = blankLines(original), nil
	case err != nil:
	case job.Redo == core.RedoMissing:
		translated, untranslated, err = translateMissing(translateCtx, p, original, earlier, language)
	default:
		translated, err = p.Translate(translateCtx, original, language)
		// Lines the translation could not do after trying them again stay untranslated; the rest of the film is not lost for them.
		var partial *UntranslatedError
		if errors.As(err, &partial) && len(translated) == len(original) {
			untranslated, err = partial.Lines, nil
		}
	}
	// A film being made for the first time whose translation failed as a whole, or that has no translator, is still a film: its subtitles are the original lines, every one untranslated, and the job keeps why, so translating its missing lines later translates it (core.Job.TranslationErr). The batches a failed translation did stay kept beside the video for that. A job that was stopped stays stopped, and a redo fails, leaving the film's subtitles as they were.
	if err != nil && job.Redo == "" && ctx.Err() == nil && !errors.Is(err, context.Canceled) {
		notTranslated = err.Error()
		translated, untranslated, err = blankLines(original), len(original), nil
	}
	if err != nil {
		return fail("translate", err)
	}
	// The titles of the chapters the site marks are translated as the lines were, unless the translation failed as a whole; a job stopped while they are translated stops.
	if notTranslated == "" {
		if err := translateChapters(ctx, p, job.VideoPath, language, same); err != nil {
			return fail("translate", err)
		}
	}

	enter(core.StageExporting)
	// A redo exports the film's transcript (TranscriptOf) in place of the one it kept aside.
	if job.Redo != "" || transcript == "" {
		transcript = TranscriptOf(job.VideoPath)
	}
	subtitles, err := p.Export(withTranscript(ctx, transcript), job.VideoPath, original, translated)
	if err != nil {
		return fail("export", err)
	}
	// The subtitles hold every line now; what was kept to resume them is done with, unless the translation did not get to the end.
	if kept != "" && notTranslated == "" {
		_ = os.Remove(kept)
	}
	if job.Redo == core.RedoAll {
		_ = os.Remove(pendingTranscriptPath(job.VideoPath))
		_ = os.Remove(pendingHeardPath(job.VideoPath))
		if heard != "" {
			job.SpokenLanguage = heard
		}
	}
	_ = os.Remove(proofreadTranscriptPath(job.VideoPath))
	// A film whose video this run downloaded or kept has one again, as after a video download that failed and left the film at a quality that keeps it.
	switch {
	case sound:
		job.SubtitlesOnly = removeSound(job.VideoPath)
	case !job.Redo.TranslatesOnly():
		job.SubtitlesOnly = false
	}
	job.Proofread, job.SameLanguage = proofread, same
	job.PlatformSubtitles, job.LocalLines = platform, localLines
	job.SubtitlePath = subtitles
	job.CompletedAt = time.Now()
	job.Language, job.Untranslated, job.TranslationErr = language, untranslated, notTranslated
	job.Redo, job.RedoLanguage = "", ""

	enter(core.StageComplete)
	return job
}

// downloadFailKind is why job's download failed, as err says (ClassifyDownload): a site that still wants a login while the browser's is borrowed has let that login run out.
func downloadFailKind(job core.Job, err error) core.FailKind {
	kind := ClassifyDownload(err.Error())
	if job.Cookies != "" && (kind == core.FailLogin || kind == core.FailAge) {
		kind = core.FailLoginExpired
	}
	return kind
}

// blankLines are the times of lines with no text, the translation of lines none of which was translated.
func blankLines(lines []core.Segment) []core.Segment {
	out := make([]core.Segment, len(lines))
	for i, s := range lines {
		out[i] = core.Segment{Start: s.Start, End: s.End}
	}
	return out
}

// filmTitle is what a job's video is called, for the proofreader to judge its words by: the title the site gave it, or a local video's file name.
func filmTitle(job core.Job) string {
	if title := strings.TrimSpace(job.Title); title != "" {
		return title
	}
	if job.VideoPath == "" {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(job.VideoPath), filepath.Ext(job.VideoPath))
}

// Downloaded reports whether a link's job already downloaded its video to path in an earlier run: the file is there, holds something, and no partial download lies beside it, which would mean it never merged and Download has to fetch it again.
func Downloaded(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	return !holdsDownloadLeftovers(filepath.Dir(path))
}

// translateMissing translates the lines of original whose translation in earlier is empty and keeps the others, returning every line's translation and how many are still untranslated. Only the missing lines go to p.Translate, so a film whose translation failed for a few batches does not pay for the rest again.
func translateMissing(ctx context.Context, p Pipeline, original, earlier []core.Segment, language string) ([]core.Segment, int, error) {
	out := slices.Clone(earlier)
	var missing []core.Segment
	var at []int
	for i, s := range earlier {
		if strings.TrimSpace(s.Text) == "" {
			missing = append(missing, original[i])
			at = append(at, i)
		}
	}
	if len(missing) == 0 {
		return out, 0, nil
	}
	got, err := p.Translate(ctx, missing, language)
	left := 0
	var partial *UntranslatedError
	if errors.As(err, &partial) && len(got) == len(missing) {
		left, err = partial.Lines, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if len(got) != len(missing) {
		return nil, 0, fmt.Errorf("%d translated lines for %d untranslated lines", len(got), len(missing))
	}
	for k, i := range at {
		out[i].Text = got[k].Text
	}
	return out, left, nil
}
