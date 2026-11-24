package phonesync

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"time"

	psync "kando/packages/sync"
)

// OfferState is where the pairing code the desktop offers stands.
type OfferState uint8

const (
	// OfferNone is no code: Phone Sync's settings are not open, or the server has not offered one yet.
	OfferNone OfferState = iota
	// OfferLive is a code a phone can pair with.
	OfferLive
	// OfferUsed is a code a phone has paired with.
	OfferUsed
	// OfferExpired is a code past its time (psync.CodeLifetime), or one tried wrongly psync.CodeTries times.
	OfferExpired
)

// Offer is the pairing code the desktop offers: the code, until when, and where it stands.
type Offer struct {
	Code    string
	Expires time.Time
	State   OfferState
	// Relay is the code's pairing mailbox in the relay's bucket (relay_pair.go), which the QR code carries so a phone away from the computer's network pairs too; nil while the relay is off.
	Relay *psync.PairRelay
}

// offer is the code on offer and how often it was tried wrongly. The server's mu guards it.
type offer struct {
	code    string
	expires time.Time
	tries   int
	used    bool
	// timer tells the app the code expired, so the settings say so.
	timer *time.Timer
	// mailbox is the code's pairing mailbox while the relay is on, nil while it is off; stopMailbox ends its serving, which deletes it.
	mailbox     *psync.PairRelay
	stopMailbox func()
}

// public is the offer as the settings show it, at now.
func (o *offer) public(now time.Time) Offer {
	return Offer{Code: o.code, Expires: o.expires, State: o.state(now), Relay: o.mailbox}
}

func (o *offer) state(now time.Time) OfferState {
	switch {
	case o == nil:
		return OfferNone
	case o.used:
		return OfferUsed
	case o.tries >= psync.CodeTries || !now.Before(o.expires):
		return OfferExpired
	}
	return OfferLive
}

// newCode is a random code of psync.CodeDigits digits.
func newCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err) // crypto/rand does not fail (crypto/rand.Read)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// try checks code against the one on offer at now, and counts a wrong one. It answers the protocol's error code, "" when code pairs.
func (o *offer) try(code string, now time.Time) string {
	switch o.state(now) {
	case OfferNone:
		return psync.CodeNoCode
	case OfferUsed:
		return psync.CodeCodeUsed
	case OfferExpired:
		return psync.CodeCodeExpired
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(o.code)) != 1 {
		o.tries++
		return psync.CodeWrongCode
	}
	o.used = true
	return ""
}

// pairLimit and pairWindow bound how many pairings the server takes, of any code from anyone: pairLimit in any pairWindow, so a phone on the network that tries code after code is held back besides the code's own psync.CodeTries.
const (
	pairLimit  = 10
	pairWindow = time.Minute
)

// limiter counts the pairings of the last pairWindow. The server's mu guards it.
type limiter struct{ at []time.Time }

// allow reports whether a pairing may be tried at now, and counts it when it may.
func (l *limiter) allow(now time.Time) bool {
	kept := l.at[:0]
	for _, t := range l.at {
		if now.Sub(t) < pairWindow {
			kept = append(kept, t)
		}
	}
	l.at = kept
	if len(l.at) >= pairLimit {
		return false
	}
	l.at = append(l.at, now)
	return true
}
