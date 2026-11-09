package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// WhisperModelBaseURL is the whisper.cpp ggml model repository.
const WhisperModelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"

// SpeechModel is one whisper.cpp model the speech settings can select. Its name and what it is good for are the app's to say, by ID.
type SpeechModel struct {
	ID          string
	Speed       int
	Accuracy    int
	SizeLabel   string
	File        string
	Recommended bool
}

// SpeechModels is the whisper.cpp catalog shown in speech settings, smallest first.
func SpeechModels() []SpeechModel {
	return []SpeechModel{
		{ID: "tiny", Speed: 5, Accuracy: 2, SizeLabel: "77.7 MB", File: "ggml-tiny.bin"},
		{ID: "base", Speed: 4, Accuracy: 3, SizeLabel: "148 MB", File: "ggml-base.bin"},
		{ID: "small", Speed: 3, Accuracy: 4, SizeLabel: "487.6 MB", File: "ggml-small.bin"},
		{ID: "medium", Speed: 2, Accuracy: 4, SizeLabel: "1.53 GB", File: "ggml-medium.bin"},
		{ID: "large-v3-turbo-q5", Speed: 4, Accuracy: 5, SizeLabel: "574 MB", File: "ggml-large-v3-turbo-q5_0.bin", Recommended: true},
		{ID: "large-v3-turbo", Speed: 3, Accuracy: 5, SizeLabel: "1.62 GB", File: "ggml-large-v3-turbo.bin"},
		{ID: "large-v3", Speed: 2, Accuracy: 5, SizeLabel: "3.1 GB", File: "ggml-large-v3.bin"},
	}
}

// SpeechModelByID returns the catalog entry for id.
func SpeechModelByID(id string) (SpeechModel, bool) {
	for _, model := range SpeechModels() {
		if model.ID == id {
			return model, true
		}
	}
	return SpeechModel{}, false
}

// ResolveSpeechModel returns the local file for a catalog model. configured wins when its name matches, then the first existing file in dirs.
func ResolveSpeechModel(configured string, dirs []string, file string) string {
	if file != "" && configured != "" && filepath.Base(configured) == file && fileExists(configured) {
		return configured
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, file)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

// DownloadSpeechModel fetches file from baseURL into dir and returns the finished path. A canceled download leaves nothing behind.
func DownloadSpeechModel(ctx context.Context, baseURL, file, dir string, progress func(written, total int64)) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, file)
	partial := dest + ".partial"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/"+file, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Kando")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", file, resp.StatusCode)
	}
	out, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	success := false
	defer func() {
		out.Close()
		if !success {
			os.Remove(partial)
		}
	}()
	buf := make([]byte, 256*1024)
	var written int64
	total := resp.ContentLength
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return "", err
			}
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(partial, dest); err != nil {
		return "", err
	}
	success = true
	return dest, nil
}
