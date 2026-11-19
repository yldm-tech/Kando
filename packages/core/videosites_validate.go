package core

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"
)

// MaxVideoPattern is the longest pattern of a VideoShape, in bytes.
const MaxVideoPattern = 256

// ErrVideoSite is the error ValidateVideoSite and ValidateVideoSites wrap: the site is not one Kando's browsers can read alike in Go and JavaScript, or its patterns could take a JavaScript page's thread for seconds.
var ErrVideoSite = errors.New("not a video site Go and JavaScript read alike")

// videoPatternChars are the characters a pattern may have: those of the regular expressions Go and JavaScript read alike (no flags, no named groups, no lookaround, no backreferences), which the shapes of VideoSites keep to.
var videoPatternChars = regexp.MustCompile(`^[\w\\^$/|()\[\]{}+*?.,@-]*$`)

// videoParam is the shape of a VideoShape's Param, a query parameter's name.
var videoParam = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// videoEscapes are the characters a backslash may come before in a pattern: the classes \w, \W, \d and \D, which Go and JavaScript read alike on the ASCII of a URL, and the punctuation of videoPatternChars, each itself. A digit, which JavaScript reads as a backreference, \p, which Go reads as a Unicode class and JavaScript as p, \b, \s and every other letter are left out.
const videoEscapes = `wWdD\^$/|()[]{}+*?.,@-`

// maxUnbounded is how many unbounded quantifiers (*, +, {n,}) a pattern may have. Two side by side that match the same characters already make a backtracking engine, as JavaScript's, try every way of sharing a link between them, the square of its length; three would try its cube, billions of steps for a link of MaxVideoURL.
const maxUnbounded = 2

// ValidateVideoSites checks sites as ValidateVideoSite checks each, and that no two of them have the same domain, or one a domain under another's, as VideoLink would only ever look at the first.
func ValidateVideoSites(sites []VideoSite) error {
	for i, s := range sites {
		if err := ValidateVideoSite(s); err != nil {
			return err
		}
		for _, o := range sites[:i] {
			if s.Domain == o.Domain || strings.HasSuffix(s.Domain, "."+o.Domain) || strings.HasSuffix(o.Domain, "."+s.Domain) {
				return fmt.Errorf("%w: the domains %s and %s overlap", ErrVideoSite, o.Domain, s.Domain)
			}
		}
	}
	return nil
}

// ValidateVideoSite checks that s is a site Kando's browsers read alike in Go (VideoLink) and in the JavaScript of their pages: its Domain a lowercase host name of at least two labels, each of Not a host under it, at least one shape, each with a Path that starts with ^/ and ends with $, and a Value of the same kind, anchored at both ends, exactly when it names a Param; and every pattern at most MaxVideoPattern bytes of the regular expressions both languages read alike (videoPattern). It does not look at Menu, Rows, Titles or Cut, which only the browser's pages and the extension read. The errors wrap ErrVideoSite.
func ValidateVideoSite(s VideoSite) error {
	if !videoHost(s.Domain) {
		return fmt.Errorf("%w: domain %q", ErrVideoSite, s.Domain)
	}
	for i, not := range s.Not {
		if !videoHost(not) || !strings.HasSuffix(not, "."+s.Domain) {
			return fmt.Errorf("%w: %q is not a host under %s", ErrVideoSite, not, s.Domain)
		}
		for _, o := range s.Not[:i] {
			if o == not {
				return fmt.Errorf("%w: %q twice", ErrVideoSite, not)
			}
		}
	}
	if len(s.Videos) == 0 {
		return fmt.Errorf("%w: %s has no shape of a video's link", ErrVideoSite, s.Domain)
	}
	for _, v := range s.Videos {
		if !strings.HasPrefix(v.Path, "^/") || !strings.HasSuffix(v.Path, "$") {
			return fmt.Errorf("%w: path %q does not start with ^/ and end with $", ErrVideoSite, v.Path)
		}
		if err := videoPattern(v.Path); err != nil {
			return err
		}
		if (v.Param == "") != (v.Value == "") {
			return fmt.Errorf("%w: a shape names a value exactly when it names a parameter", ErrVideoSite)
		}
		if v.Param == "" {
			continue
		}
		if !videoParam.MatchString(v.Param) {
			return fmt.Errorf("%w: parameter %q", ErrVideoSite, v.Param)
		}
		if !strings.HasPrefix(v.Value, "^") || !strings.HasSuffix(v.Value, "$") {
			return fmt.Errorf("%w: value %q does not start with ^ and end with $", ErrVideoSite, v.Value)
		}
		if err := videoPattern(v.Value); err != nil {
			return err
		}
	}
	return nil
}

// videoHost reports whether host is a lowercase host name of at least two labels of letters, digits and hyphens, none starting or ending with a hyphen, and not an address: its last label is not a number.
func videoHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for _, r := range l {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return strings.Trim(labels[len(labels)-1], "0123456789") != ""
}

// videoPattern checks that p is a regular expression Go and JavaScript read alike, and that a backtracking engine, as JavaScript's, reads in time on any link: at most MaxVideoPattern bytes of videoPatternChars; no flags, named groups or lookaround ((?); a backslash only before videoEscapes, so no backreference; braces only as a count after what they repeat ({n}, {n,}, {n,m}), and no class that starts with ], which the two read differently; then, as Go parses it, no repetition of anything that itself repeats, is optional or has alternatives, as (a+)+ or (a|aa)*, which takes such an engine exponential time on a link that almost matches, and at most maxUnbounded unbounded quantifiers.
func videoPattern(p string) error {
	if p == "" || len(p) > MaxVideoPattern {
		return fmt.Errorf("%w: a pattern is empty or over %d bytes", ErrVideoSite, MaxVideoPattern)
	}
	if !videoPatternChars.MatchString(p) || strings.Contains(p, "(?") {
		return fmt.Errorf("%w: %q has characters or groups JavaScript reads otherwise", ErrVideoSite, p)
	}
	if err := videoPatternTokens(p); err != nil {
		return err
	}
	re, err := syntax.Parse(p, syntax.Perl)
	if err != nil {
		return fmt.Errorf("%w: %q: %v", ErrVideoSite, p, err)
	}
	if nestedRepeat(re, false) {
		return fmt.Errorf("%w: %q repeats what repeats, is optional or has alternatives", ErrVideoSite, p)
	}
	if n := unbounded(re); n > maxUnbounded {
		return fmt.Errorf("%w: %q has %d unbounded quantifiers, more than %d", ErrVideoSite, p, n, maxUnbounded)
	}
	return nil
}

// countBraces is a count in braces as videoPatternTokens takes it: {n}, {n,} or {n,m}, of at most three digits each.
var countBraces = regexp.MustCompile(`^\{[0-9]{1,3}(,[0-9]{0,3})?\}`)

// videoPatternTokens reads p a character at a time for what videoPatternChars lets through but the two languages read apart: a backslash before anything but videoEscapes, a brace that is not a count after what it repeats, and a class that starts with ] (Go reads []a] as the class of ] and a, JavaScript as an empty class and a).
func videoPatternTokens(p string) error {
	inClass := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '\\':
			if i+1 >= len(p) || !strings.ContainsRune(videoEscapes, rune(p[i+1])) {
				return fmt.Errorf("%w: %q escapes what JavaScript reads otherwise", ErrVideoSite, p)
			}
			i++
		case inClass:
			if c == ']' {
				inClass = false
			}
		case c == '[':
			inClass = true
			if strings.HasPrefix(p[i+1:], "^") {
				i++
			}
			if strings.HasPrefix(p[i+1:], "]") {
				return fmt.Errorf("%w: %q has a class that starts with ]", ErrVideoSite, p)
			}
		case c == '{':
			count := countBraces.FindString(p[i:])
			if count == "" || i == 0 || strings.ContainsRune("(|^{", rune(p[i-1])) {
				return fmt.Errorf("%w: %q has a brace that is not a count", ErrVideoSite, p)
			}
			i += len(count) - 1
		case c == '}':
			return fmt.Errorf("%w: %q has a brace that is not a count", ErrVideoSite, p)
		}
	}
	if inClass {
		return fmt.Errorf("%w: %q has a class it does not close", ErrVideoSite, p)
	}
	return nil
}

// repeats reports whether re matches what it holds more than once: *, +, or a count of more than one.
func repeats(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpStar, syntax.OpPlus:
		return true
	case syntax.OpRepeat:
		return re.Max == -1 || re.Max > 1
	}
	return false
}

// nestedRepeat reports whether re has, under a repetition (inside), anything that repeats, is optional or has alternatives.
func nestedRepeat(re *syntax.Regexp, inside bool) bool {
	if inside {
		switch re.Op {
		case syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat, syntax.OpAlternate:
			return true
		}
	}
	inside = inside || repeats(re)
	for _, sub := range re.Sub {
		if nestedRepeat(sub, inside) {
			return true
		}
	}
	return false
}

// unbounded counts the quantifiers of re without an upper bound: *, + and {n,}.
func unbounded(re *syntax.Regexp) int {
	n := 0
	switch {
	case re.Op == syntax.OpStar, re.Op == syntax.OpPlus, re.Op == syntax.OpRepeat && re.Max == -1:
		n++
	}
	for _, sub := range re.Sub {
		n += unbounded(sub)
	}
	return n
}
