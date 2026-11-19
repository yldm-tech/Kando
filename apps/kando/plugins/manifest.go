// Package plugins is the format of Kando's plugin packs and what installs them: the manifest kando-plugin.json (Parse), the checks every pack passes before it is installed and again each time it is read (Validate), staging a zip or a folder into the plugins folder and swapping it in (Stage, Swap), taking one out (Remove), listing what is installed (Scan), and which packs are turned on (State, state.json). It imports no interface package; the app, the command line and a server can all use it. docs/plugin-system.md is the design.
package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/nsgo-dev/nsgo/i18n"
)

// ManifestName is the manifest's file name in every pack.
const ManifestName = "kando-plugin.json"

// manifestMax is the largest manifest read.
const manifestMax = 256 << 10

// Kind is what a pack adds to Kando.
type Kind string

// The kinds of packs. Every one is known to Parse; only those implemented (Kind.implemented) can be installed, and Validate refuses the others with ErrUnsupportedKind until their phase of docs/plugin-system.md.
const (
	KindDictionary     Kind = "dictionary"
	KindWordlist       Kind = "wordlist"
	KindGlossary       Kind = "glossary"
	KindAIService      Kind = "ai-service"
	KindStyle          Kind = "translate-style"
	KindTask           Kind = "task"
	KindExportTemplate Kind = "export-template"
	KindSite           Kind = "site"
	KindScript         Kind = "script"
)

// kinds are all the kinds, in the order Scan reads their folders.
var kinds = []Kind{KindDictionary, KindWordlist, KindGlossary, KindAIService, KindStyle, KindTask, KindExportTemplate, KindSite, KindScript}

func (k Kind) known() bool { return slices.Contains(kinds, k) }

// implemented reports whether this version installs packs of kind k.
func (k Kind) implemented() bool {
	switch k {
	case KindDictionary, KindWordlist, KindGlossary, KindAIService:
		return true
	}
	return false
}

// Text is text a pack supplies, by language tag (as i18n.Language returns it, such as "zh-Hans"); "en" is always there in a valid pack.
type Text map[string]string

// In is t in lang, else in lang's parents (i18n.Parent: zh-Hant for zh-TW), else in English. Call it only inside bindings, so the text follows a change of language.
func (t Text) In(lang string) string {
	for l := lang; l != ""; l = i18n.Parent(l) {
		if s, ok := t[l]; ok && s != "" {
			return s
		}
	}
	return t["en"]
}

// Manifest is a pack's kando-plugin.json. It has a field for each kind implemented so far; a known kind not implemented yet is read only as far as its header (Schema, ID, Kind), and Validate refuses it with ErrUnsupportedKind.
type Manifest struct {
	Schema      int         `json:"schema"`
	ID          string      `json:"id"`
	Kind        Kind        `json:"kind"`
	Version     string      `json:"version"`
	License     string      `json:"license"`
	Author      string      `json:"author,omitempty"`
	Homepage    string      `json:"homepage,omitempty"`
	Name        Text        `json:"name"`
	Description Text        `json:"description,omitempty"`
	Dictionary  *Dictionary `json:"dictionary,omitempty"`
	Wordlist    *Wordlist   `json:"wordlist,omitempty"`
	Glossary    *Glossary   `json:"glossary,omitempty"`
	Service     *Service    `json:"service,omitempty"`
}

// The dictionary formats a dictionary pack's files may be in.
const (
	FormatYomitan  = "yomitan-zip"
	FormatCCCEDICT = "cc-cedict"
	FormatTSV      = "tsv"
)

// Dictionary is a dictionary pack: its files in Format, the languages its headwords are in (From) and its definitions are in (To), and the credit its license asks for.
type Dictionary struct {
	Format      string   `json:"format"`
	Files       []string `json:"files"`
	From        []string `json:"from"`
	To          []string `json:"to"`
	Attribution Text     `json:"attribution,omitempty"`
}

// Wordlist is a word list pack: the words of Language in File, each with one of Tiers, such as a JLPT level.
type Wordlist struct {
	Language string `json:"language"`
	File     string `json:"file"`
	Tiers    []Tier `json:"tiers"`
}

// The tones a tier may be shown in, which the app maps to its theme's roles.
const (
	ToneAccent  = "accent"
	ToneWarn    = "warn"
	ToneSuccess = "success"
	ToneMuted   = "muted"
)

// Tier is one level of a word list.
type Tier struct {
	ID    string `json:"id"`
	Label Text   `json:"label"`
	Tone  string `json:"tone"`
}

// Glossary is a glossary pack: terms of From and how to translate them into To.
type Glossary struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Terms []Term `json:"terms"`
}

// Term is one entry of a glossary pack.
type Term struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// The groups an AI service pack may be in, as Settings › AI groups its services.
const (
	GroupCloud = "cloud"
	GroupLocal = "local"
)

// Service is an AI service pack: what it fills in for a service the user then gives a key. It never holds a key.
type Service struct {
	BaseURL string `json:"baseURL"`
	Model   string `json:"model,omitempty"`
	KeyURL  string `json:"keyURL,omitempty"`
	Group   string `json:"group"`
}

// Parse reads a manifest in two steps: its header (schema, id, kind) first, and then, for a kind this version implements, the whole of it strictly, a key it does not know at any level being an error. A schema other than the one this version reads (dataSchema) is ErrSchema, checked before anything else of the manifest, as a newer format may have kinds and keys this version does not know; an unknown kind is ErrUnknownKind; a known kind not implemented yet returns its header alone, for Validate to refuse with ErrUnsupportedKind, as the rest of its fields are only defined in its phase. Parse checks nothing else: that is Validate's.
func Parse(data []byte) (Manifest, error) {
	if len(data) > manifestMax {
		return Manifest{}, fmt.Errorf("%w: the manifest is over %d KiB", ErrTooLarge, manifestMax>>10)
	}
	var head struct {
		Schema int    `json:"schema"`
		ID     string `json:"id"`
		Kind   Kind   `json:"kind"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}
	if head.Schema != dataSchema {
		return Manifest{}, fmt.Errorf("%w: schema %d", ErrSchema, head.Schema)
	}
	if !head.Kind.known() {
		return Manifest{}, fmt.Errorf("%w: %q", ErrUnknownKind, head.Kind)
	}
	if !head.Kind.implemented() {
		return Manifest{Schema: head.Schema, ID: head.ID, Kind: head.Kind}, nil
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Manifest{}, fmt.Errorf("%w: more than one JSON value", ErrBadManifest)
	}
	return m, nil
}
