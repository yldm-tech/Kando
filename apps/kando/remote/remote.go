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
	// MethodFilm takes IDParams, a Film's ID, and answers that Film.
	MethodFilm = "films.get"
	// MethodSubtitles takes IDParams, a Film's ID, and answers []Cue from its subtitles.
	MethodSubtitles = "subtitles.get"
	// MethodJobs takes no params and answers []Job, the open library's jobs and those the queue runs for other libraries, as the window's 处理中 shows them: the running one, the waiting ones in order, then the open library's finished ones, newest first.
	MethodJobs = "jobs.list"
	// MethodAdd takes AddParams and answers AddResult: a job for each source, queued as 开始处理 queues them.
	MethodAdd = "jobs.add"
	// MethodCancel takes IDParams, a Job's ID, and answers the Job as it is after canceling: a running job stops, a waiting one leaves the queue.
	MethodCancel = "jobs.cancel"
	// MethodSites takes SitesParams and answers []Site, the websites Kando downloads from that the query finds.
	MethodSites = "sites.search"
	// MethodShow takes IDParams, a Film's ID or "" for the library, brings the window to the front showing it, and answers nothing (null).
	MethodShow = "app.show"
)

// The codes of an Error.
const (
	// CodeParse is a line that is not a JSON object.
	CodeParse = "parse_error"
	// CodeInvalid is a request without a method, or with params the method cannot use.
	CodeInvalid = "invalid_request"
	// CodeUnknownMethod is a method the app does not answer.
	CodeUnknownMethod = "unknown_method"
	// CodeNotFound is an ID that names no film or job of the open library.
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
	// Status is the line under the film's title in the window, in the interface's language: its stage, or why it failed.
	Status string `json:"status,omitempty"`
	JobID  string `json:"job_id,omitempty"`
	// Language is what the subtitles are translated into, as the settings name it.
	Language  string `json:"language,omitempty"`
	VideoPath string `json:"video_path,omitempty"`
	// Subtitles is the bilingual .srt, "" until the film is done; OriginalSubtitles the .srt of the speech alone.
	Subtitles         string `json:"subtitles,omitempty"`
	OriginalSubtitles string `json:"original_subtitles,omitempty"`
}

// Job is a job of the open library, or one the queue runs or will run for another library.
type Job struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Title  string `json:"title,omitempty"`
	// Stage is core.Stage: queued, downloading, transcribing, translating, exporting, complete or failed.
	Stage    string  `json:"stage"`
	Progress float64 `json:"progress"`
	// Status is the line the window shows for it, in the interface's language.
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

// AddFailed is a source MethodAdd did not queue, and why, in the app's words.
type AddFailed struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// Cue is one line of a film's subtitles: when it shows, in milliseconds from the start, what was said, and its translation, "" for a line the translation left out.
type Cue struct {
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
