package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"kando/apps/kando/cloud"
	"kando/packages/remotefs"
)

// ObjectStore is a Provider over a bucket of an object store, R2, S3, OSS, COS or another that speaks S3, under a key prefix. A folder is a key prefix: it exists while it holds an object, so MkdirAll makes nothing and an emptied folder is gone. Writes are whole objects, so a reader sees the old file or the new one. Moving a file copies it in the bucket and deletes the original, which the store does only for a file of up to 5 GiB: a larger one is cloud.ErrCopyTooLarge and stays where it was.
type ObjectStore struct {
	bucket *cloud.Bucket
	root   string
}

// NewObjectStore is an ObjectStore over bucket, rooted at the key prefix root ("" for the whole bucket, else a slash-separated folder such as "kando/films").
func NewObjectStore(bucket *cloud.Bucket, root string) (*ObjectStore, error) {
	root, err := remotefs.CleanPath(root)
	if err != nil {
		return nil, err
	}
	return &ObjectStore{bucket: bucket, root: root}, nil
}

// OpenObjectStore is an ObjectStore over the bucket cfg names, rooted at root. It does not talk to the network.
func OpenObjectStore(cfg cloud.Store, root string) (*ObjectStore, error) {
	bucket, err := cloud.OpenBucket(cfg)
	if err != nil {
		return nil, err
	}
	return NewObjectStore(bucket, root)
}

// key is the object key of the clean path p.
func (o *ObjectStore) key(p string) string {
	if o.root == "" {
		return p
	}
	if p == "" {
		return o.root
	}
	return o.root + "/" + p
}

// prefix is the key prefix of what lies in the folder at the clean path dir.
func (o *ObjectStore) prefix(dir string) string {
	if k := o.key(dir); k != "" {
		return k + "/"
	}
	return ""
}

func objectErr(err error) error {
	if errors.Is(err, cloud.ErrNoObject) {
		return fmt.Errorf("%w: %v", remotefs.ErrNotExist, err)
	}
	return err
}

// Stat is the object at p or, when there is none, the folder its key prefix makes.
func (o *ObjectStore) Stat(ctx context.Context, p string) (remotefs.Info, error) {
	p, err := remotefs.CleanPath(p)
	if err != nil {
		return remotefs.Info{}, err
	}
	if p == "" {
		return remotefs.Info{Dir: true}, nil
	}
	obj, err := o.bucket.Head(ctx, o.key(p))
	if err == nil {
		return remotefs.Info{Name: path.Base(p), Size: obj.Size, Modified: obj.Modified, ETag: obj.ETag}, nil
	}
	if !errors.Is(err, cloud.ErrNoObject) {
		return remotefs.Info{}, err
	}
	dir, err := o.bucket.HasPrefix(ctx, o.prefix(p))
	if err != nil {
		return remotefs.Info{}, err
	}
	if !dir {
		return remotefs.Info{}, fmt.Errorf("%s: %w", p, remotefs.ErrNotExist)
	}
	return remotefs.Info{Name: path.Base(p), Dir: true}, nil
}

// List is the objects and folders directly under dir's prefix. The root is never missing, only empty.
func (o *ObjectStore) List(ctx context.Context, dir string) ([]remotefs.Info, error) {
	dir, err := remotefs.CleanPath(dir)
	if err != nil {
		return nil, err
	}
	prefix := o.prefix(dir)
	objects, dirs, err := o.bucket.ListDir(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]remotefs.Info, 0, len(objects)+len(dirs))
	for _, obj := range objects {
		name := strings.TrimPrefix(obj.Key, prefix)
		if name == "" {
			// A folder marker some tools write, an empty object whose key ends in a slash.
			continue
		}
		out = append(out, remotefs.Info{Name: name, Size: obj.Size, Modified: obj.Modified, ETag: obj.ETag})
	}
	for _, d := range dirs {
		out = append(out, remotefs.Info{Name: strings.TrimSuffix(strings.TrimPrefix(d, prefix), "/"), Dir: true})
	}
	if len(out) == 0 && dir != "" && len(objects) == 0 {
		return nil, fmt.Errorf("%s: %w", dir, remotefs.ErrNotExist)
	}
	return out, nil
}

// Open reads the object at p with a ranged GET.
func (o *ObjectStore) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	p, err := remotefs.CleanFile(p)
	if err != nil {
		return nil, err
	}
	body, err := o.bucket.GetRange(ctx, o.key(p), offset, length)
	return body, objectErr(err)
}

// Write stores r as the object at p, in one PUT or, for a large one, in parts.
func (o *ObjectStore) Write(ctx context.Context, p string, r io.Reader, size int64) error {
	p, err := remotefs.CleanFile(p)
	if err != nil {
		return err
	}
	return o.bucket.PutStream(ctx, o.key(p), cloud.ContentType(p), r, size)
}

// Remove deletes the object at p; with none there, a folder of that name that holds something is ErrNotEmpty.
func (o *ObjectStore) Remove(ctx context.Context, p string) error {
	p, err := remotefs.CleanFile(p)
	if err != nil {
		return err
	}
	_, err = o.bucket.Head(ctx, o.key(p))
	switch {
	case err == nil:
		return o.bucket.Delete(ctx, o.key(p))
	case !errors.Is(err, cloud.ErrNoObject):
		return err
	}
	full, err := o.bucket.HasPrefix(ctx, o.prefix(p))
	if err != nil {
		return err
	}
	if full {
		return fmt.Errorf("%s: %w", p, remotefs.ErrNotEmpty)
	}
	return nil
}

// Move copies the object at from to to and deletes it; a folder moves object by object, after checking that none is too large to copy, and is not atomic: a failure part way leaves some objects moved.
func (o *ObjectStore) Move(ctx context.Context, from, to string) error {
	from, err := remotefs.CleanFile(from)
	if err != nil {
		return err
	}
	to, err = remotefs.CleanFile(to)
	if err != nil {
		return err
	}
	if from == to {
		return nil
	}
	if _, err := o.bucket.Head(ctx, o.key(from)); err == nil {
		return o.moveObject(ctx, o.key(from), o.key(to))
	} else if !errors.Is(err, cloud.ErrNoObject) {
		return err
	}
	if strings.HasPrefix(to+"/", from+"/") {
		return fmt.Errorf("%s into itself: %w", from, remotefs.ErrInvalidPath)
	}
	src, dst := o.prefix(from), o.prefix(to)
	objects, err := o.bucket.List(ctx, src)
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		return fmt.Errorf("%s: %w", from, remotefs.ErrNotExist)
	}
	for _, obj := range objects {
		if obj.Size > cloud.MaxCopy {
			return fmt.Errorf("%s: %w", obj.Key, cloud.ErrCopyTooLarge)
		}
	}
	for _, obj := range objects {
		if err := o.moveObject(ctx, obj.Key, dst+strings.TrimPrefix(obj.Key, src)); err != nil {
			return err
		}
	}
	return nil
}

func (o *ObjectStore) moveObject(ctx context.Context, from, to string) error {
	if err := o.bucket.Copy(ctx, from, to); err != nil {
		return objectErr(err)
	}
	return o.bucket.Delete(ctx, from)
}

// MkdirAll makes nothing: a folder of an object store is there once something is stored in it.
func (o *ObjectStore) MkdirAll(_ context.Context, dir string) error {
	_, err := remotefs.CleanPath(dir)
	return err
}

// Link presigns a GET of the object at p, good for ttl, at most cloud.MaxPresign. It does not talk to the network, nor check that the object is there.
func (o *ObjectStore) Link(ctx context.Context, p string, ttl time.Duration) (string, error) {
	p, err := remotefs.CleanFile(p)
	if err != nil {
		return "", err
	}
	return o.bucket.PresignGet(ctx, o.key(p), ttl)
}
