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
	// MethodAdd takes AddParams and answers AddResult: a job for each source, queued as 开始处理 queues them. A playlist or a channel link, or a local folder, is skipped: list its videos with MethodLists, and add those.
	MethodAdd = "jobs.add"
	// MethodLists takes ListsParams and answers []List, the videos of each playlist or channel link, or of each local folder as dropping it on the window adds them, in the order of the sources, so a client can say how many there are and ask before it adds them with MethodAdd. It may take minutes, as yt-dlp reads a long playlist page by page (ListTimeout).
	MethodLists = "lists.expand"
	// MethodCancel takes IDParams, a Job's ID, and answers the Job as it is after canceling: a running job stops, a waiting one leaves the queue.
	MethodCancel = "jobs.cancel"
	// MethodRetry takes IDParams, a Job's ID, and answers the Job as it is once queued again under the same ID, as the window's 重试 and 重新处理 queue it: a failed, canceled or complete job runs through the pipeline again. A job still running or waiting is refused.
	MethodRetry = "jobs.retry"
	// MethodTranslate takes IDParams, a Film's ID, and answers the Job of the film as it is once queued to translate the lines its translation left untranslated (Film.Untranslated), as the window's 补译未译的句子 does, without downloading or transcribing it again: every line of a film that was not translated at all (Film.TranslationError). A film with no untranslated line, or still being processed, is refused.
	MethodTranslate = "jobs.translate"
	// MethodRemove takes IDParams, a Job's ID, takes the job off the open library's shelf as the window's 移除 does, stopping it first when it is running, and answers nothing (null). Its files stay on disk.
	MethodRemove = "jobs.remove"
	// MethodMove takes MoveParams, a Film's ID and the library to move it to, moves the film there as the window's Move to Library does, with its files, its job and everything Kando keeps of it, and answers Moved once it is there, which across drives takes as long as copying it (MoveTimeout). A film still being processed is refused.
	MethodMove = "films.move"
	// MethodSites takes SitesParams and answers []Site, the websites Kando downloads from that the query finds.
	MethodSites = "sites.search"
	// MethodSubscriptions takes no params and answers []Subscription, the open library's subscriptions in the order the window's sidebar lists them.
	MethodSubscriptions = "subscriptions.list"
	// MethodSubscribe takes SubscribeParams, a channel, an uploader's space, a collection or a playlist link, subscribes the open library to it as the window's 添加订阅 does, queuing in order the videos a collection or a playlist holds now while a channel's count as seen, so only those that come after are new, and answers Subscribed; its Added says how many were queued. A link the open library follows already answers that subscription, as it is. It may take minutes, as yt-dlp reads the list (ListTimeout).
	MethodSubscribe = "subscriptions.add"
	// MethodSubscriptionCheck takes IDParams, a Subscription's ID, lists its videos now as the window's 立即检查 does, and answers SubscriptionChecked: the new ones that fit its rules are queued, at most the subscribe package's AddPerCheck of them for one that adds by itself, or wait for the user, as its rules say. A paused subscription, or one kept after its site deleted the list, is refused. It may take minutes, as yt-dlp reads the list (ListTimeout).
	MethodSubscriptionCheck = "subscriptions.check"
	// MethodShow takes ShowParams, a Film's ID or "" for the library and where to play the film from, brings the window to the front showing it, and answers ShowResult, what it shows. IDParams are ShowParams without a time.
	MethodShow = "app.show"
	// MethodBrowserState takes BrowserStateParams and answers BrowserState: what the open library has of the videos a page of the browser extension shows, and the words of its buttons, as the built-in browser's pages ask for them.
	MethodBrowserState = "browser.state"
	// MethodBrowserAdd takes BrowserAddParams, one video of a page of the browser extension, adds it as the built-in browser's 下载视频 does, and answers BrowserAdded.
	MethodBrowserAdd = "browser.add"
	// MethodBrowserShow takes BrowserShowParams and brings the window to the front showing the video's film, or the library when it has none, and answers BrowserShown.
	MethodBrowserShow = "browser.show"
)

// Language is the language the app answers in, but for the browser extension's methods (MethodBrowserState, MethodBrowserAdd), whose words the extension shows in the pages and its popup in the language the window shows, as the built-in browser's buttons do: the messages of Errors, the status lines of Films and Jobs, the Readiness of Status and the reasons of AddFailed are English, whatever language the window shows, as the command line and the MCP server say everything else in English. Machine-readable values, as codes, stages and IDs, are never translated; names that come from the user or the videos, as titles, and the target Language of Status, Film and Job, which the settings name in the user's words, are kept as they are.
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

// FilmsParams filters the films: Source is "youtube", "bilibili", "local" or another site's name as the sidebar groups them, "" for all; Query keeps the films whose title contains it, ignoring case; Spoken keeps the films heard in that language, by its code (Film.SpokenLanguage), as the sidebar's languages keep them, "" for all.
type FilmsParams struct {
	Source string `json:"source,omitempty"`
	Query  string `json:"query,omitempty"`
	Spoken string `json:"spoken_language,omitempty"`
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

// MoveParams name a film of the open library and the library MethodMove moves it to, To, by its path or its name as Status's Libraries name it.
type MoveParams struct {
	ID string `json:"id"`
	To string `json:"to"`
}

// Moved is where MethodMove moved a film: its ID in the library it is in now, which is the same job ID for a film with a job and its video's new path for one without, and that library's path and name.
type Moved struct {
	ID      string `json:"id"`
	Library string `json:"library"`
	Name    string `json:"name"`
}

// ShowResult is what MethodShow shows: Shown is the film's ID, or "" for the library, and Reason why it shows the library instead of the film asked for. AtMS is where the film plays from, as ShowParams gave it, when it was asked to.
type ShowResult struct {
	Shown  string `json:"shown"`
	Reason string `json:"reason,omitempty"`
	AtMS   *int64 `json:"at_ms,omitempty"`
}

// The reasons of a ShowResult.
const (
	// ShowInQueue is a film still being processed that cannot be watched yet, as one whose video is not downloaded yet, or one whose job failed: the library shows its card in the queue. A film being processed whose video is on disk opens, as its card's Watch does.
	ShowInQueue = "in_queue"
	// ShowNoVideo is a film whose video is not on disk, as a job that failed before its download ended.
	ShowNoVideo = "no_video"
)

// AddParams are the videos to process: links, or local files as paths or file:// URLs. A relative path is taken from Dir, the directory the client runs in.
type AddParams struct {
	Sources []string `json:"sources"`
	Dir     string   `json:"dir,omitempty"`
}

// ListsParams are the playlist and channel links, and the local folders, to list. A folder is a path or a file:// URL, a relative path taken from Dir, as in AddParams.
type ListsParams struct {
	Sources []string `json:"sources"`
	Dir     string   `json:"dir,omitempty"`
}

// List is the videos of a playlist or a channel link, or of a local folder. A channel lists its Limit most recent videos; a playlist lists all of its own; a folder the videos in it and in the folders directly in it, in their names' order, as dropping it on the window adds them, and NotVideos counts the files in them that are not videos, which are left out. Error says why a source could not be listed, as when it is not a playlist, a channel or a folder, and Videos is then empty.
type List struct {
	Source string `json:"source"`
	// Kind is "playlist", "channel" or "folder".
	Kind      string      `json:"kind,omitempty"`
	Title     string      `json:"title,omitempty"`
	Limit     int         `json:"limit,omitempty"`
	Videos    []ListVideo `json:"videos"`
	NotVideos int         `json:"not_videos,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// ListFolder is the Kind of a List of a local folder's videos.
const ListFolder = "folder"

// ListVideo is one video of a List: the link MethodAdd takes for it, a folder's video as a file:// URL, its title, how long it plays in seconds (0 when the site did not say, and for a folder's video), and whether the open library has it already, waiting, being processed or done, which MethodAdd would skip.
type ListVideo struct {
	URL       string  `json:"url"`
	Title     string  `json:"title,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	InLibrary bool    `json:"in_library,omitempty"`
}

// SitesParams is a search of the supported websites: a name, a domain or a link. Limit caps the answer, 0 for no cap.
type SitesParams struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// SubscribeParams is the link MethodSubscribe subscribes to.
type SubscribeParams struct {
	Source string `json:"source"`
}

// Subscription is a subscription of the open library to a channel, an uploader's space, a collection or a playlist, as the window's sidebar lists it and its page shows it.
type Subscription struct {
	// ID names it in MethodSubscriptionCheck.
	ID string `json:"id"`
	// Name is what the window calls it, Source the link it follows, and Kind what that is: "channel", "space" (a bilibili uploader's), "collection" (a bilibili one) or "playlist". Platform is the site, as "youtube" or "bilibili".
	Name     string `json:"name"`
	Source   string `json:"source"`
	Kind     string `json:"kind"`
	Platform string `json:"platform,omitempty"`
	// Mode is what a check does with the new videos that fit its rules: "auto" queues them, at most AddPerCheck a check, the others waiting as "ask" has them all wait for the user in the window. Filter is which new videos fit: "all", "long" (5 minutes or longer) or "keywords" (titles holding one of Keywords). Cadence is how often the app checks it while it runs: "open" each time it opens, "hourly" or "daily". Series makes the films it adds a series, which share a glossary.
	Mode     string `json:"mode"`
	Filter   string `json:"filter"`
	Keywords string `json:"keywords,omitempty"`
	Cadence  string `json:"cadence"`
	Series   bool   `json:"series,omitempty"`
	// AddPerCheck is how many new videos a check of an "auto" one queues at most.
	AddPerCheck int `json:"add_per_check"`
	// Paused is set while its checks are paused; Gone once its site says the list was deleted; Stopped once the user chose to keep it without checking it. Renamed is the new name the site gave the list, until the user answers it in the window.
	Paused  bool   `json:"paused,omitempty"`
	Gone    bool   `json:"gone,omitempty"`
	Stopped bool   `json:"stopped,omitempty"`
	Renamed string `json:"renamed,omitempty"`
	// Checked is when it was last checked, whether or not that worked, and Error why that check failed, in English (Language).
	Checked time.Time `json:"checked,omitzero"`
	Error   string    `json:"error,omitempty"`
	// Waiting is the new videos waiting for the user to add or skip them in the window, oldest first; Added how many videos it has added to the queue.
	Waiting []ListVideo `json:"waiting"`
	Added   int         `json:"added"`
}

// Subscribed is what MethodSubscribe did: the subscription, and whether the open library followed the link already.
type Subscribed struct {
	Subscription Subscription `json:"subscription"`
	Already      bool         `json:"already,omitempty"`
}

// SubscriptionChecked is what MethodSubscriptionCheck found: New new videos that fit the rules, of which Queued joined the queue and Held wait for the user, and Skipped that did not fit and were skipped. Note says, in English, why some could not be queued, as when a tool a job needs is missing. Subscription is the subscription as it is after the check.
type SubscriptionChecked struct {
	Subscription Subscription `json:"subscription"`
	New          int          `json:"new"`
	Queued       int          `json:"queued"`
	Held         int          `json:"held"`
	Skipped      int          `json:"skipped"`
	Note         string       `json:"note,omitempty"`
}

// Status is what the app is and whether a job can start.
type Status struct {
	Version string `json:"version"`
	// LibraryDir is the open library, "" when none is open.
	LibraryDir string `json:"library_dir"`
	// Ready reports whether a job can start now; Readiness is the sidebar's status line, which says what is missing when it cannot.
	Ready     bool   `json:"ready"`
	Readiness string `json:"readiness"`
	// Tools is where each program Kando runs is, by name (yt-dlp, ffmpeg, whisper-cli), "" for one that is missing. Broken says, by name, why a program whose copy is there does not start, as a Homebrew ffmpeg that cannot load a library Homebrew upgraded; a program that starts is not in it.
	Tools  map[string]string `json:"tools"`
	Broken map[string]string `json:"broken,omitempty"`
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
	// Films is how many films it holds, as the window would list them; Missing is set, and Films is 0, when its folder cannot be read, as on a drive that is not plugged in. Locked is set, and Films is 0, while it has a password that was not typed in the window since Kando started: nothing of its films shows, and the other methods refuse the open library while it is locked.
	Films   int  `json:"films"`
	Missing bool `json:"missing,omitempty"`
	Locked  bool `json:"locked,omitempty"`
}

// VideoLocation is the storage location a film's video was moved to: its name as Settings › Account & Upload names it, its kind, smb, webdav or sftp for a NAS location or the object store's (r2, s3 and the others), the video's path there, relative to the location's folder, and Missing when the location is no longer in Settings, so the video does not play until it is added back.
type VideoLocation struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Missing bool   `json:"missing,omitempty"`
}

// Film is a video of the open library, as the window lists it.
type Film struct {
	// ID names the film in MethodFilm, MethodSubtitles, MethodShow and MethodTranslate. It stays the same while the film's video stays where it is. A video no job has processed is named by its path; once a job processes it, the film is named by the job's ID, and its path still names it.
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
	// Language is what the subtitles are translated into, as the settings name it; "" for a film that was not translated, because the translation failed (TranslationError) or because the film is heard in the language its subtitles are for, whose lines are its own alone.
	Language string `json:"language,omitempty"`
	// Untranslated is how many lines of the subtitles the translation left untranslated, every line of a film that was not translated, which MethodTranslate translates. TranslationError is why a done film was not translated at all, as the AI service refusing its key, in English (Language): its subtitles are then the original lines alone.
	Untranslated     int    `json:"untranslated,omitempty"`
	TranslationError string `json:"translation_error,omitempty"`
	// SpokenLanguage is the language the speech recognition heard in the video, by its code (ja, en, zh), "" until a job transcribed it; SpeechLanguage the one the user said the video is spoken in, which its job is told to hear, "" when the user named none.
	SpokenLanguage string `json:"spoken_language,omitempty"`
	SpeechLanguage string `json:"speech_language,omitempty"`
	VideoPath      string `json:"video_path,omitempty"`
	// VideoLocation is where the video is when it was moved out of the library's folder to a storage location, nil for a video in the folder; VideoPath is then the path it had there, which the film is still known by and its subtitles are named after.
	VideoLocation *VideoLocation `json:"video_location,omitempty"`
	// Subtitles is the bilingual .srt, "" until the film is done; OriginalSubtitles the .srt of the speech alone.
	Subtitles         string `json:"subtitles,omitempty"`
	OriginalSubtitles string `json:"original_subtitles,omitempty"`
	// ResumeMS is where the player stopped last time, which the window offers to continue from, 0 when it was not watched or was watched to the end. WatchedMS is the furthest point the video was played to from its start, which UntilWatched cuts at; a seek past it does not move it, but the user marking the film watched moves it to the end, and marking it unwatched back to 0. Both are milliseconds of the video.
	ResumeMS  int64 `json:"resume_ms"`
	WatchedMS int64 `json:"watched_ms"`
}

// Job is a job of the open library, or one the queue runs or will run for another library.
type Job struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Title  string `json:"title,omitempty"`
	// Stage is core.Stage: queued, downloading, transcribing, translating, exporting, complete or failed. A job that was processing or translating a done film again and failed or was stopped is failed, with its Error or Canceled, though its film stays done with the subtitles it had.
	Stage    string  `json:"stage"`
	Progress float64 `json:"progress"`
	// Status is the line the window shows for it, in English (Language).
	Status string `json:"status,omitempty"`
	// Error is why a failed job failed; Canceled is set for one that was stopped instead.
	Error    string `json:"error,omitempty"`
	Canceled bool   `json:"canceled,omitempty"`
	// Language is what the job translates the subtitles into: for a job processing or translating a done film again, the language it was asked for, which its film's subtitles are in once it is done; "" for a complete job whose film was not translated (TranslationError).
	Language string `json:"language,omitempty"`
	// Untranslated and TranslationError are its film's, as a Film has them: a job that completed without translating its film, as when the AI service refused its key, is complete with TranslationError saying why.
	Untranslated     int    `json:"untranslated,omitempty"`
	TranslationError string `json:"translation_error,omitempty"`
	VideoPath        string `json:"video_path,omitempty"`
	Subtitles        string `json:"subtitles,omitempty"`
	// FilmID is the Film the job makes, "" before its video is downloaded. A job that was making the subtitles of a video no job had processed, and failed or was stopped, leaves that video as it was, named by its path, which FilmID is then.
	FilmID string `json:"film_id,omitempty"`
}

// AddResult is the jobs MethodAdd queued, and why each source it skipped could not be added. Note says, in English, what the client should know of the jobs it queued, as that no AI service is set up, so their subtitles will be the original lines alone.
type AddResult struct {
	Jobs    []Job       `json:"jobs"`
	Skipped []AddFailed `json:"skipped,omitempty"`
	Note    string      `json:"note,omitempty"`
}

// AddFailed is a source MethodAdd did not queue, and why, in the app's words. For a video the library already has a job for, JobID is that job, Stage its stage (a Job's) and FilmID its film once its video is on disk, so a client can follow it as if it had just been queued.
type AddFailed struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
	JobID  string `json:"job_id,omitempty"`
	Stage  string `json:"stage,omitempty"`
	FilmID string `json:"film_id,omitempty"`
}

// BrowserStateParams are the links of the videos a page shows: at most MaxAsk of the browse package's, each to one video of a site its buttons know; the app leaves out any other.
type BrowserStateParams struct {
	URLs []string `json:"urls"`
}

// BrowserState is what MethodBrowserState answers, as the built-in browser's buttons are told it (the browse package's PageState): their words, and what the open library has of each video asked about, by its link as asked: "" nothing, "queued" a job waiting or being processed, "done" a film.
type BrowserState struct {
	Text BrowserText       `json:"text"`
	Have map[string]string `json:"have"`
}

// BrowserText is the words of the page's buttons in the language the window shows: 下载视频, 已在队列中 and 已在资料库中.
type BrowserText struct {
	Add    string `json:"add"`
	Queued string `json:"queued"`
	Done   string `json:"done"`
}

// BrowserAddParams are the video a page of the browser extension adds: its link, to one video of a site the browse package's buttons know, its title as the page names it, "" when it does not, and where the page's player was in it, in seconds, 0 when it was not playing it, so the film opens there once it is done.
type BrowserAddParams struct {
	URL   string  `json:"url"`
	Title string  `json:"title,omitempty"`
	At    float64 `json:"at,omitempty"`
}

// BrowserAdded is what MethodBrowserAdd did. Have is what the library has of the video afterwards, as BrowserState says it. Shown reports that the library had it already and the window came to the front showing it, as 下载视频 does on such a video. Failed reports that it could not be added; the window then came to the front on the library, whose message bar says why. Message is what happened in the window's words, for the extension's popup.
type BrowserAdded struct {
	Have    string `json:"have"`
	Shown   bool   `json:"shown,omitempty"`
	Failed  bool   `json:"failed,omitempty"`
	Message string `json:"message,omitempty"`
}

// BrowserShowParams name the video whose film MethodBrowserShow shows, by its link.
type BrowserShowParams struct {
	URL string `json:"url"`
}

// BrowserShown is what MethodBrowserShow showed: Film reports whether it was the video's film, or its card in the queue, rather than the library alone.
type BrowserShown struct {
	Film bool `json:"film"`
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

// Timeout is how long a client waits for the app: to start when it was not running, and for each answer but those that list videos or move a film (TimeoutOf).
var Timeout = 30 * time.Second

// ListTimeout is how long a client waits for the answers of MethodLists, MethodSubscribe and MethodSubscriptionCheck, which list videos, and the app gives them: longer than the app gives yt-dlp to list one link (media.ListTimeout), so the app's own answer, an error when yt-dlp took too long, comes first.
var ListTimeout = 4 * time.Minute

// MoveTimeout is how long a client waits for MethodMove's answer, and the app gives it: as long as copying a long film to another drive may take. The move goes on in the window when it takes longer.
var MoveTimeout = 30 * time.Minute

// TimeoutOf is how long a client waits for the app's answer to method, and the app takes to give it.
func TimeoutOf(method string) time.Duration {
	switch method {
	case MethodLists, MethodSubscribe, MethodSubscriptionCheck:
		return ListTimeout
	case MethodMove:
		return MoveTimeout
	}
	return Timeout
}
