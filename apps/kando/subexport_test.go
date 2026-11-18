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

// TestExportSubtitlesSaveAsNewKandoName: a save under a name Kando keeps the film's subtitles under that is not there yet, its transcript or the one it keeps beside subtitles of the user's, is removed, so the export does not become the film's own subtitles.
func TestExportSubtitlesSaveAsNewKandoName(t *testing.T) {
	a, tt := subexportOpen(t)
	msgs := testlocale.Messages()
	dir := filepath.Dir(a.player.subs.path)
	tt.Key("mod+e")
	for _, name := range []string{"talk.srt", "talk.kando.srt"} {
		target := filepath.Join(dir, name)
		tt.SaveFile = func(ns.SaveRequest) string { return target }
		tt.Press("id:export-save-as")
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("%s: the export was kept: %v", name, err)
		}
		wantText(t, tt, msgs.SubexportKandoFile())
		if !tt.Exists("id:export-dialog") {
			t.Fatalf("%s: the dialog closed after a save it undid", name)
		}
	}
}

// TestExportSubtitlesSaveAsWhileKandoWrites: when another of the film's files changed while the panel was open, Kando was writing its subtitles, so a save over its bilingual subtitles is removed rather than an older copy of them put back.
func TestExportSubtitlesSaveAsWhileKandoWrites(t *testing.T) {
	a, tt := subexportOpen(t)
	msgs := testlocale.Messages()
	own := a.player.subs.path
	transcript := filepath.Join(filepath.Dir(own), "talk.srt")
	tt.SaveFile = func(ns.SaveRequest) string {
		if err := os.WriteFile(transcript, []byte("1\n00:00:01,000 --> 00:00:02,000\nNew.\n"), 0o644); err != nil {
			t.Error(err)
		}
		return own
	}
	tt.Key("mod+e")
	tt.Press("id:export-save-as")
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatalf("the export or an older copy was left over the film's subtitles: %v", err)
	}
	wantText(t, tt, msgs.SubexportKandoFileChanged())
}

// TestSubexportUndo: a save under one of the film's names is put back as it was when nothing else changed, removed while its job runs, and reports why when the file can be neither put back nor removed.
func TestSubexportUndo(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "v.mp4")
	bilingual := filepath.Join(dir, "v.bilingual.srt")
	if err := os.WriteFile(bilingual, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	kept := subexportKeep(video, bilingual)
	if ours, _, _ := subexportUndo(kept, video, filepath.Join(dir, "v.dual.srt"), false); ours {
		t.Fatal("a save under the export's own name was undone")
	}
	os.WriteFile(bilingual, []byte("export"), 0o644)
	if ours, changed, err := subexportUndo(kept, video, bilingual, false); !ours || changed || err != nil {
		t.Fatalf("put back = %v, %v, %v", ours, changed, err)
	}
	if data, _ := os.ReadFile(bilingual); string(data) != "old" {
		t.Fatalf("put back %q", data)
	}
	os.WriteFile(bilingual, []byte("export"), 0o644)
	if ours, changed, err := subexportUndo(kept, video, bilingual, true); !ours || !changed || err != nil {
		t.Fatalf("while busy = %v, %v, %v", ours, changed, err)
	}
	if _, err := os.Stat(bilingual); !os.IsNotExist(err) {
		t.Fatalf("while busy, the file was left: %v", err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only folder")
	}
	os.WriteFile(bilingual, []byte("export"), 0o644)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	if ours, _, err := subexportUndo(kept, video, bilingual, false); !ours || err == nil {
		t.Fatalf("in a read-only folder = %v, %v, want an error", ours, err)
	}
}

// TestExportSubtitlesNothingToWrite: an export that would write no cue, the translations of lines none of which is translated with a missing one left out, cannot be saved.
func TestExportSubtitlesNothingToWrite(t *testing.T) {
	a, tt := subexportOpen(t)
	a.player.film.delay = -3 * time.Second
	a.SetSubexportChoice(subexport.Choice{Format: core.FormatSRT, Content: core.SubTranslation, Order: core.SubTranslationFirst, Missing: core.MissingSkip, Delay: true})
	refresh(tt, a)
	tt.Key("mod+e")
	if tt.Enabled("id:export-to-folder") || tt.Enabled("id:export-save-as") {
		t.Fatal("an export with no cue can be written")
	}
	a.SetSubexportChoice(subexport.Choice{Format: core.FormatSRT, Content: core.SubOriginal, Order: core.SubTranslationFirst, Missing: core.MissingSkip, Delay: true})
	refresh(tt, a)
	if !tt.Enabled("id:export-to-folder") || !tt.Enabled("id:export-save-as") {
		t.Fatal("an export of the originals cannot be written")
	}
}
