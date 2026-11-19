package task

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/lines"
	"kando/apps/kando/plugins"
)

func sample(scope, render string) plugins.Task {
	return plugins.Task{Scope: scope, Render: render, Prompt: "Quiz the learner in {{language}}.",
		Reply: plugins.TaskReply{Fields: []plugins.TaskField{{Key: "question", Label: plugins.Text{"en": "Question"}}, {Key: "answer", Label: plugins.Text{"en": "Answer"}}}}}
}

// TestPromptAddsTheHostsRulesLast checks that the language takes the place of {{language}}, or is named after a prompt without it, and that the horizon rule and the JSON the reply must be come after the pack's prompt.
func TestPromptAddsTheHostsRulesLast(t *testing.T) {
	got := Prompt(sample(plugins.ScopeLine, plugins.RenderSections), "日本語")
	if !strings.HasPrefix(got, "Quiz the learner in 日本語.") || !strings.Contains(got, horizonRule) || !strings.HasSuffix(got, `exactly these keys, each a string (write "" for one you have nothing for): "question", "answer".`) {
		t.Fatalf("prompt %q", got)
	}
	task := sample(plugins.ScopeLine, plugins.RenderCards)
	task.Prompt = "Make flash cards."
	task.Reply.Items = 3
	got = Prompt(task, "")
	if !strings.Contains(got, "Make flash cards.\n\nWrite your answer in "+defaultLanguage+".") || !strings.Contains(got, "a JSON array of at most 3 items") {
		t.Fatalf("prompt %q", got)
	}
}

// TestRequestSendsOnlyWhatWasReached checks the message of each scope: a line with the lines before it, a word with its line, and the film so far with its earliest lines left out past ContextRunes.
func TestRequestSendsOnlyWhatWasReached(t *testing.T) {
	before := []lines.Line{lines.New(time.Second, "你好", "Hello")}
	line := lines.New(65*time.Second, "", "Goodbye")
	got := Request(sample(plugins.ScopeLine, plugins.RenderSections), Input{Film: "Film", Line: line, Before: before})
	want := "Film: Film\n\nThe lines before it:\n[0:01] 你好 / Hello\n\nThe line: [1:05] Goodbye"
	if got != want {
		t.Fatalf("line request %q, want %q", got, want)
	}
	got = Request(sample(plugins.ScopeWord, plugins.RenderSections), Input{Line: line, Word: " Good "})
	if got != "The line: [1:05] Goodbye\nThe word: Good" {
		t.Fatalf("word request %q", got)
	}
	long := strings.Repeat("a", ContextRunes/2)
	seen := []lines.Line{lines.New(0, "", "first"), lines.New(time.Second, "", long), lines.New(2*time.Second, "", long)}
	got = Request(sample(plugins.ScopeFilm, plugins.RenderList), Input{Seen: seen})
	if strings.Contains(got, "first") || strings.Count(got, long) != 1 {
		t.Fatalf("the film so far kept %d runes", len([]rune(got)))
	}
}

// TestParseReadsEachRender checks sections, a list and cards, values that are lists or numbers, the items cap, and the errors of a reply that is not the JSON or says nothing.
func TestParseReadsEachRender(t *testing.T) {
	items, err := Parse(sample(plugins.ScopeLine, plugins.RenderSections), "```json\n{\"question\": \" Why? \", \"answer\": [\"Because\", \"\", \"so\"]}\n```")
	if err != nil || len(items) != 1 || !slices.Equal(items[0], Item{"Why?", "Because\nso"}) {
		t.Fatalf("sections %q, %v", items, err)
	}
	task := sample(plugins.ScopeFilm, plugins.RenderList)
	task.Reply.Items = 2
	items, err = Parse(task, `Here: [{"question": "a", "answer": 1}, {"question": ""}, {"question": "b"}, {"question": "c"}]`)
	if err != nil || len(items) != 2 || !slices.Equal(items[0], Item{"a", "1"}) || !slices.Equal(items[1], Item{"b", ""}) {
		t.Fatalf("list %q, %v", items, err)
	}
	if _, err := Parse(task, "I cannot help."); !errors.Is(err, ErrMalformed) {
		t.Fatalf("plain text: %v", err)
	}
	if _, err := Parse(sample(plugins.ScopeLine, plugins.RenderCards), `[{"question": ""}]`); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty: %v", err)
	}
}
