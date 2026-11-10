package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// DownloadSpeechModel fetches file from baseURL into dir and returns the finished path. A download that stops for any reason but ctx ending keeps what arrived in file.partial, with what the server called that file beside it (speechPartial), and the next download of file asks the server only for the rest; one whose ctx ends, as when the user cancels it, leaves nothing behind. progress gets how much of the whole file is on disk, counting what an earlier download left, and the whole file's size, or -1 while the server has not said.
func DownloadSpeechModel(ctx context.Context, baseURL, file, dir string, progress func(written, total int64)) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, file)
	part := speechPartial{data: dest + ".partial", validator: dest + ".partial.validator"}
	url := strings.TrimRight(baseURL, "/") + "/" + file
	err := part.fetch(ctx, url, file, true, progress)
	if errors.Is(err, errRestart) {
		err = part.fetch(ctx, url, file, false, progress)
	}
	if err != nil {
		if ctx.Err() != nil {
			part.remove()
		}
		return "", err
	}
	if err := os.Rename(part.data, dest); err != nil {
		return "", err
	}
	if err := os.Remove(part.validator); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return dest, nil
}

// errRestart is speechPartial.fetch's answer when the server will not send the rest of what the partial file holds, so the download starts again from the first byte.
var errRestart = errors.New("download starts over")

// speechPartial is a speech model on its way: data holds the bytes that have arrived, and validator what the server called the file they are the start of, its ETag or else its Last-Modified. A partial file without a validator is never resumed, since nothing would tell whether the rest the server sends belongs to the same file.
type speechPartial struct {
	data, validator string
}

func (p speechPartial) remove() {
	os.Remove(p.data)
	os.Remove(p.validator)
}

// resumable is how many bytes of the file data holds and the validator to ask for the rest with, or 0 and "" when there is nothing to resume.
func (p speechPartial) resumable() (offset int64, validator string) {
	info, err := os.Stat(p.data)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return 0, ""
	}
	saved, err := os.ReadFile(p.validator)
	if err != nil {
		return 0, ""
	}
	validator = strings.TrimSpace(string(saved))
	if validator == "" {
		return 0, ""
	}
	return info.Size(), validator
}

// start empties data for a download from the first byte and records validator beside it, or drops the record when the server named the file nothing.
func (p speechPartial) start(validator string) (*os.File, error) {
	out, err := os.OpenFile(p.data, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if validator == "" {
		if err = os.Remove(p.validator); errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
	} else {
		err = os.WriteFile(p.validator, []byte(validator), 0o644)
	}
	if err != nil {
		out.Close()
		return nil, err
	}
	return out, nil
}

// fetch downloads url into data: when resume is set and data holds the start of the file, only the rest of it, which the server must send as a 206 that starts where data ends and names the same file; otherwise, or when the server answers with the whole file, from the first byte. It returns errRestart when the server will send neither the rest nor the whole file in one answer.
func (p speechPartial) fetch(ctx context.Context, url, name string, resume bool, progress func(written, total int64)) error {
	var offset int64
	var validator string
	if resume {
		offset, validator = p.resumable()
	}
	header := http.Header{}
	if offset > 0 {
		header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		header.Set("If-Range", validator)
	}
	resp, err := downloadGet(ctx, url, header)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	total := resp.ContentLength
	var out *os.File
	switch {
	case resp.StatusCode == http.StatusOK:
		offset = 0
		if out, err = p.start(responseValidator(resp)); err != nil {
			return err
		}
	case resp.StatusCode == http.StatusPartialContent && offset > 0:
		// Servers that ignore If-Range, as Hugging Face's does, send the rest of whatever file they now have, so the name is checked here too.
		start, size, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || start != offset || responseValidator(resp) != validator {
			return errRestart
		}
		switch {
		case size >= 0:
			total = size
		case total >= 0:
			total += offset
		}
		if out, err = os.OpenFile(p.data, os.O_WRONLY|os.O_APPEND, 0); err != nil {
			return err
		}
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && offset > 0:
		// There is nothing past the end of data when it already holds the whole file, as when the last download stopped just before its rename.
		_, size, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if named := responseValidator(resp); ok && size == offset && (named == "" || named == validator) {
			if progress != nil {
				progress(offset, offset)
			}
			return nil
		}
		return errRestart
	default:
		return &downloadStatusError{name: name, code: resp.StatusCode}
	}
	written, copyErr := copyProgress(out, resp.Body, offset, total, progress)
	if err := out.Close(); copyErr == nil {
		copyErr = err
	}
	if copyErr == nil && total >= 0 && written != total {
		copyErr = fmt.Errorf("%w: %s ended after %d of %d bytes", ErrDisconnected, name, written, total)
	}
	return copyErr
}

// responseValidator is what resp calls the file it serves: its strong ETag, else its Last-Modified, else "".
func responseValidator(resp *http.Response) string {
	if etag := resp.Header.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		return etag
	}
	return resp.Header.Get("Last-Modified")
}

// copyProgress copies body to out, reporting to progress after every chunk how many bytes of the file are in, starting from offset, and returns that count.
func copyProgress(out io.Writer, body io.Reader, offset, total int64, progress func(written, total int64)) (int64, error) {
	written := offset
	if offset > 0 && progress != nil {
		progress(written, total)
	}
	buf := make([]byte, 256*1024)
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return written, err
			}
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}
