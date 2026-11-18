package films

import (
	"slices"
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/sidebar"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
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
	// Select shows a page of the window, as sidebar.PageBrowse, the built-in browser, which 浏览网站 opens.
	Select(page string)
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
	// Reprocess makes the subtitles of a done film again, from its download, in the language it was translated into (the target language set now for one that was not translated), or those of a video found in the folder that has none (生成字幕). Either keeps the film as it was if its job fails or is stopped.
	Reprocess(item Film)
	// AskReprocess asks to confirm processing items again (ReprocessFilms), which transcribes each film and pays for its translation again. Reprocessing is whether its confirmation is open, the films it asked about and the language they would be translated into, "" for several in different languages; CloseReprocess closes it without processing anything. The confirmation is drawn over the window (ReprocessDialog), as the film page asks too. ReprocessFilms processes items again as Reprocess does each, one after another, and says in the message bar once how it went.
	AskReprocess(items ...Film)
	Reprocessing() (open *bool, items []Film, language string)
	CloseReprocess()
	ReprocessFilms(items ...Film)
	// TranslateAgain translates every line of a done film's subtitles again into language, without downloading or transcribing it, once the user confirms replacing a translation in another language (RetranslateDialog); TranslateMissing translates only the lines its translation left untranslated (Film.Untranslated). TargetLanguage is the target language the settings name, which 重新翻译为 ▸ offers first.
	TranslateAgain(item Film, language string)
	TranslateMissing(item Film)
	// TranslateFilmsAgain translates every line of each done film of items that has a job again into language, as TranslateAgain does one, once the user confirms replacing the translations in another language, and says in the message bar once how it went.
	TranslateFilmsAgain(items []Film, language string)
	TargetLanguage() string
	// TranslateFilms translates every line of each film of items that was not translated at all (NotTranslated), into the target language the settings name, as its own 翻译 does (TranslateMissing), behind the jobs waiting. CanTranslate reports whether an AI service is set up to do it; without one, Translate N Films does not show.
	TranslateFilms(items []Film)
	CanTranslate() bool
	// Retranslating is whether the confirmation of translating films into another language is open, the films and the language; CloseRetranslate closes it without translating anything, and Retranslate translates the films into it and closes it.
	Retranslating() (open *bool, items []Film, language string)
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
	// WatchFilter is how far the done films the page shows were watched, sidebar.WatchAll or another of the sidebar's Watch values (WatchState), among those of SourceFilter and SpokenFilter; SetWatchFilter shows the films in state from now on, as the sidebar's list does.
	WatchFilter() string
	SetWatchFilter(state string)
	// MarkWatched marks each film of items watched to its end, as playing it to its end would, without saying when it played: the check shows over its poster, and it no longer opens where it stopped. MarkUnwatched forgets that each was ever played, where it stopped, how far it played and when, so it shows as one never played. Both keep it with the library at once. How far a film was played also bounds the lines the command palette finds and sends of it, so marking it watched lets the palette find every line of it, and unwatched none.
	MarkWatched(items ...Film)
	MarkUnwatched(items ...Film)
	// Enqueue adds a job for each source, a link or a local video's file:// URL, to the open library in the language the settings name, and runs them one after another. The error is why a source was not added.
	Enqueue(sources ...string) error
	// Cancel stops job id if it is waiting or running. Its card stays, marked 已取消. Enqueue, Cancel and RunningJob are the contract of the job queue; queue.go in the main package says how much of it runs today. Job control is per job: a page cancels the job it shows, never "whatever runs".
	Cancel(id string)
	// ContinueHere lets a waiting job a paired phone paused (Film.PausedBy) wait for its turn again, as 在这台电脑上继续 on its card does.
	ContinueHere(id string)
	// Handoff is where a paired phone last left a film of the open library, offered to continue from here; CloseHandoff closes the offer until a phone leaves a film again. LeftOn is the paired phone that last left the video at path where it resumes, "" when it was this computer.
	Handoff() Handoff
	CloseHandoff()
	LeftOn(path string) string
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
	// Selection is the done films selected on the page, which the app keeps so the menu bar acts on them too (Selection).
	Selection() *Selection
	// PosterFocus asks the first poster of the grid for the keyboard focus once, as Return in the sidebar's search field does when it leaves several films; the grid sets it back to false when its first poster has taken the focus, or when it shows no poster.
	PosterFocus() *bool
	// Notice is the message bar under the top bar.
	Notice() *Notice
	// QueueExpanded is whether 处理中 shows every card rather than the first QueueShown.
	QueueExpanded() *bool
	// Language is the language the settings translate new films into, such as 简体中文.
	Language() *string
	// LibrarySort is the order the done films show in; SetLibrarySort shows them in that order from now on and keeps it for the next launch.
	LibrarySort() Order
	SetLibrarySort(o Order)
	// LibraryView is how the done films show, ViewGrid or ViewList; SetLibraryView shows them that way from now on and keeps it for the next launch.
	LibraryView() string
	SetLibraryView(view string)
	// AskRemove asks to confirm moving the films' folders, or their files, to the Trash.
	AskRemove(items ...Film)
	// Removing is the films AskRemove asked about, whether its confirmation is open, and, for a single film, whether its whole folder goes to the Trash rather than only its video and subtitles. CloseRemove closes the confirmation without moving anything.
	Removing() (open *bool, items []Film, folder bool)
	CloseRemove()
	// TrashFilms moves the films' folders, or their files, to the Trash and takes them off the shelf, and says in the message bar once how it went.
	TrashFilms(items ...Film)
	// AskRename opens the dialog that gives a film the title Kando shows for it.
	AskRename(item Film)
	// Renaming is whether the rename dialog is open, the title typed into it, and the title the film has without one given in Kando, which an empty field gives it back. CloseRename closes it and keeps the title as it was.
	Renaming() (open *bool, name *string, original string)
	CloseRename()
	// RenameFilm gives the film AskRename asked about the title typed into the dialog, and closes it. It never renames a file.
	RenameFilm()
	// MoveFilmMenu is the submenu 移到资料库 of the menu of a film, or of the films selected: the other libraries Kando knows, each moving the films there when chosen, one after another, those whose folder is not there disabled; none when there is no other library. UndoMove moves the film the message bar says was moved back (Notice.Undo), and CancelMove stops the copy it says is going (Notice.Stop). Choose opens the library at a folder, as the bar's 打开 <library> does (Notice.Library).
	MoveFilmMenu(cx *core.Scope, items ...Film) []nsdesktop.MenuItem
	UndoMove()
	CancelMove()
	Choose(dir string)
}

// Notice is the message bar under the top bar: why a link or a file was not added, or that it was.
type Notice struct {
	// Kind is "warning" or "success".
	Kind string
	Text string
	// Section is the settings section 打开设置 opens; empty shows no button.
	Section string
	// Film is the key of the film the bar's 打开 opens, its job's ID or the path of a video of the library's folder no job holds, such as the film a link or a video just added is already in the library as; empty shows no button.
	Film string
	// List is the whole playlist the bar's Add Whole Playlist adds (App.AddList), as after one of its videos was added; empty shows no button.
	List string
	// Translate shows the bar's Translate, which translates every film of the library that was not translated at all (NotTranslated, App.TranslateFilms), as the bar offers once an AI service is set up.
	Translate bool
	// Library is the folder of the library the bar's 打开 <library> opens (App.Choose), as after a film was moved there; empty shows no button. Undo shows Undo, which moves the film back (App.UndoMove), and Stop 取消, which stops the copy the bar says is going (App.CancelMove).
	Library    string
	Undo, Stop bool
	Open       bool
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
	model := core.NewMemoFunc(cx, live.Func(a, func() shelfModel {
		if a.LibraryDir() == "" {
			return shelfModel{}
		}
		return newShelfModel(a, source(), i18n.Language(cx))
	}), shelfModel.equal).Get
	// The page is one scroll view from the content column's top, as the prototype's is: the top bar lies over it, and what is under the bar scrolls beneath it and fades into it. The list scrolls itself instead, as Finder's does, its heading staying put, so while it shows the page is a column that gives the list the height left under what is above it.
	scrolled := func(*core.Scope) core.Node {
		return core.Scroll{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pct(100), Padding: core.Edges{Top: contentTop, Right: PagePadX, Bottom: pagePadBottom, Left: PagePadX}}, Children: []core.Node{
				notice{a},
				handoffBar{a},
				core.Show{
					When: live.Func(a, func() bool { return a.LibraryDir() == "" }),
					Then: func(*core.Scope) core.Node { return noLibrary{a} },
					Else: func(*core.Scope) core.Node { return shelf{a: a, source: source, model: model} },
				},
			}},
		}}
	}
	content := core.Show{When: func() bool { return model().listed() }, Then: func(*core.Scope) core.Node { return listPage{a: a, source: source, model: model} }, Else: scrolled}
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
			selectionKeys(cx, a, func() []Film { return model().done }),
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
	// list is whether the done films show as a list (ViewList), which leaves the hero out.
	list bool
	// empty is the page of a library that holds no film yet; emptySource that of a source no film comes from; emptySpoken the language, whisper's code, that none of the source's done films was heard in, "" when one was or no language is chosen; emptyWatch the watch state none of the films of the source and the language is in, "" when one is or none is chosen; query the search no done film matched, "" when it matched one or there is none.
	empty, emptySource bool
	emptySpoken        string
	emptyWatch         string
	noMatch            string
}

func (m shelfModel) equal(o shelfModel) bool {
	return slices.Equal(m.queue, o.queue) && m.hero == o.hero && m.heroShown == o.heroShown && slices.Equal(m.done, o.done) && m.list == o.list &&
		m.empty == o.empty && m.emptySource == o.emptySource && m.emptySpoken == o.emptySpoken && m.emptyWatch == o.emptyWatch && m.noMatch == o.noMatch
}

// newShelfModel is what the library shows under the message bar: the hero, 处理中 and the done grid of the films from source, the grid only those heard in the language the sidebar chose (SpokenFilter) and watched as far as it chose (WatchFilter), or what to do when there are none. 处理中 also holds the jobs the queue runs for other libraries, so a library with no film of its own shows them above what to do. While the search field holds a query, the grid shows only the done films whose names hold it, or says that none does, and the hero, which is no answer to a search, is left out, as it is while a language or a watch state is chosen; 处理中 shows every job still, so none seems to vanish while a search is on. tag is the interface's language, which orders the grid by title.
func newShelfModel(a App, source, tag string) shelfModel {
	items := a.Films()
	var m shelfModel
	m.queue = QueueOrder(BySource(a.QueueFilms(), source))
	if len(items) == 0 {
		m.empty = true
		return m
	}
	done := Shown(items, a, source, sidebar.SpokenAll, sidebar.WatchAll, "")
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
	watch := a.WatchFilter()
	seen := ByWatch(heard, a, watch)
	if len(m.queue) == 0 && len(seen) == 0 {
		m.emptyWatch = watch
		return m
	}
	query := strings.TrimSpace(*a.Query())
	m.list = a.LibraryView() == ViewList
	// The list is a table to scan and to sort, as Finder's is, and its own columns say what was watched last; the hero is the posters' opening.
	if query == "" && source == sidebar.SourceAll && spoken == sidebar.SpokenAll && watch == sidebar.WatchAll && !m.list {
		m.hero, m.heroShown = HeroFilm(a, done)
	}
	done = ByTitle(seen, query)
	m.done = Sorted(done, a.LibrarySort(), tag, a.PlayedAt, a.Duration)
	if len(done) == 0 && query != "" {
		m.noMatch = query
	}
	return m
}

// listed reports whether the page shows the done films as a list: the list was chosen and there are films to list. Otherwise the page shows the posters or what to do when there are none.
func (m shelfModel) listed() bool { return m.list && len(m.done) > 0 }

// shelf is the part of the page under the message bar while a library is open and the done films show as posters, or there are none.
type shelf struct {
	a      App
	source func() string
	model  func() shelfModel
}

// Render implements core.Node.
func (s shelf) Render(cx *core.Scope) core.Node {
	a, model := s.a, s.model
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
		core.Show{When: func() bool { return model().emptyWatch != "" }, Then: func(*core.Scope) core.Node {
			return emptyWatch{a: a, state: func() string { return model().emptyWatch }}
		}},
		core.Show{When: func() bool { return len(model().done) > 0 }, Then: func(*core.Scope) core.Node {
			return grid{a: a, done: func() []Film { return model().done }}
		}},
		core.Show{When: func() bool { return model().noMatch != "" }, Then: func(*core.Scope) core.Node {
			return noMatch{a: a, query: func() string { return model().noMatch }}
		}},
	}
}

// listPage is the page while the done films show as a list: the message bar and 处理中 over the list's heading, then the list filling the rest of the page down to its foot. What is above the list scrolls by itself once it takes more than half the page, so the list keeps the rest.
type listPage struct {
	a      App
	source func() string
	model  func() shelfModel
}

// Render implements core.Node.
func (l listPage) Render(cx *core.Scope) core.Node {
	a, model := l.a, l.model
	above := core.Scroll{Style: core.Style{FlexShrink: 1, MinHeight: core.Pt(0), MaxHeight: core.Pct(50)}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pct(100)}, Children: []core.Node{
			notice{a},
			handoffBar{a},
			core.Show{When: func() bool { return len(model().queue) > 0 }, Then: func(*core.Scope) core.Node {
				return queueSection{a: a, source: l.source, queue: func() []Film { return model().queue }}
			}},
		}},
	}}
	done := func() []Film { return model().done }
	return core.View{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0), Padding: core.Edges{Top: contentTop, Right: PagePadX, Bottom: 12, Left: PagePadX}}, Children: []core.Node{
		above,
		core.View{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0), Margin: core.Edges{Top: 26}, Gap: 10}, Children: []core.Node{
			doneHeading{a: a, done: done},
			filmList{a: a, done: done},
		}},
	}}
}

// selectionKeys binds the selection's keys while the page shows, under cx, and is the node that binds Escape: ⌘A selects every done film the page shows, in the posters or the list, and Escape, while some are selected, selects none, as in Finder. A text field with the keyboard keeps both (core.KeyNotEditing), so ⌘A selects its text through Edit › 全选 and Escape empties a search; NSGo matches a binding before the menu bar's key equivalents, so ⌘A reaches it rather than the menu.
// TODO(nsgo#651): let a menu role item run the app's own action while no text has the keyboard, so choosing Edit › 全选 with the pointer selects the films too; it only selects text (selectAll:), and the films only by ⌘A https://github.com/nsgo-dev/nsgo/issues/651
func selectionKeys(cx *core.Scope, a App, done func() []Film) core.Node {
	cx.OnKeyWith("mod+a", core.KeyOptions{When: core.KeyNotEditing}, live.Do(a, func() { a.Selection().All(Keys(core.Untracked(cx, done))) }))
	return core.Show{When: live.Func(a, func() bool { return a.Selection().Len() > 0 }), Then: func(cx *core.Scope) core.Node {
		cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyNotEditing}, live.Do(a, func() { a.Selection().Clear() }))
		return core.Group{}
	}}
}
