package cloud

import (
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Session is a short-lived key to a bucket that a service hands out, as 看懂托管 (relayhost) does for a folder of its own bucket: an access key and its secret, with the session token every request and every presigned link carries (X-Amz-Security-Token), good until Expires.
type Session struct {
	AccessKey, Secret, Token string
	Expires                  time.Time
}

// OpenSessionBucket is the bucket name at the S3-compatible endpoint in region, addressed by path as R2 is, signed with the session key s. Its presigned links carry the session token and lapse with it whatever ttl they were signed for. It does not talk to the network.
func OpenSessionBucket(endpoint, region, name string, s Session) (*Bucket, error) {
	endpoint = CompatEndpoint(endpoint)
	region = Region(region)
	name = strings.TrimSpace(name)
	if region == "" {
		region = "auto"
	}
	if endpoint == "" || name == "" || s.AccessKey == "" || s.Secret == "" || s.Token == "" {
		return nil, errors.New("the session key is incomplete")
	}
	api := s3.New(s3.Options{
		Region:                     region,
		BaseEndpoint:               aws.String(endpoint),
		Credentials:                credentials.NewStaticCredentialsProvider(s.AccessKey, s.Secret, s.Token),
		UsePathStyle:               true,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &Bucket{api: api, presign: s3.NewPresignClient(api), name: name}, nil
}
