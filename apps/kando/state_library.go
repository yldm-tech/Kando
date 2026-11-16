package main

import (
	"os"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// libraryUI is the state of the library page that is not the shelf itself (the shelf is the nodra session in state.go).
type libraryUI struct {
	// posters are the cards' frames by video path, posterMiss the videos that have none and posterBusy those being grabbed; posterDir is where frames of videos outside a folder of their own are kept (library_posters.go).
	posterDir  string
	posters    map[string]ns.ImageSource
	posterMiss map[string]bool
	posterBusy map[string]bool
	// thumbs are the pictures the videos' sites give them, by address, for the processing cards; thumbMiss the addresses that gave none and thumbBusy those being fetched. They are kept in posterDir too.
	thumbs    map[string]ns.ImageSource
	thumbMiss map[string]bool
	thumbBusy map[string]bool
	// notice is the message bar under the top bar. noticeTimer closes a message that something worked after noticeFor (noticeLater). noticeReady marks a warning of precheckReady, which the bar follows until it is fixed; noticeJob and noticeStage are the job a warning is about and the stage it spoke of, and the bar closes once the job is gone or has moved on from it, as when it is retried (libraryFollowNotice).
	notice      films.Notice
	noticeTimer *film.Timer
	noticeReady bool
	noticeJob   string
	noticeStage core.Stage
	// queueOpen shows every card of 处理中 rather than the first films.QueueShown.
	queueOpen bool
	// removing is the film whose 从资料库移除 is waiting to be confirmed, while removeOpen. removeFolder is whether its folder, rather than only its video and subtitles, goes to the Trash, as filmFiles decided when it was asked.
	removing     films.Film
	removeOpen   bool
	removeFolder bool
	// reprocessing is the film whose 重新处理 is waiting to be confirmed, while reprocessOpen (bind_films.go).
	reprocessing  films.Film
	reprocessOpen bool
	// retranslating is the film whose translation into retranslateLanguage, another than its own, is waiting to be confirmed, while retranslateOpen (bind_films.go).
	retranslating       films.Film
	retranslateLanguage string
	retranslateOpen     bool
	// renaming is the film whose 重命名… dialog is open, while renameOpen. renameName is the title typed into it, which starts as the one the film shows, and renameOriginal the title the film has without one given in Kando (library_rename.go).
	renaming       films.Film
	renameOpen     bool
	renameName     string
	renameOriginal string
	// removeRelease, renameRelease, sheetRelease and reprocessRelease give back the window's modal flag (holdModal) that the remove confirmation, the rename dialog, the new-library sheet and the confirmation of 重新处理 hold while they are open; nil while they are closed. The handlers that close the confirmations and the rename dialog give theirs back (libraryCloseRemove, libraryCloseRename, CloseReprocess); libraryHooks' afterChange gives back the sheet's, and any a dialog closed some other way left behind.
	removeRelease    func()
	renameRelease    func()
	sheetRelease     func()
	reprocessRelease func()
	// retranslateRelease is the modal flag the confirmation of 重新翻译为 holds while it is open, given back by CloseRetranslate or here.
	retranslateRelease func()
	// durations is how long each video plays, by path, read with the poster. A video that has none here is not known yet.
	durations map[string]time.Duration
	// quotes is the first bilingual line of each subtitle file read for the hero, by path.
	quotes map[string]quote
	// holdExtras keeps loadExtras from starting before the window exists: the background reads it starts report through the window. libraryHooks' restore sets it, as startup begins, and its ready clears it and loads the extras.
	holdExtras bool
	// opened are the files the system asked Kando to open (libraryOpenFile, run's desktop.OnOpenFiles), waiting to be added to the queue: videos chosen with Finder's Open With, dropped on the Dock icon or named on the command line. holdOpened keeps them waiting until the window exists, as holdExtras does: libraryHooks' restore sets it and its ready clears it and posts them. openedPosted is set while a report through update is on its way to add them, so the files of one open arrive in the queue together.
	opened       []string
	holdOpened   bool
	openedPosted bool
	// sort is the order the done grid shows its films in, one of films.Sorts, kept in settings.json (savedLibrary).
	sort string
	// translateOffered is set once the message bar has offered to translate the films that have no translation, or there was nothing to offer, since an AI service was last set up; it is cleared while none is, so setting one up offers it again (libraryOfferTranslate). libraryHooks' ready sets it when the app starts with a service set up, so a launch offers nothing.
	translateOffered bool
}

// quote is the first bilingual line of a subtitle file: the translation over the original, as the exported file writes them.
type quote struct{ translation, original string }

func newLibraryUI() libraryUI {
	return libraryUI{sort: films.SortAdded}
}

// savedLibrary is the library page's part of settings.json: the order of the done grid. The libraries themselves are the shelf's (savedShelf).
type savedLibrary struct {
	LibrarySort string `json:"librarySort,omitempty"`
}

// clean reads an order Kando does not know, such as one another version saved, as the default.
func (s *savedLibrary) clean() { s.LibrarySort = films.ValidSort(s.LibrarySort) }

// libraryHooks: the grid's order is saved and restored with the settings. The shelf's posters, video lengths and hero line are loaded whenever its films change, the hero line also when a film played, as the film played last may be the hero now, and all are held back until the window exists, because the posters report through it. Files the system asked Kando to open before the window existed are held back with them and added to the queue once it does, after every area's ready. After each change the library lets go of an open film it no longer has, and its dialogs hold or give back the window's modal flag.
func (a *app) libraryHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) { saved.savedLibrary = savedLibrary{LibrarySort: a.lib.sort} },
		restore: func(saved savedSettings) {
			a.lib.sort = films.ValidSort(saved.LibrarySort)
			a.lib.holdExtras = true
			a.lib.holdOpened = true
		},
		ready: func() {
			a.lib.translateOffered = !a.untranslatable()
			a.lib.holdExtras = false
			a.loadExtras()
			a.lib.holdOpened = false
			a.libraryPostOpened()
		},
		afterChange:    a.libraryAfterChange,
		shelfChanged:   a.loadExtras,
		libraryChanged: a.libraryDropNotice,
		// The hero shows the film played last, so a film that played may be the hero now.
		positionsChanged: a.libraryHeroQuote,
		llmChanged:       a.libraryOfferTranslate,
		toolsChanged:     func(bool) { a.libraryOfferTranslate() },
	}
}

// libraryOfferTranslate says in the message bar, once an AI service is set up, how many films of the open library have no translation, their subtitles the original lines alone (films.NotTranslated), and offers to translate them all (films.Notice.Translate), once each time a service is set up: a service set up again, another model chosen or the tools found again, offers nothing more.
func (a *app) libraryOfferTranslate() {
	if a.untranslatable() {
		a.lib.translateOffered = false
		return
	}
	if a.lib.translateOffered {
		return
	}
	a.lib.translateOffered = true
	if n := len(films.NotTranslated(a.films())); n > 0 {
		a.libraryShow(films.Notice{Kind: "warning", Text: a.messages().LibraryTranslateOffer(n), Translate: true, Open: true})
	}
}

// libraryAfterChange lets go of an open film the library no longer has (libraryFollowOpen), closes the dialogs whose reason went away (libraryModals) and keeps the message bar about what is still so (libraryFollowNotice) after each change, and reports whether it changed anything.
func (a *app) libraryAfterChange() bool {
	open := a.libraryFollowOpen()
	modals := a.libraryModals()
	notice := a.libraryFollowNotice()
	return open || modals || notice
}

// libraryFollowNotice closes the message bar, or brings it up to date, once what it says is no longer so: a warning of precheckReady follows it (precheckFollow), a warning about a job closes once the job is gone or has left the stage it spoke of, and the offer to translate the films without a translation follows how many are left (libraryOfferTranslate). It reports whether it changed the bar.
func (a *app) libraryFollowNotice() bool {
	n := &a.lib.notice
	switch {
	case !n.Open:
		return false
	case a.lib.noticeReady:
		return a.precheckFollow(&n.Text, &n.Section, &n.Open)
	case a.lib.noticeJob != "":
		if job, ok := a.library.Find(a.lib.noticeJob); ok && job.Stage == a.lib.noticeStage {
			return false
		}
		n.Open = false
		return true
	case n.Translate:
		// The offer to translate the films without a translation counts them as they are translated, and closes once none is left or no service is set up to translate them.
		count := len(films.NotTranslated(a.films()))
		text := a.messages().LibraryTranslateOffer(count)
		switch {
		case count == 0 || a.untranslatable():
			n.Open = false
		case n.Text != text:
			n.Text = text
		default:
			return false
		}
		return true
	}
	return false
}

// libraryDropNotice closes the message bar as another library opens, as what it said was about the one left; the guide's note, which is about Kando, stays.
func (a *app) libraryDropNotice() {
	if !isOnboardingTip(a.lib.notice.Text) {
		a.lib.notice.Open = false
	}
}

// libraryNoticeJob ties the warning the message bar shows to job id at stage, so it closes once the job is gone or has left that stage (libraryFollowNotice).
func (a *app) libraryNoticeJob(id string, stage core.Stage) {
	a.lib.noticeJob, a.lib.noticeStage = id, stage
}

// libraryModals keeps the window's modal flag held while the library's dialogs are open, and reports whether it held or gave it back. AskRemove, AskRename and AskReprocess hold it as the remove confirmation, the rename dialog and the confirmation of 重新处理 open, and the handlers that close them give it back; one a dialog closed some other way left behind is given back here. The new-library sheet is open while the session is making a library, which the shelf, the sidebar and the settings start, and which several of them end, so its flag is held here as the sheet opens and given back as it closes.
func (a *app) libraryModals() bool {
	changed := false
	if !a.lib.removeOpen && a.lib.removeRelease != nil {
		a.lib.removeRelease()
		a.lib.removeRelease = nil
		changed = true
	}
	if !a.lib.renameOpen && a.lib.renameRelease != nil {
		a.lib.renameRelease()
		a.lib.renameRelease = nil
		changed = true
	}
	if !a.lib.reprocessOpen && a.lib.reprocessRelease != nil {
		a.lib.reprocessRelease()
		a.lib.reprocessRelease = nil
		changed = true
	}
	if !a.lib.retranslateOpen && a.lib.retranslateRelease != nil {
		a.lib.retranslateRelease()
		a.lib.retranslateRelease = nil
		changed = true
	}
	making := a.snap().MakingLibrary
	switch {
	case making && a.lib.sheetRelease == nil:
		a.lib.sheetRelease = a.holdModal()
		changed = true
	case !making && a.lib.sheetRelease != nil:
		a.lib.sheetRelease()
		a.lib.sheetRelease = nil
		changed = true
	}
	return changed
}

// libraryShow puts n in the library's message bar, in place of what it said, which takes with it what the bar followed of that (noticeReady, noticeJob). Everything that fills the bar goes through it: warn, tell and the guide's note.
func (a *app) libraryShow(n films.Notice) {
	a.lib.notice = n
	a.lib.noticeReady, a.lib.noticeJob, a.lib.noticeStage = false, "", ""
}

// warn shows a warning in the library's message bar, which stays until it is closed or what it says is no longer so (libraryFollowNotice); section, when not empty, is the settings section that fixes it.
func (a *app) warn(text, section string) {
	a.libraryShow(films.Notice{Kind: "warning", Text: text, Section: section, Open: true})
}

// tell shows a message that something worked in the library's message bar, which closes by itself after noticeFor.
func (a *app) tell(text string) {
	a.libraryShow(films.Notice{Kind: "success", Text: text, Open: true})
	shown := a.lib.notice
	a.noticeLater(&a.lib.noticeTimer, func() {
		if a.lib.notice == shown {
			a.lib.notice.Open = false
		}
	})
}

// noticeFor is how long a message bar says that something worked before it closes by itself. A warning stays until it is closed, or until what it says is no longer so.
const noticeFor = 5 * time.Second

// noticeLater runs close after noticeFor, on the timer *timer, made under the app's scope on first use, unless it is set again first; close checks that the bar still says what it was set for, as another message may have taken its place. The library's and the browser's message bars close their messages that something worked with it. Without the app's scope, as in tests that start no app, close never runs.
func (a *app) noticeLater(timer **film.Timer, close func()) {
	if a.cx == nil {
		return
	}
	if *timer == nil {
		*timer = film.NewTimer(a.cx)
	}
	(*timer).Set(noticeFor, live.Do(a, close))
}

// loadExtras reads what the shelf shows of its films beyond the session: each poster and the video's length, and the site's picture of each processing film, off the UI thread, and the hero's first bilingual line. The shelf calls it when its films change, so the page only reads what it has; nothing is read twice.
func (a *app) loadExtras() {
	if a.lib.holdExtras {
		return
	}
	items := a.films()
	for _, item := range items {
		a.ensurePoster(item)
		if films.InQueue(item) {
			a.ensureThumbnail(item)
		}
	}
	// 处理中 also shows the jobs the queue runs for other libraries.
	for _, item := range a.QueueFilms() {
		if item.Library != "" {
			a.ensurePoster(item)
			a.ensureThumbnail(item)
		}
	}
	a.libraryHeroQuote()
}

// libraryHeroQuote reads the first bilingual line of the hero's film (films.HeroFilm), once the window is ready.
func (a *app) libraryHeroQuote() {
	if a.lib.holdExtras {
		return
	}
	if hero, ok := films.HeroFilm(a, a.films()); ok {
		a.loadQuote(hero.SubtitlePath)
	}
}

// loadQuote keeps the first bilingual line of the subtitle file at path for the hero. A subtitle file is small, and the line is read once, so it is read here rather than off the UI thread; a file that cannot be read has no line.
func (a *app) loadQuote(path string) {
	if path == "" {
		return
	}
	if _, ok := a.lib.quotes[path]; ok {
		return
	}
	var q quote
	if data, err := os.ReadFile(path); err == nil {
		if cues, err := media.ParseSRT(data); err == nil {
			q = firstQuote(cues)
		}
	}
	if a.lib.quotes == nil {
		a.lib.quotes = map[string]quote{}
	}
	a.lib.quotes[path] = q
}

// firstQuote is the first cue that has both a translation and an original, or the first cue as it is when none has.
func firstQuote(cues []core.Segment) quote {
	for _, cue := range cues {
		translation, original, ok := strings.Cut(strings.TrimSpace(cue.Text), "\n")
		if ok {
			return quote{translation: strings.TrimSpace(translation), original: strings.TrimSpace(original)}
		}
	}
	if len(cues) > 0 {
		return quote{translation: strings.TrimSpace(cues[0].Text)}
	}
	return quote{}
}
