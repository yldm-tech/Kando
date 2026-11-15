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
)

// App is the open film the player page reads and the clicks it reports. The page reads it inside its bindings through live.Func and calls its actions through live.Do and live.On, so what they change shows at once (package live).
type App interface {
	live.State
	OpenKey() string
	SetOpenKey(string)
	StopPlayback()
	Reveal(path string)
	// Cancel stops job id if it is waiting or running, as films.App.Cancel does; the page cancels the film it shows.
	Cancel(id string)
	// RunningJob is the ID of the job being processed now, or "", as films.App.RunningJob.
	RunningJob() string
	// ProcessFilm runs the open film, a video no job has processed, through the pipeline, keeping its picture.
	ProcessFilm()
	Remove(id string)
	RetryOpen(id string)
	// TranslateFilmAgain and TranslateFilmMissing translate the open film, a done one, again without downloading or transcribing it, as the library's 重新翻译 and 补译未译的句子 do: every line, or only those its translation left untranslated. The film stays open and follows its job.
	TranslateFilmAgain()
	TranslateFilmMissing()
	// StageLabel names a job's stage in the language the interface shows under cx, as the library does.
	StageLabel(cx *ns.Scope, stage core.Stage) string
	// FillsWindow reports whether the film takes the whole window, its title bar lying under the window's traffic lights, rather than the content column beside the sidebar.
	FillsWindow() bool
	// Play is the picture open now, nil until the system's player has loaded it and while there is none. PlayPath is the video opening or open, "" without one; PlayErr is why it would not play, an error ID ErrText names, or "".
	Play() Player
	PlayPath() string
	PlayErr() string
	// ErrText is what the page says for an error ID of PlayErr or Subs.Err, in the language the interface shows under cx.
	ErrText(cx *ns.Scope, id string) string
	// Poster is the file of the film's poster for the video at path, which shows while its picture opens, or "" without one.
	Poster(path string) string
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
	// FilmSide is the side panel's tab: "" for 字幕, "clips" for 片段, "info" for 信息. 样式 is shown through StyleView.
	FilmSide() string
	SetFilmSide(string)
	Seek(at time.Duration)
	// Skip moves the picture by d from where it is and says so on the picture.
	Skip(d time.Duration)
	// PrevLine, NextLine and ReplayLine play from the start of the line before, the line after or the line playing; ToggleLoopLine plays the line playing over and over, or stops, and LoopLine reports whether it does. Each says so on the picture.
	PrevLine()
	NextLine()
	ReplayLine()
	ToggleLoopLine()
	LoopLine() bool
	Cue() *Look
	// LookChanged keeps the look after the style view changed it.
	LookChanged()
	// SubMode is which lines of a bilingual cue show: SubBilingual, SubTranslation, SubOriginal or SubOff.
	SubMode() string
	SetSubMode(mode string)
	// SubOrder is which line of a bilingual cue goes on top: SubTranslationFirst or SubOriginalFirst.
	SubOrder() string
	SetSubOrder(order string)
	// Untranslated is UntranslatedDim or UntranslatedBlank: what SubTranslation shows for a cue that has no translation.
	Untranslated() string
	// YieldSubs moves the subtitles up while the player's controls show.
	YieldSubs() bool
	// Delay shifts the subtitles against the picture: a positive delay shows each line later.
	Delay() time.Duration
	SetDelay(d time.Duration)
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
	// Search is the query of the subtitle list; SearchFocus asks the list's search field for the keyboard focus once.
	Search() *string
	SearchFocus() *bool
	// OSD is the message shown over the picture, "" for none; the app takes it away after a moment.
	OSD() string
	LibraryDir() string
	LibraryPath() string
	// ClipPoster is a frame of the film at start, JPEG, for a 片段's tile, once it has been read.
	ClipPoster(start time.Duration) ([]byte, bool)
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
	// Fix is the label of the button that opens 账号 to clear a blocked upload, as 去登录; "" when none helps.
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

// Info is what the 信息 tab lists of the open film.
type Info struct {
	Folder string
	Files  []File
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
	// Untranslated is how many lines of a done film's subtitles the translation left untranslated.
	Untranslated int
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

// maxCrop is the most of a picture's width or height PictureFit cuts off to fill the stage: enough for a 16:9 or 1.85:1 film in a 16:10 window, not for a 2.39:1 film or a 4:3 one, which lose too much of their edges and show whole with bars instead.
const maxCrop = 0.15

// PictureFit fills the stage, stageW×stageH, with a picture of videoW×videoH, as the prototype does: the title bar and the transport float over the picture, with no black bars under them. A picture whose shape is far from the stage's would lose more than maxCrop of it, so it shows whole (core.Contain) instead.
func PictureFit(stageW, stageH, videoW, videoH ns.Points) ns.ResizeMode {
	if stageW <= 0 || stageH <= 0 || videoW <= 0 || videoH <= 0 {
		return ns.Cover
	}
	picture, stage := videoW/videoH, stageW/stageH
	if 1-min(picture, stage)/max(picture, stage) > maxCrop {
		return ns.Contain
	}
	return ns.Cover
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
