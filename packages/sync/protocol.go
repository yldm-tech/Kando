// Package sync is Kando's phone sync protocol, which Kando on the desktop serves to the phones paired with it (yldm-tech/Kando#178): the requests' paths and the JSON they carry, the pairing URL the desktop's QR code holds, the short form of the desktop certificate's fingerprint, and a client, which the desktop's tests and the phone use. Protocol v1 (this file) is the paths a phone reads the libraries, films, subtitles, posters and videos by on the local network. Protocol v2 (v2.go) adds messages that send links to the desktop and follow and control its queue, and that read and write the learning state, positions, saved lines, notes, review schedules and corrections, merged field by field (state.go), carried alike by the local network (PathCall) and the storage relay, with Desk answering them on the desktop whichever transport brings them, Journal numbering the changes phones wait for and Ledger keeping the desktop's copy of each library's learning state.
//
// The desktop answers over HTTPS with a self-signed certificate, which the phone pins by its SHA-256 fingerprint (Fingerprint) rather than trusting any authority. A phone pairs once with the one-time code the desktop shows (PathPair), and sends the device token it gets back as a bearer token from then on.
package sync

import "time"

// Version is the protocol's version, the first part of every path.
const Version = "v1"

// The paths of protocol v1. {lib} is a Library's ID and {id} a Film's.
const (
	// PathPair takes a PairRequest by POST and answers a PairResponse. It is the one path that needs no token.
	PathPair = "/v1/pair"
	// PathLibraries answers Libraries.
	PathLibraries = "/v1/libraries"
	// PathFilms is "/v1/libraries/{lib}/films", which answers Films (FilmsPath).
	PathFilms = "/v1/libraries/{lib}/films"
	// PathSubtitles is "/v1/films/{id}/subtitles", which answers Subtitles.
	PathSubtitles = "/v1/films/{id}/subtitles"
	// PathPoster is "/v1/films/{id}/poster", the film's poster as an image file.
	PathPoster = "/v1/films/{id}/poster"
	// PathVideo is "/v1/films/{id}/video", the film's video file, which answers Range requests and carries an ETag.
	PathVideo = "/v1/films/{id}/video"
)

// FilmsPath is PathFilms for library lib.
func FilmsPath(lib string) string { return "/v1/libraries/" + lib + "/films" }

// QueryWith and WithSubtitlesOnly ask PathFilms for the films that keep no video too (Film.SubtitlesOnly), as "?with=subtitles_only". A phone from before them knew a film by its video alone, so a desktop lists them only to a phone that asks.
const (
	QueryWith         = "with"
	WithSubtitlesOnly = "subtitles_only"
)

// SubtitlesPath is PathSubtitles for film id.
func SubtitlesPath(id string) string { return "/v1/films/" + id + "/subtitles" }

// PosterPath is PathPoster for film id.
func PosterPath(id string) string { return "/v1/films/" + id + "/poster" }

// VideoPath is PathVideo for film id.
func VideoPath(id string) string { return "/v1/films/" + id + "/video" }

// PairRequest is what a phone sends to PathPair: the code the desktop shows, and the phone's name and platform as the desktop lists the device.
type PairRequest struct {
	Code     string `json:"code"`
	Device   string `json:"device"`
	Platform string `json:"platform"`
}

// The platforms a PairRequest names.
const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
)

// PairResponse is the desktop's answer to a pairing: the device token the phone sends from then on (Authorization: Bearer), the ID the desktop lists the device under, and the desktop's name.
type PairResponse struct {
	Token    string `json:"token"`
	DeviceID string `json:"device_id"`
	Desktop  string `json:"desktop"`
}

// Library is a library of the desktop's: its ID, its name as the desktop shows it, and how many films it holds that a phone can play. A locked library is never listed.
type Library struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Films int    `json:"films"`
}

// Libraries is PathLibraries' answer.
type Libraries struct {
	Libraries []Library `json:"libraries"`
}

// Film is a film of a library that a phone can play or study: one that is done processing, with its video on the desktop's disk, or, listed only to a phone that asks for them (WithSubtitlesOnly), one that keeps no video (SubtitlesOnly).
type Film struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Source is where the film comes from, as the desktop's sidebar files it: youtube, bilibili, web or local.
	Source string `json:"source"`
	// DurationMS is how long the video plays, 0 when the desktop does not know yet.
	DurationMS int64 `json:"duration_ms"`
	// Spoken is the language spoken in the film, as whisper's code (ja, en), "" when it is not known; Translation is the language its subtitles are translated into, as the desktop names it (简体中文), "" for a film that was not translated.
	Spoken      string `json:"spoken"`
	Translation string `json:"translation"`
	// Untranslated is how many lines the translation left untranslated.
	Untranslated int `json:"untranslated"`
	// Poster is the path of the film's poster (PosterPath), "" for a film that has none yet.
	Poster string `json:"poster"`
	// VideoSize is the size of the video file in bytes, 0 for a film that keeps none.
	VideoSize int64 `json:"video_size"`
	// SubtitlesOnly is a film the desktop keeps no video of, only its subtitles (仅字幕): PathVideo answers it not_found, and its lines are studied without the picture until the desktop downloads the video (OpFilmVideo).
	SubtitlesOnly bool `json:"subtitles_only,omitempty"`
	// Updated is when the film's video or subtitles last changed on the desktop, so a phone knows to fetch them again.
	Updated time.Time `json:"updated"`
}

// Films is PathFilms' answer.
type Films struct {
	Films []Film `json:"films"`
}

// Cue is a line of a film's subtitles: when it shows, in milliseconds from the start, what is said, and its translation, "" where the line was not translated.
type Cue struct {
	StartMS     int64  `json:"start_ms"`
	EndMS       int64  `json:"end_ms"`
	Original    string `json:"original"`
	Translation string `json:"translation"`
}

// Subtitles is PathSubtitles' answer.
type Subtitles struct {
	Cues []Cue `json:"cues"`
}

// ErrorBody is what the desktop answers with a status of 400 or more: a code from the ones below, which a phone tells the user by, and a message in English for logs.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// The codes of ErrorBody.
const (
	// CodeUnauthorized is a request without a token, or with one the desktop does not know or has revoked (status 401).
	CodeUnauthorized = "unauthorized"
	// CodeNoCode is a pairing while the desktop offers none: Phone Sync's settings are not open (status 403).
	CodeNoCode = "no_code"
	// CodeWrongCode is a pairing with a code that is not the one offered (status 403).
	CodeWrongCode = "wrong_code"
	// CodeCodeUsed is a pairing with a code a phone has paired with already (status 403).
	CodeCodeUsed = "code_used"
	// CodeCodeExpired is a pairing with a code past its time, or one tried wrongly too many times (status 403).
	CodeCodeExpired = "code_expired"
	// CodeTooMany is a pairing that came too soon after many others (status 429).
	CodeTooMany = "too_many_requests"
	// CodeNotFound is a library or film the desktop does not serve, a locked library's among them, or one without what was asked for, as a poster (status 404).
	CodeNotFound = "not_found"
	// CodeInvalid is a request the desktop cannot read (status 400).
	CodeInvalid = "invalid"
	// CodeFailed is anything else that went wrong on the desktop (status 500).
	CodeFailed = "failed"
)

// Pairing's limits: how long a code is good for, and how many wrong tries end it.
const (
	CodeLifetime = 5 * time.Minute
	CodeTries    = 5
	// CodeDigits is how many digits a code has.
	CodeDigits = 6
)
