package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/subexport"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// subexportOpen is "talk" open and loaded, its first line's translation corrected to 哈喽。 and its second line (Bye., untranslated) marked as no speech, then a third line added to its subtitles, untranslated; the dialog is not open yet.
func subexportOpen(t *testing.T) (*app, *nsgotest.App) {
	t.Helper()
	a, tt, updates, item := playerFilm(t, 12, nil)
	srt := talkSubtitles + "3\n00:00:07,000 --> 00:00:08,000\nAgain.\n\n"
	if err := os.WriteFile(item.SubtitlePath, []byte(srt), 0o644); err != nil {
		t.Fatal(err)
	}
	hello := "哈喽。"
	edits := []core.Correction{
		{Span: core.Span{Start: time.Second, End: 2500 * time.Millisecond}, Translation: &hello},
		{Span: core.Span{Start: 4 * time.Second, End: 5 * time.Second}, NoSpeech: true},
	}
	if err := media.SaveCorrections(media.CorrectionsPath(item.SubtitlePath), edits); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	return a, tt
}

// TestExportSubtitlesFromTheFileMenu opens 导出字幕… from the File menu, which is off with no film open, and exports to the film's folder: the user's correction is in the file, the line marked as no speech is not, the delay is in its times, and the dialog closes and says what it wrote.
func TestExportSubtitlesFromTheFileMenu(t *testing.T) {
	a, tt := subexportOpen(t)
	useMenuBar(t, a, tt)
	msgs := testlocale.Messages()
	a.player.film.delay = 500 * time.Millisecond
	refresh(tt, a)
	item := findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupFile(), msgs.SubexportMenu())
	if item.Disabled || item.Shortcut != "mod+e" {
		t.Fatalf("导出字幕… is %+v", item)
	}
	choose(tt, a, item)
	if !tt.Exists("id:export-dialog") || !a.modalOpen() {
		t.Fatalf("the dialog did not open: %q", tt.Texts())
	}
	wantText(t, tt, strings.Join([]string{msgs.SubexportCountLines(2), msgs.SubexportCountTranslated(1, 2), msgs.SubexportCountMissing(1, msgs.SubexportMissingSkip()), msgs.SubexportCountEdited(1), msgs.SubexportCountNoSpeech(1)}, " · "))
	wantText(t, tt, msgs.SubexportDelayAt(film.DelayTextIn(msgs, 500*time.Millisecond)))
	want := filepath.Join(filepath.Dir(a.player.subs.path), "talk.dual.srt")
	if got := a.SubexportPath(); got != want {
		t.Fatalf("exports to %q, want %q", got, want)
	}
	tt.Press("id:export-to-folder")
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if got, wantSRT := string(data), "1\n00:00:01,500 --> 00:00:03,000\n哈喽。\nHello.\n\n2\n00:00:07,500 --> 00:00:08,500\nAgain.\n"; got != wantSRT {
		t.Fatalf("exported\n%q\nwant\n%q", got, wantSRT)
	}
	if tt.Exists("id:export-dialog") || a.modalOpen() {
		t.Fatal("the dialog stayed open")
	}
	wantText(t, tt, msgs.SubexportDone("talk.dual.srt"))
	if srt, _ := os.ReadFile(a.player.subs.path); !strings.HasPrefix(string(srt), talkSubtitles) {
		t.Fatalf("the film's own subtitles changed: %s", srt)
	}

	tt.Press("id:film-back")
	if item := findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupFile(), msgs.SubexportMenu()); !item.Disabled {
		t.Fatal("导出字幕… is on with no film open")
	}
}

// TestExportSubtitlesChoices exports the translations alone as WebVTT without the delay, a placeholder for the line without one, through the dialog's controls, and checks the choice is saved for the next time.
func TestExportSubtitlesChoices(t *testing.T) {
	a, tt := subexportOpen(t)
	msgs := testlocale.Messages()
	a.player.film.delay = 500 * time.Millisecond
	refresh(tt, a)
	tt.Key("mod+e")
	if !tt.Exists("id:export-dialog") {
		t.Fatalf("⌘E did not open the dialog: %q", tt.Texts())
	}
	tt.Press("id:export-format-vtt")
	tt.Press("id:export-content-translation")
	if tt.Exists("id:export-order") {
		t.Fatal("the bilingual order shows for an export of the translations")
	}
	tt.Press("id:export-delay")
	a.SetSubexportChoice(func() subexport.Choice { c := a.SubexportChoice(); c.Missing = core.MissingPlaceholder; return c }())
	refresh(tt, a)
	tt.Press("id:export-to-folder")
	data, err := os.ReadFile(filepath.Join(filepath.Dir(a.player.subs.path), "talk.translation.vtt"))
	if err != nil {
		t.Fatal(err)
	}
	want := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.500\n哈喽。\n\n2\n00:00:07.000 --> 00:00:08.000\n" + msgs.SubexportPlaceholder() + "\n"
	if string(data) != want {
		t.Fatalf("exported\n%q\nwant\n%q", data, want)
	}
	var saved savedSettings
	a.areasSave(&saved)
	b := newApp(nil, func(fn func()) { fn() })
	b.areasRestore(saved)
	if got := b.SubexportChoice(); got != (subexport.Choice{Format: core.FormatVTT, Content: core.SubTranslation, Order: core.SubTranslationFirst, Missing: core.MissingPlaceholder}) {
		t.Fatalf("restored %+v", got)
	}
}

// TestExportSubtitlesSaveAs saves the export where the user chose, and puts back the film's own bilingual subtitles when the user saves over them.
func TestExportSubtitlesSaveAs(t *testing.T) {
	a, tt := subexportOpen(t)
	msgs := testlocale.Messages()
	own := a.player.subs.path
	before, _ := os.ReadFile(own)
	elsewhere := filepath.Join(t.TempDir(), "mine.srt")
	tt.SaveFile = func(r ns.SaveRequest) string {
		if r.Name != "talk.dual.srt" {
			t.Errorf("the panel suggests %q", r.Name)
		}
		return own
	}
	tt.Key("mod+e")
	tt.Press("id:export-save-as")
	if after, _ := os.ReadFile(own); string(after) != string(before) {
		t.Fatalf("saving over the film's subtitles left\n%s", after)
	}
	wantText(t, tt, msgs.SubexportKandoFile())
	if !tt.Exists("id:export-dialog") {
		t.Fatal("the dialog closed after a save it put back")
	}
	tt.SaveFile = func(ns.SaveRequest) string { return elsewhere }
	tt.Press("id:export-save-as")
	data, err := os.ReadFile(elsewhere)
	if err != nil || !strings.Contains(string(data), "哈喽。\nHello.") || strings.Contains(string(data), "Bye.") {
		t.Fatalf("saved %q, %v", data, err)
	}
	if tt.Exists("id:export-dialog") || a.modalOpen() {
		t.Fatal("the dialog stayed open")
	}
	wantText(t, tt, msgs.SubexportDone("mine.srt"))
}
