package main

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/lookup"
	"kando/packages/core"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// A paired phone looks a word up and has a line explained through the desktop: without an AI service the desktop says so; with one it asks it, as the picture's lookup and Explain Line do, in the film's language and with its title, keeps the word among the words looked up here, and answers it again from there.
func TestPhoneSyncLooksUpAndExplainsThroughTheAIServiceSet(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || !slices.Contains(h.Ops, psync.OpLookUp) || !slices.Contains(h.Ops, psync.OpExplain) {
		t.Fatalf("hello %+v, %v", h, err)
	}
	req := psync.WordLookup{Library: phoneLibraryID(dir), Film: "yt", Word: "Hello", Line: "Hello there"}
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) { return c.LookUp(ctx, req) })
	phoneWantCode(t, err, http.StatusConflict, psync.CodeNoAI)

	srv, seen := fakeLLM(t, `{"word":"hello","reading":"/həˈləʊ/","meaning":"int. 你好"}`)
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
		a.prefs.language = "简体中文"
	})
	e, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) { return c.LookUp(ctx, req) })
	if err != nil || e.Word != "hello" || e.Reading != "/həˈləʊ/" || e.Meaning != "int. 你好" || e.Language != "简体中文" {
		t.Fatalf("lookup %+v, %v", e, err)
	}
	sent := strings.Join(*seen, "\n")
	for _, want := range []string{"影片：Rust Talk", "字幕：Hello there", "要查的词：Hello", "用简体中文解释这个词"} {
		if !strings.Contains(sent, want) {
			t.Fatalf("sent %q, want %q in it", sent, want)
		}
	}
	asked := len(*seen)
	if e, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) { return c.LookUp(ctx, req) }); err != nil || e.Meaning != "int. 你好" || len(*seen) != asked {
		t.Fatalf("looked up again: %+v, %v, the AI service asked %d times", e, err, len(*seen)-asked)
	}

	*seen = nil
	x, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Explanation, error) {
		return c.Explain(ctx, psync.LineExplain{Library: phoneLibraryID(dir), Film: "yt", Index: 1, Lines: []psync.Cue{
			{StartMS: 0, Original: "Hi.", Translation: "嗨。"}, {StartMS: 1000, Original: "Hello there", Translation: "你好"}}})
	})
	if err != nil || x.Model != "m1" || x.Language != "简体中文" || !strings.Contains(x.Text, "hello") {
		t.Fatalf("explain %+v, %v", x, err)
	}
	sent = strings.Join(*seen, "\n")
	for _, want := range []string{"[0:00] 嗨。 / Hi.", "要讲解的一句：[0:01] Hello there", "用简体中文讲解这一句的原文"} {
		if !strings.Contains(sent, want) {
			t.Fatalf("sent %q, want %q in it", sent, want)
		}
	}
}

// A word looked up on the picture answers the phone's lookup of it from the same cache, for a film whose speech language the user named over the one heard and whose subtitles are in its own language: the phone's query is the picture's (vocabQuery), and the AI service is not asked.
func TestPhoneSyncLookUpSharesThePicturesCache(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	onEngine(a, func() {
		for i := range a.library.Jobs {
			if a.library.Jobs[i].ID == "yt" {
				a.library.Jobs[i].SpokenLanguage, a.library.Jobs[i].SpeechLanguage, a.library.Jobs[i].SameLanguage = "en", "ja", true
			}
		}
	})
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	srv, seen := fakeLLM(t, `{"word":"hello","meaning":"from the AI"}`)
	var q lookup.Query
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
		a.prefs.language = "English"
		i := slices.IndexFunc(a.library.Jobs, func(j core.Job) bool { return j.ID == "yt" })
		item := films.FromJob(a.library.Jobs[i], "", "")
		q = vocabQuery(item, a.prefs.language, "Hello", "Hello there")
		a.vocabCache().Put(q, lookup.Entry{Word: "hello", Meaning: "int. 你好"}, time.Now())
	})
	if q.Spoken != "ja" || q.Language != "简体中文" {
		t.Fatalf("the picture's query %+v", q)
	}
	req := psync.WordLookup{Library: phoneLibraryID(dir), Film: "yt", Word: "Hello", Line: "Hello there"}
	e, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) { return c.LookUp(ctx, req) })
	if err != nil || e.Meaning != "int. 你好" || e.Language != "简体中文" || len(*seen) != 0 {
		t.Fatalf("lookup %+v, %v, the AI service asked %d times", e, err, len(*seen))
	}
}
