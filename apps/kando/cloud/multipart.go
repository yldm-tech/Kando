package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// Resume is how far an upload of a file in parts (Bucket.PutFile) got, which the caller keeps, so an upload that stopped, the app quit or the network gone, goes on from the parts the store took rather than from the start: the store's ID for the upload, the size of its parts, the file as it was when the upload began (its size and when it changed), and the parts taken, by number with the ETag the store gave each. The zero Resume starts an upload anew.
type Resume struct {
	UploadID string         `json:"uploadId,omitempty"`
	PartSize int64          `json:"partSize,omitempty"`
	Size     int64          `json:"size,omitempty"`
	Modified time.Time      `json:"modified,omitzero"`
	Parts    []UploadedPart `json:"parts,omitempty"`
}

// UploadedPart is a part the store took: its number, from 1, and its ETag.
type UploadedPart struct {
	N    int32  `json:"n"`
	ETag string `json:"etag"`
}

// The sizes of an upload: a file of up to onePut bytes goes in one PUT, a larger one in parts of at least minPart bytes, as many as maxParts, all of one size but the last, as R2 asks. Every store takes a part of 5 MiB or more and up to 10,000 of them, and one PUT of up to 5 GiB (maxPut), a little less on R2. Tests shorten onePut and minPart.
var (
	onePut  int64 = 64 << 20
	minPart int64 = 32 << 20
)

const maxParts = 10_000

// partSize is the size of the parts of a file of size bytes: minPart, or for a file larger than maxParts of those, its size over maxParts, in whole MiB.
func partSize(size int64) int64 {
	n := max(minPart, (size+maxParts-1)/maxParts)
	if n > minPart {
		n = (n + 1<<20 - 1) &^ (1<<20 - 1)
	}
	return n
}

// PutFile stores the file at path at key, reading it as it goes, with progress told every 256 KB. A file of up to 64 MiB goes in one PUT; a larger one, which may be well over the 5 GiB one PUT stores, goes in parts, one after the other, and save is told after the upload begins and after each part what to keep to go on later (resume, the zero Resume to start anew); an upload of another size or time of the file, or one the store no longer knows, starts anew, and one the file changed since is left for the store to drop. Like Upload it has no deadline: ctx is the caller's cancel, and a part the store stopped taking fails with ErrStalled.
func (b *Bucket) PutFile(ctx context.Context, key, path, contentType string, resume Resume, save func(Resume) error, progress func(done, total int64)) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.ErrInvalid
	}
	size := info.Size()
	if contentType == "" {
		contentType = ContentType(path)
	}
	if size <= onePut {
		return put(ctx, &client{api: b.api}, b.name, File{Key: key, Path: path, ContentType: contentType}, size, 0, size, progress)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	same := resume.UploadID != "" && resume.Size == size && resume.Modified.Equal(info.ModTime()) && resume.PartSize == partSize(size)
	if same {
		taken, err := b.parts(ctx, key, resume.UploadID)
		switch {
		case errors.Is(err, errNoUpload):
			same = false
		case err != nil:
			return err
		default:
			// The parts are as the store lists them, which a part it took after the last save is among.
			resume.Parts = taken
		}
	}
	if !same {
		if resume.UploadID != "" {
			// The file changed or the store forgot the upload: the parts taken of it are of no use.
			_, _ = b.api.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(b.name), Key: aws.String(key), UploadId: aws.String(resume.UploadID)})
		}
		out, err := b.api.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(b.name), Key: aws.String(key), ContentType: aws.String(contentType)})
		if err != nil {
			return err
		}
		resume = Resume{UploadID: aws.ToString(out.UploadId), PartSize: partSize(size), Size: size, Modified: info.ModTime()}
		if err := save(resume); err != nil {
			return err
		}
	}
	count := int32((size + resume.PartSize - 1) / resume.PartSize)
	taken := map[int32]bool{}
	var done int64
	for _, p := range resume.Parts {
		if p.N >= 1 && p.N <= count && !taken[p.N] {
			taken[p.N] = true
			done += min(resume.PartSize, size-int64(p.N-1)*resume.PartSize)
		}
	}
	if progress != nil {
		progress(done, size)
	}
	for n := int32(1); n <= count; n++ {
		if taken[n] {
			continue
		}
		at := int64(n-1) * resume.PartSize
		length := min(resume.PartSize, size-at)
		var etag string
		err := sendWatched(ctx, io.NewSectionReader(file, at, length), done, size, progress, func(ctx context.Context, body io.ReadSeeker) error {
			out, err := b.api.UploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(b.name), Key: aws.String(key), UploadId: aws.String(resume.UploadID),
				PartNumber: aws.Int32(n), Body: body, ContentLength: aws.Int64(length)}, s3.WithAPIOptions(v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware))
			if err == nil {
				etag = aws.ToString(out.ETag)
			}
			return err
		})
		if err != nil {
			return err
		}
		done += length
		taken[n] = true
		resume.Parts = append(resume.Parts, UploadedPart{N: n, ETag: etag})
		if err := save(resume); err != nil {
			return err
		}
	}
	parts := slices.Clone(resume.Parts)
	slices.SortFunc(parts, func(a, b UploadedPart) int { return int(a.N - b.N) })
	parts = slices.CompactFunc(parts, func(a, b UploadedPart) bool { return a.N == b.N })
	completed := make([]types.CompletedPart, 0, len(parts))
	for _, p := range parts {
		if p.N <= count {
			completed = append(completed, types.CompletedPart{PartNumber: aws.Int32(p.N), ETag: aws.String(p.ETag)})
		}
	}
	_, err = b.api.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: aws.String(b.name), Key: aws.String(key), UploadId: aws.String(resume.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completed}})
	return err
}

// errNoUpload is parts' error for an upload the store does not know: completed, dropped, or never begun.
var errNoUpload = errors.New("no such upload")

// parts are the parts the store took of upload id at key, page after page.
func (b *Bucket) parts(ctx context.Context, key, id string) ([]UploadedPart, error) {
	var out []UploadedPart
	pages := s3.NewListPartsPaginator(b.api, &s3.ListPartsInput{Bucket: aws.String(b.name), Key: aws.String(key), UploadId: aws.String(id)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			var none *types.NoSuchUpload
			var api smithy.APIError
			if errors.As(err, &none) || (errors.As(err, &api) && api.ErrorCode() == "NoSuchUpload") {
				return nil, fmt.Errorf("%w: %v", errNoUpload, err)
			}
			return nil, err
		}
		for _, p := range page.Parts {
			out = append(out, UploadedPart{N: aws.ToInt32(p.PartNumber), ETag: aws.ToString(p.ETag)})
		}
	}
	return out, nil
}

// sendWatched has send send r, base of total bytes already sent, under a watchdog that ends it once the store has taken no byte for stallTimeout, as put does a whole file.
func sendWatched(ctx context.Context, r io.ReadSeeker, base, total int64, progress func(done, total int64), send func(context.Context, io.ReadSeeker) error) error {
	watched, cancel := context.WithCancel(ctx)
	defer cancel()
	var stalled atomic.Bool
	timer := time.AfterFunc(stallTimeout, func() {
		stalled.Store(true)
		cancel()
	})
	defer timer.Stop()
	err := send(watched, &progressReader{r: r, base: base, total: total, fn: progress, timer: timer})
	if err != nil && ctx.Err() == nil && stalled.Load() {
		return fmt.Errorf("%w: %v", ErrStalled, err)
	}
	return err
}
