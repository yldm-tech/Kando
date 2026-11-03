package cloud

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	// KindR2 uploads to Cloudflare R2.
	KindR2 = "r2"
	// KindS3 uploads to Amazon S3.
	KindS3 = "s3"
	// KindOSS uploads to Alibaba Cloud OSS.
	KindOSS = "oss"
	// KindCOS uploads to Tencent Cloud COS.
	KindCOS = "cos"
	// KindCompat uploads to another S3-compatible endpoint.
	KindCompat = "compat"
)

// Store is one bucket the user owns.
type Store struct {
	Kind      string
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	Secret    string
	PathStyle bool
}

type client struct {
	api *s3.Client
}

// NewR2 builds an S3 client aimed at the R2 endpoint. It does not talk to the network.
func NewR2(endpoint, accessKey, secret string) (Client, error) {
	endpoint = Endpoint(endpoint)
	accessKey = strings.TrimSpace(accessKey)
	secret = strings.TrimSpace(secret)
	if endpoint == "" || accessKey == "" || secret == "" {
		return nil, errors.New("r2 is incomplete")
	}
	api := s3.New(s3.Options{
		Region:                     "auto",
		BaseEndpoint:               aws.String(endpoint),
		Credentials:                credentials.NewStaticCredentialsProvider(accessKey, secret, ""),
		UsePathStyle:               true,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
	})
	return &client{api: api}, nil
}

// Open builds the client for the selected kind. It does not talk to the network.
func Open(cfg Store) (Client, error) {
	switch cfg.Kind {
	case KindS3:
		return NewS3(cfg.Region, cfg.AccessKey, cfg.Secret)
	case KindOSS:
		return NewOSS(cfg.Region, cfg.AccessKey, cfg.Secret)
	case KindCOS:
		return NewCOS(cfg.Region, cfg.AccessKey, cfg.Secret)
	case KindCompat:
		return NewCompat(cfg.Endpoint, cfg.Region, cfg.AccessKey, cfg.Secret, cfg.PathStyle)
	default:
		return NewR2(cfg.Endpoint, cfg.AccessKey, cfg.Secret)
	}
}

func (c *client) Put(ctx context.Context, bucket, key, contentType string, body io.Reader, size int64) error {
	_, err := c.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	return err
}
