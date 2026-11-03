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

// SpeechModel is one whisper.cpp model the speech settings can select.
type SpeechModel struct {
	ID          string
	Name        string
	Blurb       string
	Speed       int
	Accuracy    int
	SizeLabel   string
	File        string
	Recommended bool
}

// SpeechModels is the whisper.cpp catalog shown in speech settings, smallest first.
func SpeechModels() []SpeechModel {
	return []SpeechModel{
		{ID: "tiny", Name: "Tiny", Blurb: "只想快速试一下。错字较多，只适合发音清楚的英语。", Speed: 5, Accuracy: 2, SizeLabel: "77.7 MB", File: "ggml-tiny.bin"},
		{ID: "base", Name: "Base", Blurb: "清晰的英语讲解、播客，几乎秒出。", Speed: 4, Accuracy: 3, SizeLabel: "148 MB", File: "ggml-base.bin"},
		{ID: "small", Name: "Small", Blurb: "日常视频够用，中文和其他语言也基本可靠。", Speed: 3, Accuracy: 4, SizeLabel: "487.6 MB", File: "ggml-small.bin"},
		{ID: "medium", Name: "Medium", Blurb: "口音重、语速快的内容更准，但明显更慢。", Speed: 2, Accuracy: 4, SizeLabel: "1.53 GB", File: "ggml-medium.bin"},
		{ID: "large-v3-turbo-q5", Name: "Large v3 Turbo（压缩版）", Blurb: "大多数情况选它：接近最高准确度，体积小、速度快。", Speed: 4, Accuracy: 5, SizeLabel: "574 MB", File: "ggml-large-v3-turbo-q5_0.bin", Recommended: true},
		{ID: "large-v3-turbo", Name: "Large v3 Turbo", Blurb: "效果与压缩版几乎一样，占用空间约 3 倍。", Speed: 3, Accuracy: 5, SizeLabel: "1.62 GB", File: "ggml-large-v3-turbo.bin"},
		{ID: "large-v3", Name: "Large v3", Blurb: "最准也最慢。适合嘈杂环境、多人对话或小语种电影。", Speed: 2, Accuracy: 5, SizeLabel: "3.1 GB", File: "ggml-large-v3.bin"},
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
	req.Header.Set("User-Agent", "LingoCut")
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
