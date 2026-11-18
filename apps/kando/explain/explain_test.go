package explain

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/lines"
	"kando/packages/core"
)

// TestParseReadsTheSections checks that a reply in the JSON Prompt asks for, fenced or not, gives its sections trimmed, that a model answering in plain text has its text taken as the meaning, and that a reply with nothing in it is an error.
func TestParseReadsTheSections(t *testing.T) {
	got, err := Parse("```json\n{\"meaning\": \" 弯道叫河曲。 \", \"breakdown\": \"sweeping · 大幅度的\\ncurves · 弯道\", \"translation\": \"\", \"tone\": \"平稳\", \"related\": \"be known as\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	want := Sections{Meaning: "弯道叫河曲。", Breakdown: "sweeping · 大幅度的\ncurves · 弯道", Tone: "平稳", Related: "be known as"}
	if got != want {
		t.Fatalf("Parse = %+v, want %+v", got, want)
	}
	if !got.HasMore() {
		t.Fatal("a reply with tone and related expressions has nothing more")
	}
	if got, err := Parse("这句是在介绍术语。"); err != nil || got != (Sections{Meaning: "这句是在介绍术语。"}) {
		t.Fatalf("plain text: %+v, %v", got, err)
	}
	for _, reply := range []string{"", "  ", `{"meaning": " "}`} {
		if _, err := Parse(reply); !errors.Is(err, ErrEmpty) {
			t.Fatalf("Parse(%q) error %v, want ErrEmpty", reply, err)
		}
	}
	if (Sections{Meaning: "x"}).HasMore() {
		t.Fatal("a meaning alone has more")
	}
}

// TestRequestSendsTheLineAndTheLinesBeforeIt checks that the message names the film, gives the lines before the line with their starts, and the line last, and nothing after it.
func TestRequestSendsTheLineAndTheLinesBeforeIt(t *testing.T) {
	before := []lines.Line{lines.New(62*time.Second, "河流始于山间。", "Rivers begin in the mountains.")}
	line := lines.New(70*time.Second+400*time.Millisecond, "这些弯道叫河曲。", "These sweeping curves are called meanders.")
	got := Request("Rivers", before, line)
	for _, want := range []string{"影片：Rivers", "[1:02] 河流始于山间。 / Rivers begin in the mountains.", "要讲解的一句：[1:10] These sweeping curves are called meanders.\n译文：这些弯道叫河曲。"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Request = %q, want %q in it", got, want)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "译文：这些弯道叫河曲。") {
		t.Fatalf("the line is not last: %q", got)
	}
	untranslated := Request("Rivers", nil, lines.New(time.Second, "", "Hello."))
	if strings.Contains(untranslated, "译文") || !strings.Contains(untranslated, "要讲解的一句：[0:01] Hello.") {
		t.Fatalf("untranslated line: %q", untranslated)
	}
}

// TestPromptsNameTheLanguage checks that both instructions ask for the answer in the language given, 简体中文 when none is, and that the explanation asks about the translation only for a translated line.
func TestPromptsNameTheLanguage(t *testing.T) {
	if p := Prompt("日本語", true); !strings.Contains(p, "用日本語") || !strings.Contains(p, `"translation"`) {
		t.Fatalf("Prompt = %q", p)
	}
	if p := Prompt("", false); !strings.Contains(p, "用简体中文") || strings.Contains(p, "这句的译文") {
		t.Fatalf("Prompt without translation = %q", p)
	}
	if p := FollowPrompt("한국어"); !strings.Contains(p, "用한국어") {
		t.Fatalf("FollowPrompt = %q", p)
	}
}

// TestFollowRequestCarriesTheConversation checks that a follow-up question goes with the line, what was explained and the questions asked before it, the new one last.
func TestFollowRequestCarriesTheConversation(t *testing.T) {
	line := lines.New(70*time.Second, "这些弯道叫河曲。", "These sweeping curves are called meanders.")
	got := FollowRequest("Rivers", nil, line, Sections{Meaning: "弯道叫河曲。"}, []QA{{Question: "sweeping 是什么？", Answer: "大幅度的。"}}, "为什么用 called？")
	for _, want := range []string{"These sweeping curves are called meanders.", "弯道叫河曲。", "问：sweeping 是什么？\n答：大幅度的。"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FollowRequest = %q, want %q in it", got, want)
		}
	}
	if !strings.HasSuffix(got, "追问：为什么用 called？") {
		t.Fatalf("the question is not last: %q", got)
	}
}

// TestTermsAreTheGlossarysInTheLine checks that the terms of a line are those of the glossary whose source the line holds, as a word of it, whatever its case, once each.
func TestTermsAreTheGlossarysInTheLine(t *testing.T) {
	glossary := []core.Term{
		{Source: "meander", Target: "河曲", Locked: true},
		{Source: "Oxbow lake", Target: "牛轭湖"},
		{Source: "an", Target: "一"},
		{Source: "meander", Target: "曲流"},
		{Source: " ", Target: "x"},
	}
	got := Terms("These sweeping curves are called meanders, like an oxbow lake.", glossary)
	if want := []string{"meander → 河曲", "Oxbow lake → 牛轭湖", "an → 一"}; !slices.Equal(got, want) {
		t.Fatalf("Terms = %q, want %q", got, want)
	}
	if got := Terms("Plan ahead.", glossary); len(got) != 0 {
		t.Fatalf("a term inside another word was found: %q", got)
	}
}
