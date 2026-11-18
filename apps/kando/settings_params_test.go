package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/hotkey"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// TestAIRequestsAreSavedAndUsed: the batch size and the retries chosen in Settings › AI 服务 reach the translator at once, are kept to their range, are saved only when they differ from the defaults, and come back at the next launch.
func TestAIRequestsAreSavedAndUsed(t *testing.T) {
	a, _, _ := newTestApp(t, media.Tools{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	if a.LLMBatchSize() != media.DefaultBatchSize || a.LLMRetries() != media.DefaultRetries {
		t.Fatalf("defaults %d, %d", a.LLMBatchSize(), a.LLMRetries())
	}
	a.SetLLMRequests(20, 5)
	tr := a.pipeline.(media.Tools).Translator
	if tr.BatchSize != 20 || tr.Attempts != 6 {
		t.Fatalf("translator %d lines, %d attempts", tr.BatchSize, tr.Attempts)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.LLMBatchSize != 20 || saved.LLMRetries == nil || *saved.LLMRetries != 5 {
		t.Fatalf("saved %+v", saved.savedLLM)
	}
	next, _, _ := newTestApp(t, media.Tools{})
	next.restoreLLM(saved)
	if next.LLMBatchSize() != 20 || next.LLMRetries() != 5 {
		t.Fatalf("restored %d, %d", next.LLMBatchSize(), next.LLMRetries())
	}
	a.SetLLMRequests(1000, -3)
	if a.LLMBatchSize() != media.MaxBatchSize || a.LLMRetries() != 0 {
		t.Fatalf("out of range kept as %d, %d", a.LLMBatchSize(), a.LLMRetries())
	}
	a.SetLLMRequests(media.DefaultBatchSize, media.DefaultRetries)
	data, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "llmBatchSize") || strings.Contains(string(data), "llmRetries") {
		t.Fatalf("the defaults were saved: %s", data)
	}
	s := savedLLM{LLMBatchSize: 3, LLMRetries: new(int)}
	*s.LLMRetries = 99
	s.clean()
	if s.LLMBatchSize != media.MinBatchSize || *s.LLMRetries != media.MaxRetries {
		t.Fatalf("cleaned %d, %d", s.LLMBatchSize, *s.LLMRetries)
	}
}

// TestContextLinesAreSavedAndReachTheJob: the context lines chosen in Settings › 翻译 are saved unless they are the default, come back at the next launch and go with the translator of the next job.
func TestContextLinesAreSavedAndReachTheJob(t *testing.T) {
	a, _, _ := newTestApp(t, media.Tools{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	if a.TranslateContextLines() != media.DefaultContextLines {
		t.Fatalf("default %d", a.TranslateContextLines())
	}
	a.SetTranslateContextLines(120)
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.TranslateContextLines != 120 {
		t.Fatalf("saved %+v", saved.savedTranslate)
	}
	next, _, _ := newTestApp(t, media.Tools{})
	next.restoreTranslate(saved)
	if next.TranslateContextLines() != 120 {
		t.Fatalf("restored %d", next.TranslateContextLines())
	}
	a.SetTranslateContextLines(1)
	if a.TranslateContextLines() != settings.MinContextLines {
		t.Fatalf("kept %d", a.TranslateContextLines())
	}
	a.SetTranslateContextLines(media.DefaultContextLines)
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	if loadSavedSettings(a.settingsPath).TranslateContextLines != 0 {
		t.Fatal("the default was saved")
	}
}

// TestSpeechParametersAreSavedAndShownAsTheCommand: the threads and the proper noun hints of Settings › 语音识别 reach whisper's arguments, are saved and restored, and the page shows the whisper-cli command they make.
func TestSpeechParametersAreSavedAndShownAsTheCommand(t *testing.T) {
	a, tt, _ := newTestApp(t, media.Tools{WhisperModel: "/models/ggml-small.bin"})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.SetSpeechThreads(4)
	a.SetSpeechHints("Monterey\nKando")
	a.SetSpeechThreads(7)
	if got := a.SpeechThreads(); got != 4 {
		t.Fatalf("threads %d after an unknown count", got)
	}
	command := settings.CommandLine(a.SpeechCommand())
	if want := `whisper-cli -m ggml-small.bin -t 4 -l auto --prompt "Monterey, Kando" -np -pp -oj -of transcript -f audio.wav`; command != want {
		t.Fatalf("command %q, want %q", command, want)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.WhisperThreads != 4 || saved.WhisperHints != "Monterey\nKando" {
		t.Fatalf("saved %+v", saved.savedSpeech)
	}
	next, _, _ := newTestApp(t, media.Tools{})
	next.restoreSpeech(saved)
	if next.SpeechThreads() != 4 || next.SpeechHints() != "Monterey\nKando" {
		t.Fatalf("restored %d, %q", next.SpeechThreads(), next.SpeechHints())
	}
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	tt.Resize(880, 1800)
	wantText(t, tt, "识别参数")
	wantText(t, tt, command)
	s := savedSpeech{WhisperThreads: 3, WhisperHints: " \n"}
	s.clean()
	if s.WhisperThreads != 0 || s.WhisperHints != "" {
		t.Fatalf("cleaned %+v", s)
	}
}

// TestAppearanceFollowsTheChoice: Settings › 通用 › 外观 shows the windows light or dark at once, is saved, and following the system again saves nothing.
func TestAppearanceFollowsTheChoice(t *testing.T) {
	a, _, _ := newTestApp(t, media.Tools{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	if a.settingsAppearance() != core.AppearanceSystem {
		t.Fatalf("starts in %v", a.settingsAppearance())
	}
	a.SetAppearance(settings.AppearanceDark)
	if !a.cx.Env().Dark() || a.cx.Env().Appearance() != core.AppearanceDark {
		t.Fatal("the app is not dark")
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.Appearance != settings.AppearanceDark {
		t.Fatalf("saved %q", saved.Appearance)
	}
	next, _, _ := newTestApp(t, media.Tools{})
	next.restoreInterface(saved)
	if next.settingsAppearance() != core.AppearanceDark {
		t.Fatalf("restored %v", next.settingsAppearance())
	}
	a.SetAppearance(settings.AppearanceLight)
	if a.cx.Env().Dark() {
		t.Fatal("the app is still dark")
	}
	a.SetAppearance("sepia")
	if a.cx.Env().Appearance() != core.AppearanceSystem || loadSavedSettings(a.settingsPath).Appearance != "" {
		t.Fatal("an unknown appearance does not follow the system")
	}
}

// TestShortcutPresetMovesThePlayersKeys: the mpv preset moves the line keys to mpv's subtitle seek and the volume to 9 and 0, the menu and the settings page list the keys it gives, it is saved, and Kando's own keys come back with the system preset.
func TestShortcutPresetMovesThePlayersKeys(t *testing.T) {
	a, _, _ := libraryWith(t, "Talk")
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	keyOf := func(label string) string {
		for _, s := range a.shortcuts(a.cx) {
			if s.Label == label {
				return s.Key
			}
		}
		t.Fatalf("no row %q", label)
		return ""
	}
	m := locales.In(locales.Default)
	if keyOf(m.PlayerKeyPrevLine()) != "a" {
		t.Fatal("the line key is not Kando's")
	}
	a.SetShortcutPreset(hotkey.PresetMPV)
	if keyOf(m.PlayerKeyPrevLine()) != "ctrl+left" || keyOf(m.PlayerKeyVolumeUp()) != "0" || keyOf(m.PlayerKeyReplayLine()) != "r" {
		t.Fatalf("mpv keys %q %q %q", keyOf(m.PlayerKeyPrevLine()), keyOf(m.PlayerKeyVolumeUp()), keyOf(m.PlayerKeyReplayLine()))
	}
	if !slices.ContainsFunc(a.Shortcuts(a.cx), func(s settings.Shortcut) bool { return s.Key == "ctrl+left" }) {
		t.Fatal("the settings page does not list the preset's keys")
	}
	if loadSavedSettings(a.settingsPath).ShortcutPreset != hotkey.PresetMPV {
		t.Fatal("the preset was not saved")
	}
	a.SetShortcutPreset("vlc")
	if a.ShortcutPreset() != hotkey.PresetMPV {
		t.Fatal("an unknown preset was taken")
	}
	a.SetShortcutPreset(hotkey.PresetSystem)
	if keyOf(m.PlayerKeyPrevLine()) != "a" {
		t.Fatal("Kando's keys did not come back")
	}
}
