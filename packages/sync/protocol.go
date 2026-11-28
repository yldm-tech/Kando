// Package sync is Kando's phone sync protocol, which Kando on the desktop serves to the phones paired with it (yldm-tech/Kando#178): the requests' paths and the JSON they carry, the pairing URL the desktop's QR code holds, the short form of the desktop certificate's fingerprint, and a client, which the desktop's tests and the phone use. Protocol v1 (this file) is the paths a phone reads the libraries, films, subtitles, posters and videos by on the local network. Protocol v2 (v2.go) adds messages that send links to the desktop and follow and control its queue, and that read and write the learning state, positions, saved lines, notes, review schedules and corrections, merged field by field (state.go), carried alike by the local network (PathCall) and the storage relay, with Desk answering them on the desktop whichever transport brings them, Journal numbering the changes phones wait for and Ledger keeping the desktop's copy of each library's learning state.
//
// The desktop answers over HTTPS with a self-signed certificate, which the phone pins by its SHA-256 fingerprint (Fingerprint) rather than trusting any authority. A phone pairs once with the one-time code the desktop shows (PathPair), and sends the device token it gets back as a bearer token from then on.
package sync

import (
	"strings"
	"time"
)

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
	// PathVideo is "/v1/films/{id}/video", the film's video file, which answers Range requests and names the file by an ETag, or, for a film the desktop serves from a NAS or a storage location that keeps no ETag (SMB, SFTP), by its Last-Modified date alone, either of which If-Range takes.
	PathVideo = "/v1/films/{id}/video"
)

// FilmsPath is PathFilms for library lib.
func FilmsPath(lib string) string { return "/v1/libraries/" + lib + "/films" }

// QueryWith asks PathFilms or PathLibraries for kinds a phone from before them did not know, so a desktop lists them only to a phone that asks, as "?with=subtitles_only,sound": WithSubtitlesOnly for the films that keep no video (Film.SubtitlesOnly), which a phone knew a film by; WithSound for the films of sound alone (Film.Sound), whose file a phone from before them would keep as an MP4 it cannot play (Extension); WithMaking for the films still being made (Film.Making), which a phone from before them would keep as done; WithLocked on PathLibraries for password-protected libraries still locked for this phone (Library.Locked).
const (
	QueryWith         = "with"
	WithSubtitlesOnly = "subtitles_only"
	WithSound         = "sound"
	WithMaking        = "making"
	WithLocked        = "locked"
)

// AsksWith reports whether query, a request's QueryWith, asks for kind among the kinds it names.
func AsksWith(query, kind string) bool {
	for _, k := range strings.Split(query, ",") {
		if strings.TrimSpace(k) == kind {
			return true
		}
	}
	return false
}

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

// Library is a library of the desktop's: its ID, its name as the desktop shows it, and how many films it holds that a phone can play. A locked library is omitted unless the phone asks with WithLocked, in which case Locked is true, Films is 0, and none of its films is served until the phone unlocks it (OpLibraryUnlock). HasPassword is true for any password-protected library (locked or unlocked) when WithLocked was asked, so the phone can offer to lock it again (OpLibraryLock).
type Library struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Films       int    `json:"films"`
	Locked      bool   `json:"locked,omitzero"`
	HasPassword bool   `json:"has_password,omitzero"`
}

// Libraries is PathLibraries' answer.
type Libraries struct {
	Libraries []Library `json:"libraries"`
}

// Film is a film of a library that a phone can play or study: one that is done processing, with its video on the desktop's disk, or, listed only to a phone that asks for them, one that keeps no video (WithSubtitlesOnly, SubtitlesOnly) and one still being made (WithMaking, Making).
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
	// Sound is a film of sound alone, as a podcast's mp3, listed only to a phone that asks for them (WithSound): PathVideo answers its file, of an audio media type (MediaType), which the player plays without a picture, and VideoSize is its size.
	Sound bool `json:"sound,omitempty"`
	// Next is the ID of the film that plays after this one, as the desktop goes on from it (a course's next lesson, a disc's next track, a playlist's next video), "" for none; the phone plays it next once it keeps it. A film the desktop lists the phone not, as one still being made, may be named.
	Next string `json:"next,omitempty"`
	// Updated is when the film's video or subtitles last changed on the desktop, so a phone knows to fetch them again.
	Updated time.Time `json:"updated"`
	// Making is a film whose job is waiting or running, listed only to a phone that asks for such films (WithMaking): its lines grow while it is made, nearest where it is watched first (OpFilmWatch); PathVideo answers what plays of it so far (FilmWatching.Plays), not_found while nothing does; and once it is done it is listed as any other film, under the same ID.
	Making bool `json:"making,omitempty"`
	// SourceKey names the file of a NAS or a server the film was made of, for a film the desktop indexed or copied from one of its media sources of a kind a phone reaches itself (SMB, SFTP, WebDAV): remotefs.SourceKey of the file's place and size, which says neither where the server is nor what the file is called. A phone that reaches the same server computes the same key for the file and takes this film's subtitles and learning state for it while it plays the video from the server itself. "" for any other film. It is an addition to v1, left out of the JSON when empty, so a phone from before it reads the films as before (TestSourceKeyIsAnAddition).
	SourceKey string `json:"source_key,omitzero"`
	// Work names the work the desktop shows the film in as one card (a show's episodes, a course's lessons, a playlist that is one): the same for each of its films and for no other film of that desktop, of any of its libraries. WorkTitle is the card's name, and WorkIndex the film's place in the work from 1, in the order it plays, which a phone orders the work's films by, as it lists only those that are done. All empty for a film the desktop shows on a card of its own, as one of a work the user split apart there. An addition to v1, left out of the JSON when empty (TestWorksAreAnAddition).
	Work      string `json:"work,omitzero"`
	WorkTitle string `json:"work_title,omitzero"`
	WorkIndex int    `json:"work_index,omitzero"`
	// Watched is a film the desktop played to its end, and PlayedAt when it last played there, zero for one it never played; with where each side stopped (the learning state's positions, which the desktop drops for a film watched to its end) they say which of a work's films a phone goes on with. An addition to v1, left out of the JSON when empty.
	Watched  bool      `json:"watched,omitzero"`
	PlayedAt time.Time `json:"played_at,omitzero"`
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
	// CodeWrongPassword is a library unlock whose password did not match (status 403).
	CodeWrongPassword = "wrong_password"
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
