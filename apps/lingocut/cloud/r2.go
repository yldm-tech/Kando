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

// R2Config is the user's own Cloudflare R2 bucket.
type R2Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	Secret    string
}

type r2Client struct {
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
	return &r2Client{api: api}, nil
}

func (c *r2Client) Put(ctx context.Context, bucket, key, contentType string, body io.Reader, size int64) error {
	_, err := c.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	return err
}
