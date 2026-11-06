package cloud

import (
	"errors"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// CompatEndpoint accepts a full http or https URL for an S3-compatible service.
func CompatEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return ""
	}
	return raw
}

// NewCompat builds a client for an S3-compatible endpoint. It does not talk to the network.
func NewCompat(endpoint, region, accessKey, secret string, pathStyle bool) (Client, error) {
	endpoint = CompatEndpoint(endpoint)
	region = Region(region)
	accessKey = strings.TrimSpace(accessKey)
	secret = strings.TrimSpace(secret)
	if endpoint == "" || region == "" || accessKey == "" || secret == "" {
		return nil, errors.New("s3 compatible endpoint is incomplete")
	}
	api := s3.New(s3.Options{
		Region:                     region,
		BaseEndpoint:               aws.String(endpoint),
		Credentials:                credentials.NewStaticCredentialsProvider(accessKey, secret, ""),
		UsePathStyle:               pathStyle,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &client{api: api}, nil
}
