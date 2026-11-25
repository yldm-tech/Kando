package remotefs

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/fs"
	"net"
	"strings"
	"syscall"

	"github.com/cloudsoda/go-smb2"
	"github.com/studio-b12/gowebdav"
	"golang.org/x/crypto/ssh"
)

// Failure is why OpenRemote failed, as far as the error tells, for a message the person can act on. The errors themselves are English and may hold the server's address, so they go to the log, not on screen.
type Failure int

const (
	// FailOther is an error none of the others names.
	FailOther Failure = iota
	// FailHostKey is a HostKeyError: the SFTP server's key is not trusted yet, or is not the trusted one.
	FailHostKey
	// FailSignIn is a user name, password or key the server refused.
	FailSignIn
	// FailDenied is a user the server let in but not into the folder.
	FailDenied
	// FailNoShare is an SMB server without the share named.
	FailNoShare
	// FailNoRoot is a root folder that is not on the server.
	FailNoRoot
	// FailBadRoot is a root folder that climbs above where it starts.
	FailBadRoot
	// FailUnreachable is a server that could not be found or connected to.
	FailUnreachable
	// FailTimeout is a server that did not answer in time.
	FailTimeout
	// FailCertificate is an HTTPS server whose certificate is not trusted.
	FailCertificate
	// FailPrivateKey is an SFTP private key that cannot be read (ErrPrivateKey).
	FailPrivateKey
	// FailSigning is an SMB server that did not sign its messages, or signed in a guest, which cannot sign, while Remote.RequireSigning asked for signing.
	FailSigning
)

// NTSTATUS codes an SMB server answers a sign-in, a mount or an open with (MS-ERREF 2.3.1).
const (
	smbLogonFailure     = 0xC000006D
	smbAccountRestrict  = 0xC000006E
	smbPasswordExpired  = 0xC0000071
	smbAccountDisabled  = 0xC0000072
	smbAccessDenied     = 0xC0000022
	smbBadNetworkName   = 0xC00000CC
	smbPasswordMustSet  = 0xC0000224
	smbAccountLockedOut = 0xC0000234
)

// Diagnose is why err, an error of OpenRemote or of a provider it opened, happened.
func Diagnose(err error) Failure {
	if err == nil {
		return FailOther
	}
	var (
		hk   *HostKeyError
		smb  *smb2.ResponseError
		bad  *smb2.InvalidResponseError
		dav  gowebdav.StatusError
		dns  *net.DNSError
		op   *net.OpError
		netE net.Error
	)
	switch {
	case errors.As(err, &hk):
		return FailHostKey
	case errors.Is(err, ErrPrivateKey):
		return FailPrivateKey
	case errors.Is(err, ErrInvalidPath):
		return FailBadRoot
	case errors.As(err, &bad) && strings.Contains(bad.Message, "sign"):
		// go-smb2 says so only in words: "guest account doesn't support signing", "packet failed signature verification".
		return FailSigning
	case errors.As(err, &smb):
		switch smb.Code {
		case smbLogonFailure, smbAccountRestrict, smbPasswordExpired, smbAccountDisabled, smbPasswordMustSet, smbAccountLockedOut:
			return FailSignIn
		case smbAccessDenied:
			return FailDenied
		case smbBadNetworkName:
			return FailNoShare
		}
	case errors.As(err, &dav):
		switch dav.Status {
		case 401:
			return FailSignIn
		case 403:
			return FailDenied
		case 404:
			return FailNoRoot
		}
	}
	switch {
	case errors.Is(err, ErrNotExist):
		return FailNoRoot
	case errors.Is(err, fs.ErrPermission):
		return FailDenied
	case sshAuthFailed(err):
		return FailSignIn
	case certificateFailed(err):
		return FailCertificate
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netE) && netE.Timeout():
		return FailTimeout
	case errors.As(err, &dns), errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return FailUnreachable
	case errors.As(err, &op) && op.Op == "dial":
		return FailUnreachable
	}
	return FailOther
}

// sshAuthFailed is whether err is x/crypto/ssh's refusal of every way of signing in, which it gives only as text.
func sshAuthFailed(err error) bool {
	return strings.Contains(err.Error(), "ssh: unable to authenticate")
}

// certificateFailed is whether err is a TLS certificate that did not verify.
func certificateFailed(err error) bool {
	var (
		verify    *tls.CertificateVerificationError
		authority x509.UnknownAuthorityError
		hostname  x509.HostnameError
		invalid   x509.CertificateInvalidError
	)
	return errors.As(err, &verify) || errors.As(err, &authority) || errors.As(err, &hostname) || errors.As(err, &invalid)
}

// Fingerprint is the SHA-256 fingerprint ("SHA256:...") of a host key written as a line of authorized_keys, as Remote.HostKey and HostKeyError.Key hold it, the form HostKeyError.Fingerprint has; "" when key is not one.
func Fingerprint(key string) string {
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(key)))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(pub)
}
