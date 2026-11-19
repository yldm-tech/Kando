package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/lines"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
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

// TestEditingALineKeepsItApartFromTheSubtitles selects the playing line with E, edits its translation, and checks that the edit shows, is kept in the film's corrections file rather than in its subtitles, and that Undo and Redo, named in the Edit menu, take it back and make it again.
func TestEditingALineKeepsItApartFromTheSubtitles(t *testing.T) {
	a, tt, sidecar := correctOpen(t)
	useMenuBar(t, a, tt)
	msgs := testlocale.Messages()
	tt.Key("e")
	if a.correctSelected() != 0 || a.player.play.Playing() || !tt.Exists("id:film-correct-bar") {
		t.Fatalf("E selected %d, playing %v, bar %v; %q", a.correctSelected(), a.player.play.Playing(), tt.Exists("id:film-correct-bar"), tt.Texts())
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
	if q := a.lib.quotes[a.player.subs.path]; q.translation != "哈喽。" {
		t.Fatalf("the library hero quotes %+v", q)
	}
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
	if edits, err := media.LoadCorrections(sidecar); err != nil || len(edits) != 0 {
		t.Fatalf("Undo of the only correction left %+v, %v", edits, err)
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
	tt.Key("e")
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
	tt.Key("e")
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
	tt.Key("e")
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

// TestAShortLineMovesOnlyTheWayPressed checks that on a line already shorter than correctNudgeShortest, as whisper writes for "Yes.", the keys that would shorten it leave it as it is rather than moving its start earlier or its end later, and the keys that lengthen it still work.
func TestAShortLineMovesOnlyTheWayPressed(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	if err := media.SaveCorrections(media.CorrectionsPath(item.SubtitlePath), []core.Correction{{Span: core.Span{Start: time.Second, End: 2500 * time.Millisecond}, EndShift: -1300 * time.Millisecond}}); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1100*time.Millisecond)
	tt.Key("e")
	if a.correctSelected() != 0 {
		t.Fatalf("E selected %d", a.correctSelected())
	}
	tt.Key("ctrl+alt+right")
	tt.Key("ctrl+alt+shift+left")
	if cue := a.playerCues()[0]; cue.Start != time.Second || cue.End != 1200*time.Millisecond || len(a.correct.undo) != 0 {
		t.Fatalf("shortening a 200ms line moved it to %v–%v, steps %d", cue.Start, cue.End, len(a.correct.undo))
	}
	tt.Key("ctrl+alt+left")
	tt.Key("ctrl+alt+shift+right")
	if cue := a.playerCues()[0]; cue.Start != 900*time.Millisecond || cue.End != 1300*time.Millisecond {
		t.Fatalf("lengthening it moved it to %v–%v", cue.Start, cue.End)
	}
}

// TestEscapeInTheFieldsPutsThemAway checks, with the app's own keys bound, that Escape while the fields that edit a line show puts them away and keeps the line selected, a second lets go of the line, and only a third leaves the film.
func TestEscapeInTheFieldsPutsThemAway(t *testing.T) {
	a, tt, _ := correctOpen(t)
	tt.Key("e")
	tt.Key("enter")
	if !tt.Exists("id:film-correct-editor") {
		t.Fatal("Return on the selected line did not edit it")
	}
	tt.Key("escape")
	if tt.Exists("id:film-correct-editor") || a.correctSelected() != 0 || a.openKey != "talk" {
		t.Fatalf("Escape in the fields: editor %v, selected %d, open %q", tt.Exists("id:film-correct-editor"), a.correctSelected(), a.openKey)
	}
	tt.Key("escape")
	if a.correctSelected() >= 0 || a.openKey != "talk" {
		t.Fatalf("Escape on the selection: selected %d, open %q", a.correctSelected(), a.openKey)
	}
	tt.Key("escape")
	if a.openKey != "" {
		t.Fatalf("Escape with nothing selected kept %q open", a.openKey)
	}
}

// TestAnEmptyOriginalIsNotKept checks that submitting a line with its original field empty keeps nothing and says why, as a cue holding only its translation would read back as an original.
func TestAnEmptyOriginalIsNotKept(t *testing.T) {
	a, tt, sidecar := correctOpen(t)
	tt.Key("e")
	tt.Key("enter")
	tt.Clear("id:film-correct-original")
	tt.Press("id:film-correct-submit")
	wantText(t, tt, testlocale.Messages().CorrectOriginalEmpty())
	if !tt.Exists("id:film-correct-editor") {
		t.Fatal("the fields went away")
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("a correction was kept: %v", err)
	}
	if got := correctTexts(a); got[0] != "你好。\nHello." {
		t.Fatalf("lines %q", got)
	}
}

// TestUndoWaitsForTheSubtitlesRead checks that while the open film's subtitles are read again, as after a job writes them, Undo neither runs nor writes the corrections file, which the read would then put back as it was before, and that it works again once they are read.
func TestUndoWaitsForTheSubtitlesRead(t *testing.T) {
	a, tt, updates := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	sidecar := media.CorrectionsPath(a.player.subs.path)
	tt.Key("e")
	tt.Key("ctrl+alt+left")
	if edits, _ := media.LoadCorrections(sidecar); len(edits) != 1 {
		t.Fatalf("corrections %+v", edits)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(a.player.subs.path, later, later); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, a.reloadOpenSubtitles)
	if a.player.subs.ready {
		t.Fatal("the subtitles were read at once")
	}
	if a.correctCanUndo() {
		t.Fatal("Undo is offered while the subtitles are read")
	}
	a.correctUndo()
	if edits, _ := media.LoadCorrections(sidecar); len(edits) != 1 {
		t.Fatalf("Undo wrote the corrections while the subtitles were read: %+v", edits)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	if !a.correctCanUndo() {
		t.Fatal("Undo is not offered once the subtitles are read")
	}
	apply(tt, a, a.correctUndo)
	if edits, _ := media.LoadCorrections(sidecar); len(edits) != 0 {
		t.Fatalf("Undo left %+v", edits)
	}
}

var _ film.Corrector = (*app)(nil)

// correctNames are the lines of a film whose translation writes the name John as 约翰 in three lines, one of which the user corrected already, and whose original misspells it as Jon in two.
const correctNames = "1\n00:00:01,000 --> 00:00:02,500\n约翰来了。\nJon is here.\n\n2\n00:00:04,000 --> 00:00:05,000\n约翰，等等。\nJon, wait.\n\n3\n00:00:06,000 --> 00:00:07,000\n这是约翰的书。\nThis is John's book.\n\n4\n00:00:08,000 --> 00:00:09,000\n乔纳森走了。\nJonathan left.\n\n"

// correctNamesOpen is the film of correctNames open and loaded, held on its first line, its third line's translation corrected to 这是约翰的。, translated by the AI service of tools, and the path of its corrections file.
func correctNamesOpen(t *testing.T, p media.Pipeline) (*app, *nsgotest.App, chan func(), string) {
	t.Helper()
	a, tt, updates, item := playerFilm(t, 12, p)
	if err := os.WriteFile(item.SubtitlePath, []byte(correctNames), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecar := media.CorrectionsPath(item.SubtitlePath)
	edited := "这是约翰的。"
	if err := media.SaveCorrections(sidecar, []core.Correction{{Span: core.Span{Start: 6 * time.Second, End: 7 * time.Second}, Translation: &edited}}); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	return a, tt, updates, sidecar
}

// correctEdit edits the translation of line i of the open film, selected with E, to translation, as typed in its field and saved.
func correctEdit(t *testing.T, a *app, tt *nsgotest.App, i int, translation string) {
	t.Helper()
	apply(tt, a, func() { a.SelectLine(i) })
	tt.Key("enter")
	tt.Clear("id:film-correct-translation")
	tt.Type("id:film-correct-translation", translation)
	tt.Press("id:film-correct-submit")
}

// TestShortcutsFollowTheDesign checks the keys the prototype gives correcting a line: E selects the line playing, and Return no longer does; I explains the line selected in the side panel, or with none the one playing; ⇧E asks for other translations of it.
func TestShortcutsFollowTheDesign(t *testing.T) {
	tools, asks := explainLLM(t)
	a, tt, updates, _ := correctNamesOpen(t, tools)
	tt.Key("enter")
	if a.correctSelected() >= 0 {
		t.Fatal("Return selected a line")
	}
	tt.Key("e")
	if a.correctSelected() != 0 {
		t.Fatalf("E selected %d", a.correctSelected())
	}
	apply(tt, a, func() { a.StepSelection(1) })
	tt.Key("i")
	waitUntil(t, tt, a, updates, func() bool { return a.explain.line.open && !a.explain.line.busy })
	if sent := strings.Join(asks(), "\n"); a.explain.line.index != 1 || a.player.filmSide != film.SideExplain || !strings.Contains(sent, "要讲解的一句：[0:04] Jon, wait.") {
		t.Fatalf("I explained line %d in side %q, sent %q, want the selected line", a.explain.line.index, a.player.filmSide, sent)
	}
	rows := map[string]string{}
	for _, s := range a.shortcuts(a.cx) {
		if s.Group == groupSubtitles {
			rows[s.Key] = s.Label
		}
	}
	msgs := testlocale.Messages()
	if rows["e"] != msgs.CorrectKeySelect() || rows["shift+e"] != msgs.CorrectKeyCandidates() || rows["i"] != msgs.PaletteExplainLine() {
		t.Fatalf("subtitle rows %q", rows)
	}
}

// TestThisLineIsWrongOffersCandidates checks 这句不对: ⇧E on the line playing asks the AI service for other translations of it, sending the line with the lines around it and the glossary, holds the film and shows them under the line as it is, the first highlighted; ↓ and ↑ move the highlight, Escape puts them away with the line unchanged, and Return keeps the one highlighted as the user's correction, which Undo names and takes back.
func TestThisLineIsWrongOffersCandidates(t *testing.T) {
	srv, seen := fakeLLM(t, `["约翰到了。", "约翰在这儿。"]`)
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	a, tt, updates, sidecar := correctNamesOpen(t, tools)
	useMenuBar(t, a, tt)
	msgs := testlocale.Messages()
	tt.Key("shift+e")
	if a.correctSelected() != 0 || !tt.Exists("id:film-candidates") || a.player.play.Playing() {
		t.Fatalf("⇧E: selected %d, list %v", a.correctSelected(), tt.Exists("id:film-candidates"))
	}
	wantText(t, tt, msgs.CorrectCandidatesBusy())
	waitUntil(t, tt, a, updates, func() bool { return a.Candidates().Open && !a.Candidates().Busy })
	sent := strings.Join(*seen, "\n")
	for _, want := range []string{"Jon is here.", "约翰来了。", "Jon, wait.", "简体中文"} {
		if !strings.Contains(sent, want) {
			t.Fatalf("the request lacks %q: %s", want, sent)
		}
	}
	for _, want := range []string{msgs.CorrectCandidatesTitle(), "约翰来了。", "约翰到了。", "约翰在这儿。"} {
		wantText(t, tt, want)
	}
	if c := a.Candidates(); c.At != 1 || len(c.Options) != 2 {
		t.Fatalf("candidates %+v", c)
	}
	tt.Key("down")
	tt.Key("down")
	tt.Key("up")
	tt.Key("up")
	tt.Key("up")
	if c := a.Candidates(); c.At != 0 {
		t.Fatalf("↓↓↑↑↑ highlighted %d", c.At)
	}
	tt.Key("escape")
	if tt.Exists("id:film-candidates") || a.correctSelected() != 0 || a.openKey == "" {
		t.Fatalf("Escape: list %v, selected %d, open %q", tt.Exists("id:film-candidates"), a.correctSelected(), a.openKey)
	}
	tt.Key("shift+e")
	waitUntil(t, tt, a, updates, func() bool { return a.Candidates().Open && !a.Candidates().Busy })
	tt.Key("down")
	tt.Key("enter")
	if got := correctTexts(a)[0]; got != "约翰在这儿。\nJon is here." {
		t.Fatalf("Return kept %q", got)
	}
	if tt.Exists("id:film-candidates") || a.correctSelected() != 0 {
		t.Fatal("the list stayed, or the line was let go")
	}
	wantText(t, tt, msgs.CorrectCandidateKept())
	edits, _ := media.LoadCorrections(sidecar)
	if len(edits) != 2 || edits[1].Translation == nil || *edits[1].Translation != "约翰在这儿。" {
		t.Fatalf("corrections %+v", edits)
	}
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupEdit(), msgs.CorrectUndo(msgs.CorrectStepCandidate(1))))
	if got := correctTexts(a)[0]; got != "约翰来了。\nJon is here." {
		t.Fatalf("Undo left %q", got)
	}
}

// TestThisLineIsWrongNeedsAnAIService checks that 这句不对 without an AI service says so and offers its settings, and that Return then puts the list away.
func TestThisLineIsWrongNeedsAnAIService(t *testing.T) {
	a, tt, _, _ := correctNamesOpen(t, nil)
	tt.Key("shift+e")
	wantText(t, tt, testlocale.Messages().CorrectCandidatesNoAI())
	if !tt.Exists("id:film-candidates-settings") {
		t.Fatal("no settings offered")
	}
	tt.Key("enter")
	if tt.Exists("id:film-candidates") || len(a.correct.undo) != 0 {
		t.Fatal("Return did not put the list away, or changed the line")
	}
}

// TestAFixSpreadsAcrossTheFilm checks that an edit changing words other lines hold asks whether to change them too, naming how many and how many of those the user corrected; that ⌘↩ changes them all, as one change Undo takes back whole, and the glossary term that wrote the words, which it locks; and that Escape leaves them, keeping the film open.
func TestAFixSpreadsAcrossTheFilm(t *testing.T) {
	a, tt, updates, sidecar := correctNamesOpen(t, nil)
	useMenuBar(t, a, tt)
	msgs := testlocale.Messages()
	waitUntil(t, tt, a, updates, func() bool { return a.gloss.filmRead })
	apply(tt, a, func() {
		if !a.glossaryPut(core.Term{Source: "John", Target: "约翰", Scope: core.ScopeFilm, Origin: core.OriginAuto}) {
			t.Fatal("the term was not kept")
		}
	})
	correctEdit(t, a, tt, 0, "琼恩来了。")
	if want := msgs.CorrectSpreadAskEdited("约翰", "琼恩", 2, 1); !hasText(tt, want) {
		t.Fatalf("no %q in %q", want, tt.Texts())
	}
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupSubtitles(), msgs.CorrectKeySpreadAll()))
	want := []string{"琼恩来了。\nJon is here.", "琼恩，等等。\nJon, wait.", "这是琼恩的。\nThis is John's book.", "乔纳森走了。\nJonathan left."}
	if got := correctTexts(a); !slices.Equal(got, want) {
		t.Fatalf("lines %q, want %q", got, want)
	}
	wantText(t, tt, msgs.CorrectSpreadDoneTerm(2))
	if term, ok := a.glossaryTerm("John"); !ok || term.Target != "琼恩" || !term.Locked || term.Origin != core.OriginUser {
		t.Fatalf("term %+v", term)
	}
	edits, _ := media.LoadCorrections(sidecar)
	if len(edits) != 3 {
		t.Fatalf("corrections %+v", edits)
	}
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupEdit(), msgs.CorrectUndo(msgs.CorrectStepSpread("约翰", "琼恩"))))
	if got := correctTexts(a); got[1] != "约翰，等等。\nJon, wait." || got[2] != "这是约翰的。\nThis is John's book." || got[0] != "琼恩来了。\nJon is here." {
		t.Fatalf("Undo left %q", got)
	}
	if term, _ := a.glossaryTerm("John"); term.Target != "约翰" || term.Locked {
		t.Fatalf("Undo left the term %+v", term)
	}
	// The original: Jon to John, which Jonathan does not hold.
	apply(tt, a, func() { a.SelectLine(1) })
	tt.Key("enter")
	tt.Clear("id:film-correct-original")
	tt.Type("id:film-correct-original", "John, wait.")
	tt.Press("id:film-correct-submit")
	if want := msgs.CorrectSpreadAsk("Jon", "John", 1); !hasText(tt, want) {
		t.Fatalf("no %q in %q", want, tt.Texts())
	}
	tt.Key("escape")
	wantText(t, tt, msgs.CorrectSpreadOnly())
	if a.openKey == "" || a.correct.spread != nil {
		t.Fatalf("Escape: open %q, bar %v", a.openKey, a.correct.spread != nil)
	}
	if got := correctTexts(a)[0]; got != "琼恩来了。\nJon is here." {
		t.Fatalf("Escape changed %q", got)
	}
}

// correctJon are the lines of a film whose originals name Jon and never John, his name alone being the original of its second line, and whose translations write him 约翰.
const correctJon = "1\n00:00:01,000 --> 00:00:02,500\n约翰，等等。\nJon, wait.\n\n2\n00:00:04,000 --> 00:00:05,000\n约翰。\nJon\n\n3\n00:00:06,000 --> 00:00:07,000\n问约翰。\nAsk Jon.\n\n"

// correctJonOpen is the film of correctJon open and loaded, held on its first line.
func correctJonOpen(t *testing.T) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	a, tt, updates, item := playerFilm(t, 12, nil)
	if err := os.WriteFile(item.SubtitlePath, []byte(correctJon), 0o644); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	return a, tt, updates
}

// TestAFixTakingANameOutOfTheOriginalsLeavesNoneEmpty checks that an edit taking a name out of an original offers to take it out of the other originals that hold it, but not of one that is the name alone, which would be left empty and read back as a translation; that ⌘↩ takes it out with the space before it; and that Undo puts it back.
func TestAFixTakingANameOutOfTheOriginalsLeavesNoneEmpty(t *testing.T) {
	a, tt, _ := correctJonOpen(t)
	useMenuBar(t, a, tt)
	msgs := testlocale.Messages()
	apply(tt, a, func() { a.SelectLine(0) })
	tt.Key("enter")
	tt.Clear("id:film-correct-original")
	tt.Type("id:film-correct-original", ", wait.")
	tt.Press("id:film-correct-submit")
	if s := a.Spread(); !s.Open || s.From != "Jon" || s.To != "" || s.Lines != 1 {
		t.Fatalf("spread %+v", s)
	}
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupSubtitles(), msgs.CorrectKeySpreadAll()))
	want := []string{"约翰，等等。\n, wait.", "约翰。\nJon", "问约翰。\nAsk."}
	if got := correctTexts(a); !slices.Equal(got, want) {
		t.Fatalf("lines %q, want %q", got, want)
	}
	wantText(t, tt, msgs.CorrectSpreadDone(1))
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupEdit(), msgs.CorrectUndo(msgs.CorrectStepSpread("Jon", ""))))
	// Undo takes back the spread alone: the first line keeps its own edit, made before it.
	if got := correctTexts(a); got[2] != "问约翰。\nAsk Jon." || got[0] != "约翰，等等。\n, wait." {
		t.Fatalf("Undo left %q", got)
	}
}

// TestAFixLeavesATermForOtherWordsAlone checks that a fix of a translation spread across the film does not change a glossary term that writes the same translation for words the film's lines do not hold: every film's term for John is left as it is when the film's Jon, also written 约翰, becomes 琼恩.
func TestAFixLeavesATermForOtherWordsAlone(t *testing.T) {
	a, tt, updates := correctJonOpen(t)
	useMenuBar(t, a, tt)
	msgs := testlocale.Messages()
	waitUntil(t, tt, a, updates, func() bool { return a.gloss.filmRead })
	apply(tt, a, func() {
		a.gloss.store.Set("", core.Term{Source: "John", Target: "约翰", Scope: core.ScopeGlobal, Origin: core.OriginAuto})
	})
	correctEdit(t, a, tt, 0, "琼恩，等等。")
	if s := a.Spread(); !s.Open || s.Lines != 2 {
		t.Fatalf("spread %+v", s)
	}
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupSubtitles(), msgs.CorrectKeySpreadAll()))
	want := []string{"琼恩，等等。\nJon, wait.", "琼恩。\nJon", "问琼恩。\nAsk Jon."}
	if got := correctTexts(a); !slices.Equal(got, want) {
		t.Fatalf("lines %q, want %q", got, want)
	}
	wantText(t, tt, msgs.CorrectSpreadDone(2))
	if term, ok := a.glossaryTerm("John"); !ok || term.Target != "约翰" || term.Locked || term.Scope != core.ScopeGlobal {
		t.Fatalf("term %+v, %v", term, ok)
	}
}

// TestAFixOfAPackTermIsTheFilms checks that a fix spread across the film over the translation a glossary pack's term wrote gives the film a term of its own, as adding one on the 术语 tab does (AddTerm), not every film's, which the pack's term is (pluginTerms): glossary.json is left alone.
func TestAFixOfAPackTermIsTheFilms(t *testing.T) {
	a, tt, updates := correctJonOpen(t)
	useMenuBar(t, a, tt)
	msgs := testlocale.Messages()
	waitUntil(t, tt, a, updates, func() bool { return a.gloss.filmRead })
	apply(tt, a, func() {
		a.plugins.packs = []plugins.Pack{{Manifest: plugins.Manifest{ID: "example.names", Kind: plugins.KindGlossary, Glossary: &plugins.Glossary{From: "en", To: "zh-Hans", Terms: []plugins.Term{{Source: "Jon", Target: "约翰"}}}}}}
		a.plugins.order = []string{"example.names"}
	})
	if term, ok := a.glossaryTerm("Jon"); ok {
		t.Fatalf("the pack's term was the film's: %+v", term)
	}
	correctEdit(t, a, tt, 0, "琼恩，等等。")
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupSubtitles(), msgs.CorrectKeySpreadAll()))
	wantText(t, tt, msgs.CorrectSpreadDoneTerm(2))
	if term, ok := a.glossaryTerm("Jon"); !ok || term.Target != "琼恩" || !term.Locked || term.Origin != core.OriginUser || term.Scope != core.ScopeFilm {
		t.Fatalf("the user's term %+v, %v", term, ok)
	}
	if _, global := a.gloss.store.Terms(""); len(global) != 0 {
		t.Fatalf("the fix went into every film's terms: %+v", global)
	}
}

// TestTheSpreadBarGoesAfterAWhile checks that the bar asking about spreading a fix goes by itself once correctSpreadFor passes unanswered, leaving the other lines as they are.
func TestTheSpreadBarGoesAfterAWhile(t *testing.T) {
	a, tt, _ := correctJonOpen(t)
	correctEdit(t, a, tt, 0, "琼恩，等等。")
	if !a.Spread().Open {
		t.Fatal("no bar asked about the fix")
	}
	tt.Advance(correctSpreadFor - time.Second)
	refresh(tt, a)
	if !a.Spread().Open {
		t.Fatal("the bar went too soon")
	}
	tt.Advance(2 * time.Second)
	refresh(tt, a)
	if a.Spread().Open || a.correct.spread != nil {
		t.Fatal("the bar stayed")
	}
	if got := correctTexts(a)[1]; got != "约翰。\nJon" {
		t.Fatalf("the bar going changed %q", got)
	}
}

// TestClosingTheCandidatesStopsAskingForThem checks that Escape while the other translations of a line are on their way stops the request, and that its reply, coming after the list was opened again, does not show in the new one, which shows its own reply.
func TestClosingTheCandidatesStopsAskingForThem(t *testing.T) {
	var asked atomic.Int32
	arrived, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The body read, the server sees the client go (http.Request.Context).
		io.Copy(io.Discard, r.Body)
		if asked.Add(1) == 1 {
			close(arrived)
			select {
			case <-r.Context().Done():
				close(canceled)
			case <-release:
			}
			return
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": `["约翰到了。"]`}}}})
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	a, tt, updates, _ := correctNamesOpen(t, tools)
	tt.Key("shift+e")
	if c := a.Candidates(); !c.Open || !c.Busy {
		t.Fatalf("⇧E: %+v", c)
	}
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("no request for the candidates")
	}
	tt.Key("escape")
	if a.Candidates().Open || a.correct.pick != nil {
		t.Fatal("Escape left the list")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the request went on after the list closed")
	}
	tt.Key("shift+e")
	// The first request's reply, its cancellation, reaches the engine while the second list is open.
	for wait := time.After(300 * time.Millisecond); ; {
		select {
		case fn := <-updates:
			apply(tt, a, fn)
			continue
		case <-wait:
		}
		break
	}
	if c := a.Candidates(); !c.Open || !c.Busy || c.Say != nil || c.NeedsAI {
		t.Fatalf("the first reply reached the second list: %+v", c)
	}
	close(release)
	waitUntil(t, tt, a, updates, func() bool { return !a.Candidates().Busy })
	if c := a.Candidates(); !slices.Equal(c.Options, []string{"约翰到了。"}) || c.Say != nil {
		t.Fatalf("candidates %+v", c)
	}
}
