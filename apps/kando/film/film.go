package film

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/kando/live"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// App is the open film the player page reads and the clicks it reports. The page reads it inside its bindings through live.Func and calls its actions through live.Do and live.On, so what they change shows at once (package live).
type App interface {
	live.State
	Corrector
	Glossary
	Reviewer
	Progresser
	Referencer
	Notebook
	Streamer
	Explainer
	Recapper
	OpenKey() string
	SetOpenKey(string)
	StopPlayback()
	Reveal(path string)
	// Cancel stops job id if it is waiting or running, as films.App.Cancel does; the page cancels the film it shows.
	Cancel(id string)
	// RunningJob is the ID of the job being processed now, or "", as films.App.RunningJob.
	RunningJob() string
	// ProcessFilm runs the open film, a video found in the library's folder that no job has processed, through the pipeline, keeping its picture: at once for one without subtitles, once confirmed for one whose subtitles found beside it it would replace.
	ProcessFilm()
	Remove(id string)
	RetryOpen(id string)
	// ReprocessFilm asks to process the open film, a done one, again entirely, as the library's 重新处理 does, once the user confirms; the film stays open and follows its job.
	ReprocessFilm()
	// TranslateFilmAgain and TranslateFilmMissing translate the open film, a done one, again without downloading or transcribing it, as the library's 重新翻译为 and 补译未译的句子 do: every line into language, once the user confirms replacing a translation in another language, or only those its translation left untranslated. The film stays open and follows its job. TargetLanguage is the target language the settings name, which 重新翻译为 ▸ offers first.
	TranslateFilmAgain(language string)
	TranslateFilmMissing()
	TargetLanguage() string
	// SetFilmSpeechLanguage names code, whisper's code for a language, as the one spoken in the open film, a done one with a job, which Process Again transcribes it in from then on rather than the one the settings name; ReprocessFilm transcribes it again, once the user confirms.
	SetFilmSpeechLanguage(code string)
	// StageLabel names a job's stage in the language the interface shows under cx, as the library does.
	StageLabel(cx *ns.Scope, stage core.Stage) string
	// JobStatus is what the open film's job is doing and how far it has gone, as its card in the library says, in the language the interface shows under cx; "" while no job of it waits or runs.
	JobStatus(cx *ns.Scope) string
	// RedoFailure is why processing or translating the open film, a done one, again failed, in the language the interface shows under cx, and the settings section that fixes it, "" for none; "" unless its last redo failed. The film keeps the subtitles it had.
	RedoFailure(cx *ns.Scope) (text, section string)
	// FillsWindow reports whether the film takes the whole window, its title bar lying under the window's traffic lights, rather than the content column beside the sidebar.
	FillsWindow() bool
	// Play is the picture open now, nil until the system's player has loaded it and while there is none. PlayPath is the video opening or open, "" without one; PlayErr is why it would not play, an error ID ErrText names, or "".
	Play() Player
	PlayPath() string
	PlayErr() string
	// ErrText is what the page says for an error ID of PlayErr or Subs.Err, in the language the interface shows under cx.
	ErrText(cx *ns.Scope, id string) string
	// PosterFile is the file of the film's poster for the video at path, which shows while its picture opens, or "" without one.
	PosterFile(path string) string
	// PlayHead is where the picture is, or where the one opening will start; PlayLength is the video's length, or 0 before it is known.
	PlayHead() time.Duration
	PlayLength() time.Duration
	// Video is what the system's player plays and how, which the page's core.Video binds (Stage).
	Video() Video
	// VideoRef is the system player's ref, through which the app seeks it.
	VideoRef() *ns.Ref
	// VideoLoaded, VideoProgress, VideoEnded and VideoFailed are what the system's player reports. VideoProgress reports whether the app's state changed in a way the bindings read, such as a looped line going back to its start, which a tick of the clock alone does not.
	VideoLoaded(info ns.VideoInfo)
	VideoProgress(position, duration time.Duration) (changed bool)
	VideoEnded()
	VideoFailed(err error)
	Subs() Subs
	// FilmSide is the side panel's tab: "" for 字幕, "clips" for 片段, "terms" for 术语, SideNotes for 笔记, "info" for 信息. 样式 is shown through StyleView.
	FilmSide() string
	SetFilmSide(string)
	Seek(at time.Duration)
	// Jumps counts the user's moves of the picture: a seek, a skip, a move by line. The subtitle list follows the playing line again after one.
	Jumps() int
	// Skip moves the picture by d from where it is and says so on the picture.
	Skip(d time.Duration)
	// PrevLine, NextLine and ReplayLine play from the start of the line before, the line after or the line playing; ToggleLoopLine plays the line playing over and over, or stops, and LoopLine reports whether it does. Each says so on the picture.
	PrevLine()
	NextLine()
	ReplayLine()
	ToggleLoopLine()
	LoopLine() bool
	// LoopSpan is the span looped, in the subtitles' time, end 0 for the video's end, ok false while none is; LoopThrough has the loop run through cue i too, from its start or to its end, as a click with Shift on a line of the list does.
	LoopSpan() (start, end time.Duration, ok bool)
	LoopThrough(i int)
	// PauseLines reports whether the film pauses at the end of each line, for shadowing or dictation; TogglePauseLines turns it on or off and says so on the picture.
	PauseLines() bool
	TogglePauseLines()
	Cue() *Look
	// LookChanged keeps the look after the style view changed it.
	LookChanged()
	// SubMode is which lines of a bilingual cue show: SubBilingual, SubTranslation, SubOriginal or SubOff.
	SubMode() string
	SetSubMode(mode string)
	// SubOrder is which line of a bilingual cue goes on top: SubTranslationFirst or SubOriginalFirst.
	SubOrder() string
	SetSubOrder(order string)
	// Peek is the index of the cue the picture shows both lines of, whatever SubMode, or -1.
	Peek() int
	// HeldLine is the index of the cue the film is held at the end of, by Pause After Each Line or a looped line's stop, or -1. The picture, the subtitle list and the held-line card show it, though the playhead has stopped just past its end, where the next cue may have started (playingCue).
	HeldLine() int
	// Untranslated is UntranslatedDim or UntranslatedBlank: what SubTranslation shows for a cue that has no translation.
	Untranslated() string
	// YieldSubs moves the subtitles up while the player's controls show.
	YieldSubs() bool
	// Delay shifts the subtitles against the picture: a positive delay shows each line later.
	Delay() time.Duration
	SetDelay(d time.Duration)
	// SubLift is how far the open film's subtitles were moved up over its picture from the place their look puts them (Look.High), as a share of the picture's height, negative for down, so they keep their place as the window changes size; 0 for a film whose subtitles were never moved, which show at that place. It is for moving them off a film's own burned-in subtitles or other text. SetSubLift moves them there for this film alone and keeps it with the film, 0 putting them back.
	SubLift() float32
	SetSubLift(lift float32)
	// SetSubRoom tells the app where the lines on the picture show and how far a drag could take them, as the picture lays them out, so the keys that move them (NudgeLift) start from where they show and stop where a drag stops.
	SetSubRoom(room SubRoom)
	// SubTip reports whether the tip that the subtitles can be dragged shows over them, as it does once, the first time a film plays with subtitles.
	SubTip() bool
	// Panel reports whether the side panel is on, and StyleView whether it shows the subtitle style in place of its tabs. A film without a picture shows the panel whether it is on or not.
	Panel() bool
	TogglePanel()
	StyleView() bool
	SetStyleView(on bool)
	// ModeMenu is whether the subtitle mode menu is open.
	ModeMenu() *bool
	// Rate is how fast films play, one of Rates, 1 as recorded; SetRate plays them at rate and says so on the picture. SpeedMenu is whether the speed menu is open.
	Rate() float64
	SetRate(rate float64)
	SpeedMenu() *bool
	// SpeedKeys are the keys that step the speed down and up on os, as the shortcut table has them under the preset chosen in Settings › 快捷键, for the speed button's tooltip: ⇧, and ⇧. with Kando's own keys, [ and ] under mpv's.
	SpeedKeys(os ns.OS) (slower, faster string)
	// Search is the query of the subtitle list; SearchFocus asks the list's search field for the keyboard focus once.
	Search() *string
	SearchFocus() *bool
	// SearchFocused is set while the search field holds the keyboard focus, so the app's Escape empties the field before it closes anything.
	SearchFocused() *bool
	// OSD is the message shown over the picture, "" for none; the app takes it away after a moment. Flash shows text there.
	OSD() string
	Flash(text string)
	LibraryDir() string
	LibraryPath() string
	// ClipPoster is a frame of the film at start, JPEG, for a 片段's tile, once it has been read.
	ClipPoster(start time.Duration) ([]byte, bool)
	// ImportChapters is the setting 导入平台章节: whether a film whose site marks chapters is cut at them unless the user chose otherwise for it (CutsAtSite). SetFilmChapterUse cuts the open film at its site's chapters or by time, which the film keeps.
	ImportChapters() bool
	SetFilmChapterUse(use core.ChapterUse)
	// Info is the open film's folder and files, read off the engine's goroutine when it opened.
	Info() Info
	// FilmUpload is the upload of the open film; BeginFilmUpload starts it.
	FilmUpload() Upload
	BeginFilmUpload()
	// CancelUpload stops the upload running now.
	CancelUpload()
	// UploadNote is whether the upload capsule shows, after the upload button was clicked and until it is closed.
	UploadNote() *bool
	// OpenSettings shows the settings at section, one of the settings package's Section values.
	OpenSettings(section string)
	// ToggleFullScreen enters or leaves full screen.
	ToggleFullScreen()
	// Volume is how loud films play, from 0 to 1, and Muted whether they are silenced, keeping Volume. SetVolume sets it and unmutes; ToggleMute mutes or unmutes. Both say so on the picture.
	Volume() float64
	Muted() bool
	SetVolume(volume float64)
	ToggleMute()
	// FeedbackSubtitles opens the 反馈问题 dialog for this film's subtitles.
	FeedbackSubtitles()
	// MoveOpenFilmMenu is the submenu 移到资料库 of ⋯ for the open film: the other libraries Kando knows, each moving the film there when chosen, which closes it; none when there is no other library.
	MoveOpenFilmMenu(cx *ns.Scope) []nsdesktop.MenuItem
	// StoredOpenMenu is the items of ⋯ that move the open film's video to a storage location or back to the library's folder; none when it cannot move or there is no location.
	StoredOpenMenu(cx *ns.Scope) []nsdesktop.MenuItem
	// ExplainLine has the AI service explain cue i, its meaning, key words and grammar, in the side panel (SideExplain), and holds the film.
	ExplainLine(i int)
	// LineSaved reports whether cue i is among the lines the learner saved; ToggleSaveLine saves it, or removes it from them, and says which on the picture.
	LineSaved(i int) bool
	ToggleSaveLine(i int)
	// SavedLines is the indexes of the cues saved, in order. SavedOnly is whether the subtitle list shows them alone.
	SavedLines() []int
	SavedOnly() *bool
	// Lookup is the word looked up on the picture, the zero Lookup while none is. LookUpWord looks up word, clicked in the original of cue i on the picture; CloseLookup closes its card; ToggleLookupWord puts the word in the vocabulary, or takes it out; ExplainLookupLine has the AI service explain the line it was clicked in; LookupSettings opens the AI service's settings, for a lookup no AI service could answer.
	Lookup() Lookup
	LookUpWord(i int, word string)
	CloseLookup()
	ToggleLookupWord()
	ExplainLookupLine()
	LookupSettings()
}

// Lookup is the word looked up on the picture, as its card shows it: the word, its reading and what it means in Line, the line it was clicked in; Term, the film's glossary's term for it, "source → target", "" when the glossary has none; Busy while the AI service is asked, Failed when no meaning came, with Say saying why, NeedsAI when the AI service's settings are what helps; Saved once the word is in the vocabulary.
type Lookup struct {
	Open                         bool
	Word, Reading, Meaning, Line string
	Term                         string
	Busy, Failed, NeedsAI, Saved bool
	Say                          func(*ns.Scope) string
}

// Video is what the system's player plays and how. Source is "" while there is nothing to play, as while a video the system cannot open is copied into one it can (Remux).
type Video struct {
	Source string
	Paused bool
	Muted  bool
	// Volume runs from 0 to 1; a volume of 0 plays muted, as the system's player plays 0 at full volume.
	Volume float32
	Rate   float32
}

// Upload is the manual upload of a film that already has a local video to the storage set in 账号.
type Upload struct {
	Show    bool
	Enabled bool
	Label   string
	Detail  string
	// Status is UploadBusy, UploadDone, UploadFailed, UploadCanceled, UploadBlocked, or "" with nothing to report.
	Status string
	// Progress is how much of a busy upload is sent, from 0 to 1.
	Progress float64
	// Canceling is whether a busy upload was asked to stop and waits for the storage's client to give up, when its cancel button is off.
	Canceling bool
	// Fix is the label of the button that opens 账号 to clear a blocked upload, or one that failed because of the storage filled in there, as 去设置; "" when none helps.
	Fix string
}

// Upload statuses, which the upload capsule shows with a dot of its own color.
const (
	UploadBusy     = "busy"
	UploadDone     = "done"
	UploadFailed   = "failed"
	UploadCanceled = "canceled"
	UploadBlocked  = "blocked"
)

// Player is the picture currently open.
type Player interface {
	Playing() bool
	Toggle()
	Head() time.Duration
	Length() time.Duration
}

// Subs is the subtitle file open on the film page. Err is an error ID, which App.ErrText names.
type Subs struct {
	Key   string
	Ready bool
	Err   string
	Cues  []core.Segment
}

// Info is what the 信息 tab lists of the open film. Chapters are the chapters its site marks, which its job kept beside the video (media.ReadChapters), and ChapterUse whether the user chose to cut it at them or by time, "" to follow the setting; a film without chapters has none.
type Info struct {
	Folder     string
	Files      []File
	Chapters   []core.Chapter
	ChapterUse core.ChapterUse
}

// File is a file in a film's folder.
type File struct {
	Name string
	Size int64
}

// Item is the open film as the player shows it, which the app makes from the library's film.
type Item struct {
	Key, Title, Name, Source, Detail string
	Stage                            core.Stage
	JobID, VideoPath, SubtitlePath   string
	// Language is what the job translated into, such as 简体中文; empty for a video no job has processed.
	Language string
	// Spoken is the language the speech recognition heard in the video, as whisper's code (ja, en); empty when it is not known.
	Spoken string
	// SpeechLanguage is the language the user said the video is spoken in, as whisper's code, which Process Again transcribes it in; empty when the user named none.
	SpeechLanguage string
	// Untranslated is how many lines of a done film's subtitles the translation left untranslated.
	Untranslated int
	// TranslationErr is why a done film translated none of its lines, its subtitles being the original lines alone; empty for one that was translated.
	TranslationErr string
	// SameLanguage is true for a film heard in the language its subtitles are for, which was not translated: its subtitles are its own lines alone, which every mode but 关闭 shows as 原文 does (ownMode).
	SameLanguage bool
	// PlatformSubtitles is true for a film whose original lines are its site's subtitles, checked against the speech, and LocalLines how many of their lines the speech recognition replaced.
	PlatformSubtitles bool
	LocalLines        int
	// SubtitlesOnly is true for a film that keeps no video, which streams from its site when watched, and VideoComing while the download of its video waits or runs (DownloadVideo).
	SubtitlesOnly bool
	VideoComing   bool
}

// title is how the film is named in the title bar.
func title(item Item) string {
	if item.Name != "" {
		return item.Name
	}
	return item.Title
}

// SplitCue is a bilingual cue's two lines: the translation over the original, or the original alone in a cue that has no translation.
func SplitCue(text string) (translation, original string) { return splitCue(text) }

// LineText is a cue as Copy Line puts it on the clipboard: its lines as the subtitle list shows them, the translation over the original, or the one line of a cue that has no translation.
func LineText(text string) string {
	translation, original := splitCue(text)
	if translation == "" {
		return original
	}
	if original == "" {
		return translation
	}
	return translation + "\n" + original
}

func splitCue(text string) (translation, original string) {
	text = strings.TrimSpace(text)
	parts := strings.SplitN(text, "\n", 2)
	if len(parts) == 1 {
		return "", strings.TrimSpace(parts[0])
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func cueClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	h := ms / 3_600_000
	m := ms / 60_000 % 60
	s := ms / 1000 % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// cueClockTenths is cueClock with the tenth of a second, as the subtitle list shows a line's start ("0:10.4").
func cueClockTenths(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%s.%d", cueClock(d), d.Milliseconds()/100%10)
}

// CueAt is the subtitle line covering at. A zero end means the line stays until the next one.
func CueAt(cues []core.Segment, at time.Duration) (core.Segment, bool) {
	i := cueIndex(cues, at)
	if i < 0 {
		return core.Segment{}, false
	}
	return cues[i], true
}

// playingCue is the index of the cue the picture shows at the playhead, as cueIndex finds it, or the one the film is held at the end of (App.HeldLine), or -1.
func (s *screen) playingCue(list []core.Segment) int {
	at := s.playhead() - s.a.Delay()
	if i := s.a.HeldLine(); i >= 0 && i < len(list) {
		return i
	}
	return cueIndex(list, at)
}

// cueIndex is the index of the line CueAt finds, or -1.
func cueIndex(cues []core.Segment, at time.Duration) int {
	found := -1
	for i, cue := range cues {
		if cue.Start > at {
			break
		}
		if cue.End > at || cue.End == 0 {
			found = i
		}
	}
	return found
}

// NextCueChange is how far after at, in the subtitles' time, a line starts or ends, which changes what CueAt finds: the next moment the picture's subtitles change. ok is false when none does.
func NextCueChange(cues []core.Segment, at time.Duration) (time.Duration, bool) {
	next, ok := time.Duration(0), false
	consider := func(t time.Duration) {
		if t > at && (!ok || t-at < next) {
			next, ok = t-at, true
		}
	}
	for _, cue := range cues {
		consider(cue.Start)
		if cue.End > 0 {
			consider(cue.End)
		}
		if cue.Start > at && ok && cue.Start-at >= next {
			break
		}
	}
	return next, ok
}

// CueNear is where at stands among the subtitle lines, by index: cur is the line covering at, as CueAt finds it, or -1 between lines; next is the first line starting after at; prev is the last line starting before cur does, or, between lines, the last one started by at. Each is -1 when there is none. The lines are in the order they start, as cueIndex takes them.
func CueNear(cues []core.Segment, at time.Duration) (prev, cur, next int) {
	cur = cueIndex(cues, at)
	prev, next = -1, -1
	for i, cue := range cues {
		if cue.Start > at {
			next = i
			break
		}
	}
	for i := len(cues) - 1; i >= 0; i-- {
		if cur >= 0 && cues[i].Start < cues[cur].Start || cur < 0 && cues[i].Start <= at {
			prev = i
			break
		}
	}
	return prev, cur, next
}

func seconds(v float64) time.Duration { return time.Duration(v * float64(time.Second)) }

// shorten cuts s to n characters, the last an ellipsis, as the library shortens a title.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// displayPath is path with the home folder written ~, as the library shows paths.
func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if prefix := home + string(filepath.Separator); strings.HasPrefix(path, prefix) {
		return "~" + string(filepath.Separator) + path[len(prefix):]
	}
	return path
}
