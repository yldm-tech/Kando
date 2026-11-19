package plugins

import (
	"fmt"
	"net/url"
	"unicode/utf8"
)

// The checks a listing of a pack is held to where it is not a manifest, as an entry of the community index (plugins/index): the same rules Validate holds a manifest's fields to, so an index cannot list a pack under an id, a name or an address its manifest could not have.

// Installable reports whether this version of Kando installs packs of kind k: a kind it knows and implements.
func (k Kind) Installable() bool { return k.known() && k.implemented() }

// ZipMax is the size of the largest pack file of kind k Stage takes.
func ZipMax(k Kind) int64 { return limitsOf(k).zip }

// CheckID checks id as a manifest's id of kind k: <publisher>.<name>, and for a data pack not under kando., which is Kando's own.
func CheckID(id string, k Kind) error { return checkID(id, k) }

// CheckVersion checks a version as a manifest's: letters, digits, dots, plus signs and hyphens, at most 32 bytes.
func CheckVersion(v string) error {
	if len(v) > versionMax || !versionPattern.MatchString(v) {
		return fmt.Errorf("%w: version %q", ErrBadField, v)
	}
	return nil
}

// CheckLicense cleans the license *s of what a reader cannot see and checks it as a manifest's: not empty, at most 64 characters.
func CheckLicense(s *string) error {
	*s = cleanText(*s, false)
	if *s == "" || utf8.RuneCountInString(*s) > licenseMax {
		return fmt.Errorf("%w: license %q", ErrBadField, *s)
	}
	return nil
}

// CheckName checks and cleans a pack's name as a manifest's: English is there, each language a canonical tag, each at most 40 characters once cleaned, and none reads as Kando's own or a built-in AI service's (ReservedNames).
func CheckName(t Text) error {
	if err := checkText("name", t, nameMax, true, false); err != nil {
		return err
	}
	for _, name := range t {
		if reserved(name) {
			return fmt.Errorf("%w: %q", ErrReservedName, name)
		}
	}
	return nil
}

// CheckDescription checks and cleans a pack's description as a manifest's: optional, but with English when there is one, each at most 500 characters once cleaned.
func CheckDescription(t Text) error { return checkText("description", t, textMax, false, true) }

// CheckLanguage checks that lang is a language tag in its canonical form, such as "zh-Hans".
func CheckLanguage(lang string) error { return checkLanguage(lang) }

// CheckHomepage checks a homepage as a manifest's: an https address with a host, nothing a reader cannot see, no user name or password; http only for this computer.
func CheckHomepage(raw string) error { return checkURL(raw) }

// CheckHTTPS checks raw as an address Kando downloads from: https alone, never http, not even for this computer, with a host, no user name or password, nothing a reader cannot see, at most 2048 bytes.
func CheckHTTPS(raw string) error {
	if err := checkURL(raw); err != nil {
		return err
	}
	if u, _ := url.Parse(raw); u.Scheme != "https" {
		return fmt.Errorf("%w: %q", ErrInsecureURL, raw)
	}
	return nil
}
