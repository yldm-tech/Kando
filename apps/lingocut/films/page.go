package films

import (
	"time"

	"lingocut/apps/lingocut/sidebar"

	"github.com/egoist/mygo/plugins/glass"
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
	// DropFiles adds the videos among files dropped on the page to the queue.
	DropFiles(paths []string)
	Poster(path string) ui.ImageSource
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
	JobFilm(id string) (Film, bool)
	// SourceFilter is the sidebar source the page shows, sidebar.SourceAll or another sidebar.Source value.
	SourceFilter() string
	// Enqueue adds a job for each source, a link or a local video's file:// URL, to the open library in the language the settings name, and runs them one after another. The error is why a source was not added.
	Enqueue(sources ...string) error
	// Cancel stops job id if it is waiting or running. Its card stays, marked 已取消. Enqueue, Cancel and RunningJob are the contract of the job queue; queue.go in the main package says how much of it runs today. Job control is per job: a page cancels the job it shows, never "whatever runs".
	Cancel(id string)
	// RunningJob is the ID of the job being processed now, or "" when none is. A job waiting in the queue is not running; its Film.Stage is core.StageQueued.
	RunningJob() string
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
	Open    bool
}

// pagePadBottom is the room under the library page, as under the other pages of the content column.
const pagePadBottom = 28

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
		// The header bar spans the content column from the window's top, as a unified toolbar spans the pane beside a sidebar; what is under it keeps the page's padding at its sides and bottom.
		topBar(c, t, a, source)
		ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Padding(0, sidebar.PagePadX, pagePadBottom, sidebar.PagePadX).Children(func() {
			notice(c, a)
			if a.LibraryDir() == "" {
				noLibrary(c, a)
			} else {
				shelf(c, t, a, items, source)
			}
		})
		if dragging {
			dropZone(c, t)
		}
	})
	newLibrary(c, t, a)
	removeDialog(c, a)
}

// shelf is the scrolling part of the page: the hero, 处理中 and the done grid of the films from source.
func shelf(c *ui.Context, t *ui.Theme, a App, items []Film, source string) {
	shown := BySource(items, source)
	if len(items) == 0 {
		emptyLibrary(c, a)
		return
	}
	if len(shown) == 0 {
		emptySource(c, t)
		return
	}
	var queue, done []Film
	for _, item := range shown {
		if InQueue(item) {
			queue = append(queue, item)
		} else {
			done = append(done, item)
		}
	}
	// The shelf reaches out over the page's padding at either side, its content padded back in, so that the scroll view does not cut off the hero's and the cards' shadows at the content's edges, and its scroll edge and scroll bar span the page.
	ui.Box(c).Grow(1).MinHeight(0).Margin(0, -sidebar.PagePadX).AlignItems(ui.Stretch).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).WidthPercent(100).Gap(26).Padding(14, sidebar.PagePadX, 32, sidebar.PagePadX).Children(func() {
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
			})
		})
		// The top of the shelf fades into the page as it scrolls under the top bar, as macOS fades content under a bar of controls.
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(14).PassThrough().Material(glass.ScrollEdge{Background: t.Background})
	})
}
