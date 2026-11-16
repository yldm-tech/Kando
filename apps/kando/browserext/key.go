package browserext

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// PublicKey is the public half of the extension's key, as manifest.json's "key" carries it: an RSA key's SubjectPublicKeyInfo in DER, in base64. It fixes the extension's ID in the browsers built on Chromium wherever its folder is loaded from (https://developer.chrome.com/docs/extensions/reference/manifest/key), so the host's manifest can allow it (ChromeOrigin). The private half, which a store upload is signed with, is not in the repository.
const PublicKey = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAvAc4uSNOZm4rxEMZghi6i0uYI8DbjAlBaSl9xVXtlo7sSJoKgum6ol3tispzERB7kKUJ2T5rCDHjo8Hb2sOXx1CFRzbMtTRs4zosr8h8cYNT7W7LPghKH5D39VESNos2RROiLhJ1m2kxUhOgnlZbSP8xSTjIoT6PKleeL5VylfrEXeFPxFTHYILn/ZDGhmSvGnDt2FdHZHE5jIQp6gU4amDbBsO9evf4xhl1/q/gG6eOvgfnQvYtLF4iVnq6N2EbzVElfFisJEMvUc5LqjUhuYgRGmn6TfYnJxWUcoBWfW1SjStOT+BbQg6iWln5o4vhtQ85pzsSZc9VPoc/Rvc9IQIDAQAB"

// ChromeID is the extension's ID in the browsers built on Chromium, as Chromium derives it from PublicKey: the first 16 bytes of the key's SHA-256, in hexadecimal written with the letters a to p for 0 to f (components/crx_file/id_util.cc).
func ChromeID() string {
	der, err := base64.StdEncoding.DecodeString(PublicKey)
	if err != nil {
		panic("browserext: PublicKey: " + err.Error())
	}
	sum := sha256.Sum256(der)
	id := []byte(hex.EncodeToString(sum[:16]))
	for i, c := range id {
		if c <= '9' {
			id[i] = 'a' + c - '0'
		} else {
			id[i] = 'a' + 10 + c - 'a'
		}
	}
	return string(id)
}

// ChromeOrigin is the extension's origin in the browsers built on Chromium, which they pass the host and its manifest allows.
func ChromeOrigin() string { return "chrome-extension://" + ChromeID() + "/" }
