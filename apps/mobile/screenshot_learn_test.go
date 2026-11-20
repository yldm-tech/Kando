package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/mobile/store"
	phonestudy "kando/apps/mobile/study"
	"kando/packages/study"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// watchedFor keeps, in the phone's data folder dir, minutes of films played on each of the last days, the earliest first, today last.
func watchedFor(t *testing.T, dir string, minutes ...int) {
	t.Helper()
	days := map[string]float64{}
	for i, m := range minutes {
		days[study.Day(now().AddDate(0, 0, i-len(minutes)+1))] = float64(m * 60)
	}
	data, err := json.Marshal(days)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "learning"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "learning", "watched.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// learnSample keeps on the phone, in dir, the talk downloaded from a computer no longer around, with a poster, and the learning state of its library: the study records and a few more words and a line, as the phone has them after a sync; and the daily reminder on. The scenes start from it rather than from a sync, which does not finish under nsgo test -target macos.
func learnSample(t *testing.T, dir string) {
	t.Helper()
	f := keepTalk(t, dir)
	st := store.Open(dir)
	f.Poster = "poster.png"
	writePoster(t, filepath.Join(st.FilmDir(f.Desktop, f.Film.ID), f.Poster))
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	prefs, err := st.Prefs()
	if err != nil {
		t.Fatal(err)
	}
	prefs.Remind = true
	if err := st.SavePrefs(prefs); err != nil {
		t.Fatal(err)
	}
	word := func(id, text, reading, meaning, context string, box int, due, saved time.Duration) sync.Record {
		return sync.Record{Kind: sync.KindWord, ID: id, Film: "f1", Fields: map[string]sync.Field{
			sync.FieldWord: field(text), sync.FieldReading: field(reading), sync.FieldMeaning: field(meaning), sync.FieldSpoken: field("ja"),
			sync.FieldContext: field(context), sync.FieldTitle: field("Morning Talk"), sync.FieldStartMS: field(3000),
			sync.FieldSavedAt: field(now().Add(saved)), sync.FieldReview: field(sync.Review{Box: box, Due: now().Add(due)}),
		}}
	}
	records := append(studyRecords(),
		word("w-station", "駅", "えき", "车站", "駅はどこですか。", 0, -2*time.Hour, -30*time.Minute),
		word("w-river", "川", "かわ", "河；河流", "川がきれいですね。", 5, 30*24*time.Hour, -96*time.Hour),
		word("w-train", "電車", "でんしゃ", "电车", "電車で行きます。", 1, 2*24*time.Hour, -120*time.Hour),
		sync.Record{Kind: sync.KindLine, ID: "l-see", Film: "f1", Fields: map[string]sync.Field{
			sync.FieldOriginal: field("See you."), sync.FieldTranslation: field("再见。"), sync.FieldTitle: field("Morning Talk"),
			sync.FieldStartMS: field(7000), sync.FieldEndMS: field(9000), sync.FieldSavedAt: field(now().Add(-2 * time.Hour)), sync.FieldReview: field(sync.Review{Box: 1, Due: now().Add(24 * time.Hour)}),
		}})
	if err := store.Open(dir).SaveLearning("gonedesk", "lib", phonestudy.Library{Records: records}); err != nil {
		t.Fatal(err)
	}
}

// writePoster writes a placeholder poster at path: a 16:9 picture, warm at its top and dusk blue at its bottom.
func writePoster(t *testing.T, path string) {
	t.Helper()
	const w, h = 480, 270
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		f := float64(y) / h
		c := color.RGBA{uint8(240 - 190*f), uint8(170 - 110*f), uint8(110 + 40*f), 255}
		for x := range w {
			img.Set(x, y, c)
		}
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := png.Encode(out, img); err != nil {
		t.Fatal(err)
	}
}

// learnScrollTo drags Learn up until what selector matches is near its top.
func (p *phone) learnScrollTo(selector string) {
	p.t.Helper()
	top := p.app.Find("id:learn-scroll").Frame.Origin.Y
	if d := p.app.Find(selector).Frame.Origin.Y - top - 16; d > 0 {
		p.app.Scroll("id:learn-scroll", d)
	}
	p.app.Advance(300 * time.Millisecond)
}

func TestScreenshotsLearnStudy(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	learnSample(t, dir)
	watchedFor(t, dir, 12, 18, 0, 26, 9, 15, 12)
	p := startPhone(t, dir)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDue(3) })
	shot(p, "learn-study")
	p.learnScrollTo("id:learn-vocabulary-section")
	shot(p, "learn-lists")
	p.learnScrollTo("id:learn-lines")
	shot(p, "saved-lines")
	p.app.Press("id:learn-word-w-morning")
	p.app.WaitFor("id:word-sheet", wait)
	// The sheet slides up over its transition.
	p.app.Advance(600 * time.Millisecond)
	shot(p, "learn-word")
}

// Paired with a computer whose library has nothing saved yet: the review card says so with OK, and the word list is its empty card.
func TestScreenshotsLearnEmpty(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-ok", wait)
	p.app.WaitFor("id:learn-empty", wait)
	shot(p, "learn-empty")
}

func TestScreenshotsReview(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	learnSample(t, dir)
	p := startPhone(t, dir)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-start", wait)
	p.app.Press("id:learn-review-start")
	p.app.WaitFor("id:review-card", wait)
	shot(p, "review-card")
	p.app.Press("id:review-show")
	p.app.WaitFor("id:review-answer", wait)
	// The card turns over in half a second.
	p.app.Advance(600 * time.Millisecond)
	shot(p, "review-answer")
	p.app.Press("id:review-known")
	p.app.Press("id:review-show")
	p.app.Press("id:review-again")
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.WaitFor("id:review-done", wait)
	// The bar fills over its transition.
	p.app.Advance(400 * time.Millisecond)
	shot(p, "review-done")
}

func TestScreenshotsSubtitleStyle(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-style", wait)
	p.app.Press("id:settings-style")
	p.app.WaitFor("id:style-preview", wait)
	p.app.Press("id:style-size-large")
	p.app.Press("id:style-color-cyan")
	shot(p, "settings-style")
}

func TestScreenshotsRevoked(t *testing.T) {
	shot := screenshots(t)
	d := studyDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	// Revoked before the phone starts, the first sync hears it.
	d.RevokeAll()
	p := startPhone(t, dir, secrets)
	p.app.WaitFor("id:revoked-page", wait)
	shot(p, "revoked")
}

func TestScreenshotsExport(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	learnSample(t, dir)
	p := startPhone(t, dir)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-export", wait)
	p.app.Press("id:learn-export")
	p.app.WaitUntil(wait, func() bool { return p.texts("id:export-count") == msgs.ExportCount(5, 2) })
	// The sheet slides up over its transition.
	p.app.Advance(600 * time.Millisecond)
	shot(p, "export")
}

func TestScreenshotsDictation(t *testing.T) {
	shot := screenshots(t)
	dir := t.TempDir()
	learnSample(t, dir)
	p := startPhone(t, dir, nsgotest.Environment(core.Environment{Locale: lang, Audio: true}))
	p.app.AudioDurations = map[string]time.Duration{filepath.Join(store.Open(dir).FilmDir("gonedesk", "f1"), "video.mp4"): 10 * time.Second}
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-review-start", wait)
	p.app.Press("id:learn-review-dictation")
	p.app.Press("id:learn-review-start")
	p.app.WaitFor("id:dictation-card", wait)
	shot(p, "dictation")
	p.app.Type("id:dictation-input", "おはよう")
	p.app.Press("id:dictation-check")
	p.app.WaitFor("id:dictation-result", wait)
	shot(p, "dictation-checked")
}

func TestScreenshotsComputer(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.SetService("OpenAI", "gpt-4.1-mini")
	// The computer has the user's storage set up, so its page says how the storage serves the phone.
	d.StartRelay(t, synctest.NewBucket(t, now), now, relayTurn)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	// On a target the app keeps its files apart from the test's, so the scene waits for what it shows rather than for the sync's files.
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-computer")
	p.app.WaitFor("id:computer-status", wait)
	shot(p, "computer")
	p.app.Scroll("id:computer-note", 2000)
	shot(p, "computer-bottom")
}

func TestScreenshotsSyncConflicts(t *testing.T) {
	shot := screenshots(t)
	d, key := conflictDesktop(t)
	p := openConflicts(t, d)
	p.app.WaitFor("id:conflict-"+key, wait)
	shot(p, "sync-conflicts")
	p.app.Press("id:conflict-both-" + key)
	p.app.WaitFor("id:conflicts-done", wait)
	shot(p, "sync-conflicts-done")
}
