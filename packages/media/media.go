package media

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Pipeline is the cross-platform boundary for media processing.
// Implementations will be provided by the desktop application and platform
// adapters without leaking OS-specific details into the core package.
type Pipeline interface {
	Download(ctx context.Context, source, outputTemplate string) (string, error)
	Transcribe(videoPath string) (string, error)
	Translate(subtitlePath string, language string) (string, error)
	Export(videoPath string, subtitlePaths ...string) (string, error)
}

var ErrDownloaderNotFound = errors.New("yt-dlp was not found; install yt-dlp and try again")

type Downloader struct {
	Binary string
}

func (d Downloader) Download(ctx context.Context, source, outputTemplate string) (string, error) {
	if strings.TrimSpace(source) == "" {
		return "", errors.New("video source is empty")
	}
	if strings.TrimSpace(outputTemplate) == "" {
		return "", errors.New("download output template is empty")
	}

	binary := d.Binary
	if binary == "" {
		binary = "yt-dlp"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return "", ErrDownloaderNotFound
	}

	args := []string{"--no-playlist", "--print", "after_move:filepath", "-o", outputTemplate, source}
	cmd := exec.CommandContext(ctx, binary, args...)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("yt-dlp failed: %w", err)
	}

	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", errors.New("yt-dlp completed without returning a file path")
	}
	return path, nil
}

func (d Downloader) Transcribe(string) (string, error) {
	return "", errors.New("transcription is not implemented")
}
func (d Downloader) Translate(string, string) (string, error) {
	return "", errors.New("translation is not implemented")
}
func (d Downloader) Export(string, ...string) (string, error) {
	return "", errors.New("export is not implemented")
}
