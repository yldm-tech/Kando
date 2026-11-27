package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"kando/packages/core"
)

// Tools runs the pipeline with yt-dlp, ffmpeg and whisper.cpp installed on this machine, and translates with an OpenAI-compatible chat API.
type Tools struct {
	// OutputDir receives downloaded videos and their subtitles.
	OutputDir string
	// DownloadTemp holds what yt-dlp writes while a video downloads (the separate video and sound tracks of a split format, .part files), one folder per source, so the library under OutputDir only ever sees the merged video. Empty uses a folder in the system's temp folder. When it is not on the library's volume a hidden folder of the library is used instead (downloadTempFor).
	DownloadTemp string
	// YTDLP, FFmpeg and Whisper are executable paths; empty means not found.
	YTDLP   string
	FFmpeg  string
	Whisper string
	// Deno is the path of the Deno that yt-dlp solves YouTube's JavaScript challenges with (ytdlpRuntimeArgs); empty means not found, and yt-dlp then looks for one on PATH itself. It is not one of the programs a job needs (Check): yt-dlp downloads without it from the sites that set no challenge, and from YouTube only what it serves without one.
	Deno string
	// EveryAPI is the path of the everyapi command-line program, whose sign-in EveryAPI translates with when it has no API key and Kando is not signed in itself (UseEveryAPICLI); empty means not found. Kando does not download it.
	EveryAPI string
	// EveryAPILogin is Kando's own EveryAPI sign-in, when the user signed in in the app, nil when nobody did (UseEveryAPILogin).
	EveryAPILogin EveryAPILogin
	// WhisperModel is the path of a ggml model file for whisper.cpp.
	WhisperModel string
	// SpeechLanguage is the language spoken in the videos, as whisper.cpp's -l names it (one of SpeechLanguages); empty lets whisper detect it. A job that names the language of its own video (core.Job.SpeechLanguage) is heard in that one instead (Make).
	SpeechLanguage string
	// WhisperThreads is how many threads whisper.cpp recognizes speech with (-t), 0 for as many as suit this computer (WhisperThreadsAuto). WhisperHints are words the videos say that whisper should spell as written, such as names and terms (--prompt), "" for none.
	WhisperThreads int
	WhisperHints   string
	Translator     ChatTranslator
	// Engine, when set, is the translation source Translate translates a film's lines with in place of Translator, such as a machine translation service a plugin offers; nil translates with Translator, as Kando always has. Only the translation goes to it: proofreading, the final proofread and term suggestions stay with Translator. EngineName is what the viewer knows it by, which a job it fails names (StageError.Source); one left empty is named by the engine's Go type (engineName). With Engine set and Translator not set up (chats), Make leaves out proofreading, the final proofread and term suggestions rather than fail at them.
	Engine     SegmentTranslator
	EngineName string
	// Cookies is the browser whose login Download borrows, as yt-dlp's --cookies-from-browser names it (chrome, safari, firefox, edge, brave); empty borrows none.
	Cookies string
	// OEmbed is YouTube's oEmbed endpoint, which names a video yt-dlp could not look up, as one that is age-restricted or blocked where this machine is; empty asks nothing.
	OEmbed string
	// SiteCookies gives the cookies the app's own web pages hold for source's site, and the user agent of the page that got them ("" when unknown), so that a site the user signed in to there downloads as that user. Download passes them to yt-dlp in a cookies file of its own (--cookies) beside the browser it borrows (Cookies). nil, an error or no cookies download without them.
	SiteCookies func(ctx context.Context, source string) (cookies []Cookie, userAgent string, err error)
	// CookieDir is where the cookies file of a download is written, in a folder of its own that goes once yt-dlp exits; empty uses the system's temp folder.
	CookieDir string
	// Offline is offline mode: Download refuses a link and ListVideos every source with ErrOffline, and the translator refuses a service that is not on this computer (ChatTranslator.Offline), each before anything is sent. A local video is still read where it is.
	Offline bool
	// OfflineSwitch, when set, is offline mode as it is now, shared by every copy of Tools: a job under way holds a copy made as it started, so offline mode turned on later still keeps that job from downloading or reaching a service off this computer in the steps it has not started yet. Tools are offline when Offline or the switch is on (offlineNow).
	OfflineSwitch *atomic.Bool
	// Sources, when set, copies the videos of the user's media sources (kando-source:// addresses) into the library for Download; nil fails such a video with ErrNoSourceReader.
	Sources SourceReader
}

// EveryAPI is the default translation provider; setting KANDO_LLM_BASE_URL switches to any other OpenAI-compatible provider.
const (
	EveryAPIBaseURL = "https://api.everyapi.ai/v1"
	EveryAPIModel   = "claude-haiku-5-5"
)

// The reasons the everyapi command-line program's sign-in cannot translate, which the app words differently: the program is not on this computer any more, or it is but nobody signed in to it. The program's sign-in (UseEveryAPICLI) returns them as they are, so errors.Is tells them apart and a failed job's text contains their words.
var (
	ErrEveryAPIMissing   = errors.New("the everyapi command-line program was not found; install it from https://everyapi.ai or set EVERYAPI_API_KEY")
	ErrEveryAPISignedOut = errors.New("EveryAPI is not signed in; run `everyapi auth login`")
)

// The reasons Kando's own EveryAPI sign-in cannot translate, which name no command-line program: nobody is signed in, neither in Kando nor in the everyapi program, or the sign-in ended (EveryAPI no longer takes it, as after it was signed out on the website); or the sign-in settings.json says there is was not found where Kando keeps it (the system's secure store). CheckTranslate returns ErrEveryAPILoginSignedOut for EveryAPI without a key or a sign-in, and the in-app sign-in returns both as they are.
var (
	ErrEveryAPILoginSignedOut = errors.New("EveryAPI is not signed in; sign in to EveryAPI in Kando's AI settings or enter an API key")
	ErrEveryAPILoginMissing   = errors.New("Kando's EveryAPI sign-in was not found on this computer; sign in to EveryAPI again in Kando's AI settings")
)

// CredentialSource is where a translator's Credential comes from (ChatTranslator.CredentialSource).
type CredentialSource uint8

const (
	// CredentialNone is a translator without a Credential: its key is its APIKey, or it needs none.
	CredentialNone CredentialSource = iota
	// CredentialCLI is the sign-in of the everyapi command-line program (Tools.EveryAPI).
	CredentialCLI
	// CredentialLogin is Kando's own EveryAPI sign-in (Tools.EveryAPILogin).
	CredentialLogin
)

// ToolsFromEnv configures Tools from KANDO_* environment variables and the executables it can find.
func ToolsFromEnv() Tools {
	outputDir := os.Getenv("KANDO_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = DefaultLibraryDir()
	}
	translator := ChatTranslator{
		BaseURL: os.Getenv("KANDO_LLM_BASE_URL"),
		APIKey:  os.Getenv("KANDO_LLM_API_KEY"),
		Model:   os.Getenv("KANDO_LLM_MODEL"),
	}
	// EveryAPI defaults apply only without a custom provider, so its key and model names never reach another API.
	if translator.BaseURL == "" {
		translator.BaseURL = EveryAPIBaseURL
		if translator.APIKey == "" {
			translator.APIKey = os.Getenv("EVERYAPI_API_KEY")
		}
		if translator.Model == "" {
			translator.Model = EveryAPIModel
		}
	}
	downloadTemp, cookieDir := "", ""
	if cache, err := os.UserCacheDir(); err == nil {
		downloadTemp = filepath.Join(cache, "Kando", "downloads")
		cookieDir = filepath.Join(cache, "Kando")
	}
	tools := Tools{
		OutputDir:    outputDir,
		DownloadTemp: downloadTemp,
		YTDLP:        lookTool("yt-dlp"),
		FFmpeg:       lookTool("ffmpeg"),
		Whisper:      lookTool("whisper-cli"),
		Deno:         lookTool("deno"),
		EveryAPI:     lookTool("everyapi"),
		WhisperModel: os.Getenv("KANDO_WHISPER_MODEL"),
		Translator:   translator,
		OEmbed:       YouTubeOEmbed,
		CookieDir:    cookieDir,
	}
	return tools.UseEveryAPICLI()
}

// UseEveryAPICLI gives the translator the sign-in it uses when it is EveryAPI without an API key: Kando's own (EveryAPILogin) when the user signed in in the app, else the everyapi command-line program's when the program was found (EveryAPI), else none; and takes the sign-in away otherwise, as when a key was entered or another service chosen. So a key, from KANDO_LLM_API_KEY, EVERYAPI_API_KEY or the settings, comes before Kando's sign-in, and that before the program's. Whoever changes the translator's endpoint or key, or finds the program again, calls it again; it never drops Kando's own sign-in, which only UseEveryAPILogin changes.
func (t Tools) UseEveryAPICLI() Tools {
	t.Translator.Credential, t.Translator.Renew, t.Translator.CredentialSource = nil, nil, CredentialNone
	if t.Translator.BaseURL != EveryAPIBaseURL || t.Translator.APIKey != "" {
		return t
	}
	switch {
	case t.EveryAPILogin != nil:
		login := t.EveryAPILogin
		t.Translator.Credential = func(ctx context.Context, renew bool) (Endpoint, error) {
			endpoint, err := login(ctx, nil)
			if err != nil || !renew {
				return endpoint, err
			}
			return login(ctx, &endpoint)
		}
		t.Translator.Renew = func(ctx context.Context, rejected Endpoint) (Endpoint, error) { return login(ctx, &rejected) }
		t.Translator.CredentialSource = CredentialLogin
	case t.EveryAPI != "":
		t.Translator.Credential, t.Translator.CredentialSource = everyAPICredential(t.EveryAPI), CredentialCLI
	}
	return t
}

// EveryAPILogin is Kando's own EveryAPI sign-in: the key to send, with rejected nil, or, with rejected the endpoint it gave that the API refused, a key in place of that one. A sign-in that renews its key once for every request refused at the same time gives the requests that come after the renewal the key it got, as their rejected key is no longer its key.
type EveryAPILogin func(ctx context.Context, rejected *Endpoint) (Endpoint, error)

// UseEveryAPILogin makes login Kando's own EveryAPI sign-in, which the translator then uses as UseEveryAPICLI says; nil signs Kando out, and the translator falls back to the everyapi command-line program's sign-in when there is one.
func (t Tools) UseEveryAPILogin(login EveryAPILogin) Tools {
	t.EveryAPILogin = login
	return t.UseEveryAPICLI()
}

// EveryAPIUnsigned reports whether t is EveryAPI without an API key and without a sign-in, neither Kando's own nor the everyapi command-line program's, so it cannot translate until the user signs in or enters a key.
func (t ChatTranslator) EveryAPIUnsigned() bool {
	return t.Credential == nil && t.BaseURL == EveryAPIBaseURL && t.APIKey == ""
}

// LibraryExt is Kando's own library bundle suffix.
const LibraryExt = ".kando"

// DefaultLibraryDir is the bundle that holds one folder per video.
func DefaultLibraryDir() string {
	return filepath.Join(moviesDir(), "影视库"+LibraryExt)
}

// VimpoLibraryDir is the other app's library. Kando does not store its videos there.
func VimpoLibraryDir() string {
	return filepath.Join(moviesDir(), "影视库.vimpo")
}

// MoviesDir is the folder a new library is created in unless the user picks another place.
func MoviesDir() string { return moviesDir() }

// VideosDir is the user's videos folder as the system names it, when it does: the Videos known folder on Windows, wherever it was moved, and the XDG_VIDEOS_DIR of a Linux desktop, named in the user's language. main sets it before the tools are looked up; empty, as on macOS and in tests, the folder is ~/Movies on macOS and ~/Videos elsewhere.
var VideosDir string

func moviesDir() string {
	if VideosDir != "" {
		return VideosDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	folder := "Videos"
	if runtime.GOOS == "darwin" {
		folder = "Movies"
	}
	return filepath.Join(home, folder)
}

// extraToolDirs are the package managers' folders lookTool looks in after PATH (packageManagerDirs).
var extraToolDirs = packageManagerDirs(runtime.GOOS, os.Getenv)

// packageManagerDirs are the folders package managers put their programs in on goos, with getenv reading the environment. Apps opened from Finder or a desktop launcher get a minimal PATH that omits Homebrew's and Linuxbrew's. On Windows winget, Scoop and Chocolatey put the programs they install, or links to them, in a folder of their own, which they add to the PATH in the registry; Kando's PATH, read as it started, has it only once Kando restarts, so without these 重新检测 would not find a program installed since.
func packageManagerDirs(goos string, getenv func(string) string) []string {
	if goos != "windows" {
		return []string{"/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin"}
	}
	var dirs []string
	under := func(base string, elem ...string) {
		if base != "" {
			dirs = append(dirs, filepath.Join(append([]string{base}, elem...)...))
		}
	}
	// winget links a portable package's programs in the user's folder, or in Program Files for one installed for every user.
	under(getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links")
	under(getenv("ProgramFiles"), "WinGet", "Links")
	// Scoop keeps a shim of each program under $SCOOP, ~\scoop unless the user moved it, and under $SCOOP_GLOBAL, %ProgramData%\scoop, for one installed for every user.
	if scoop := getenv("SCOOP"); scoop != "" {
		under(scoop, "shims")
	} else {
		under(getenv("USERPROFILE"), "scoop", "shims")
	}
	if global := getenv("SCOOP_GLOBAL"); global != "" {
		under(global, "shims")
	} else {
		under(getenv("ProgramData"), "scoop", "shims")
	}
	// Chocolatey keeps a shim of each program in the bin folder of $ChocolateyInstall, %ProgramData%\chocolatey unless moved.
	if choco := getenv("ChocolateyInstall"); choco != "" {
		under(choco, "bin")
	} else {
		under(getenv("ProgramData"), "chocolatey", "bin")
	}
	return dirs
}

// lookTool finds the program name: the first of its Candidates, which is not run to see whether it works; the app does that (Probe) off the UI thread and may choose another.
func lookTool(name string) string {
	if found := Candidates(name); len(found) > 0 {
		return found[0]
	}
	return ""
}

// Candidates is every copy of the program name Kando could run, in the order it prefers them: the one Kando downloaded for itself (OwnTool), unless its kit is a Fallback; the one on PATH; those in the package managers' folders; and last Kando's own when its kit is a Fallback. A copy reached two ways, as Homebrew's folder both on PATH and among the package managers', is listed once.
func Candidates(name string) []string {
	var found []string
	seen := map[string]bool{}
	add := func(path string) {
		if path == "" {
			return
		}
		key := path
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			key = resolved
		}
		if !seen[key] {
			seen[key] = true
			found = append(found, path)
		}
	}
	own := OwnTool(name)
	kit, _ := KitFor(name)
	if !kit.Fallback {
		add(own)
	}
	if path, err := exec.LookPath(name); err == nil {
		add(path)
	}
	// LookPath of a path checks the file as it does those on PATH: an executable file, or on Windows one with an extension of PATHEXT, as ffmpeg.exe.
	for _, dir := range extraToolDirs {
		if path, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			add(path)
		}
	}
	if kit.Fallback {
		add(own)
	}
	return found
}

// Relook finds yt-dlp, ffmpeg, whisper-cli, Deno and the everyapi command-line program again, and signs the translator in through everyapi as UseEveryAPICLI does, so a program installed since Kando started is used without restarting it. The output directory and the translator's endpoint, model and key stay as they are.
func (t Tools) Relook() Tools {
	t.YTDLP = lookTool("yt-dlp")
	t.FFmpeg = lookTool("ffmpeg")
	t.Whisper = lookTool("whisper-cli")
	t.Deno = lookTool("deno")
	t.EveryAPI = lookTool("everyapi")
	return t.UseEveryAPICLI()
}

// Check reports which of the download and speech tools a job needs are missing. The translator is not one of them: a job without it still makes the film's subtitles, the original lines alone, and Make checks it before translating (CheckTranslate).
func (t Tools) Check() error {
	var errs []error
	for _, tool := range []struct{ name, path string }{{"yt-dlp", t.YTDLP}, {"ffmpeg", t.FFmpeg}, {"whisper-cli", t.Whisper}} {
		if tool.path == "" {
			errs = append(errs, fmt.Errorf("%s is not installed", tool.name))
		}
	}
	if t.WhisperModel == "" {
		errs = append(errs, errors.New("KANDO_WHISPER_MODEL is not set"))
	} else if _, err := os.Stat(t.WhisperModel); err != nil {
		errs = append(errs, fmt.Errorf("whisper model: %w", err))
	}
	return errors.Join(errs...)
}

// hearing is how Hear hears the speech of a video under ctx: with the whisper model WhisperModel, in the language a film the user said is spoken in a language of its own is heard in (withSpeechLanguage), else in the one the settings name, "" for whichever whisper detects (hearer).
func (t Tools) hearing(ctx context.Context) (model, language string) {
	language = speechLanguageOf(ctx)
	if language == "" {
		language = strings.TrimSpace(t.SpeechLanguage)
	}
	return t.WhisperModel, language
}

// CheckTranslate reports what translating needs and is missing, which Check leaves out (TranslateChecker): the translator's model and its sign-in, or, with Engine set, what Engine says it needs (its own CheckTranslate) and offline mode for one not on this computer.
func (t Tools) CheckTranslate() error {
	// A source of its own is checked by itself, as it needs neither the model nor the sign-in.
	if t.Engine != nil {
		if err := t.engineOffline(); err != nil {
			return err
		}
		if tc, ok := t.Engine.(TranslateChecker); ok {
			if err := tc.CheckTranslate(); err != nil {
				return t.engineErr(err)
			}
		}
		return nil
	}
	// The sign-in asks EveryAPI's servers, so offline mode refuses before it, as it refuses the translation.
	if err := t.withOffline().offline(); err != nil {
		return err
	}
	var errs []error
	if t.Translator.Model == "" {
		errs = append(errs, errors.New("KANDO_LLM_MODEL is not set"))
	}
	if t.Translator.Credential != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, err := t.Translator.Credential(ctx, false)
		cancel()
		if err != nil {
			errs = append(errs, err)
		}
	} else if t.Translator.EveryAPIUnsigned() {
		errs = append(errs, ErrEveryAPILoginSignedOut)
	}
	return errors.Join(errs...)
}

// Download fetches source with yt-dlp into its own folder under OutputDir and returns the video's path; a local video's is where it is, and a media source's video is copied into the library by Sources (copySource). While it runs it reports to the context's Trace the video's title and thumbnail, once yt-dlp has looked the video up, and how far the download has gone. When yt-dlp fails before it could say what the video is called, a YouTube video's title and thumbnail are asked of OEmbed, so the card that says why it failed can name it.
//
// yt-dlp downloads into a folder kept for source (downloadTempFor), in DownloadTemp or a hidden folder of the library, and moves only the merged video into the library, where the library watcher would otherwise take a finished video track for a video of its own while the sound track is still downloading (GatherVideos). The folder is removed once the video is in the library and kept when the download fails, so a retry resumes it.
func (t Tools) Download(ctx context.Context, source string) (string, error) {
	if path, ok := localVideoPath(source); ok {
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		return path, nil
	}
	// A video of a media source is copied by the app's reader, not yt-dlp (copySource).
	if IsSourceRef(source) {
		return t.copySource(ctx, source)
	}
	if t.offlineNow() {
		return "", ErrOffline
	}
	temp, err := t.downloadTempFor(source)
	if err != nil {
		return "", err
	}
	named := false
	path, err := t.ytdlpDownload(ctx, source, temp, &named)
	// An earlier Kando renamed the video track of a download that had not merged yet to video.<ext> and left the other track beside it; yt-dlp takes that silent file for the finished video. Leftovers beside the video mean it never merged, so it is removed with them and downloaded again.
	if err == nil && holdsDownloadLeftovers(filepath.Dir(path)) && pathInside(t.OutputDir, path) {
		if err = removeUnmerged(path); err == nil {
			path, err = t.ytdlpDownload(ctx, source, temp, &named)
		}
	}
	if err != nil {
		if !named && ctx.Err() == nil && t.OEmbed != "" {
			if info, ok := lookupOEmbed(ctx, t.OEmbed, source); ok {
				reportInfo(ctx, info)
			}
		}
		return "", err
	}
	_ = os.RemoveAll(temp)
	// The library's hidden folder goes too once no other download is left in it; a folder that still holds one is not empty and stays.
	if filepath.Base(filepath.Dir(temp)) == libraryDownloads {
		_ = os.Remove(filepath.Dir(temp))
	}
	return path, nil
}

// ytdlpDownload runs yt-dlp once for source, with temp as the folder of its partial files, and returns the path it says the video was saved to. named is set once yt-dlp has said what the video is called.
func (t Tools) ytdlpDownload(ctx context.Context, source, temp string, named *bool) (string, error) {
	reader := &ytdlpReader{
		progress: func(p core.Progress) { reportProgress(ctx, p) },
		info: func(info Info) {
			*named = *named || info.Title != ""
			reportInfo(ctx, info)
		},
		chapters: func(chapters []core.Chapter) { reportChapters(ctx, chapters) },
	}
	args := []string{
		"--no-playlist",
		// --no-playlist only picks the video of a link that names both a video and its list. A link to a list alone that core.ParseLink does not recognise as one, on any of the sites yt-dlp reads, would still download every video of the list into the library; with this yt-dlp stops after the first, exiting with ytdlpMaxDownloadsExit.
		"--max-downloads", "1",
		"-f", formatFor(ctx),
		"--merge-output-format", "mp4",
		"-P", t.OutputDir,
		// A relative temp path would be taken as inside the library.
		"-P", "temp:" + temp,
		"-o", "%(title).80B [%(id)s]/video.%(ext)s",
		"--print", ytdlpPicturesTemplate,
		"--print", ytdlpInfoTemplate,
		"--print", ytdlpFormatsTemplate,
		"--print", "after_move:filepath",
		// --print implies --quiet, which hides the progress unless asked for again.
		"--progress", "--newline",
		"--progress-template", ytdlpProgressTemplate,
	}
	// yt-dlp looks for ffmpeg, which merges the video with its sound, on PATH, which an app opened from Finder does not extend to Homebrew's folder; without it the video is saved silent.
	if t.FFmpeg != "" {
		args = append(args, "--ffmpeg-location", t.FFmpeg)
	}
	args = append(args, t.ytdlpRuntimeArgs()...)
	if t.Cookies != "" {
		args = append(args, "--cookies-from-browser", t.Cookies)
	}
	// The cookies of the app's own pages go in a file made for this run, which yt-dlp also writes back to as it exits; runFollowing returns only once it has, on every path, so the folder goes after it.
	if file, userAgent, done := t.siteCookiesFile(ctx, source); file != "" {
		defer done()
		args = append(args, "--cookies", file)
		if userAgent != "" {
			args = append(args, "--user-agent", userAgent)
		}
	}
	if _, stderr, err := runFollowing(ctx, false, reader.line, t.YTDLP, append(args, "--", source)...); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != ytdlpMaxDownloadsExit || reader.path == "" {
			return "", withNoJSRuntime(withClosedBrowserData(err, t.Cookies), stderr, t.Deno)
		}
	}
	if reader.path == "" {
		return "", errors.New("yt-dlp did not report the downloaded file")
	}
	return reader.path, nil
}

// ytdlpRuntimeArgs has yt-dlp solve YouTube's JavaScript challenges with the Deno Kando found (Tools.Deno), by its path: yt-dlp looks for Deno only on PATH, which an app opened from Finder or a desktop launcher does not extend to Homebrew's folder, and Kando's own Deno is in neither. Without one, the formats YouTube serves only to a client that solved them are missing, which a signed-in download, that yt-dlp asks of YouTube's web clients, fails for ("The page needs to be reloaded", "No video formats found"). Empty when Kando found no Deno, and yt-dlp looks on PATH as it would.
func (t Tools) ytdlpRuntimeArgs() []string {
	if t.Deno == "" {
		return nil
	}
	return []string{"--js-runtimes", "deno:" + t.Deno}
}

// ErrNoJSRuntime is wrapped by a download's error when yt-dlp failed to solve the site's JavaScript challenge while Kando found no Deno to give it (Tools.Deno), which no update of yt-dlp mends: a Deno does.
var ErrNoJSRuntime = errors.New("yt-dlp found no JavaScript runtime")

// ytdlpChallengeSigns are how yt-dlp warns that it could not solve YouTube's JavaScript challenge (extractor/youtube/jsc): it found none of the runtimes it may run, which it says only of the clients it asks signed out, or the challenge failed, which it says of every client, signed in too, whether it had no runtime or its solver scripts are older than YouTube's player.
var ytdlpChallengeSigns = []string{"No supported JavaScript runtime could be found", "challenge solving failed"}

// withNoJSRuntime wraps err, a failed download's, in ErrNoJSRuntime when stderr, what yt-dlp printed, warns that it could not solve the challenge while deno, the Deno Kando gave it, is empty: yt-dlp looks only on PATH, which Kando searched too (lookTool), so it had none. With a Deno the failure is its solver scripts', which an update of yt-dlp mends (core.FailOutdated).
func withNoJSRuntime(err error, stderr, deno string) error {
	if deno != "" || !slices.ContainsFunc(ytdlpChallengeSigns, func(sign string) bool { return strings.Contains(stderr, sign) }) {
		return err
	}
	return fmt.Errorf("%w (%w)", err, ErrNoJSRuntime)
}

// ytdlpMaxDownloadsExit is how yt-dlp exits once it has downloaded as many videos as --max-downloads allows, which it does after the one video every download asks for, and also when that video was already downloaded. The video it names (after_move:filepath) is then there, and the download succeeded.
const ytdlpMaxDownloadsExit = 101

// libraryDownloads is the hidden folder of the library that holds partial downloads when DownloadTemp is not on the library's volume. The library's readers skip names that start with a dot.
const libraryDownloads = ".kando-downloads"

// downloadTempFor is the absolute folder that holds source's partial download, the same for the same source and library. It is a folder of DownloadTemp when that is on the library's volume, and otherwise one in the library's hidden libraryDownloads: yt-dlp moves the merged video into the library with a rename only on the same volume, and across volumes copies it into video.<ext> in place, so a download stopped during the copy would leave a cut-off video that the next try takes for a finished one.
func (t Tools) downloadTempFor(source string) (string, error) {
	sum := sha256.Sum256([]byte(source))
	name := hex.EncodeToString(sum[:8])
	base := t.DownloadTemp
	if base == "" {
		base = filepath.Join(os.TempDir(), "kando-downloads")
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	library, err := filepath.Abs(t.OutputDir)
	if err != nil {
		return "", err
	}
	if os.MkdirAll(base, 0o755) == nil && sameVolume(base, library) {
		return filepath.Join(base, name), nil
	}
	return filepath.Join(library, libraryDownloads, name), nil
}

// sameVolume reports whether directories a and b are known to be on one volume. A platform without directory identities (IdentifyDir) never knows.
func sameVolume(a, b string) bool {
	devA, _, okA := IdentifyDir(a).Parts()
	devB, _, okB := IdentifyDir(b).Parts()
	return okA && okB && devA == devB
}

// removeUnmerged removes video and the files beside it that a download leaves before it merges (downloadLeftover).
func removeUnmerged(video string) error {
	folder := filepath.Dir(video)
	entries, err := os.ReadDir(folder)
	if err != nil {
		return err
	}
	remove := []string{video}
	for _, entry := range entries {
		if !entry.IsDir() && downloadLeftover(entry.Name()) {
			remove = append(remove, filepath.Join(folder, entry.Name()))
		}
	}
	for _, path := range remove {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// localVideoPath reports the disk path of a file:// video the user picked. Other URLs are downloaded.
func localVideoPath(source string) (string, bool) {
	parsed, err := url.Parse(source)
	if err != nil {
		return "", false
	}
	return core.FilePath(parsed)
}

// NoAudioStream reports whether ffmpeg's error says the input had no sound to extract: with -vn and nothing but a picture left, it has no stream to write ("Output file does not contain any stream", "Output file #0 does not contain any stream" before ffmpeg 7).
func NoAudioStream(ffmpegError string) bool {
	return strings.Contains(ffmpegError, "does not contain any stream")
}

// WhisperArgs are the arguments whisper-cli is run with to recognize the speech of audio, a 16 kHz mono WAV file, in language, whisper's code or "auto", writing its lines as JSON to out.json: the model, the threads (WhisperThreads), the language, the hints (WhisperHints) and the progress on stderr (-pp), which -np leaves alone. Settings › 语音识别 shows them as the command it runs.
func (t Tools) WhisperArgs(language, out, audio string) []string {
	return append(t.whisperBase(language), "-np", "-pp", "-oj", "-of", out, "-f", audio)
}

// WhisperPrompt is the hints typed in Settings › 语音识别 as whisper.cpp's --prompt takes them: one line of words separated by commas, the blank lines and the spaces around each dropped, "" for none. whisper reads the prompt as text said before the audio, so the words it lists are spelled as they are.
func WhisperPrompt(hints string) string {
	var words []string
	for _, line := range strings.FieldsFunc(hints, func(r rune) bool { return r == '\n' || r == ',' || r == '，' || r == '、' }) {
		if word := strings.TrimSpace(line); word != "" {
			words = append(words, word)
		}
	}
	return strings.Join(words, ", ")
}

// WhisperThreadChoices are the thread counts Settings › 语音识别 offers besides 0, as many as suit this computer.
var WhisperThreadChoices = []int{2, 4, 8}

// WhisperThreadsAuto is how many threads whisper.cpp recognizes speech with when WhisperThreads is 0 (whisperThreadCount).
func WhisperThreadsAuto() int { return whisperThreadCount() }

// whisperThreadCount uses the available CPU without letting a large machine
// create more decoder contention than whisper.cpp can use effectively. Four is
// whisper-cli's default, but it leaves most modern Macs idle during decoding.
func whisperThreadCount() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}

// SegmentTranslator translates a film's lines into language, as Pipeline.Translate does: ChatTranslator, or a source of its own set as Tools.Engine. One that leaves some lines untranslated returns every line with an *UntranslatedError. One that implements TranslateChecker is asked before a film is translated what it needs and is missing, and one whose Local method reports true is used in offline mode too.
type SegmentTranslator interface {
	Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error)
}

// localSource is a translation source that may say it runs on this computer, as ChatTranslator does, which offline mode leaves on.
type localSource interface {
	Local() bool
}

// engineOffline is ErrOffline, as Engine's failure (engineErr), while offline mode is on and Engine is not on this computer, as the translator refuses a service that is not.
func (t Tools) engineOffline() error {
	if local, ok := t.Engine.(localSource); ok && local.Local() {
		return nil
	}
	if t.offlineNow() {
		return t.engineErr(ErrOffline)
	}
	return nil
}

// engineName is what Engine's failures name it by: EngineName, or, when the caller set none, which is a mistake, the engine's Go type, so a failure is never taken for the AI service's.
func (t Tools) engineName() string {
	if name := strings.TrimSpace(t.EngineName); name != "" {
		return name
	}
	return fmt.Sprintf("%T", t.Engine)
}

// engineErr is err as Engine's failure, which names it (EngineOf).
func (t Tools) engineErr(err error) error {
	return &engineError{name: t.engineName(), err: err}
}

// chats reports whether Translator is set up to do what Kando asks of an AI service apart from translating, which goes to it even with Engine set: proofreading what the speech recognition heard, the final proofread and term suggestions. Without Engine it always is, as a job without its translator is told so before translating (CheckTranslate). With Engine it is when Translator has a model, an address and a key or a sign-in, and offline mode leaves it on; whether the sign-in still works is not asked, as that asks EveryAPI's servers. Make leaves those steps out when it is not (chatter).
func (t Tools) chats() bool {
	if t.Engine == nil {
		return true
	}
	tr := t.withOffline()
	switch {
	case strings.TrimSpace(tr.Model) == "", strings.TrimSpace(tr.BaseURL) == "":
		return false
	case tr.EveryAPIUnsigned():
		return false
	}
	return tr.offline() == nil
}

// Translate translates segments into language with Engine when one is set, else with Translator (ChatTranslator.Translate). Engine's failure names it (StageError.Source); offline mode refuses one not on this computer before anything is sent.
func (t Tools) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	if t.Engine == nil {
		return t.withOffline().Translate(ctx, segments, language)
	}
	if err := t.engineOffline(); err != nil {
		return nil, err
	}
	translated, err := t.Engine.Translate(ctx, segments, language)
	if err != nil {
		err = t.engineErr(err)
	}
	return translated, err
}

// Proofread corrects what whisper misheard through the translator (ChatTranslator.Proofread).
func (t Tools) Proofread(ctx context.Context, segments []core.Segment, title string) ([]core.Segment, error) {
	return t.withOffline().Proofread(ctx, segments, title)
}

// everyAPICredentialVersion is the output version of `everyapi auth credential` this package understands; the CLI bumps it only for incompatible changes.
const everyAPICredentialVersion = 1

// everyAPICredential reads the signed-in EveryAPI CLI's relay key through `everyapi auth credential`, which refreshes OAuth keys under its own lock and resolves the gateway region. renew invalidates a key the gateway rejected. It returns ErrEveryAPISignedOut when nobody is signed in to the CLI, and ErrEveryAPIMissing when the program has gone since it was found.
func everyAPICredential(cli string) func(context.Context, bool) (Endpoint, error) {
	return func(ctx context.Context, renew bool) (Endpoint, error) {
		args := []string{"auth", "credential", "--format=json"}
		if renew {
			args = append(args, "--invalidate")
		}
		out, err := run(ctx, cli, args...)
		if err != nil {
			switch {
			case strings.Contains(err.Error(), "EVERYAPI_CREDENTIAL_ERROR:not_logged_in"):
				return Endpoint{}, ErrEveryAPISignedOut
			case errors.Is(err, fs.ErrNotExist):
				return Endpoint{}, ErrEveryAPIMissing
			}
			return Endpoint{}, fmt.Errorf("EveryAPI credential: %w", err)
		}
		var cred struct {
			Version int    `json:"version"`
			BaseURL string `json:"base_url"`
			APIKey  string `json:"api_key"`
		}
		if err := json.Unmarshal([]byte(out), &cred); err != nil {
			return Endpoint{}, fmt.Errorf("EveryAPI credential: %w", err)
		}
		if cred.Version != everyAPICredentialVersion {
			return Endpoint{}, fmt.Errorf("EveryAPI credential version %d is not supported; update Kando", cred.Version)
		}
		if cred.BaseURL == "" || cred.APIKey == "" {
			return Endpoint{}, errors.New("EveryAPI credential is missing its base URL or key")
		}
		return Endpoint{BaseURL: cred.BaseURL, APIKey: cred.APIKey}, nil
	}
}

// run executes a tool and returns its stdout, folding the end of stderr into the error. Its failure is not logged: probes such as EmbeddedCover, a poster frame or the EveryAPI credential fail as their answer (no cover art, not signed in), and logging those would fill the log with failures that are not ones. A job's stage uses runStage.
func run(ctx context.Context, name string, args ...string) (string, error) {
	return runTool(ctx, false, name, args...)
}

// runStage is run for a step whose failure is a real one, as a job's stage or a yt-dlp update: a failed tool's stderr is logged too (logToolFailure).
func runStage(ctx context.Context, name string, args ...string) (string, error) {
	return runTool(ctx, true, name, args...)
}

func runTool(ctx context.Context, logFailure bool, name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	err := retryBusy(ctx, func() error {
		stdout.Reset()
		stderr.Reset()
		cmd := Command(ctx, name, args...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		return cmd.Run()
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if logFailure {
			logToolFailure(name, err, stderr.String())
		}
		return "", fmt.Errorf("%s: %w: %s", filepath.Base(name), err, toolError(stderr.String()))
	}
	return stdout.String(), nil
}

// maxLoggedStderr caps how much of a failed tool's stderr goes to the log: its last 64 KiB, where the error is.
const maxLoggedStderr = 64 << 10

// logToolFailure writes what a failed tool printed on stderr to the log, up to its last maxLoggedStderr bytes, before toolError shortens it for the job's error. It names the tool by its file name and leaves out its arguments, which hold the user's links and files; a tool stopped by its context is not a failure and is not logged.
func logToolFailure(name string, err error, stderr string) {
	if stderr = tail(stderr, maxLoggedStderr); stderr == "" {
		stderr = "(nothing on stderr)"
	}
	log.Printf("%s failed: %v\n%s", filepath.Base(name), err, stderr)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "..." + strings.ToValidUTF8(s[len(s)-n:], "")
}
