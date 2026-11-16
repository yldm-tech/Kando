package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// WhisperModelBaseURL is the whisper.cpp ggml model repository.
const WhisperModelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"

// WhisperMirrorBaseURL is the same repository on hf-mirror.com, which serves Hugging Face's files under the same paths and is reachable where huggingface.co often is not, as in mainland China.
const WhisperMirrorBaseURL = "https://hf-mirror.com/ggerganov/whisper.cpp/resolve/main"

// SpeechSourceMirror is the download source of the whisper models that is hf-mirror.com. The zero source, "", is Hugging Face itself.
const SpeechSourceMirror = "hf-mirror"

// SpeechSources are the download sources of the whisper models, in the order the speech settings list them: Hugging Face, then hf-mirror.com.
func SpeechSources() []string { return []string{"", SpeechSourceMirror} }

// SpeechSourceBaseURL is where source serves the whisper models from: hf-mirror.com for SpeechSourceMirror, Hugging Face for anything else.
func SpeechSourceBaseURL(source string) string {
	if source == SpeechSourceMirror {
		return WhisperMirrorBaseURL
	}
	return WhisperModelBaseURL
}

// ErrSpeechModelMismatch is wrapped by DownloadSpeechModel's error when what the server sent is not the model's file: its size or its SHA-256 is not the one pinned in the catalog.
var ErrSpeechModelMismatch = errors.New("download does not match the model's pinned checksum")

// SpeechModel is one whisper.cpp model the speech settings can select. Its name and what it is good for are the app's to say, by ID.
type SpeechModel struct {
	ID          string
	Speed       int
	Accuracy    int
	SizeLabel   string
	File        string
	Recommended bool
	// SHA256 and Size are those of File in ggerganov/whisper.cpp on Hugging Face, as its Git LFS pointer gives them; a download that does not match them is thrown away (DownloadSpeechModel), whichever source served it.
	SHA256 string
	Size   int64
}

// SpeechModels is the whisper.cpp catalog shown in speech settings, smallest first.
func SpeechModels() []SpeechModel {
	return []SpeechModel{
		{ID: "tiny", Speed: 5, Accuracy: 2, SizeLabel: "77.7 MB", File: "ggml-tiny.bin", SHA256: "be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21", Size: 77691713},
		{ID: "base", Speed: 4, Accuracy: 3, SizeLabel: "148 MB", File: "ggml-base.bin", SHA256: "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe", Size: 147951465},
		{ID: "small", Speed: 3, Accuracy: 4, SizeLabel: "487.6 MB", File: "ggml-small.bin", SHA256: "1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b", Size: 487601967},
		{ID: "medium", Speed: 2, Accuracy: 4, SizeLabel: "1.53 GB", File: "ggml-medium.bin", SHA256: "6c14d5adee5f86394037b4e4e8b59f1673b6cee10e3cf0b11bbdbee79c156208", Size: 1533763059},
		{ID: "large-v3-turbo-q5", Speed: 4, Accuracy: 5, SizeLabel: "574 MB", File: "ggml-large-v3-turbo-q5_0.bin", SHA256: "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2", Size: 574041195, Recommended: true},
		{ID: "large-v3-turbo", Speed: 3, Accuracy: 5, SizeLabel: "1.62 GB", File: "ggml-large-v3-turbo.bin", SHA256: "1fc70f774d38eb169993ac391eea357ef47c88757ef72ee5943879b7e8e2bc69", Size: 1624555275},
		{ID: "large-v3", Speed: 2, Accuracy: 5, SizeLabel: "3.1 GB", File: "ggml-large-v3.bin", SHA256: "64d182b440b98d5203c4f9bd541544d84c605196c4f7b845dfa11fb23594d1e2", Size: 3095033483},
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

// SpeechLanguages are the languages the speech settings let the user name as the one spoken in the videos, as whisper.cpp's -l codes, in the order the settings list them: the languages Kando's first-run guide offers, then others whisper recognizes well.
func SpeechLanguages() []string {
	return []string{"en", "zh", "ja", "ko", "fr", "es", "de", "pt", "it", "ru", "nl", "pl", "uk", "tr", "ar", "hi", "id", "vi", "th"}
}

// KnownSpeechLanguage reports whether code is one of SpeechLanguages.
func KnownSpeechLanguage(code string) bool {
	return slices.Contains(SpeechLanguages(), code)
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

// ErrNotSpeechModel is CheckSpeechModelFile's answer for a file that is not a whisper.cpp model.
var ErrNotSpeechModel = errors.New("not a whisper.cpp ggml model")

// CheckSpeechModelFile reports whether path is a whisper.cpp model, as the catalog's files and whisper-cli's other ggml models are: it starts with the ggml magic 0x67676d6c written little-endian, which whisper-cli checks before loading one. It returns ErrNotSpeechModel for any other file and the error that kept it from reading one it could not open.
func CheckSpeechModelFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return ErrNotSpeechModel
		}
		return err
	}
	if string(magic) != "lmgg" {
		return ErrNotSpeechModel
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

// DownloadSpeechModel fetches model's file from baseURL into dir, checks it against the size and SHA-256 the catalog pins for it, and returns the finished path. A download that stops for any reason but ctx ending keeps what arrived in file.partial, with what the server called that file beside it (speechPartial), and the next download of file asks the server only for the rest; one whose ctx ends, as when the user cancels it, leaves nothing behind, and so does one that is not the model's file (ErrSpeechModelMismatch), since its rest would not make it so. progress gets how much of the whole file is on disk, counting what an earlier download left, and the whole file's size.
func DownloadSpeechModel(ctx context.Context, baseURL string, model SpeechModel, dir string, progress func(written, total int64)) (string, error) {
	if model.SHA256 == "" || model.Size <= 0 {
		return "", fmt.Errorf("%s: no checksum pinned", model.File)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, model.File)
	part := speechPartial{data: dest + ".partial", validator: dest + ".partial.validator"}
	url := strings.TrimRight(baseURL, "/") + "/" + model.File
	err := part.fetch(ctx, url, model.File, model.Size, true, progress)
	if errors.Is(err, errRestart) {
		err = part.fetch(ctx, url, model.File, model.Size, false, progress)
	}
	if err == nil {
		err = checkSpeechDownload(ctx, part.data, model)
	}
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, ErrSpeechModelMismatch) {
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

// checkSpeechDownload fails with ErrSpeechModelMismatch unless the file at path is model's: its size and SHA-256 those pinned in the catalog. A ctx that ends while it reads the file ends the check with ctx's error.
func checkSpeechDownload(ctx context.Context, path string, model SpeechModel) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() != model.Size {
		return fmt.Errorf("%w: %s is %d bytes, want %d", ErrSpeechModelMismatch, model.File, info.Size(), model.Size)
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, ctxReader{ctx: ctx, r: f}); err != nil {
		return err
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != model.SHA256 {
		return fmt.Errorf("%w: %s has checksum %s, want %s", ErrSpeechModelMismatch, model.File, got, model.SHA256)
	}
	return nil
}

// ctxReader reads r until ctx ends, then fails with ctx's error, so reading a model of gigabytes stops when its download is canceled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
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

// fetch downloads url into data: when resume is set and data holds the start of the file, only the rest of it, which the server must send as a 206 that starts where data ends and names the same file; otherwise, or when the server answers with the whole file, from the first byte. It returns errRestart when the server will send neither the rest nor the whole file in one answer, and fails with ErrSpeechModelMismatch before writing anything when the server says the file is not want bytes long. progress is told want as the whole size when the server does not say.
func (p speechPartial) fetch(ctx context.Context, url, name string, want int64, resume bool, progress func(written, total int64)) error {
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
		if total >= 0 && total != want {
			return fmt.Errorf("%w: %s is %d bytes, want %d", ErrSpeechModelMismatch, name, total, want)
		}
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
		if total >= 0 && total != want {
			return fmt.Errorf("%w: %s is %d bytes, want %d", ErrSpeechModelMismatch, name, total, want)
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
	whole := total
	if whole < 0 {
		whole = want
	}
	written, copyErr := copyProgress(out, resp.Body, offset, whole, progress)
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
