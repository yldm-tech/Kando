package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kando/packages/core"
)

// Tools runs the pipeline with yt-dlp, ffmpeg and whisper.cpp installed on this machine, and translates with an OpenAI-compatible chat API.
type Tools struct {
	// OutputDir receives downloaded videos and their subtitles.
	OutputDir string
	// YTDLP, FFmpeg and Whisper are executable paths; empty means not found.
	YTDLP   string
	FFmpeg  string
	Whisper string
	// WhisperModel is the path of a ggml model file for whisper.cpp.
	WhisperModel string
	Translator   ChatTranslator
}

// EveryAPI is the default translation provider; setting KANDO_LLM_BASE_URL switches to any other OpenAI-compatible provider.
const (
	EveryAPIBaseURL = "https://api.everyapi.ai/v1"
	EveryAPIModel   = "claude-haiku-5-5"
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
		if translator.APIKey == "" {
			if cli := lookTool("everyapi"); cli != "" {
				translator.Credential = everyAPICredential(cli)
			}
		}
		if translator.Model == "" {
			translator.Model = EveryAPIModel
		}
	}
	return Tools{
		OutputDir:    outputDir,
		YTDLP:        lookTool("yt-dlp"),
		FFmpeg:       lookTool("ffmpeg"),
		Whisper:      lookTool("whisper-cli"),
		WhisperModel: os.Getenv("KANDO_WHISPER_MODEL"),
		Translator:   translator,
	}
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

func moviesDir() string {
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

// Apps opened from Finder or a desktop launcher get a minimal PATH that omits package manager directories.
var extraToolDirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin"}

func lookTool(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	for _, dir := range extraToolDirs {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path
		}
	}
	return ""
}

// Relook finds yt-dlp, ffmpeg and whisper-cli again. The output directory and the translator stay as they are.
func (t Tools) Relook() Tools {
	t.YTDLP = lookTool("yt-dlp")
	t.FFmpeg = lookTool("ffmpeg")
	t.Whisper = lookTool("whisper-cli")
	return t
}

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
	} else if t.Translator.BaseURL == EveryAPIBaseURL && t.Translator.APIKey == "" {
		errs = append(errs, errors.New("EveryAPI is not signed in; run `everyapi auth login` or set EVERYAPI_API_KEY"))
	}
	return errors.Join(errs...)
}

func (t Tools) Download(ctx context.Context, source string) (string, error) {
	if path, ok := localVideoPath(source); ok {
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		return path, nil
	}
	out, err := run(ctx, t.YTDLP,
		"--no-playlist",
		"-f", "bv*+ba/b",
		"--merge-output-format", "mp4",
		"-P", t.OutputDir,
		"-o", "%(title).80B [%(id)s]/video.%(ext)s",
		"--print", "after_move:filepath",
		"--", source,
	)
	if err != nil {
		return "", err
	}
	path := lastLine(out)
	if path == "" {
		return "", errors.New("yt-dlp did not report the downloaded file")
	}
	return path, nil
}

// localVideoPath reports the disk path of a file:// video the user picked. Other URLs are downloaded.
func localVideoPath(source string) (string, bool) {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "file" || parsed.Path == "" || (parsed.Host != "" && parsed.Host != "localhost") {
		return "", false
	}
	return parsed.Path, true
}

func (t Tools) Transcribe(ctx context.Context, videoPath string) ([]core.Segment, error) {
	if strings.TrimSpace(videoPath) == "" {
		return nil, errors.New("video path is empty")
	}
	dir, err := os.MkdirTemp("", "kando-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	// whisper.cpp expects 16 kHz mono 16-bit PCM.
	audio := filepath.Join(dir, "audio.wav")
	if _, err := run(ctx, t.FFmpeg, "-nostdin", "-loglevel", "error", "-y", "-i", videoPath, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", audio); err != nil {
		return nil, err
	}
	transcript := filepath.Join(dir, "transcript")
	if _, err := run(ctx, t.Whisper, "-m", t.WhisperModel, "-l", "auto", "-np", "-oj", "-of", transcript, "-f", audio); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(transcript + ".json")
	if err != nil {
		return nil, err
	}
	return parseWhisperJSON(data)
}

func (t Tools) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	return t.Translator.Translate(ctx, segments, language)
}

func (t Tools) Export(_ context.Context, videoPath string, original, translated []core.Segment) (string, error) {
	return exportSubtitles(videoPath, original, translated)
}

// everyAPICredentialVersion is the output version of `everyapi auth credential` this package understands; the CLI bumps it only for incompatible changes.
const everyAPICredentialVersion = 1

// everyAPICredential reads the signed-in EveryAPI CLI's relay key through `everyapi auth credential`, which refreshes OAuth keys under its own lock and resolves the gateway region. renew invalidates a key the gateway rejected.
func everyAPICredential(cli string) func(context.Context, bool) (Endpoint, error) {
	return func(ctx context.Context, renew bool) (Endpoint, error) {
		args := []string{"auth", "credential", "--format=json"}
		if renew {
			args = append(args, "--invalidate")
		}
		out, err := run(ctx, cli, args...)
		if err != nil {
			if strings.Contains(err.Error(), "EVERYAPI_CREDENTIAL_ERROR:not_logged_in") {
				return Endpoint{}, errors.New("EveryAPI is not signed in; run `everyapi auth login`")
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

// run executes a tool and returns its stdout, folding the end of stderr into the error.
func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", fmt.Errorf("%s: %w: %s", filepath.Base(name), err, tail(stderr.String(), 400))
	}
	return stdout.String(), nil
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
