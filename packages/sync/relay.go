package sync

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// The storage relay (yldm-tech/Kando#219) carries protocol v2 between a phone and its desktop when they are not on the same network, through the user's own bucket (R2, S3, OSS, COS or another S3-compatible store, the one set in 账号与上传). The bucket is a mailbox: the phone leaves calls in it, the desktop answers them there and writes what the phone follows. The messages are v2's Call and Reply, unchanged, so a Desk answers them as it answers the local network's, and what one does over the relay it does the same on the local network.
//
// Credentials. A phone never holds a key of the bucket. The desktop, which does, gives each phone links presigned for the few objects that phone reads and writes (RelayGrant), good for RelayURLLifetime, the most S3's signatures allow, and gives it fresh ones well before they lapse. The first grant, with the key the phone's messages are sealed with (RelayJoin), is handed over the local network, by OpRelayJoin on PathCall, once the phone has paired, or with the pairing itself to a phone that paired through the relay (relay_pair.go), sealed there under the key of the code's pairing mailbox, which only the QR code the desktop shows carries; the relay refuses OpRelayJoin, so the key never travels through the bucket unsealed or sealed under a key the store could learn. Every later grant comes in the phone's state.json (RelayState.Grant), whose own link is among the grant's links: a phone that reads its state.json at least once while its links are good keeps them good for as long as the desktop runs now and then. A phone whose links lapsed joins again once it is back on the desktop's network, or pairs again through the relay.
//
// Layout. Everything of a desktop is under RelayPrefix, "kando-relay/{desktop}/", {desktop} named by its certificate's fingerprint, so several computers share a bucket. Under it, for each phone {device} (its device ID, PairResponse.DeviceID):
//
//	inbox/{device}/{slot}       a Call from the phone (RelayMessage), written once by its slot's PUT link
//	outbox/{device}/{slot}      the Reply to the call in that slot, read by the slot's GET link
//	outbox/{device}/state.json  what the phone follows (RelayState), read by the grant's State link
//
// Slots. A grant holds RelaySlots slots, each a random name with a link that PUTs the slot's call and one that GETs its reply. A phone puts one message in a slot and never another: it uses the slots of the newest grant it has and forgets the older grant's that it has not used, keeping the reply links of those it used until it has read their replies (RelayPhone.Waiting). The desktop makes a new grant once half its slots are used or it is RelayRegrant old, and every join gets a new one, so a phone that joins again never gets back slots it spent. A slot used twice all the same is answered once: the desktop deletes the second message unanswered, and the phone, which checks that the reply's ID is its call's, sends that call again in another slot.
//
// Delivery. At least once. The phone PUTs a message to a slot until the store takes it, then GETs the slot's reply until it is there (404 until then). The desktop lists the inboxes every RelayPoll while it runs, answers each message, writes its reply, and only then deletes the message; a message whose reply is already there (the desktop stopped between the two) is deleted without being answered again. A phone that hears no reply may send the same Call, with the same ID, in another slot: the Desk answers a call it has answered with the reply it gave (ReplyMemory), so the work is done once while the desktop runs; across a restart of the desktop, the operations themselves keep it so (a link already queued answers that job, Existing; cancel, retry and the learning state's merge are idempotent).
//
// Order. Each message carries the phone's sequence number (RelayMessage.Seq), counting up. The desktop answers a phone's messages that one listing finds in the order of their numbers; messages found by later listings come after. A phone that needs one call answered before another sends the second once it has the first's reply.
//
// What the relay does not carry. OpEvents does not wait over the relay: it answers at once, and the phone follows the changes by reading state.json, which holds the jobs' list and, for each library, the revision its learning state last changed at, and which the desktop writes whenever either changes (at most every RelayMinWrite) and at least every RelayHeartbeat while it runs. A phone takes a state.json RelayStale old for a desktop that is off, and says the desktop will pick up its messages when it is back.
//
// Encryption at rest. Every object of the relay is sealed (SealRelay) with AES-256-GCM under its phone's own key, RelayKeySize random bytes the desktop makes when the phone joins, with the object's name below the prefix (as "inbox/{device}/{slot}") as additional data, so a message cannot be read, changed, or moved to another slot or phone without failing to open. The store sees the names, the sizes and the times, nothing else. The films the desktop puts in the bucket for phones to stream or download (films/, relay_films.go) are not sealed, so the phone's system player can play them from their links; their folders' names are random, and only the links in a phone's state.json reach them.
//
// Revocation. Unpairing a phone deletes its inbox and outbox and forgets its key: its links then reach nothing, and anything it still PUTs to a slot whose link has not lapsed is deleted unread, as is any message of a device the desktop does not know. A link stays good until it lapses whatever the desktop does, so the user who wants a lost phone's film links cut at once changes the store's key in 账号与上传, which ends every link signed with the old one.

// RelayVersion is the version of RelayState.
const RelayVersion = 1

// The relay's limits.
const (
	// RelaySlots is how many slots a grant holds.
	RelaySlots = 16
	// RelayURLLifetime is how long a grant's links stay good: the most SigV4 signs for.
	RelayURLLifetime = 7 * 24 * time.Hour
	// RelayRegrant is the age at which the desktop gives a phone a new grant.
	RelayRegrant = 24 * time.Hour
	// RelayPoll is how often the desktop lists the inboxes while it runs.
	RelayPoll = 20 * time.Second
	// RelayMinWrite is the least time between two state.json writes for a phone, however fast the jobs move.
	RelayMinWrite = 10 * time.Second
	// RelayHeartbeat is how often the desktop writes a phone's state.json while it runs even when nothing changed, so the phone knows it is on.
	RelayHeartbeat = 5 * time.Minute
	// RelayStale is how old a state.json is when a phone takes the desktop to be off.
	RelayStale = 3 * RelayHeartbeat
	// RelayKeySize is the size of a phone's key, in bytes.
	RelayKeySize = 32
	// MaxRelayObject is the largest message (a sealed RelayMessage) the desktop reads: a phone does not send a larger one (ErrRelayCallTooLarge), and the desktop deletes a larger one unread.
	MaxRelayObject = 1 << 20
	// MaxRelayReply is the largest reply or state.json a phone reads, as large as the local network's client reads a reply: the desktop answers a call whose reply would be larger with CodeInvalid.
	MaxRelayReply = 16 << 20
)

// OpRelayJoin takes nothing and answers RelayJoin: the phone's key and its first grant. A desktop lists it in Hello.Ops while its relay is on, and answers it on the local network only.
const OpRelayJoin = "relay.join"

// RelayJoin is the answer to OpRelayJoin.
type RelayJoin struct {
	// Key is the key the phone's objects are sealed with (SealRelay), RelayKeySize bytes, base64 in JSON. A phone joining again gets the same key, until it is unpaired.
	Key []byte `json:"key"`
	// Grant is a new grant, never one handed out before, so none of its slots is spent.
	Grant RelayGrant `json:"grant"`
}

// RelayGrant is the links a phone reaches the relay by.
type RelayGrant struct {
	// Minted is when the desktop made the grant: a phone keeps the newest. Expires is when its links lapse.
	Minted  time.Time `json:"minted"`
	Expires time.Time `json:"expires"`
	// State is the GET link of the phone's state.json.
	State string      `json:"state"`
	Slots []RelaySlot `json:"slots"`
}

// RelaySlot is a slot: its name, the link that PUTs a message to it and the link that GETs the reply.
type RelaySlot struct {
	ID    string `json:"id"`
	Put   string `json:"put"`
	Reply string `json:"reply"`
}

// RelayMessage is what a phone puts in a slot: its sequence number, when it sent it, and the call.
type RelayMessage struct {
	Seq  int64     `json:"seq"`
	Sent time.Time `json:"sent"`
	Call Call      `json:"call"`
}

// RelayState is a phone's state.json: what the phone follows of the desktop while they are apart.
type RelayState struct {
	// V is RelayVersion. A phone reads a later version's fields it knows and skips the rest, as fields are only added under one version.
	V int `json:"v"`
	// Desktop is the desktop's name; Written when it wrote this, which a phone compares with RelayStale.
	Desktop string    `json:"desktop"`
	Written time.Time `json:"written"`
	// Hello is what the desktop answers OpHello with, for this phone.
	Hello Hello `json:"hello"`
	// Jobs is the jobs the phone follows, as OpJobs answers them.
	Jobs JobList `json:"jobs"`
	// Learning is, by library ID, the revision of Jobs' epoch at which the library's learning state last changed since the desktop started: a phone that sees one later than it saw sends OpStateGet for that library.
	Learning map[string]int64 `json:"learning"`
	// Grant is the phone's newest grant.
	Grant RelayGrant `json:"grant"`
	// Shelf is the films the desktop put in the bucket for its phones, with links to their files (relay_films.go); nil while it shares none, as a desktop of before them.
	Shelf *RelayShelf `json:"shelf,omitempty"`
}

// RelayPrefix is the folder of a desktop's relay in the bucket, named by its certificate's fingerprint (Fingerprint).
func RelayPrefix(fingerprint []byte) string {
	return "kando-relay/" + hex.EncodeToString(fingerprint[:min(len(fingerprint), 8)]) + "/"
}

// RelayInbox is the name, below RelayPrefix, of the message in device's slot.
func RelayInbox(device, slot string) string { return "inbox/" + device + "/" + slot }

// RelayReply is the name of the reply to the message in device's slot.
func RelayReply(device, slot string) string { return "outbox/" + device + "/" + slot }

// RelayStateName is the name of device's state.json.
func RelayStateName(device string) string { return "outbox/" + device + "/state.json" }

// relayMagic starts every sealed object, naming the format.
const relayMagic = "KRL1"

// ErrRelaySealed is OpenRelay's error for an object that is not one sealed with the key under the name: another key's, another object's, or changed.
var ErrRelaySealed = errors.New("the relay's object does not open with this key")

// SealRelay seals plain, the object's JSON, under key for the object named name (below RelayPrefix): "KRL1", a random 12-byte nonce, then plain encrypted with AES-256-GCM with name as additional data.
func SealRelay(key []byte, name string, plain []byte) ([]byte, error) {
	aead, err := relayAEAD(key)
	if err != nil {
		return nil, err
	}
	// Seal appends and grows out itself: no capacity hint sized from len(plain), whose sum could overflow.
	out := make([]byte, len(relayMagic)+aead.NonceSize())
	copy(out, relayMagic)
	nonce := out[len(relayMagic):]
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(out, nonce, plain, []byte(name)), nil
}

// OpenRelay opens what SealRelay sealed under key for name.
func OpenRelay(key []byte, name string, sealed []byte) ([]byte, error) {
	aead, err := relayAEAD(key)
	if err != nil {
		return nil, err
	}
	head := len(relayMagic) + aead.NonceSize()
	if len(sealed) < head+aead.Overhead() || string(sealed[:len(relayMagic)]) != relayMagic {
		return nil, ErrRelaySealed
	}
	plain, err := aead.Open(nil, sealed[len(relayMagic):head], sealed[head:], []byte(name))
	if err != nil {
		return nil, ErrRelaySealed
	}
	return plain, nil
}

func relayAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != RelayKeySize {
		return nil, errors.New("a relay key is 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// NewRelayKey is a new phone's key.
func NewRelayKey() []byte {
	key := make([]byte, RelayKeySize)
	_, _ = rand.Read(key)
	return key
}

// newSlotID is a slot's name: 16 random bytes in hex, which never reads as state.json.
func newSlotID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// relaySlotID reports whether name is a slot's name as newSlotID makes them.
func relaySlotID(name string) bool {
	if len(name) != 32 {
		return false
	}
	_, err := hex.DecodeString(name)
	return err == nil && strings.ToLower(name) == name
}

// RelayBucket is the bucket as the relay uses it, keys being full object keys (with RelayPrefix). The desktop's is its store's (apps/kando/cloud); a phone has none.
type RelayBucket interface {
	// Get reads the object at key, at most MaxRelayObject bytes, ErrRelayTooLarge for a longer one; one the bucket does not hold is ErrNoObject.
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, body []byte) error
	// Exists reports whether the bucket holds an object at key.
	Exists(ctx context.Context, key string) (bool, error)
	// List is the objects whose keys start with prefix.
	List(ctx context.Context, prefix string) ([]RelayObject, error)
	// Delete removes the object at key; one not there is no error.
	Delete(ctx context.Context, key string) error
	// PresignGet and PresignPut are links that GET or PUT key with no credentials of their own, good for ttl.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// RelayObject is an object as RelayBucket.List finds it.
type RelayObject struct {
	Key      string
	Modified time.Time
}

// ErrNoObject is the error for an object the bucket does not hold.
var ErrNoObject = errors.New("no such object")

// ErrRelayTooLarge is RelayBucket.Get's error for an object larger than MaxRelayObject.
var ErrRelayTooLarge = errors.New("the relay's object is too large")
