package cloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// MaxCopy is the largest object one CopyObject copies, on S3, R2, OSS and COS alike.
const MaxCopy int64 = 5 << 30

// ErrCopyTooLarge is Copy's error for an object larger than MaxCopy; nothing is copied. Copying a larger one would take a copy in parts, which the bucket does not do.
var ErrCopyTooLarge = errors.New("object is larger than one copy can store")

// GetRange reads the object at key from offset, length bytes of it or, with length -1, to its end, as the store sends it rather than all at once: the caller closes the body. One the bucket does not hold is ErrNoObject; an offset at or past the end reads nothing.
func (b *Bucket) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 || length < -1 {
		return nil, fmt.Errorf("%s: bad range %d+%d", key, offset, length)
	}
	if length == 0 {
		return io.NopCloser(strings.NewReader("")), nil
	}
	in := &s3.GetObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)}
	if offset > 0 || length > 0 {
		span := "bytes=" + strconv.FormatInt(offset, 10) + "-"
		if length > 0 {
			span += strconv.FormatInt(offset+length-1, 10)
		}
		in.Range = aws.String(span)
	}
	out, err := b.api.GetObject(ctx, in)
	if err != nil {
		var status interface{ HTTPStatusCode() int }
		if errors.As(err, &status) && status.HTTPStatusCode() == http.StatusRequestedRangeNotSatisfiable {
			return io.NopCloser(strings.NewReader("")), nil
		}
		return nil, missing(err)
	}
	return out.Body, nil
}

// PutStream stores size bytes read from r at key, replacing what was there only once all of them are stored, as a store does with one PUT or a completed upload in parts. A body of up to 32 MiB, or up to 64 MiB that can go back to its start (a file), goes in one PUT; a larger one goes in parts, read one part into memory at a time, and an upload that fails is dropped. Unlike PutFile it does not go on from where an earlier one stopped. Like Upload it has no deadline: ctx is the caller's cancel, and a store that stopped taking the body fails with ErrStalled.
func (b *Bucket) PutStream(ctx context.Context, key, contentType string, r io.Reader, size int64) error {
	if size < 0 {
		return fmt.Errorf("%s: unknown size", key)
	}
	if contentType == "" {
		contentType = ContentType(key)
	}
	seeker, seekable := r.(io.ReadSeeker)
	if size <= minPart || (seekable && size <= onePut) {
		if seekable {
			// The SDK sends a body shorter than its length as it is, so a short one is caught here rather than stored.
			left, err := remaining(seeker)
			if err != nil {
				return err
			}
			if left < size {
				return fmt.Errorf("%s: %d of %d bytes: %w", key, left, size, io.ErrUnexpectedEOF)
			}
		} else {
			body := make([]byte, size)
			if _, err := io.ReadFull(r, body); err != nil {
				return err
			}
			seeker = bytes.NewReader(body)
		}
		return sendWatched(ctx, seeker, 0, size, nil, func(ctx context.Context, body io.ReadSeeker) error {
			return (&client{api: b.api}).Put(ctx, b.name, key, contentType, body, size)
		})
	}
	out, err := b.api.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(b.name), Key: aws.String(key), ContentType: aws.String(contentType)})
	if err != nil {
		return err
	}
	id := out.UploadId
	if err := b.putParts(ctx, key, id, r, size); err != nil {
		// The parts sent are of no use; the abort goes out even when ctx is what ended the upload.
		_, _ = b.api.AbortMultipartUpload(context.WithoutCancel(ctx), &s3.AbortMultipartUploadInput{Bucket: aws.String(b.name), Key: aws.String(key), UploadId: id})
		return err
	}
	return nil
}

// remaining is how many bytes r holds from where it is, which it is left at.
func remaining(r io.Seeker) (int64, error) {
	at, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	if _, err := r.Seek(at, io.SeekStart); err != nil {
		return 0, err
	}
	return end - at, nil
}

// putParts sends size bytes of r as the parts of upload id at key, one after the other, and completes the upload.
func (b *Bucket) putParts(ctx context.Context, key string, id *string, r io.Reader, size int64) error {
	part := partSize(size)
	buf := make([]byte, part)
	var completed []types.CompletedPart
	var done int64
	for n := int32(1); done < size; n++ {
		length := min(part, size-done)
		if _, err := io.ReadFull(r, buf[:length]); err != nil {
			return err
		}
		var etag *string
		err := sendWatched(ctx, bytes.NewReader(buf[:length]), done, size, nil, func(ctx context.Context, body io.ReadSeeker) error {
			out, err := b.api.UploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(b.name), Key: aws.String(key), UploadId: id,
				PartNumber: aws.Int32(n), Body: body, ContentLength: aws.Int64(length)}, s3.WithAPIOptions(v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware))
			if err == nil {
				etag = out.ETag
			}
			return err
		})
		if err != nil {
			return err
		}
		completed = append(completed, types.CompletedPart{PartNumber: aws.Int32(n), ETag: etag})
		done += length
	}
	_, err := b.api.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: aws.String(b.name), Key: aws.String(key), UploadId: id,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completed}})
	return err
}

// ListDir is what lies directly under prefix, which ends in a slash for a folder: the objects there, and the folders, each a key prefix ending in a slash, in the order of their keys, page after page.
func (b *Bucket) ListDir(ctx context.Context, prefix string) ([]Object, []string, error) {
	var objects []Object
	var dirs []string
	pages := s3.NewListObjectsV2Paginator(b.api, &s3.ListObjectsV2Input{Bucket: aws.String(b.name), Prefix: aws.String(prefix), Delimiter: aws.String("/")})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, o := range page.Contents {
			objects = append(objects, Object{Key: aws.ToString(o.Key), Size: aws.ToInt64(o.Size), Modified: aws.ToTime(o.LastModified), ETag: aws.ToString(o.ETag)})
		}
		for _, p := range page.CommonPrefixes {
			dirs = append(dirs, aws.ToString(p.Prefix))
		}
	}
	return objects, dirs, nil
}

// HasPrefix reports whether the bucket holds any object whose key starts with prefix, asking for one key only.
func (b *Bucket) HasPrefix(ctx context.Context, prefix string) (bool, error) {
	out, err := b.api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(b.name), Prefix: aws.String(prefix), MaxKeys: aws.Int32(1)})
	if err != nil {
		return false, err
	}
	return len(out.Contents) > 0, nil
}

// Copy copies the object at from to to, in the bucket, replacing what was at to, without the body passing through Kando: ErrNoObject when the bucket does not hold from, ErrCopyTooLarge when it is larger than one CopyObject copies.
func (b *Bucket) Copy(ctx context.Context, from, to string) error {
	src, err := b.Head(ctx, from)
	if err != nil {
		return err
	}
	if src.Size > MaxCopy {
		return fmt.Errorf("%s: %w", from, ErrCopyTooLarge)
	}
	segments := strings.Split(from, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	_, err = b.api.CopyObject(ctx, &s3.CopyObjectInput{Bucket: aws.String(b.name), Key: aws.String(to), CopySource: aws.String(url.PathEscape(b.name) + "/" + strings.Join(segments, "/"))})
	if err != nil {
		var api smithy.APIError
		if errors.As(err, &api) && api.ErrorCode() == "EntityTooLarge" {
			return fmt.Errorf("%s: %w: %v", from, ErrCopyTooLarge, err)
		}
		return missing(err)
	}
	return nil
}
