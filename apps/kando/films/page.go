package films

import (
	"slices"
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/sidebar"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
)

// App is the shelf the films page reads and the clicks it reports.
type App interface {
	live.State
	LibraryDir() string
	MakingLibrary() bool
	SaveErr() string
	Page() string
	// Films is the open library's films, the shelf's order (List): the jobs that belong on it, then the videos found in its folder.
	Films() []Film
	// Source is the link being typed into the top bar.
	Source() *string
	PickLibrary()
	// OpenSites opens the 支持的网站 sheet.
	OpenSites()
	// BeginNew opens the new-library sheet.
	BeginNew()
	CancelMaking()
	LibraryPlace() string
	PickLibraryParent()
	CreateLibrary()
	// NewLibraryName is the name typed into the new-library sheet; BindName edits it, and reports why it cannot.
	NewLibraryName() string
	BindName(edit func(*string) bool) error
	// NewLibraryPath is where the new-library sheet would create the library, or why it cannot.
	NewLibraryPath() (string, error)
	// Start adds the links in the top bar to the queue, or says in the notice why it cannot.
	Start()
	// AddList lists the videos of the playlist at source and asks before it adds them, as the message bar's Add Whole Playlist does (Notice.List).
	AddList(source string)
	// PickVideos asks for local videos and adds them to the queue.
	PickVideos()
	// PickVideosShortcut is the shortcut bound to PickVideos, in NSGo's form ("mod+o"), which the tooltip of 本地视频 writes as the platform does (⌘O on macOS, Ctrl+O elsewhere).
	PickVideosShortcut() string
	// DropFiles adds the videos among files dropped on the page to the queue.
	DropFiles(paths []string)
	// Poster is the cover of the video at path, the zero ImageSource until there is one.
	Poster(path string) core.ImageSource
	// Thumbnail is the picture the film's site gives it, once it has been fetched, or the zero ImageSource.
	Thumbnail(item Film) core.ImageSource
	// Duration is how long the video at path plays, 0 until it is known.
	Duration(path string) time.Duration
	// Quote is the first bilingual line of a film's subtitles, for the hero: the translation and the original. Both are empty until the subtitles have been read.
	Quote(item Film) (translation, original string)
	Open(item Film)
	// OpenAt opens a film and starts its picture at at.
	OpenAt(item Film, at time.Duration)
	Reveal(path string)
	// Reprocess makes the subtitles of a done film again, from its download, in the target language set now, or those of a video found in the folder that has none (生成字幕). Either keeps the film as it was if its job fails or is stopped.
	Reprocess(item Film)
	// AskReprocess asks to confirm Reprocess, which transcribes the film and pays for its translation again. Reprocessing is whether its confirmation is open, the film it asked about and the target language set now, which the film would be translated into; CloseReprocess closes it without processing anything. The confirmation is drawn over the window (ReprocessDialog), as the film page asks too.
	AskReprocess(item Film)
	Reprocessing() (open *bool, item Film, language string)
	CloseReprocess()
	// TranslateAgain translates every line of a done film's subtitles again into language, without downloading or transcribing it, once the user confirms replacing a translation in another language (RetranslateDialog); TranslateMissing translates only the lines its translation left untranslated (Film.Untranslated). TargetLanguage is the target language the settings name, which 重新翻译为 ▸ offers first.
	TranslateAgain(item Film, language string)
	TranslateMissing(item Film)
	TargetLanguage() string
	// Retranslating is whether the confirmation of translating a film into another language is open, the film and the language; CloseRetranslate closes it without translating anything, and Retranslate translates the film into it and closes it.
	Retranslating() (open *bool, item Film, language string)
	CloseRetranslate()
	Retranslate()
	// Remove drops a job's card; its video, if any, stays in the library.
	Remove(id string)
	Retry(id string)
	// Failures is how many failed jobs of 处理中 from source, the sidebar source the page shows (sidebar.SourceAll for every one), RetryAll would queue again, those whose fix needs no choice of the user's (not a login, a members-only, private, region-locked or DRM video), and how many RemoveFailed would take off; a cancelled job counts for neither. RetryAll queues them again, oldest first, behind the jobs waiting; RemoveFailed removes their cards, keeping their files. Neither touches a failure from another source, whose card 处理中 does not show.
	Failures(source string) (retryable, failed int)
	RetryAll(source string)
	RemoveFailed(source string)
	// Withdraw is the × of a waiting or running card: a waiting job that has done nothing yet (no video, no transcript, no film it redoes) leaves the queue and the shelf, and any other is cancelled (Cancel), its card staying.
	Withdraw(id string)
	// FailureHint says, for a failed job, what went wrong and the settings section that fixes it ("" when none does). FailureDetail is its whole error, as the tool said it, which the card's tooltip shows. ReportFailure opens the feedback dialog to report it, with the job's stage and error attached.
	FailureHint(item Film) (reason, section string)
	FailureDetail(item Film) string
	ReportFailure(id string)
	// TranslationHint says, for a done film that translated none of its lines (Film.TranslationErr), why in the user's words and the settings section that fixes it ("" when none does).
	TranslationHint(item Film) (reason, section string)
	// FailureRemedy is what a failed card says and offers when the app knows why its download failed; false shows FailureHint instead. RunRemedy does the action of the remedy's button the user clicked.
	FailureRemedy(item Film) (Remedy, bool)
	RunRemedy(item Film, action string)
	// LinkFocus asks the link field for the keyboard focus once, as 换个链接 does; the field sets it back to false when it has taken the focus.
	LinkFocus() *bool
	// Query is the text typed into the sidebar's search field: the done films whose names hold it are the ones the page shows (ByTitle). 清除搜索 empties it.
	Query() *string
	JobFilm(id string) (Film, bool)
	// SourceFilter is the sidebar source the page shows, sidebar.SourceAll or another sidebar.Source value.
	SourceFilter() string
	// SpokenFilter is the language heard in the done films the page shows, sidebar.SpokenAll or whisper's code for one, among those of SourceFilter.
	SpokenFilter() string
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
	// PlayedAt is when the video at path last played, the zero time when it never did.
	PlayedAt(path string) time.Time
	// Furthest is how far the video at path has ever played on from its start, 0 when it never did; unlike ResumeAt it stays once the video was watched to the end.
	Furthest(path string) time.Duration
	// Notice is the message bar under the top bar.
	Notice() *Notice
	// QueueExpanded is whether 处理中 shows every card rather than the first QueueShown.
	QueueExpanded() *bool
	// Language is the language the settings translate new films into, such as 简体中文.
	Language() *string
	// LibrarySort is the order the done grid shows its films in, one of Sorts; SetLibrarySort shows them in order from now on and keeps it for the next launch.
	LibrarySort() string
	SetLibrarySort(order string)
	// AskRemove asks to confirm moving a film's folder to the Trash.
	AskRemove(item Film)
	// Removing is the film AskRemove asked about, whether its confirmation is open, and whether its whole folder goes to the Trash rather than only its video and subtitles. CloseRemove closes the confirmation without moving anything.
	Removing() (open *bool, item Film, folder bool)
	CloseRemove()
	// TrashFilm moves a film's folder to the Trash and takes it off the shelf.
	TrashFilm(item Film)
	// AskRename opens the dialog that gives a film the title Kando shows for it.
	AskRename(item Film)
	// Renaming is whether the rename dialog is open, the title typed into it, and the title the film has without one given in Kando, which an empty field gives it back. CloseRename closes it and keeps the title as it was.
	Renaming() (open *bool, name *string, original string)
	CloseRename()
	// RenameFilm gives the film AskRename asked about the title typed into the dialog, and closes it. It never renames a file.
	RenameFilm()
}

// Notice is the message bar under the top bar: why a link or a file was not added, or that it was.
type Notice struct {
	// Kind is "warning" or "success".
	Kind string
	Text string
	// Section is the settings section 打开设置 opens; empty shows no button.
	Section string
	// Film is the job ID of the film the bar's 打开 opens, such as the film a link just added is already in the library as; empty shows no button.
	Film string
	// List is the whole playlist the bar's Add Whole Playlist adds (App.AddList), as after one of its videos was added; empty shows no button.
	List string
	Open bool
}

// The library's content, as the prototype lays it out: inset PagePadX from the content column's sides, starting contentTop down, under the top bar that it scrolls beneath, with pagePadBottom under its last part.
const (
	PagePadX      core.Points = 24
	contentTop    core.Points = 64
	pagePadBottom core.Points = 32
)

// View is the library page: the message bar, the shelf, the top bar over them and the dialogs of the library. The film open over it is the main package's (libraryColumn), so the shelf does not import the player.
type View struct{ App App }

// Render implements core.Node.
func (v View) Render(cx *core.Scope) core.Node {
	a := v.App
	// dragging is set while files are dragged over the page, which shows the drop zone over it.
	dragging := core.NewSignal(cx, false)
	source := live.Memo(cx, a, func() string { return pageSource(a) })
	// The page is one scroll view from the content column's top, as the prototype's is: the top bar lies over it, and what is under the bar scrolls beneath it and fades into it.
	content := core.Scroll{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pct(100), Padding: core.Edges{Top: contentTop, Right: PagePadX, Bottom: pagePadBottom, Left: PagePadX}}, Children: []core.Node{
			notice{a},
			core.Show{
				When: live.Func(a, func() bool { return a.LibraryDir() == "" }),
				Then: func(*core.Scope) core.Node { return noLibrary{a} },
				Else: func(*core.Scope) core.Node { return shelf{a: a, source: source} },
			},
		}},
	}}
	return core.View{
		TestID: "films-page",
		Style:  core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), AlignItems: core.AlignStretch},
		// The page takes the files dropped on it, as the innermost view that takes drops; the main column takes those dropped elsewhere in the window.
		OnFileDragEnter: func() { dragging.Set(true) },
		OnFileDragLeave: func() { dragging.Set(false) },
		OnFileDrop: live.On(a, func(uris []string) {
			dragging.Set(false)
			if paths := desktop.DroppedPaths(uris); len(paths) > 0 {
				a.DropFiles(paths)
			}
		}),
		Children: []core.Node{
			content,
			topBar{a: a, source: source},
			core.Show{When: dragging.Get, Then: func(*core.Scope) core.Node { return dropZone{} }},
			newLibrary{a},
			removeDialog{a},
			renameDialog{a},
		},
	}
}

// pageSource is the sidebar source the page shows: YouTube's page shows YouTube's films, the library page the source the sidebar chose.
func pageSource(a App) string {
	if a.Page() == sidebar.PageYouTube {
		return sidebar.SourceYouTube
	}
	return a.SourceFilter()
}

// shelfModel is what the shelf shows under the message bar, worked out from the app at once so its parts follow one state: the films of 处理中 in QueueOrder, the hero's film, the done grid's films in its order, and which of the empty pages shows.
type shelfModel struct {
	queue []Film
	hero  Film
	// heroShown is whether the hero shows, hero being its film.
	heroShown bool
	done      []Film
	// empty is the page of a library that holds no film yet; emptySource that of a source no film comes from; emptySpoken the language, whisper's code, that none of the source's done films was heard in, "" when one was or no language is chosen; query the search no done film matched, "" when it matched one or there is none.
	empty, emptySource bool
	emptySpoken        string
	noMatch            string
}

func (m shelfModel) equal(o shelfModel) bool {
	return slices.Equal(m.queue, o.queue) && m.hero == o.hero && m.heroShown == o.heroShown && slices.Equal(m.done, o.done) &&
		m.empty == o.empty && m.emptySource == o.emptySource && m.emptySpoken == o.emptySpoken && m.noMatch == o.noMatch
}

// newShelfModel is what the library shows under the message bar: the hero, 处理中 and the done grid of the films from source, the grid only those heard in the language the sidebar chose (SpokenFilter), or what to do when there are none. 处理中 also holds the jobs the queue runs for other libraries, so a library with no film of its own shows them above what to do. While the search field holds a query, the grid shows only the done films whose names hold it, or says that none does, and the hero, which is no answer to a search, is left out, as it is while a language is chosen; 处理中 shows every job still, so none seems to vanish while a search is on. tag is the interface's language, which orders the grid by title.
func newShelfModel(a App, source, tag string) shelfModel {
	items := a.Films()
	var m shelfModel
	m.queue = QueueOrder(BySource(a.QueueFilms(), source))
	if len(items) == 0 {
		m.empty = true
		return m
	}
	done := Shown(items, source, sidebar.SpokenAll, "")
	if len(m.queue) == 0 && len(done) == 0 {
		m.emptySource = true
		return m
	}
	spoken := a.SpokenFilter()
	heard := BySpoken(done, spoken)
	if len(m.queue) == 0 && len(heard) == 0 {
		m.emptySpoken = spoken
		return m
	}
	query := strings.TrimSpace(*a.Query())
	if query == "" && source == sidebar.SourceAll && spoken == sidebar.SpokenAll {
		m.hero, m.heroShown = HeroFilm(a, done)
	}
	done = ByTitle(heard, query)
	m.done = Sorted(done, a.LibrarySort(), tag, a.PlayedAt)
	if len(done) == 0 && query != "" {
		m.noMatch = query
	}
	return m
}

// shelf is the part of the page under the message bar while a library is open.
type shelf struct {
	a      App
	source func() string
}

// Render implements core.Node.
func (s shelf) Render(cx *core.Scope) core.Node {
	a := s.a
	model := core.NewMemoFunc(cx, live.Func(a, func() shelfModel { return newShelfModel(a, s.source(), i18n.Language(cx)) }), shelfModel.equal).Get
	return core.Group{
		core.Show{When: func() bool { return model().heroShown }, Then: func(*core.Scope) core.Node {
			return hero{a: a, item: func() Film { return model().hero }, compact: func() bool { return len(model().queue) > 0 }}
		}},
		core.Show{When: func() bool { return len(model().queue) > 0 }, Then: func(*core.Scope) core.Node {
			return queueSection{a: a, source: s.source, queue: func() []Film { return model().queue }}
		}},
		core.Show{When: func() bool { return model().empty }, Then: func(*core.Scope) core.Node { return emptyLibrary{a} }},
		core.Show{When: func() bool { return model().emptySource }, Then: func(*core.Scope) core.Node { return emptySource{} }},
		core.Show{When: func() bool { return model().emptySpoken != "" }, Then: func(*core.Scope) core.Node {
			return emptySpoken{code: func() string { return model().emptySpoken }}
		}},
		core.Show{When: func() bool { return len(model().done) > 0 }, Then: func(*core.Scope) core.Node {
			return grid{a: a, done: func() []Film { return model().done }}
		}},
		core.Show{When: func() bool { return model().noMatch != "" }, Then: func(*core.Scope) core.Node {
			return noMatch{a: a, query: func() string { return model().noMatch }}
		}},
	}
}
