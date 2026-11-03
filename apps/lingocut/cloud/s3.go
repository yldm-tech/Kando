package cloud

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// NewS3 builds a client for the regional Amazon S3 endpoint. It does not talk to the network.
func NewS3(region, accessKey, secret string) (Client, error) {
	region = Region(region)
	accessKey = strings.TrimSpace(accessKey)
	secret = strings.TrimSpace(secret)
	if region == "" || accessKey == "" || secret == "" {
		return nil, errors.New("s3 is incomplete")
	}
	api := s3.New(s3.Options{
		Region:                     region,
		Credentials:                credentials.NewStaticCredentialsProvider(accessKey, secret, ""),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
	})
	return &client{api: api}, nil
}
