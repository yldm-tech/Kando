package main

import (
	"strings"
	"testing"
	"time"

	"kando/packages/sync"
)

// A line explained offers to fix its translation: the fix shows on the card at once and goes to the computer as a correction of the line; with 同时加进术语表 on, the words the learner names go into the film's glossary on the computer, locked, and the Info tab lists the glossary as the computer has it.
func TestAFixOnThePhoneGoesToTheComputerWithItsTerm(t *testing.T) {
	d := newDesktop(t)
	d.SetExplanation("A greeting.")
	d.SetGlossary("lib", "f1", "", true, sync.Term{Source: "morning", Target: "早上", Scope: sync.TermGlobal, Locked: true})
	p, _ := studyPlayer(t, d)
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-fix", wait)
	p.app.Press("id:player-explain-fix")
	p.app.WaitFor("id:player-fix", wait)
	if got := p.app.Find("id:player-fix-original").Text; got != "How are you?" {
		t.Fatalf("the sheet fixes %q", got)
	}
	if got := p.app.Find("id:player-fix-draft").Text; got != "你好吗？" || !p.app.Find("id:player-fix-save").Props.Disabled {
		t.Fatalf("the field reads %q; Save on before a change", got)
	}
	p.app.Clear("id:player-fix-draft")
	p.app.Type("id:player-fix-draft", "你过得怎么样？")
	p.app.WaitFor("id:player-fix-term-switch", wait)
	p.app.Press("id:player-fix-term-switch")
	p.app.WaitFor("id:player-fix-term-source", wait)
	// The term's translation follows the words the fix wrote; its words are the learner's to name, and must be in the line.
	if got := p.app.Find("id:player-fix-term-target").Text; got != "过得怎么样" {
		t.Fatalf("the term's translation reads %q", got)
	}
	if !p.app.Find("id:player-fix-save").Props.Disabled {
		t.Fatal("Save is on with the term's words missing")
	}
	p.app.Type("id:player-fix-term-source", "How are")
	p.app.WaitUntil(wait, func() bool { return !p.app.Find("id:player-fix-save").Props.Disabled })
	p.app.Press("id:player-fix-save")
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerFixSavedTerm(), wait)
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:player"), "你过得怎么样？") })

	p.app.WaitUntil(wait, func() bool {
		for _, r := range d.Records("lib", sync.KindCorrection) {
			if r.ID == sync.CorrectionID("f1", 3000, 4500) {
				return fieldText(r, sync.FieldTranslation) == "你过得怎么样？"
			}
		}
		return false
	})
	g := d.Glossary("lib", "f1")
	if len(g.Terms) != 2 || g.Terms[0] != (sync.Term{Source: "How are", Target: "过得怎么样", Scope: sync.TermFilm, Locked: true}) {
		t.Fatalf("the computer's glossary is %+v", g)
	}

	p.app.Press("id:player-tab-info")
	p.app.WaitFor("id:player-terms", wait)
	got := p.texts("id:player-terms")
	for _, want := range []string{msgs.PlayerTerms(), msgs.PlayerTermsOnComputer(), "How are", "过得怎么样", "morning", "早上"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the glossary reads %q, want %q in it", got, want)
		}
	}
}

// A film whose computer shares no glossary lists none and fixes a line without a term; one with an empty glossary says so.
func TestAFixWithoutAGlossary(t *testing.T) {
	d := newDesktop(t)
	d.SetExplanation("A greeting.")
	d.SetNoGlossary(true)
	p, _ := studyPlayer(t, d)
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-fix", wait)
	p.app.Press("id:player-explain-fix")
	p.app.WaitFor("id:player-fix", wait)
	if p.app.Exists("id:player-fix-term-switch") {
		t.Fatal("a computer that shares no glossary is offered a term")
	}
	p.app.Clear("id:player-fix-draft")
	p.app.Type("id:player-fix-draft", "最近好吗？")
	p.app.Press("id:player-fix-save")
	p.app.WaitFor("id:player-toasts >> text:"+msgs.PlayerFixSaved(), wait)
	p.app.Press("id:player-tab-info")
	p.app.WaitFor("id:player-info-length", wait)
	if p.app.Exists("id:player-terms") {
		t.Fatal("a glossary is listed for a computer that shares none")
	}
}

// The Info tab says when the film has no terms yet; a phone the computer made read-only is offered no fix.
func TestAReadOnlyPhoneFixesNothing(t *testing.T) {
	d := newDesktop(t)
	d.SetExplanation("A greeting.")
	d.SetReadOnly(true)
	p, _ := studyPlayer(t, d)
	p.app.Press("id:player-tab-info")
	p.app.WaitFor("id:player-terms-empty", wait)
	p.at(3200 * time.Millisecond)
	p.app.Press("id:player-explain-line")
	p.app.WaitFor("id:player-explain-text", wait)
	p.app.WaitUntil(wait, func() bool { return !p.app.Exists("id:player-explain-fix") })
}
