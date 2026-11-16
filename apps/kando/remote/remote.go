// Package remote is how other processes drive the running Kando: the app listens on a Unix domain socket (SocketPath), and the kando command line (cli) and its MCP server (mcpserver) send it requests there. It speaks newline-delimited JSON, one request per line and one answer per line in order, as MyGo's automation plugin does. It imports no area of the window: the app answers each method on its UI thread (Serve's handler), and the clients only see the types here.
package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// The methods the app answers. Each names its params and result type.
const (
	// MethodStatus takes no params and answers Status.
	MethodStatus = "status"
	// MethodFilms takes FilmsParams and answers []Film, the open library's films, newest first as the window lists them.
	MethodFilms = "films.list"
	// MethodFilm takes IDParams, a Film's ID, and answers that Film. A Job's ID names the film the job makes, so a job's film can be read before its video is downloaded.
	MethodFilm = "films.get"
	// MethodSubtitles takes SubtitlesParams, a Film's ID and the part of its subtitles to read, and answers []Cue from its subtitles. IDParams are SubtitlesParams that ask for all of them.
	MethodSubtitles = "subtitles.get"
	// MethodSearch takes SearchParams and answers []Line: the lines of the open library's subtitles that hold the query, film by film in the order MethodFilms lists them, and line by line in the order they show.
	MethodSearch = "lines.search"
	// MethodJobs takes no params and answers []Job, the open library's jobs and those the queue runs for other libraries, as the window's 处理中 shows them: the running one, the waiting ones in order, then the finished ones, newest first: the open library's and those the queue ran to the end for other libraries since the app started.
	MethodJobs = "jobs.list"
	// MethodJob takes IDParams, a Job's ID, and answers that Job, as MethodJobs lists it.
	MethodJob = "jobs.get"
	// MethodAdd takes AddParams and answers AddResult: a job for each source, queued as 开始处理 queues them.
	MethodAdd = "jobs.add"
	// MethodCancel takes IDParams, a Job's ID, and answers the Job as it is after canceling: a running job stops, a waiting one leaves the queue.
	MethodCancel = "jobs.cancel"
	// MethodRetry takes IDParams, a Job's ID, and answers the Job as it is once queued again under the same ID, as the window's 重试 and 重新处理 queue it: a failed, canceled or complete job runs through the pipeline again. A job still running or waiting is refused.
	MethodRetry = "jobs.retry"
	// MethodRemove takes IDParams, a Job's ID, takes the job off the open library's shelf as the window's 移除 does, stopping it first when it is running, and answers nothing (null). Its files stay on disk.
	MethodRemove = "jobs.remove"
	// MethodSites takes SitesParams and answers []Site, the websites Kando downloads from that the query finds.
	MethodSites = "sites.search"
	// MethodShow takes ShowParams, a Film's ID or "" for the library and where to play the film from, brings the window to the front showing it, and answers ShowResult, what it shows. IDParams are ShowParams without a time.
	MethodShow = "app.show"
)

// Language is the language the app answers in: the messages of Errors, the status lines of Films and Jobs, the Readiness of Status and the reasons of AddFailed are English, whatever language the window shows, as the command line and the MCP server say everything else in English. Machine-readable values, as codes, stages and IDs, are never translated; names that come from the user or the videos, as titles, and the target Language of Status, Film and Job, which the settings name in the user's words, are kept as they are.
const Language = "en"

// The codes of an Error.
const (
	// CodeParse is a line that is not a JSON object.
	CodeParse = "parse_error"
	// CodeInvalid is a request without a method, or with params the method cannot use.
	CodeInvalid = "invalid_request"
	// CodeUnknownMethod is a method the app does not answer.
	CodeUnknownMethod = "unknown_method"
	// CodeNotFound is an ID that names no film or job of the open library, or a film without subtitles.
	CodeNotFound = "not_found"
	// CodeNotReady is a request the app cannot carry out yet: no library is open, or something a job needs is missing. Its message says what, in the app's words.
	CodeNotReady = "not_ready"
	// CodeFailed is anything else that went wrong.
	CodeFailed = "failed"
)

// Request is one line a client writes.
type Request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is the app's line for the request with the same ID: a Result, or an Error.
type Response struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Error is a request the app could not answer, with a Code to test and a Message to read.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf makes an Error with code.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// IsCode reports whether err is an Error with code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// IDParams names one film or job.
type IDParams struct {
	ID string `json:"id"`
}

// FilmsParams filters the films: Source is "youtube", "bilibili", "local" or another site's name as the sidebar groups them, "" for all; Query keeps the films whose title contains it, ignoring case.
type FilmsParams struct {
	Source string `json:"source,omitempty"`
	Query  string `json:"query,omitempty"`
}

// SubtitlesParams name a film and the part of its subtitles to read: the cues that start at FromMS or later, before ToMS when it is set, and with Until UntilWatched, only those the learner has watched (Film.WatchedMS, with WatchedSlackMS), none for a film not played yet. Times are milliseconds from the start of the video.
type SubtitlesParams struct {
	ID     string `json:"id"`
	FromMS int64  `json:"from_ms,omitempty"`
	ToMS   int64  `json:"to_ms,omitempty"`
	Until  string `json:"until,omitempty"`
}

// UntilWatched is the Until of SubtitlesParams and SearchParams that keeps only the lines the learner has watched, as the app's palette and its questions about a film do, so that nothing of what comes after is given away.
const UntilWatched = "watched"

// WatchedSlackMS is how far past Film.WatchedMS a line may start and still count as watched, in milliseconds.
const WatchedSlackMS = 500

// SearchParams are a search of the subtitles' lines: those whose original or translation contains Query, ignoring case; only FilmID's when it is set, else every film's of the open library; with Until UntilWatched only the watched ones (SubtitlesParams). Limit caps the answer, 0 for no cap.
type SearchParams struct {
	Query  string `json:"query"`
	FilmID string `json:"film_id,omitempty"`
	Until  string `json:"until,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// ShowParams name what MethodShow shows: a Film's ID, "" for the library, and, for a film, AtMS, where in its subtitles to play it from in milliseconds, as a Cue's or Line's StartMS gives it; the app adds the film's subtitle delay. Without AtMS a film opens as a click on its poster opens it, and one open already stays as it is.
type ShowParams struct {
	ID   string `json:"id"`
	AtMS *int64 `json:"at_ms,omitempty"`
}

// ShowResult is what MethodShow shows: Shown is the film's ID, or "" for the library, and Reason why it shows the library instead of the film asked for. AtMS is where the film plays from, as ShowParams gave it, when it was asked to.
type ShowResult struct {
	Shown  string `json:"shown"`
	Reason string `json:"reason,omitempty"`
	AtMS   *int64 `json:"at_ms,omitempty"`
}

// The reasons of a ShowResult.
const (
	// ShowInQueue is a film still being processed: the library shows its card in the queue.
	ShowInQueue = "in_queue"
	// ShowNoVideo is a film whose video is not on disk, as a job that failed before its download ended.
	ShowNoVideo = "no_video"
)

// AddParams are the videos to process: links, or local files as paths or file:// URLs. A relative path is taken from Dir, the directory the client runs in.
type AddParams struct {
	Sources []string `json:"sources"`
	Dir     string   `json:"dir,omitempty"`
}

// SitesParams is a search of the supported websites: a name, a domain or a link. Limit caps the answer, 0 for no cap.
type SitesParams struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// Status is what the app is and whether a job can start.
type Status struct {
	Version string `json:"version"`
	// LibraryDir is the open library, "" when none is open.
	LibraryDir string `json:"library_dir"`
	// Ready reports whether a job can start now; Readiness is the sidebar's status line, which says what is missing when it cannot.
	Ready     bool   `json:"ready"`
	Readiness string `json:"readiness"`
	// Tools is where each program Kando runs is, by name (yt-dlp, ffmpeg, whisper-cli), "" for one that is missing.
	Tools map[string]string `json:"tools"`
	// SpeechModel is the whisper model file in use, TranslationModel the AI service's model, and Language what subtitles are translated into.
	SpeechModel      string `json:"speech_model"`
	TranslationModel string `json:"translation_model"`
	Language         string `json:"language"`
	// Running is the ID of the job being processed, Queued how many wait after it.
	Running string `json:"running,omitempty"`
	Queued  int    `json:"queued"`
	// Libraries are the libraries the app knows, as its library switcher lists them: the open one, if any, among them. The tools see only the open one; the user switches libraries in the window.
	Libraries []Library `json:"libraries"`
}

// Library is a library the app knows.
type Library struct {
	Path string `json:"path"`
	// Name is the name the window gives it.
	Name string `json:"name"`
	Open bool   `json:"open"`
	// Films is how many films it holds, as the window would list them; Missing is set, and Films is 0, when its folder cannot be read, as on a drive that is not plugged in.
	Films   int  `json:"films"`
	Missing bool `json:"missing,omitempty"`
}

// Film is a video of the open library, as the window lists it.
type Film struct {
	// ID names the film in MethodFilm, MethodSubtitles and MethodShow. It stays the same while the film's video stays where it is.
	ID     string `json:"id"`
	Title  string `json:"title"`
	Source string `json:"source"`
	// Link is the address the video was downloaded from, "" for a local video.
	Link string `json:"link,omitempty"`
	// Stage is the stage of the job that made it (core.Stage), "" for a video no job has processed.
	Stage string `json:"stage,omitempty"`
	// Progress is how far that stage has gone, from 0 to 1, or -1 when its tool has not said.
	Progress float64 `json:"progress"`
	// Status is the line under the film's title in the window, in English (Language): its stage, or why it failed.
	Status string `json:"status,omitempty"`
	JobID  string `json:"job_id,omitempty"`
	// Language is what the subtitles are translated into, as the settings name it.
	Language  string `json:"language,omitempty"`
	VideoPath string `json:"video_path,omitempty"`
	// Subtitles is the bilingual .srt, "" until the film is done; OriginalSubtitles the .srt of the speech alone.
	Subtitles         string `json:"subtitles,omitempty"`
	OriginalSubtitles string `json:"original_subtitles,omitempty"`
	// ResumeMS is where the player stopped last time, which the window offers to continue from, 0 when it was not watched or was watched to the end. WatchedMS is the furthest point the video was played to from its start, which UntilWatched cuts at; a seek past it does not move it. Both are milliseconds of the video.
	ResumeMS  int64 `json:"resume_ms"`
	WatchedMS int64 `json:"watched_ms"`
}

// Job is a job of the open library, or one the queue runs or will run for another library.
type Job struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Title  string `json:"title,omitempty"`
	// Stage is core.Stage: queued, downloading, transcribing, translating, exporting, complete or failed.
	Stage    string  `json:"stage"`
	Progress float64 `json:"progress"`
	// Status is the line the window shows for it, in English (Language).
	Status string `json:"status,omitempty"`
	// Error is why a failed job failed; Canceled is set for one that was stopped instead.
	Error     string `json:"error,omitempty"`
	Canceled  bool   `json:"canceled,omitempty"`
	Language  string `json:"language,omitempty"`
	VideoPath string `json:"video_path,omitempty"`
	Subtitles string `json:"subtitles,omitempty"`
	// FilmID is the Film the job makes, "" before its video is downloaded.
	FilmID string `json:"film_id,omitempty"`
}

// AddResult is the jobs MethodAdd queued, and why each source it skipped could not be added.
type AddResult struct {
	Jobs    []Job       `json:"jobs"`
	Skipped []AddFailed `json:"skipped,omitempty"`
}

// AddFailed is a source MethodAdd did not queue, and why, in the app's words. For a video the library already has a job for, JobID is that job, Stage its stage (a Job's) and FilmID its film once its video is on disk, so a client can follow it as if it had just been queued.
type AddFailed struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
	JobID  string `json:"job_id,omitempty"`
	Stage  string `json:"stage,omitempty"`
	FilmID string `json:"film_id,omitempty"`
}

// Cue is one line of a film's subtitles: when it shows, in milliseconds from the start, what was said, and its translation, "" for a line the translation left out.
type Cue struct {
	StartMS     int64  `json:"start_ms"`
	EndMS       int64  `json:"end_ms"`
	Text        string `json:"text"`
	Translation string `json:"translation,omitempty"`
}

// Line is a line of a film's subtitles that MethodSearch found: the film, by its ID and title, and the line as a Cue has it.
type Line struct {
	FilmID      string `json:"film_id"`
	Title       string `json:"title"`
	StartMS     int64  `json:"start_ms"`
	EndMS       int64  `json:"end_ms"`
	Text        string `json:"text"`
	Translation string `json:"translation,omitempty"`
}

// Site is a website Kando downloads from.
type Site struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// SocketPath is where the app listens: KANDO_SOCKET when it is set, as tests and a second copy of the app set it, otherwise kando.sock beside the settings (the user's config directory's Kando folder). Without a config directory, as when neither HOME nor XDG_CONFIG_HOME is set, it is kando.sock in XDG_RUNTIME_DIR, the user's own runtime folder, when that is set, else in a Kando folder of the temporary folder, which may be shared with other users, so Listen only uses a folder that is the user's alone (privateFolder).
func SocketPath() string {
	if path := os.Getenv("KANDO_SOCKET"); path != "" {
		return path
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "Kando", "kando.sock")
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "kando.sock")
	}
	return filepath.Join(os.TempDir(), "Kando", "kando.sock")
}

// Timeout is how long a client waits for the app: to start when it was not running, and for each answer.
var Timeout = 30 * time.Second
