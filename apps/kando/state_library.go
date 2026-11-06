package main

import (
	"os"
	"strings"
	"time"

	"kando/apps/kando/films"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// libraryUI is the state of the library page that is not the shelf itself (the shelf is the nodra session in state.go).
type libraryUI struct {
	// posters are the cards' frames by video path, posterMiss the videos that have none and posterBusy those being grabbed; posterDir is where frames of videos outside a folder of their own are kept (library_posters.go).
	posterDir  string
	posters    map[string]ui.ImageSource
	posterMiss map[string]bool
	posterBusy map[string]bool
	// notice is the message bar under the top bar.
	notice films.Notice
	// queueOpen shows every card of 处理中 rather than the first films.QueueShown.
	queueOpen bool
	// removing is the film whose 从资料库移除 is waiting to be confirmed, while removeOpen. removeFolder is whether its folder, rather than only its video and subtitles, goes to the Trash, as filmFiles decided when it was asked.
	removing     films.Film
	removeOpen   bool
	removeFolder bool
	// removeRelease and sheetRelease give back the window's modal flag (holdModal) that the remove confirmation and the new-library sheet hold while they are open; nil while they are closed. libraryHooks' afterFrame gives each back once its dialog has closed.
	removeRelease func()
	sheetRelease  func()
	// durations is how long each video plays, by path, read with the poster. A video that has none here is not known yet.
	durations map[string]time.Duration
	// quotes is the first bilingual line of each subtitle file read for the hero, by path.
	quotes map[string]quote
	// holdExtras keeps loadExtras from starting before the window exists: the background reads it starts report through the window. libraryHooks' restore sets it, as startup begins, and its ready clears it and loads the extras.
	holdExtras bool
}

// quote is the first bilingual line of a subtitle file: the translation over the original, as the exported file writes them.
type quote struct{ translation, original string }

func newLibraryUI() libraryUI {
	return libraryUI{}
}

// savedLibrary is the library page's part of settings.json. The libraries themselves are the shelf's (savedShelf).
type savedLibrary struct{}

func (s *savedLibrary) clean() {}

// libraryHooks: the shelf's posters, video lengths and hero line are loaded whenever its films change, and held back until the window exists, because the posters report through it. After each frame the library's dialogs hold or give back the window's modal flag.
func (a *app) libraryHooks() areaHooks {
	return areaHooks{
		restore: func(savedSettings) { a.lib.holdExtras = true },
		ready: func() {
			a.lib.holdExtras = false
			a.loadExtras()
		},
		afterFrame:   a.libraryModals,
		shelfChanged: a.loadExtras,
	}
}

// libraryModals keeps the window's modal flag held while the library's dialogs are open. AskRemove holds it as the remove confirmation opens, and it is given back here once the confirmation has closed. The new-library sheet is open while the session is making a library, which the shelf and the sidebar start, so the flag is held here from the frame that first draws the sheet until the frame after it closes.
func (a *app) libraryModals() {
	if !a.lib.removeOpen && a.lib.removeRelease != nil {
		a.lib.removeRelease()
		a.lib.removeRelease = nil
	}
	making := a.snap().MakingLibrary
	switch {
	case making && a.lib.sheetRelease == nil:
		a.lib.sheetRelease = a.holdModal()
	case !making && a.lib.sheetRelease != nil:
		a.lib.sheetRelease()
		a.lib.sheetRelease = nil
	}
}

// warn shows a warning in the library's message bar; section, when not empty, is the settings section that fixes it.
func (a *app) warn(text, section string) {
	a.lib.notice = films.Notice{Kind: "warning", Text: text, Section: section, Open: true}
}

// tell shows a message that something worked in the library's message bar.
func (a *app) tell(text string) {
	a.lib.notice = films.Notice{Kind: "success", Text: text, Open: true}
}

// loadExtras reads what the shelf shows of its films beyond the session: each poster and the video's length, off the UI thread, and the hero's first bilingual line. The shelf calls it when its films change, so the page only reads what it has; nothing is read twice.
func (a *app) loadExtras() {
	if a.lib.holdExtras {
		return
	}
	items := a.films()
	for _, item := range items {
		a.ensurePoster(item.VideoPath)
	}
	if hero, ok := films.HeroFilm(items); ok {
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
