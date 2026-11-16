package cloud

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/smithy-go"
)

// Reason is what went wrong with an upload, for the app to say in the interface's language what to do about it.
type Reason int

const (
	// ReasonOther is an error none of the others names; Failure.Detail has the storage's own code and message when it answered.
	ReasonOther Reason = iota
	// ReasonStalled is ErrStalled, or a connection that timed out.
	ReasonStalled
	// ReasonOffline is a connection that could not be made or broke off.
	ReasonOffline
	// ReasonUnreachable is an endpoint whose host does not resolve: a wrong endpoint or region, or a bucket name a virtual-hosted address cannot hold.
	ReasonUnreachable
	// ReasonInsecure is an https endpoint whose certificate is not trusted, or an endpoint that does not speak TLS, as http behind an https address.
	ReasonInsecure
	// ReasonKeys is a storage that does not take the access key or the secret.
	ReasonKeys
	// ReasonDenied is keys that may not write to the bucket.
	ReasonDenied
	// ReasonNoBucket is a bucket that does not exist.
	ReasonNoBucket
	// ReasonRegion is a bucket that is in another region than the one the request was signed for.
	ReasonRegion
	// ReasonTooLarge is a file larger than one PUT stores (ErrTooLarge, EntityTooLarge).
	ReasonTooLarge
)

// Failure is what an upload's error says: why it failed, the host it could not reach for ReasonUnreachable and ReasonInsecure, and for ReasonOther the storage's code and message, "" when the storage did not answer.
type Failure struct {
	Reason Reason
	Host   string
	Detail string
}

// Diagnose reads an upload's error. The caller's own cancel is not a failure and is not looked for here.
func Diagnose(err error) Failure {
	if errors.Is(err, ErrStalled) {
		return Failure{Reason: ReasonStalled}
	}
	if errors.Is(err, ErrTooLarge) {
		return Failure{Reason: ReasonTooLarge}
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		if reason, ok := apiReasons[api.ErrorCode()]; ok {
			return Failure{Reason: reason}
		}
		var status interface{ HTTPStatusCode() int }
		if errors.As(err, &status) {
			switch status.HTTPStatusCode() {
			case http.StatusForbidden:
				return Failure{Reason: ReasonDenied}
			case http.StatusMovedPermanently:
				return Failure{Reason: ReasonRegion}
			}
		}
		detail := api.ErrorCode()
		if msg := api.ErrorMessage(); msg != "" && msg != detail {
			detail += ": " + msg
		}
		return Failure{Reason: ReasonOther, Detail: detail}
	}
	host := ""
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if parsed, parseErr := url.Parse(urlErr.URL); parseErr == nil {
			host = parsed.Host
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		if host == "" {
			host = dnsErr.Name
		}
		return Failure{Reason: ReasonUnreachable, Host: host}
	}
	var certErr *tls.CertificateVerificationError
	var authorityErr x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var invalidErr x509.CertificateInvalidError
	var recordErr tls.RecordHeaderError
	// net/http turns the tls.RecordHeaderError of an http server behind an https address into an error of its own, which it only writes as text.
	plainAnswer := strings.Contains(err.Error(), "server gave HTTP response to HTTPS client")
	if plainAnswer || errors.As(err, &certErr) || errors.As(err, &authorityErr) || errors.As(err, &hostnameErr) || errors.As(err, &invalidErr) || errors.As(err, &recordErr) {
		return Failure{Reason: ReasonInsecure, Host: host}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return Failure{Reason: ReasonStalled}
	}
	var opErr *net.OpError
	if urlErr != nil || errors.As(err, &opErr) {
		return Failure{Reason: ReasonOffline}
	}
	return Failure{Reason: ReasonOther}
}

// apiReasons are the error codes of S3 and the S3-compatible services that name a reason.
var apiReasons = map[string]Reason{
	"SignatureDoesNotMatch":        ReasonKeys,
	"InvalidAccessKeyId":           ReasonKeys,
	"InvalidSecurity":              ReasonKeys,
	"InvalidToken":                 ReasonKeys,
	"ExpiredToken":                 ReasonKeys,
	"AccessDenied":                 ReasonDenied,
	"AllAccessDisabled":            ReasonDenied,
	"AccountProblem":               ReasonDenied,
	"NoSuchBucket":                 ReasonNoBucket,
	"PermanentRedirect":            ReasonRegion,
	"AuthorizationHeaderMalformed": ReasonRegion,
	"IncorrectEndpoint":            ReasonRegion,
	"EntityTooLarge":               ReasonTooLarge,
}
