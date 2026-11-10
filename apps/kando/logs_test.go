package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/packages/core"
	"kando/packages/media"
)

// logsAt points the logs folder at a new folder under a temporary home, opens a's log file there as main does once the window is ready, and closes it when t ends. It returns the log file's path.
func logsAt(t *testing.T, a *app) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Library", "Logs", "Kando")
	was, out, flags := logsDir, log.Writer(), log.Flags()
	logsDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() {
		a.logsClose()
		logsDir = was
		log.SetOutput(out)
		log.SetFlags(flags)
	})
	a.logsOpen()
	if a.logs.file == nil {
		t.Fatal("the log file did not open")
	}
	return filepath.Join(dir, logName)
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLogFileIsMadeInTheLogsFolder(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	path := logsAt(t, a)
	log.Printf("remote: no control socket: refused")
	got := readLog(t, path)
	for _, want := range []string{"Kando " + appVersion + " started on", "remote: no control socket: refused"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log lacks %q:\n%s", want, got)
		}
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("log file %v, %v: others may read it", info, err)
	}
	a.logsClose()
	log.Printf("after quitting")
	if got := readLog(t, path); !strings.Contains(got, "Kando is quitting") || strings.Contains(got, "after quitting") {
		t.Fatalf("log after closing:\n%s", got)
	}
}

// A folder the log cannot be written in leaves the app without a log file, and without the menu's row that would show it.
func TestNoLogFileWhereTheFolderCannotBeMade(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	was := logsDir
	logsDir = func() (string, error) { return filepath.Join(blocker, "Logs"), nil }
	t.Cleanup(func() { logsDir = was })
	a.logsOpen()
	if a.logs.file != nil {
		a.logsClose()
		t.Fatal("a log file opened under a file")
	}
	if rows := a.logsKeys(); rows[0].When() {
		t.Fatal("显示日志文件 is enabled without a log file")
	}
}

func TestLogFileRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), logName)
	w, err := openLogFile(path, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	for i := range 12 {
		if _, err := w.Write([]byte(strings.Repeat(string(rune('a'+i)), 39) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	// Two 40-byte entries fit in 100 bytes, so the twelve fill six files: kando.log keeps the last two, and the two before it are kept.
	for file, want := range map[string]string{logName: "k", "kando.1.log": "i", "kando.2.log": "g"} {
		got := readLog(t, filepath.Join(filepath.Dir(path), file))
		if len(got) != 80 || got[0] != want[0] {
			t.Fatalf("%s holds %q", file, got)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "kando.3.log")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("kando.3.log was kept: %v", err)
	}
}

// A log file that was already full when the app quit is moved aside when the app opens it again.
func TestFullLogFileIsMovedAsideOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, logName)
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 100)), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := openLogFile(path, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	w.Write([]byte("new\n"))
	if got := readLog(t, path); got != "new\n" {
		t.Fatalf("kando.log holds %q", got)
	}
	if got := readLog(t, filepath.Join(dir, "kando.1.log")); got != strings.Repeat("x", 100) {
		t.Fatalf("kando.1.log holds %q", got)
	}
}

// Whatever is shaped like a credential reaches neither the file nor stderr.
func TestLogFileHidesTokens(t *testing.T) {
	var echo strings.Builder
	path := filepath.Join(t.TempDir(), logName)
	w, err := openLogFile(path, logLimit, &echo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	w.Write([]byte("Authorization: Bearer abcdefghijklmnop; key sk-proj-abcdefghijklmnopqrstuv\n"))
	for _, got := range []string{readLog(t, path), echo.String()} {
		if strings.Contains(got, "abcdefghijklmnop") || !strings.Contains(got, "Bearer ***") {
			t.Fatalf("logged %q", got)
		}
	}
}

func TestShowLogFileIsAHelpMenuRow(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	rows := a.logsKeys()
	if len(rows) != 1 || rows[0].Key != 0 || rows[0].Mods != 0 || rows[0].Group != groupHelp || rows[0].Label != "显示日志文件" {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].When() {
		t.Fatal("the row is enabled before there is a log file")
	}
	path := logsAt(t, a)
	if !rows[0].When() {
		t.Fatal("the row is disabled with a log file")
	}
	var shown string
	was := logsReveal
	logsReveal = func(p string) { shown = p }
	t.Cleanup(func() { logsReveal = was })
	// The user removed the file in Finder; it is made again, so there is a file to show.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	rows[0].Run()
	if shown != path {
		t.Fatalf("showed %q, want %q", shown, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the log file was not made again: %v", err)
	}
	var found bool
	for _, menu := range a.menuBar() {
		if menu.Label != "帮助" {
			continue
		}
		for _, item := range menu.Submenu {
			found = found || item.Label == "显示日志文件" && !item.Disabled
		}
	}
	if !found {
		t.Fatal("the 帮助 menu has no enabled 显示日志文件")
	}
}

// logsFailPipeline downloads with a yt-dlp that warns at length before it fails, and runs the other stages as fakePipeline does.
type logsFailPipeline struct {
	fakePipeline
	tools media.Tools
}

func (p *logsFailPipeline) Download(ctx context.Context, source string) (string, error) {
	return p.tools.Download(ctx, source)
}

// A job that fails is logged once, with the stage it failed in and its whole error, and the whole of what yt-dlp printed, which the job's error shortens; the jobs that had failed before the shelf was read are not.
func TestFailedJobIsLogged(t *testing.T) {
	ytdlp := filepath.Join(t.TempDir(), "yt-dlp")
	script := "#!/bin/sh\necho 'WARNING: first warning of many' >&2\ni=0\nwhile [ $i -lt 60 ]; do echo \"WARNING: [youtube] retrying fragment $i\" >&2; i=$((i+1)); done\necho 'ERROR: [youtube] x: Private video' >&2\nexit 1\n"
	if err := os.WriteFile(ytdlp, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(&logsFailPipeline{tools: media.Tools{YTDLP: ytdlp, OutputDir: t.TempDir()}})
	shelfPath := filepath.Join(t.TempDir(), "library.json")
	a.openLibrary(shelfPath)
	a.library.Upsert(core.Job{ID: "old", Source: "https://example.com/old", Stage: core.StageFailed, Err: errors.New("download: an old failure")})
	if err := a.library.Save(); err != nil {
		t.Fatal(err)
	}
	a.openLibrary(shelfPath)
	path := logsAt(t, a)
	if err := a.Enqueue("https://www.youtube.com/watch?v=x"); err != nil {
		t.Fatal(err)
	}
	var id string
	for _, job := range a.library.Jobs {
		if job.ID != "old" {
			id = job.ID
		}
	}
	settle(t, a, tt, updates)
	if job := stageOf(t, a, id); job.Stage != core.StageFailed || strings.Contains(job.Err.Error(), "first warning of many") {
		t.Fatalf("job %+v", job)
	}
	a.areasShelfChanged()
	got := readLog(t, path)
	for _, want := range []string{"yt-dlp failed: exit status 1", "WARNING: first warning of many", "retrying fragment 30", "job " + id + " failed while downloading: download: yt-dlp: exit status 1: ERROR: [youtube] x: Private video"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log lacks %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "job "+id+" failed") != 1 || strings.Contains(got, "an old failure") {
		t.Fatalf("log:\n%s", got)
	}
}

// logsTranslateFails fails the translation with what an HTTP client says of the AI service it could not reach.
type logsTranslateFails struct{ fakePipeline }

func (logsTranslateFails) Translate(context.Context, []core.Segment, string) ([]core.Segment, error) {
	return nil, errors.New(`Post "https://llm.private.example/v1/chat/completions": 401 Unauthorized: key sk-kando-secret-0123456789 rejected; storage r2-secret-value-xyz`)
}

// The log keeps neither the AI service's address nor its key, nor any other credential the app holds.
func TestFailedJobLogHasNoSecrets(t *testing.T) {
	a, tt, updates := newTestApp(&logsTranslateFails{})
	a.prefs.llmBase = "https://llm.private.example/v1"
	a.prefs.llmKey = "sk-kando-secret-0123456789"
	a.cloud.r2Secret = "r2-secret-value-xyz"
	a.openLibrary(filepath.Join(t.TempDir(), "library.json"))
	path := logsAt(t, a)
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	got := readLog(t, path)
	if !strings.Contains(got, "failed while translating: translate: Post \"***/chat/completions\": 401 Unauthorized: key *** rejected; storage ***") {
		t.Fatalf("log:\n%s", got)
	}
	for _, secret := range []string{"llm.private.example", "sk-kando-secret", "r2-secret-value"} {
		if strings.Contains(got, secret) {
			t.Fatalf("log keeps %q:\n%s", secret, got)
		}
	}
}
