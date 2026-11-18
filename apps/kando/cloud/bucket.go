package cloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// Bucket is one bucket of the user's, for the storage relay (yldm-tech/Kando#219), which keeps small objects in it, reads and lists them, and hands phones presigned links rather than a key: get, put, head, list and delete, and presign a GET or a PUT. It works alike for every kind of store, all of which speak S3.
type Bucket struct {
	api     *s3.Client
	presign *s3.PresignClient
	name    string
}

// Object is an object of a bucket as a listing or a HEAD tells it.
type Object struct {
	Key      string
	Size     int64
	Modified time.Time
	ETag     string
}

// ErrNoObject is the error of Get and Head for a key the bucket does not hold.
var ErrNoObject = errors.New("no such object")

// ErrObjectTooLarge is Get's error for an object larger than the limit it was given; nothing past the limit is read.
var ErrObjectTooLarge = errors.New("object is larger than expected")

// MaxPresign is the longest a presigned link stays good: the most SigV4 signs for, on S3 and on R2, OSS and COS alike. Presign shortens a longer one to it.
const MaxPresign = 7 * 24 * time.Hour

// OpenBucket is cfg's bucket. It does not talk to the network.
func OpenBucket(cfg Store) (*Bucket, error) {
	opened, err := Open(cfg)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(cfg.Bucket)
	if name == "" {
		return nil, errors.New("no bucket")
	}
	c := opened.(*client)
	return &Bucket{api: c.api, presign: s3.NewPresignClient(c.api), name: name}, nil
}

// Name is the bucket's name.
func (b *Bucket) Name() string { return b.name }

// Get reads the object at key, at most limit bytes of it: a longer one is ErrObjectTooLarge, one the bucket does not hold ErrNoObject.
func (b *Bucket) Get(ctx context.Context, key string, limit int64) ([]byte, error) {
	out, err := b.api.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)})
	if err != nil {
		return nil, missing(err)
	}
	defer out.Body.Close()
	if out.ContentLength != nil && *out.ContentLength > limit {
		return nil, fmt.Errorf("%s: %w", key, ErrObjectTooLarge)
	}
	data, err := io.ReadAll(io.LimitReader(out.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: %w", key, ErrObjectTooLarge)
	}
	return data, nil
}

// Put stores body, a small object, at key, replacing what was there.
func (b *Bucket) Put(ctx context.Context, key string, body []byte) error {
	_, err := b.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(b.name),
		Key:           aws.String(key),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String("application/octet-stream"),
	})
	return err
}

// Head is the object at key without its body, ErrNoObject when the bucket does not hold it.
func (b *Bucket) Head(ctx context.Context, key string) (Object, error) {
	out, err := b.api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)})
	if err != nil {
		return Object{}, missing(err)
	}
	return Object{Key: key, Size: aws.ToInt64(out.ContentLength), Modified: aws.ToTime(out.LastModified), ETag: aws.ToString(out.ETag)}, nil
}

// List is every object whose key starts with prefix, in the order of their keys, page after page.
func (b *Bucket) List(ctx context.Context, prefix string) ([]Object, error) {
	var out []Object
	pages := s3.NewListObjectsV2Paginator(b.api, &s3.ListObjectsV2Input{Bucket: aws.String(b.name), Prefix: aws.String(prefix)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			out = append(out, Object{Key: aws.ToString(o.Key), Size: aws.ToInt64(o.Size), Modified: aws.ToTime(o.LastModified), ETag: aws.ToString(o.ETag)})
		}
	}
	return out, nil
}

// Delete removes the object at key; one the bucket does not hold is no error, as S3 answers.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	_, err := b.api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)})
	if err != nil && errors.Is(missing(err), ErrNoObject) {
		return nil
	}
	return err
}

// PresignGet is a link that reads the object at key with a plain GET, good for ttl, at most MaxPresign. Making it does not talk to the network.
func (b *Bucket) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := b.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)}, s3.WithPresignExpires(presignTTL(ttl)))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// PresignPut is a link that stores a body at key with a plain PUT, without headers of its own, good for ttl, at most MaxPresign. Making it does not talk to the network.
func (b *Bucket) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := b.presign.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)}, s3.WithPresignExpires(presignTTL(ttl)))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func presignTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 || ttl > MaxPresign {
		return MaxPresign
	}
	return ttl
}

// missing is err as ErrNoObject when it says the object is not there: NoSuchKey to a GET, NotFound or a bare 404 to a HEAD, which has no body to name a code in.
func missing(err error) error {
	var noKey *types.NoSuchKey
	var notFound *types.NotFound
	if errors.As(err, &noKey) || errors.As(err, &notFound) {
		return fmt.Errorf("%w: %v", ErrNoObject, err)
	}
	var api smithy.APIError
	if errors.As(err, &api) && (api.ErrorCode() == "NoSuchKey" || api.ErrorCode() == "NotFound") {
		return fmt.Errorf("%w: %v", ErrNoObject, err)
	}
	var status interface{ HTTPStatusCode() int }
	if errors.As(err, &status) && status.HTTPStatusCode() == http.StatusNotFound && (api == nil || api.ErrorCode() != "NoSuchBucket") {
		return fmt.Errorf("%w: %v", ErrNoObject, err)
	}
	return err
}
