package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/saved"
	"kando/packages/media"
	"kando/packages/study"
	psync "kando/packages/sync"
)

// A phone reads the open library's vocabulary with the lines, reviews a word, and the desktop keeps the word's new schedule in its study record; a word removed on the phone is removed on the desktop for good, with the sound kept of its line. A word the phone adds keeps its line's end.
func TestPhoneSyncSyncsTheVocabulary(t *testing.T) {
	a, p, dir, talk, c, clock := phoneStateLibrary(t)
	lib := phoneLibraryID(dir)
	a.saved.book.AddWord(saved.Word{Text: "borrow", Spoken: "en", Meaning: "借用", Language: "简体中文", Context: "Rust borrows.", Video: a.saved.book.Key(talk), Start: time.Second, Film: "Rust Talk"})
	a.saved.book.AddWord(saved.Word{Text: "owner", Spoken: "en", Meaning: "所有者"})
	a.savedWrite()
	refresh(p.tt, a)

	h, err := phoneAsk(t, p, c.Hello)
	if err != nil || !slices.Contains(h.State, psync.KindWord) {
		t.Fatalf("hello %+v, %v; want words among the state", h, err)
	}
	got, err := phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, "", 0) })
	if err != nil {
		t.Fatal(err)
	}
	borrow := a.saved.book.Words[a.saved.book.WordIndex("borrow", "en")]
	word := phoneStateFind(t, got.Records, psync.KindWord, borrow.ID)
	if word.Film != "yt" || phoneStateString(t, word.Fields[psync.FieldWord]) != "borrow" || phoneStateString(t, word.Fields[psync.FieldMeaning]) != "借用" {
		t.Errorf("word %+v", word)
	}

	clock.Add(time.Minute)
	now := clock.Now()
	due := now.Add(study.Intervals[0])
	base := word.Fields[psync.FieldReview].Stamp
	owner := a.saved.book.Words[a.saved.book.WordIndex("owner", "en")].ID
	for _, id := range []string{borrow.ID, owner} {
		kept := saved.StoredSound(a.saved.book, id)
		if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(kept, []byte("kept\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	at := func(v any) psync.Field { return psync.Field{Value: psync.Value(v), Stamp: psync.Stamp{At: now}} }
	put := []psync.Record{
		{Kind: psync.KindWord, ID: borrow.ID, Fields: map[string]psync.Field{psync.FieldReview: {Value: psync.Value(psync.Review{Box: 1, Due: due}), Stamp: psync.Stamp{At: now}, Base: &base}}},
		{Kind: psync.KindWord, ID: owner, Removed: &psync.Stamp{At: now}},
		{Kind: psync.KindWord, ID: "phoneword", Film: "yt", Fields: map[string]psync.Field{
			psync.FieldWord: at("trait"), psync.FieldSpoken: at("en"), psync.FieldContext: at("Traits are shared."), psync.FieldStartMS: at(4000), psync.FieldEndMS: at(5500),
			psync.FieldSavedAt: at(now), psync.FieldReview: at(psync.Review{Due: now}),
		}},
	}
	if _, err := phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) { return c.PutState(ctx, lib, put) }); err != nil {
		t.Fatal(err)
	}
	i := a.saved.book.WordIndex("borrow", "en")
	if i < 0 || a.saved.book.Words[i].Box != 1 || !a.saved.book.Words[i].Due.Equal(due) || a.saved.book.Words[i].Stamps.Review.Device != a.phone.devices[0].ID {
		t.Errorf("the desktop's word %+v", a.saved.book.Words)
	}
	if a.saved.book.WordIndex("owner", "en") >= 0 {
		t.Error("the word removed on the phone is still in the vocabulary")
	}
	if saved.HasSound(saved.StoredSound(a.saved.book, owner)) || !saved.HasSound(saved.StoredSound(a.saved.book, borrow.ID)) {
		t.Error("the sound of the word removed on the phone stayed, or the other's went")
	}
	if i := a.saved.book.WordIndex("trait", "en"); i < 0 || a.saved.book.Words[i].Start != 4*time.Second || a.saved.book.Words[i].End != 5500*time.Millisecond {
		t.Errorf("the phone's word is %+v", a.saved.book.Words)
	}
	if disk, err := study.Load(dir); err != nil || disk.WordIndex("owner", "en") >= 0 || disk.Words[disk.WordIndex("borrow", "en")].Box != 1 {
		t.Errorf("the study record on disk did not keep the phone's changes: %v", err)
	}

	// The desktop grades the word itself: the phone hears it among the changes.
	clock.Add(time.Minute)
	a.saved.book.GradeWord("borrow", "en", false, clock.Now())
	a.savedWrite()
	refresh(p.tt, a)
	since, err := phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, got.Epoch, got.Rev) })
	if err != nil {
		t.Fatal(err)
	}
	var rv psync.Review
	w := phoneStateFind(t, since.Records, psync.KindWord, borrow.ID)
	if err := json.Unmarshal(w.Fields[psync.FieldReview].Value, &rv); err != nil || rv.Box != 0 {
		t.Errorf("the desktop's review reached the phone as %+v, %v", w, err)
	}
}

// A phone reads the language this computer translates into, by its tag, and changes it, which settings.json keeps; a language the computer does not translate into is refused.
func TestPhoneSyncSharesTheLanguageTranslatedInto(t *testing.T) {
	a, p, _, _, c, _ := phoneStateLibrary(t)
	a.prefs.language = "简体中文"
	h, err := phoneAsk(t, p, c.Hello)
	if err != nil || !slices.Contains(h.Ops, psync.OpSettingsPut) {
		t.Fatalf("hello %+v, %v; want the settings among the operations", h, err)
	}
	got, err := phoneAsk(t, p, c.Settings)
	if err != nil || got.Target != "zh-Hans" || got.TargetName != "简体中文" {
		t.Fatalf("settings %+v, %v", got, err)
	}
	got, err = phoneAsk(t, p, func(ctx context.Context) (psync.Settings, error) {
		return c.PutSettings(ctx, psync.SettingsPut{Target: "ja"})
	})
	if err != nil || got.Target != "ja" || a.prefs.language != "日本語" {
		t.Fatalf("put answered %+v, %v; the desktop translates into %q", got, err, a.prefs.language)
	}
	var file savedSettings
	data, err := os.ReadFile(a.settingsPath)
	if err == nil {
		err = json.Unmarshal(data, &file)
	}
	if err != nil || file.Language != "日本語" {
		t.Fatalf("settings.json keeps %q, %v", file.Language, err)
	}
	_, err = phoneAsk(t, p, func(ctx context.Context) (psync.Settings, error) {
		return c.PutSettings(ctx, psync.SettingsPut{Target: "xx"})
	})
	phoneWantCode(t, err, http.StatusBadRequest, psync.CodeInvalid)

	// A change settings.json cannot keep is not made: the phone is answered the error, and the desktop keeps the language it had.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	a.settingsPath = filepath.Join(blocker, "settings.json")
	_, err = phoneAsk(t, p, func(ctx context.Context) (psync.Settings, error) {
		return c.PutSettings(ctx, psync.SettingsPut{Target: "ko"})
	})
	if err == nil || a.prefs.language != "日本語" {
		t.Fatalf("a put settings.json cannot keep answered %v; the desktop translates into %q", err, a.prefs.language)
	}

	// A language typed into an older version has no tag, and its own name.
	a.prefs.language = "Klingon"
	got, err = phoneAsk(t, p, c.Settings)
	if err != nil || got.Target != "" || got.TargetName != "Klingon" {
		t.Fatalf("settings %+v, %v", got, err)
	}
}

// A phone sees the AI service this computer translates with, by its name and model and never its address or key, and turns the context switch on and off, which settings.json keeps.
func TestPhoneSyncSharesTheAIServiceAndTheContextSwitch(t *testing.T) {
	a, p, _, _, c, _ := phoneStateLibrary(t)
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1", Model: "gpt-4.1-mini", APIKey: "sk-secret"}}
	a.prefs.translateContext = true
	got, err := phoneAsk(t, p, c.Settings)
	if err != nil || got.Service != "OpenAI" || got.Model != "gpt-4.1-mini" || got.Context == nil || !*got.Context {
		t.Fatalf("settings %+v, %v; want OpenAI's gpt-4.1-mini with the context on", got, err)
	}
	raw, err := json.Marshal(got)
	if err != nil || bytes.Contains(raw, []byte("sk-secret")) || bytes.Contains(raw, []byte("api.openai.com")) {
		t.Fatalf("the settings a phone reads carry the service's key or address: %s", raw)
	}

	got, err = phoneAsk(t, p, func(ctx context.Context) (psync.Settings, error) {
		return c.PutSettings(ctx, psync.SettingsPut{Context: new(false)})
	})
	if err != nil || got.Context == nil || *got.Context || a.prefs.translateContext {
		t.Fatalf("put answered %+v, %v; the desktop's context is %v", got, err, a.prefs.translateContext)
	}
	var file savedSettings
	data, err := os.ReadFile(a.settingsPath)
	if err == nil {
		err = json.Unmarshal(data, &file)
	}
	if err != nil || file.TranslateContext == nil || *file.TranslateContext {
		t.Fatalf("settings.json keeps the context as %v, %v", file.TranslateContext, err)
	}

	// Without a service set up, none is named.
	a.pipeline = media.Tools{}
	got, err = phoneAsk(t, p, c.Settings)
	if err != nil || got.Service != "" || got.Model != "" {
		t.Fatalf("settings without a service %+v, %v", got, err)
	}
}

// A phone reads how long films of the library played on each device each day, this computer's among them, and puts its own, which the study record keeps, so the stats on either side add up every device's; it cannot put another device's.
func TestPhoneSyncSharesTheTimeWatched(t *testing.T) {
	a, p, dir, _, c, clock := phoneStateLibrary(t)
	lib := phoneLibraryID(dir)
	a.saved.book.Watch(clock.Now(), 10*time.Minute)
	a.savedWrite()
	refresh(p.tt, a)
	h, err := phoneAsk(t, p, c.Hello)
	if err != nil || !slices.Contains(h.State, psync.KindWatched) {
		t.Fatalf("hello %+v, %v; want the time watched among the state", h, err)
	}
	got, err := phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, "", 0) })
	if err != nil {
		t.Fatal(err)
	}
	day := study.Day(clock.Now())
	mine := phoneStateFind(t, got.Records, psync.KindWatched, psync.WatchedID(a.saved.book.Device, day))
	var ms float64
	if err := json.Unmarshal(mine.Fields[psync.FieldWatchedMS].Value, &ms); err != nil || ms != 600_000 {
		t.Fatalf("this computer's time reads %v, %v", ms, err)
	}

	phone := a.phone.devices[0].ID
	put := []psync.Record{{Kind: psync.KindWatched, ID: psync.WatchedID(phone, day), Fields: map[string]psync.Field{psync.FieldWatchedMS: {Value: psync.Value(300_000), Stamp: psync.Stamp{At: clock.Now()}}}}}
	if _, err := phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) { return c.PutState(ctx, lib, put) }); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(a.saved.book.Watched, study.Watched{Day: day, Device: phone, Time: 5 * time.Minute}) {
		t.Errorf("the study record's time watched %+v; want the phone's five minutes", a.saved.book.Watched)
	}
	if disk, err := study.Load(dir); err != nil || !slices.Contains(disk.Watched, study.Watched{Day: day, Device: phone, Time: 5 * time.Minute}) {
		t.Errorf("the study record on disk keeps %+v, %v", disk.Watched, err)
	}

	other := []psync.Record{{Kind: psync.KindWatched, ID: psync.WatchedID(a.saved.book.Device, day), Fields: map[string]psync.Field{psync.FieldWatchedMS: {Value: psync.Value(0)}}}}
	_, err = phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) { return c.PutState(ctx, lib, other) })
	phoneWantCode(t, err, http.StatusBadRequest, psync.CodeInvalid)
}
