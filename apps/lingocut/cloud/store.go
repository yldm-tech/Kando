package cloud

import (
	"context"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// File is one object to put. Key is the object name and Path is the local file.
type File struct {
	Key         string
	Path        string
	ContentType string
}

// Client stores one object. The R2 client implements it.
type Client interface {
	Put(ctx context.Context, bucket, key, contentType string, body io.Reader, size int64) error
}

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

// ObjectKey is {email}/{library}/{video folder}/{file}.
func ObjectKey(email, library, videoPath string) string {
	folder := filepath.Base(filepath.Dir(videoPath))
	name := filepath.Base(videoPath)
	if strings.TrimSpace(library) == "" {
		library = "library"
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

// Upload puts each file in order and reports bytes written across the whole set.
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
		sizes[i] = info.Size()
		total += info.Size()
	}
	var done int64
	for i, file := range files {
		handle, err := os.Open(file.Path)
		if err != nil {
			return err
		}
		body := &progressReader{r: handle, base: done, total: total, fn: progress}
		kind := file.ContentType
		if kind == "" {
			kind = ContentType(file.Path)
		}
		err = client.Put(ctx, bucket, file.Key, kind, body, sizes[i])
		handle.Close()
		if err != nil {
			return err
		}
		done += sizes[i]
		if progress != nil {
			progress(done, total)
		}
	}
	return nil
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

// progressReader counts bytes as the caller reads them.
type progressReader struct {
	r     io.Reader
	base  int64
	read  int64
	total int64
	fn    func(written, total int64)
	mark  int64
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	p.read += int64(n)
	if p.fn != nil && (p.read-p.mark >= 256*1024 || err == io.EOF) {
		p.mark = p.read
		p.fn(p.base+p.read, p.total)
	}
	return n, err
}
