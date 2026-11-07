package film

import (
	"fmt"
	"strings"
	"time"

	"kando/apps/kando/films"
	"kando/packages/core"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// App is the open film the player page reads and the clicks it reports.
type App interface {
	OpenKey() string
	SetOpenKey(string)
	StopPlayback()
	Reveal(path string)
	// Cancel stops job id if it is waiting or running, as films.App.Cancel does; the page cancels the film it shows.
	Cancel(id string)
	// RunningJob is the ID of the job being processed now, or "", as films.App.RunningJob.
	RunningJob() string
	StartFilm(item films.Film)
	Remove(id string)
	RetryOpen(id string)
	Play() Player
	PlayPath() string
	PlayErr() string
	// PlayHead is where the picture is, or where the one being opened will start; PlayLength is the video's length, or 0 before it is known.
	PlayHead() time.Duration
	PlayLength() time.Duration
	Subs() Subs
	SubList() *ui.ListState
	ClipList() *ui.ListState
	// FilmSide is the side panel's tab: "" for 字幕, "clips" for 片段, "info" for 信息.
	FilmSide() string
	SetFilmSide(string)
	Seek(at time.Duration)
	// Skip moves the picture by d from where it is and says so on the picture.
	Skip(d time.Duration)
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
	// Search is the query of the subtitle list; SearchFocus asks the list's search field for the keyboard focus once.
	Search() *string
	SearchFocus() *bool
	// Follow is the cue the subtitle list last scrolled to, so it scrolls only when the playing cue changes.
	Follow() *int
	// Scrub is the scrubber's value in seconds while held is true, as the pointer drags it.
	Scrub() (value *float64, held *bool)
	// OSD is the message shown over the picture and how much longer it shows; a new message starts showing at now.
	OSD(now time.Time) (string, time.Duration)
	// Pointer reports where the pointer is over the player at now; LastMove is when it last moved.
	Pointer(x, y float32, now time.Time)
	LastMove() time.Time
	LibraryDir() string
	// Info is the open film's folder and files, read off the UI thread when it opened.
	Info() Info
	UploadView(item films.Film) Upload
	BeginUpload(item films.Film)
	// UploadNote is whether the upload capsule shows, after the upload button was clicked and until it is closed.
	UploadNote() *bool
	// OpenSettings shows the settings at section, one of the settings package's Section values.
	OpenSettings(section string)
	// ToggleFullScreen enters or leaves full screen.
	ToggleFullScreen()
	// FeedbackSubtitles opens the 反馈问题 dialog for this film's subtitles.
	FeedbackSubtitles()
}

// Upload is the manual upload of a film that already has a local video to the storage set in 账号.
type Upload struct {
	Show    bool
	Enabled bool
	Label   string
	Detail  string
	// Status is UploadBusy, UploadDone, UploadFailed, UploadBlocked, or "" with nothing to report.
	Status string
	// Progress is how much of a busy upload is sent, from 0 to 1.
	Progress float64
	// Fix is the label of the button that opens 账号 to clear a blocked upload, as 去登录; "" when none helps.
	Fix string
}

// Upload statuses, which the upload capsule shows with a dot of its own color.
const (
	UploadBusy    = "busy"
	UploadDone    = "done"
	UploadFailed  = "failed"
	UploadBlocked = "blocked"
)

// Player is the picture currently open.
type Player interface {
	Widget() *ui.VideoPlayer
	Playing() bool
	Settle()
	Toggle()
	Head() time.Duration
	Length() time.Duration
}

// Subs is the subtitle file open on the film page.
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

type screen struct{ App }

// The player's measures, from the prototype (template 401-640).
const (
	// idleAfter is how long the pointer rests while the film plays before the controls fade.
	idleAfter = 2500 * time.Millisecond
	// panelWidth is the side panel's, and panelRoom the room the subtitles leave for it.
	panelWidth  = 340
	panelRoom   = 372
	panelMotion = 350 * time.Millisecond
)

// onGlass is the text over the player's glass surfaces, which stay dark over any picture.
var onGlass = ui.Hex("#FFFFFF")

// Show draws the open film over the whole window in dark, the player's theme, which stays dark in the light window. The picture fills the window, or the room left of the side panel while it shows; the title bar, the transport and the subtitles float over it.
func Show(c *ui.Context, dark *ui.Theme, a App, item films.Film) {
	s := screen{a}
	now := c.Now()
	root := ui.Themed(c, dark).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Background(ui.Hex("#000000")).Clip()
	if x, y, over := root.PointerPosition(); over {
		s.Pointer(x, y, now)
	}
	shown := s.chromeShown(c, now)
	target := float32(0)
	if shown {
		target = 1
	}
	fade := root.Animate("chrome", target, 350*time.Millisecond)
	panelTarget := float32(0)
	if s.panelShown() {
		panelTarget = panelRoom
	}
	panelSpace := root.Animate("film-panel-room", panelTarget, panelMotion)
	root.Children(func() {
		s.stage(c, dark, item, shown, panelSpace)
		s.gradients(c, fade)
		s.bigPlay(c, panelSpace)
		s.subtitles(c, shown, panelSpace)
		s.osd(c, now)
		s.uploadCapsule(c, item, panelSpace)
		s.topBar(c, item, fade, shown)
		s.transport(c, dark, fade, shown, panelSpace)
		if panelSpace > 0 || panelTarget > 0 {
			s.panel(c, dark, item, panelSpace)
		}
	})
}

// panelShown reports whether the side panel shows: when it is on, and always while there is no picture, as for a film still downloading, one whose video is gone or any film without ffmpeg. Such a film has no transport, whose toggle is the only control that turns the panel back on, and the panel's subtitle list is all it has to show.
func (s screen) panelShown() bool {
	return s.Panel() || (s.Play() == nil && s.PlayPath() == "")
}

// chromeShown reports whether the title bar and the transport show: always while the film is held or a menu is open, and while playing until the pointer has rested for idleAfter.
func (s screen) chromeShown(c *ui.Context, now time.Time) bool {
	if s.Play() == nil || !s.Play().Playing() || *s.ModeMenu() {
		return true
	}
	rest := now.Sub(s.LastMove())
	if rest >= idleAfter {
		return false
	}
	c.After(idleAfter - rest)
	return true
}

// stage is the picture, or what stands for it while it opens or cannot play. A click on it starts or holds the film. The pointer hides over it with the controls. While the side panel shows, the stage is the room left of it, as the subtitles' and the transport's are, so the panel never hides part of the picture, such as a screencast's text.
func (s screen) stage(c *ui.Context, t *ui.Theme, item films.Film, shown bool, panelSpace float32) {
	right := panelSpace
	stage := ui.Box(c).Absolute().Left(0).Top(0).Right(right).Bottom(0).Role(ui.RoleGroup).Label("画面")
	if !shown {
		stage.Cursor(ui.CursorNone)
	}
	if s.Play() != nil && stage.Clicked() {
		s.togglePlay(c)
	}
	stage.Children(func() {
		switch {
		case s.Play() != nil:
			box := stage.LayoutBounds()
			if box.W <= 0 || box.H <= 0 {
				box.W, box.H = c.Size()
			}
			w, h := s.Play().Widget().Size()
			ui.Video(c, s.Play().Widget()).Fill().Fit(pictureFit(box.W, box.H, w, h))
			s.Play().Settle()
		case s.PlayErr() != "":
			stageNote(c, s.PlayErr(), t.Text)
		case s.PlayPath() != "":
			stageNote(c, "正在打开视频…", t.TextMuted)
		case item.VideoPath == "" && item.Detail != "":
			stageNote(c, item.Detail, t.TextMuted)
		}
	})
}

// maxCrop is the most of a picture's width or height pictureFit cuts off to fill the stage: enough for a 16:9 or 1.85:1 film in a 16:10 window, not for a 2.39:1 film or a 4:3 one, which lose too much of their edges and show whole with bars instead.
const maxCrop = 0.15

// pictureFit fills the stage, stageW×stageH, with a picture of videoW×videoH, as the prototype does: the title bar and the transport float over the picture, with no black bars under them. A picture whose shape is far from the stage's would lose more than maxCrop of it, so it shows whole (ui.Contain) instead.
func pictureFit(stageW, stageH float32, videoW, videoH int) ui.Fit {
	if stageW <= 0 || stageH <= 0 || videoW <= 0 || videoH <= 0 {
		return ui.Cover
	}
	picture, stage := float32(videoW)/float32(videoH), stageW/stageH
	if 1-min(picture, stage)/max(picture, stage) > maxCrop {
		return ui.Contain
	}
	return ui.Cover
}

// stageNote is a line in the middle of the picture, in the room the side panel leaves.
func stageNote(c *ui.Context, text string, color ui.Color) {
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(80, 24).Children(func() {
		ui.Text(c, text).TextColor(color).TextAlign(ui.Center).Wrap()
	})
}

// gradients darken the top and the bottom of the picture under the title bar and the transport, and fade with them.
func (s screen) gradients(c *ui.Context, fade float32) {
	top := ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(120).PassThrough().Opacity(fade)
	top.LinearGradient(ui.LinearGradient{From: ui.RGBA(0, 0, 0, 0.55), To: ui.RGBA(0, 0, 0, 0), Angle: 180})
	bottom := ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(220).PassThrough().Opacity(fade)
	bottom.LinearGradient(ui.LinearGradient{From: ui.RGBA(0, 0, 0, 0), To: ui.RGBA(0, 0, 0, 0.62), Angle: 180})
}

// togglePlay starts or holds the picture. The button is built after the video, so the frame that handles the click has already drawn a paused frame; the extra frame is what starts the clock.
func (s screen) togglePlay(c *ui.Context) {
	if s.Play() == nil {
		return
	}
	s.Play().Toggle()
	c.Invalidate()
}

// bigPlay is the round button over a held picture, in the middle of the room the side panel leaves.
func (s screen) bigPlay(c *ui.Context, panelSpace float32) {
	if s.Play() == nil || s.Play().Playing() {
		return
	}
	right := panelSpace
	ui.Row(c).Absolute().Left(0).Right(right).TopPercent(42).Justify(ui.Center).PassThrough().Children(func() {
		btn := ui.ButtonBase(c).Size(76, 76).Margin(-38, 0, 0, 0).Radius(38).AlignItems(ui.Center).Justify(ui.Center).Label("播放")
		glassFace(btn, ui.RGBA(255, 255, 255, 0.18), 20, 0.35, 38)
		if btn.Clicked() {
			s.togglePlay(c)
		}
		btn.Children(func() {
			ui.Icon(c, iconBigPlay).Size(28, 30).TextColor(onGlass)
		})
	})
}

// glassFace gives e, whose corners are radius, the player's glass: what is under it blurred by blur, tinted with fill, inside a half-point white ring of alpha ring. A material takes the place of an element's background, so the tint is painted over the blur, under e's children, rather than set as its background.
func glassFace(e ui.Element, fill ui.Color, blur, ring, radius float32) ui.Element {
	return e.Radius(radius).Material(glass.Blur{Radius: blur}).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, fill, radius)
	}).InsetShadow(0, 0, 0, 0.5, ui.RGBA(255, 255, 255, ring))
}

// title is how the film is named in the title bar.
func title(item films.Film) string {
	if item.Name != "" {
		return item.Name
	}
	return item.Title
}

// SplitCue is a bilingual cue's two lines: the translation over the original, or the original alone in a cue that has no translation.
func SplitCue(text string) (translation, original string) { return splitCue(text) }

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
