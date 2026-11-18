package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/study"
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

func TestScreenshotsLearnStudy(t *testing.T) {
	shot := screenshots(t)
	d := studyDesktop(t)
	dir := t.TempDir()
	watchedFor(t, dir, 12, 18, 0, 26, 9, 15, 12)
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitUntil(wait, func() bool { return p.texts("id:learn-review-title") == msgs.LearnReviewDue(2) })
	// The goal's bar eases to its value over 200ms (ui.Progress).
	p.app.Advance(300 * time.Millisecond)
	shot(p, "learn-study")
	p.app.Press("id:learn-review-start")
	p.app.WaitFor("id:review-card", wait)
	shot(p, "review-card")
	p.app.Press("id:review-show")
	p.app.WaitFor("id:review-answer", wait)
	shot(p, "review-answer")
	p.app.Press("id:review-known")
	p.app.Press("id:review-show")
	p.app.Press("id:review-again")
	p.app.Press("id:review-show")
	p.app.Press("id:review-known")
	p.app.WaitFor("id:review-done", wait)
	shot(p, "review-done")
}

func TestScreenshotsVocabulary(t *testing.T) {
	shot := screenshots(t)
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-vocabulary", wait)
	p.app.Press("id:learn-vocabulary")
	p.app.WaitFor("id:vocabulary-item-w-morning", wait)
	shot(p, "vocabulary")
}

func TestScreenshotsSavedLines(t *testing.T) {
	shot := screenshots(t)
	d := studyDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(store.DesktopID(d.Fingerprint))
	p.app.WaitFor("id:tab-bar", wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-lines", wait)
	p.app.Press("id:learn-lines")
	p.app.WaitFor("id:lines-item-l-fine", wait)
	shot(p, "saved-lines")
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
