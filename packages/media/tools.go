package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"lingocut/packages/core"
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

// ToolsFromEnv configures Tools from LINGOCUT_* environment variables and the executables it can find.
func ToolsFromEnv() Tools {
	outputDir := os.Getenv("LINGOCUT_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = defaultOutputDir()
	}
	baseURL := os.Getenv("LINGOCUT_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return Tools{
		OutputDir:    outputDir,
		YTDLP:        lookTool("yt-dlp"),
		FFmpeg:       lookTool("ffmpeg"),
		Whisper:      lookTool("whisper-cli"),
		WhisperModel: os.Getenv("LINGOCUT_WHISPER_MODEL"),
		Translator: ChatTranslator{
			BaseURL: baseURL,
			APIKey:  os.Getenv("LINGOCUT_LLM_API_KEY"),
			Model:   os.Getenv("LINGOCUT_LLM_MODEL"),
		},
	}
}

func defaultOutputDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "LingoCut")
	}
	folder := "Videos"
	if runtime.GOOS == "darwin" {
		folder = "Movies"
	}
	return filepath.Join(home, folder, "LingoCut")
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

func (t Tools) Check() error {
	var errs []error
	for _, tool := range []struct{ name, path string }{{"yt-dlp", t.YTDLP}, {"ffmpeg", t.FFmpeg}, {"whisper-cli", t.Whisper}} {
		if tool.path == "" {
			errs = append(errs, fmt.Errorf("%s is not installed", tool.name))
		}
	}
	if t.WhisperModel == "" {
		errs = append(errs, errors.New("LINGOCUT_WHISPER_MODEL is not set"))
	} else if _, err := os.Stat(t.WhisperModel); err != nil {
		errs = append(errs, fmt.Errorf("whisper model: %w", err))
	}
	if t.Translator.Model == "" {
		errs = append(errs, errors.New("LINGOCUT_LLM_MODEL is not set"))
	}
	return errors.Join(errs...)
}

func (t Tools) Download(ctx context.Context, source string) (string, error) {
	out, err := run(ctx, t.YTDLP,
		"--no-playlist",
		"-f", "bv*+ba/b",
		"--merge-output-format", "mp4",
		"-P", t.OutputDir,
		"-o", "%(title).80B [%(id)s].%(ext)s",
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

func (t Tools) Transcribe(ctx context.Context, videoPath string) ([]core.Segment, error) {
	dir, err := os.MkdirTemp("", "lingocut-*")
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
