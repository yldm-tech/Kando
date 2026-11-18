package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/lines"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/remote"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// correctOpen is "talk" open and loaded, held at its first line (1 to 2.5 seconds: 你好。 over Hello.; then Bye. alone at 4 to 5 seconds), and the path of its corrections file.
func correctOpen(t *testing.T) (*app, *nsgotest.App, string) {
	t.Helper()
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	return a, tt, media.CorrectionsPath(a.player.subs.path)
}

// correctTexts are the open film's lines as the player shows them.
func correctTexts(a *app) []string {
	var out []string
	for _, c := range a.playerCues() {
		out = append(out, c.Text)
	}
	return out
}

// TestEditingALineKeepsItApartFromTheSubtitles selects the playing line with Return, edits its translation, and checks that the edit shows, is kept in the film's corrections file rather than in its subtitles, and that Undo and Redo, named in the Edit menu, take it back and make it again.
func TestEditingALineKeepsItApartFromTheSubtitles(t *testing.T) {
	a, tt, sidecar := correctOpen(t)
	useMenuBar(t, a, tt)
	msgs := testlocale.Messages()
	tt.Key("enter")
	if a.correctSelected() != 0 || a.player.play.Playing() || !tt.Exists("id:film-correct-bar") {
		t.Fatalf("Return selected %d, playing %v, bar %v; %q", a.correctSelected(), a.player.play.Playing(), tt.Exists("id:film-correct-bar"), tt.Texts())
	}
	tt.Key("enter")
	if !tt.Exists("id:film-correct-editor") {
		t.Fatal("Return on the selected line did not edit it")
	}
	if got := tt.Find("id:film-correct-translation").Text; got != "你好。" {
		t.Fatalf("the field holds %q", got)
	}
	tt.Clear("id:film-correct-translation")
	tt.Type("id:film-correct-translation", "哈喽。")
	tt.Press("id:film-correct-submit")
	if got := correctTexts(a); got[0] != "哈喽。\nHello." {
		t.Fatalf("lines %q", got)
	}
	wantText(t, tt, msgs.CorrectSaved())
	if a.correctSelected() >= 0 || !a.player.play.Playing() {
		t.Fatal("submitting kept the line selected or the film held")
	}
	srt, _ := os.ReadFile(a.player.subs.path)
	if string(srt) != talkSubtitles {
		t.Fatalf("the subtitles were written: %s", srt)
	}
	edits, err := media.LoadCorrections(sidecar)
	if err != nil || len(edits) != 1 || edits[0].Translation == nil || *edits[0].Translation != "哈喽。" || edits[0].Original != nil {
		t.Fatalf("corrections %+v, %v", edits, err)
	}
	undo := msgs.CorrectUndo(msgs.CorrectStepEdit(1))
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupEdit(), undo))
	if got := correctTexts(a); got[0] != "你好。\nHello." {
		t.Fatalf("after Undo %q", got)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("Undo of the only correction left its file: %v", err)
	}
	wantText(t, tt, msgs.CorrectUndone(msgs.CorrectStepEdit(1)))
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupEdit(), msgs.CorrectRedo(msgs.CorrectStepEdit(1))))
	if got := correctTexts(a); got[0] != "哈喽。\nHello." {
		t.Fatalf("after Redo %q", got)
	}
	findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupEdit(), undo)
	findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupEdit(), msgs.MenuRedo())
}

// TestTheSelectedLineMovesAndGoes checks the selection's keys: ⇧→ selects the next line, ⌃⌥← and ⌃⌥⇧→ move its start and end, ⌘⌫ marks it as no speech, which leaves it out of the film until 恢复标为无语音的句子 brings it back, and Escape lets go of a line without leaving the film.
func TestTheSelectedLineMovesAndGoes(t *testing.T) {
	a, tt, sidecar := correctOpen(t)
	msgs := testlocale.Messages()
	tt.Key("enter")
	tt.Key("shift+right")
	if a.correctSelected() != 1 {
		t.Fatalf("⇧→ selected %d", a.correctSelected())
	}
	tt.Key("ctrl+alt+left")
	tt.Key("ctrl+alt+left")
	tt.Key("ctrl+alt+shift+right")
	cue := a.playerCues()[1]
	if cue.Start != 3800*time.Millisecond || cue.End != 5100*time.Millisecond {
		t.Fatalf("moved to %v–%v", cue.Start, cue.End)
	}
	wantText(t, tt, msgs.CorrectEndAt("0:05.1"))
	if a.correct.undo[len(a.correct.undo)-1].kind != correctKindEnd || len(a.correct.undo) != 3 {
		t.Fatalf("steps %+v", a.correct.undo)
	}
	edits, _ := media.LoadCorrections(sidecar)
	if len(edits) != 1 || edits[0].StartShift != -200*time.Millisecond || edits[0].EndShift != 100*time.Millisecond || edits[0].Span != (core.Span{Start: 4 * time.Second, End: 5 * time.Second}) {
		t.Fatalf("corrections %+v", edits)
	}
	tt.Key("escape")
	if a.correctSelected() >= 0 || a.openKey == "" {
		t.Fatalf("Escape: selected %d, open %q", a.correctSelected(), a.openKey)
	}
	tt.Key("enter")
	tt.Key("mod+backspace")
	if got := correctTexts(a); len(got) != 1 {
		t.Fatalf("lines after no speech %q", got)
	}
	if a.correctNoSpeechCount() != 1 {
		t.Fatal("no line counts as no speech")
	}
	a.correctRestoreNoSpeech()
	refresh(tt, a)
	if got := correctTexts(a); len(got) != 2 {
		t.Fatalf("lines after restoring %q", got)
	}
	wantText(t, tt, msgs.CorrectRestored(1))
}

// TestCorrectionsSurviveTheSubtitlesWrittenAgain checks that a line the user corrected keeps the correction once a job writes the film's subtitles again, as translating it again does, and that the control socket, MCP and the phone, the palette's lines and the player all read it.
func TestCorrectionsSurviveTheSubtitlesWrittenAgain(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	original := "Hi."
	sidecar := media.CorrectionsPath(item.SubtitlePath)
	if err := media.SaveCorrections(sidecar, []core.Correction{
		{Span: core.Span{Start: time.Second, End: 2500 * time.Millisecond}, Original: &original},
		{Span: core.Span{Start: 4 * time.Second, End: 5 * time.Second}, NoSpeech: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(item.SubtitlePath, []byte("1\n00:00:01,000 --> 00:00:02,500\n嗨。\nHello.\n\n2\n00:00:04,000 --> 00:00:05,000\n再见。\nBye.\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	if got := correctTexts(a); len(got) != 1 || got[0] != "嗨。\nHi." {
		t.Fatalf("the player shows %q", got)
	}
	cues, err := remoteFilmCues(remote.Film{Title: "talk", Subtitles: item.SubtitlePath})
	if err != nil || len(cues) != 1 || cues[0].Text != "Hi." || cues[0].Translation != "嗨。" {
		t.Fatalf("the socket reads %+v, %v", cues, err)
	}
	read, err := lines.Read(item.SubtitlePath)
	if err != nil || len(read) != 1 || read[0].Original != "Hi." {
		t.Fatalf("the palette reads %+v, %v", read, err)
	}
}

// TestCorrectingNeedsAReadableCorrectionsFile checks that a corrections file that cannot be read is not written over: the lines show as the machine wrote them and a correction says why it cannot be kept.
func TestCorrectingNeedsAReadableCorrectionsFile(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	sidecar := media.CorrectionsPath(item.SubtitlePath)
	if err := os.WriteFile(sidecar, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("enter")
	tt.Key("mod+backspace")
	wantText(t, tt, testlocale.Messages().CorrectUnreadable())
	if data, _ := os.ReadFile(sidecar); string(data) != "{not json" {
		t.Fatalf("the file was written over: %s", data)
	}
	if len(a.playerCues()) != 2 {
		t.Fatal("a line went")
	}
}

// TestCorrectionsMoveWithTheFilm checks that a film's corrections go where its video goes.
func TestCorrectionsMoveWithTheFilm(t *testing.T) {
	_, _, _, item := playerFilm(t, 12, nil)
	sidecar := media.CorrectionsPath(item.VideoPath)
	if sidecar != media.CorrectionsPath(item.SubtitlePath) || filepath.Dir(sidecar) != filepath.Dir(item.VideoPath) {
		t.Fatalf("the video names %s, its subtitles %s", sidecar, media.CorrectionsPath(item.SubtitlePath))
	}
	if err := media.SaveCorrections(sidecar, []core.Correction{{Span: core.Span{Start: time.Second, End: 2 * time.Second}, NoSpeech: true}}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range media.Companions(item.VideoPath) {
		found = found || p == sidecar
	}
	if !found {
		t.Fatal("the corrections are not among the film's files")
	}
}

var _ film.Corrector = (*app)(nil)
