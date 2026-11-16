package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// File is one object to put. Key is the object name and Path is the local file.
type File struct {
	Key         string
	Path        string
	ContentType string
}

// Client stores one object. The R2 and S3 clients implement it. body can go back to its start, as the SDK needs for a retry.
type Client interface {
	Put(ctx context.Context, bucket, key, contentType string, body io.ReadSeeker, size int64) error
}

// maxPut is the largest object one PUT stores: 5 GiB on S3, OSS and COS, and a little less on R2, which says EntityTooLarge. Tests shorten it.
var maxPut int64 = 5 << 30

// ErrTooLarge is an upload's error when a file is larger than one PUT stores; it fails before a byte is sent.
var ErrTooLarge = errors.New("file is larger than one upload can store")

// ErrStalled is an upload's error when the storage stopped taking the file: no byte of it went out, and no answer came, for stallTimeout. It does not wrap context.Canceled, which is the caller's own cancel.
var ErrStalled = errors.New("upload stalled")

// stallTimeout is how long an upload waits for the storage to take its next bytes, or to answer once the file is sent, before it gives up with ErrStalled. It is longer than the SDK's longest wait between retries, 20 seconds. Tests shorten it.
var stallTimeout = time.Minute

// Endpoint accepts a full URL or a Cloudflare account id. An account id becomes the R2 S3 endpoint.
func Endpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		return raw
	}
	return "https://" + raw + ".r2.cloudflarestorage.com"
}

// Region accepts an AWS region name such as ap-northeast-1.
func Region(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " /\\") || strings.Contains(raw, "://") {
		return ""
	}
	return raw
}

// sharedPrefix is the first segment of an object key when no Google account is signed in. An email always holds an @, so it never names the same folder as an account.
const sharedPrefix = "kando"

// ObjectKey is {email}/{library}/{video folder}/{file}, or kando/{library}/{video folder}/{file} when email is blank because no account is signed in.
func ObjectKey(email, library, videoPath string) string {
	folder := filepath.Base(filepath.Dir(videoPath))
	name := filepath.Base(videoPath)
	if strings.TrimSpace(library) == "" {
		library = "library"
	}
	if strings.TrimSpace(email) == "" {
		email = sharedPrefix
	}
	return path.Join(cleanKey(email), cleanKey(library), cleanKey(folder), cleanKey(name))
}

// SiblingKey keeps the video's prefix and swaps in the subtitle file name.
func SiblingKey(videoKey, subtitlePath string) string {
	return path.Join(path.Dir(videoKey), cleanKey(filepath.Base(subtitlePath)))
}

// ContentType picks a type from the file extension.
func ContentType(filePath string) string {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".mov":
		return "video/quicktime"
	case ".srt":
		return "application/x-subrip"
	default:
		if ext := filepath.Ext(filePath); ext != "" {
			if kind := mime.TypeByExtension(ext); kind != "" {
				return kind
			}
		}
		return "application/octet-stream"
	}
}

// Upload puts each file in order and reports bytes written across the whole set. It has no deadline, which would end a long upload that is still going: ctx is the caller's cancel, and an upload the storage stopped taking fails with ErrStalled.
func Upload(ctx context.Context, client Client, bucket string, files []File, progress func(written, total int64)) error {
	sizes := make([]int64, len(files))
	var total int64
	for i, file := range files {
		info, err := os.Stat(file.Path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.ErrInvalid
		}
		if info.Size() > maxPut {
			return fmt.Errorf("%s: %w", filepath.Base(file.Path), ErrTooLarge)
		}
		sizes[i] = info.Size()
		total += info.Size()
	}
	var done int64
	for i, file := range files {
		if err := put(ctx, client, bucket, file, sizes[i], done, total, progress); err != nil {
			return err
		}
		done += sizes[i]
		if progress != nil {
			progress(done, total)
		}
	}
	return nil
}

// put stores one file of size bytes, base of total already sent, under a watchdog that ends the request once the storage has taken no byte for stallTimeout.
func put(ctx context.Context, client Client, bucket string, file File, size, base, total int64, progress func(written, total int64)) error {
	handle, err := os.Open(file.Path)
	if err != nil {
		return err
	}
	defer handle.Close()
	watched, cancel := context.WithCancel(ctx)
	defer cancel()
	var stalled atomic.Bool
	timer := time.AfterFunc(stallTimeout, func() {
		stalled.Store(true)
		cancel()
	})
	defer timer.Stop()
	body := &progressReader{r: handle, base: base, total: total, fn: progress, timer: timer}
	kind := file.ContentType
	if kind == "" {
		kind = ContentType(file.Path)
	}
	err = client.Put(watched, bucket, file.Key, kind, body, size)
	if err != nil && ctx.Err() == nil && stalled.Load() {
		// The SDK's error is the watchdog's cancel; it is kept as text only, so the stall does not read as the caller's cancel.
		return fmt.Errorf("%w: %v", ErrStalled, err)
	}
	return err
}

func cleanKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return "_"
	}
	var b strings.Builder
	for _, r := range value {
		if r == '/' || r == '\\' || r < 32 {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

// progressReader counts bytes as the caller reads them, and puts timer back to a full stallTimeout whenever bytes are read. Seek moves the file, for the SDK going back to the start of the body for a retry, and the count with it.
type progressReader struct {
	r     io.ReadSeeker
	base  int64
	read  int64
	total int64
	fn    func(written, total int64)
	mark  int64
	timer *time.Timer
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.timer.Reset(stallTimeout)
	}
	p.read += int64(n)
	if p.fn != nil && (p.read-p.mark >= 256*1024 || err == io.EOF) {
		p.mark = p.read
		p.fn(p.base+p.read, p.total)
	}
	return n, err
}

func (p *progressReader) Seek(offset int64, whence int) (int64, error) {
	at, err := p.r.Seek(offset, whence)
	if err != nil {
		return at, err
	}
	p.read, p.mark = at, at
	return at, nil
}
