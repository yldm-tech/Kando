package cloud

import (
	"errors"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// COS turns a region id or a full URL into the COS endpoint and the signing region.
// ap-guangzhou becomes https://cos.ap-guangzhou.myqcloud.com and ap-guangzhou.
func COS(raw string) (endpoint, region string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw == "" || strings.ContainsAny(raw, " \\") {
		return "", ""
	}
	if !strings.Contains(raw, "://") && !strings.Contains(raw, ".") {
		if strings.Contains(raw, "/") {
			return "", ""
		}
		return "https://cos." + raw + ".myqcloud.com", raw
	}
	host := raw
	scheme := "https"
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" {
			return "", ""
		}
		if parsed.Scheme != "https" && parsed.Scheme != "http" {
			return "", ""
		}
		host = parsed.Hostname()
		scheme = parsed.Scheme
	}
	region = cosRegion(host)
	if region == "" {
		return "", ""
	}
	return scheme + "://cos." + region + ".myqcloud.com", region
}

func cosRegion(host string) string {
	host = strings.TrimSuffix(host, ".myqcloud.com")
	host = strings.TrimPrefix(host, "cos.")
	if host == "" || strings.Contains(host, ".") || strings.Contains(host, "/") {
		return ""
	}
	return host
}

// NewCOS builds a client for the region's Tencent COS endpoint. It does not talk to the network.
func NewCOS(region, accessKey, secret string) (Client, error) {
	endpoint, signing := COS(region)
	accessKey = strings.TrimSpace(accessKey)
	secret = strings.TrimSpace(secret)
	if endpoint == "" || signing == "" || accessKey == "" || secret == "" {
		return nil, errors.New("cos is incomplete")
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
