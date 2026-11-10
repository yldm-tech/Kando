package films

import (
	"time"

	"kando/apps/kando/sidebar"

	"github.com/egoist/mygo/ui"
)

// App is the shelf the films page reads and the clicks it reports.
type App interface {
	LibraryDir() string
	MakingLibrary() bool
	SaveErr() string
	Page() string
	OpenKey() string
	SetOpenKey(string)
	// Source is the link being typed into the top bar.
	Source() *string
	PlaybackOpen() bool
	StopPlayback()
	PickLibrary()
	// OpenSites opens the 支持的网站 sheet.
	OpenSites()
	// BeginNew opens the new-library sheet.
	BeginNew()
	CancelMaking()
	LibraryPlace() string
	PickLibraryParent()
	CreateLibrary()
	BindName(edit func(*string) bool) error
	// NewLibraryPath is where the new-library sheet would create the library, or why it cannot.
	NewLibraryPath() (string, error)
	// Start adds the link in the top bar to the queue, or says in the notice why it cannot.
	Start()
	// PickVideos asks for local videos and adds them to the queue.
	PickVideos()
	// PickVideosShortcut is the shortcut bound to PickVideos, which the tooltip of 本地视频 writes as the platform does (⌘O on macOS, Ctrl+O elsewhere).
	PickVideosShortcut() (ui.Modifiers, ui.Key)
	// DropFiles adds the videos among files dropped on the page to the queue.
	DropFiles(paths []string)
	Poster(path string) ui.ImageSource
	// Thumbnail is the picture the film's site gives it, once it has been fetched, or nil.
	Thumbnail(item Film) ui.ImageSource
	// Duration is how long the video at path plays, 0 until it is known.
	Duration(path string) time.Duration
	// Quote is the first bilingual line of a film's subtitles, for the hero: the translation and the original. Both are empty until the subtitles have been read.
	Quote(item Film) (translation, original string)
	Open(item Film)
	// OpenAt opens a film and starts its picture at at.
	OpenAt(item Film, at time.Duration)
	Reveal(path string)
	// Reprocess makes the subtitles of a done film again.
	Reprocess(item Film)
	// Remove drops a job's card; its video, if any, stays in the library.
	Remove(id string)
	Retry(id string)
	// FailureHint says, for a failed job, what went wrong and the settings section that fixes it ("" when none does).
	FailureHint(item Film) (reason, section string)
	// FailureRemedy is what a failed card says and offers when the app knows why its download failed; false shows FailureHint instead. RunRemedy does the action of the remedy's button the user clicked.
	FailureRemedy(item Film) (Remedy, bool)
	RunRemedy(item Film, action string)
	// LinkFocus asks the link field for the keyboard focus once, as 换个链接 does; the field sets it back to false when it has taken the focus.
	LinkFocus() *bool
	JobFilm(id string) (Film, bool)
	// SourceFilter is the sidebar source the page shows, sidebar.SourceAll or another sidebar.Source value.
	SourceFilter() string
	// Enqueue adds a job for each source, a link or a local video's file:// URL, to the open library in the language the settings name, and runs them one after another. The error is why a source was not added.
	Enqueue(sources ...string) error
	// Cancel stops job id if it is waiting or running. Its card stays, marked 已取消. Enqueue, Cancel and RunningJob are the contract of the job queue; queue.go in the main package says how much of it runs today. Job control is per job: a page cancels the job it shows, never "whatever runs".
	Cancel(id string)
	// RunningJob is the ID of the job being processed now, or "" when none is. A job waiting in the queue is not running; its Film.Stage is core.StageQueued.
	RunningJob() string
	// QueueFilms is the films of 处理中 in the shelf's order, newest first (Queue): the open library's that are waiting, running, failed or were cancelled, and those the queue runs or will run for another library, with Film.Library naming it.
	QueueFilms() []Film
	// OpenSettings shows the settings at section, one of the settings package's Section values.
	OpenSettings(section string)
	// ResumeAt is where the video at path stopped playing last time, or 0.
	ResumeAt(path string) time.Duration
	// Notice is the message bar under the top bar.
	Notice() *Notice
	// QueueExpanded is whether 处理中 shows every card rather than the first QueueShown.
	QueueExpanded() *bool
	// AskRemove asks to confirm moving a film's folder to the Trash.
	AskRemove(item Film)
	// Removing is the film AskRemove asked about, whether its confirmation is open, and whether its whole folder goes to the Trash rather than only its video and subtitles.
	Removing() (open *bool, item Film, folder bool)
	// TrashFilm moves a film's folder to the Trash and takes it off the shelf.
	TrashFilm(item Film)
}

// Notice is the message bar under the top bar: why a link or a file was not added, or that it was.
type Notice struct {
	// Kind is "warning" or "success", as breeze.InfoBar takes it.
	Kind string
	Text string
	// Section is the settings section 打开设置 opens; empty shows no button.
	Section string
	// Film is the job ID of the film the bar's 打开 opens, such as the film a link just added is already in the library as; empty shows no button.
	Film string
	Open bool
}

// The library's content, as the prototype lays it out: inset PagePadX from the content column's sides, starting contentTop down, under the top bar that it scrolls beneath, with pagePadBottom under its last part.
const (
	PagePadX      float32 = 24
	contentTop    float32 = 64
	pagePadBottom float32 = 32
)

// Show draws the library page, or the open film over it. showFilm draws the open film; the page stays here so the shelf does not import the player.
func Show(c *ui.Context, t *ui.Theme, a App, items []Film, showFilm func(*ui.Context, *ui.Theme, Film)) {
	if key := a.OpenKey(); key != "" {
		if item, ok := Find(items, key); ok {
			showFilm(c, t, item)
			return
		}
		if item, ok := a.JobFilm(key); ok {
			showFilm(c, t, item)
			return
		}
		a.SetOpenKey("")
	}
	if a.OpenKey() == "" && a.PlaybackOpen() {
		a.StopPlayback()
	}
	source := a.SourceFilter()
	if a.Page() == sidebar.PageYouTube {
		source = sidebar.SourceYouTube
	}
	page := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
	if files := page.DroppedFiles(); len(files) > 0 {
		a.DropFiles(files)
	}
	dragging := page.FileDragOver()
	page.Children(func() {
		// The page is one scroll view from the content column's top, as the prototype's is: the top bar lies over it, and what is under the bar scrolls beneath it and fades into it.
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).WidthPercent(100).Padding(contentTop, PagePadX, pagePadBottom, PagePadX).Children(func() {
				notice(c, a)
				if a.LibraryDir() == "" {
					noLibrary(c, a)
				} else {
					shelf(c, t, a, items, source)
				}
			})
		})
		topBar(c, t, a, source)
		if dragging {
			dropZone(c, t)
		}
	})
	newLibrary(c, t, a)
	removeDialog(c, a)
}

// shelf is what the library shows under the message bar: the hero, 处理中 and the done grid of the films from source, or what to do when there are none. 处理中 also holds the jobs the queue runs for other libraries, so a library with no film of its own shows them above what to do.
func shelf(c *ui.Context, t *ui.Theme, a App, items []Film, source string) {
	queue := BySource(a.QueueFilms(), source)
	if len(items) == 0 {
		if len(queue) > 0 {
			queueSection(c, t, a, QueueOrder(queue))
		}
		emptyLibrary(c, a)
		return
	}
	var done []Film
	for _, item := range BySource(items, source) {
		if !InQueue(item) {
			done = append(done, item)
		}
	}
	if len(queue) == 0 && len(done) == 0 {
		emptySource(c, t)
		return
	}
	if source == sidebar.SourceAll {
		if item, ok := HeroFilm(done); ok {
			hero(c, a, item)
		}
	}
	if len(queue) > 0 {
		queueSection(c, t, a, QueueOrder(queue))
	}
	if len(done) > 0 {
		grid(c, t, a, done, source)
	}
}
