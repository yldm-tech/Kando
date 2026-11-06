package cloud

import (
	"errors"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// OSS turns a region id or a full URL into the S3-compatible endpoint and the signing region.
// cn-hangzhou becomes https://s3.oss-cn-hangzhou.aliyuncs.com and cn-hangzhou.
func OSS(raw string) (endpoint, region string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw == "" || strings.ContainsAny(raw, " \\") {
		return "", ""
	}
	if !strings.Contains(raw, "://") {
		if strings.Contains(raw, "/") {
			return "", ""
		}
		id := strings.TrimPrefix(raw, "oss-")
		internal := ""
		if strings.HasSuffix(id, "-internal") {
			id = strings.TrimSuffix(id, "-internal")
			internal = "-internal"
		}
		if id == "" || strings.Contains(id, ".") {
			return "", ""
		}
		return "https://s3.oss-" + id + internal + ".aliyuncs.com", id
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", ""
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", ""
	}
	host := parsed.Hostname()
	region = ossRegion(host)
	if region == "" {
		return "", ""
	}
	if strings.HasPrefix(host, "oss-") {
		host = "s3." + host
	}
	return parsed.Scheme + "://" + host, region
}

func ossRegion(host string) string {
	host = strings.TrimSuffix(host, ".aliyuncs.com")
	host = strings.TrimPrefix(host, "s3.")
	if !strings.HasPrefix(host, "oss-") {
		return ""
	}
	host = strings.TrimPrefix(host, "oss-")
	host = strings.TrimSuffix(host, "-internal")
	if host == "" || strings.Contains(host, ".") {
		return ""
	}
	return host
}

// NewOSS builds a client for the region's Aliyun OSS S3 endpoint. It does not talk to the network.
func NewOSS(region, accessKey, secret string) (Client, error) {
	endpoint, signing := OSS(region)
	accessKey = strings.TrimSpace(accessKey)
	secret = strings.TrimSpace(secret)
	if endpoint == "" || signing == "" || accessKey == "" || secret == "" {
		return nil, errors.New("oss is incomplete")
	}
	api := s3.New(s3.Options{
		Region:                     signing,
		BaseEndpoint:               aws.String(endpoint),
		Credentials:                credentials.NewStaticCredentialsProvider(accessKey, secret, ""),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &client{api: api}, nil
}
