// Package plugins is the format of Kando's plugin packs and what installs them: the manifest kando-plugin.json (Parse), the checks every pack passes before it is installed and again each time it is read (Validate), staging a zip or a folder into the plugins folder and swapping it in (Stage, Swap), taking one out (Remove), listing what is installed (Scan), and which packs are turned on (State, state.json). It imports no interface package; the app, the command line and a server can all use it. docs/plugin-system.md is the design.
package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
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
	case KindDictionary, KindWordlist, KindGlossary, KindAIService, KindSite, KindScript, KindTask, KindStyle, KindExportTemplate:
		return true
	}
	return false
}

// TextOf is the text of message in each language of tags, as Kando writes a pack of its own: tags are canonical language tags, "en" among them.
func TextOf(tags []string, message func(tag string) string) Text {
	out := Text{}
	for _, tag := range tags {
		out[tag] = message(tag)
	}
	return out
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

// Manifest is a pack's kando-plugin.json. It has a field for each kind implemented so far; a known kind not implemented yet is read only as far as its header (Schema, ID, Kind), and Validate refuses it with ErrUnsupportedKind. A script plugin (schema 2) has the fields after Service: the oldest Kando it runs on, its signed package, what it may reach (host permissions and HTTP requests) and which of those it can do without, what it adds (Contributes), the secrets of other services it sends through Kando (Credentials), its settings and the events it hears; docs/plugin-system.md 4.2.
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
	Site        *Site       `json:"site,omitempty"`
	Task        *Task       `json:"task,omitempty"`
	Style       *Style      `json:"style,omitempty"`
	Template    *Template   `json:"template,omitempty"`

	Kando        string            `json:"kando,omitempty"`
	Script       *Script           `json:"script,omitempty"`
	Capabilities *pkg.Capabilities `json:"capabilities,omitempty"`
	Optional     []Perm            `json:"optional,omitempty"`
	Contributes  *Contributes      `json:"contributes,omitempty"`
	Credentials  []Credential      `json:"credentials,omitempty"`
	Settings     []Setting         `json:"settings,omitempty"`
	Events       []string          `json:"events,omitempty"`
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

// Site is a site pack: sites whose links to one video Kando's built-in browser knows by their shape, as it knows those of its own sites (kcore.VideoSites), for 下载视频 in its toolbar. It is data alone: no script, no cookies or sign-in, no arguments for yt-dlp, and no place for buttons in the pages. Each domain works only once the user allowed it (State.Allowed), and the downloads of its links borrow no sign-in (core.Job.PluginSite).
type Site struct {
	Sites []SiteDomain `json:"sites"`
}

// SiteDomain is one site of a site pack, as a kcore.VideoSite says it: its domain, which takes in the hosts under it, the hosts under it whose pages are no video's (Not), and the shapes of its videos' links (Videos).
type SiteDomain struct {
	Domain string      `json:"domain"`
	Not    []string    `json:"not,omitempty"`
	Videos []SiteVideo `json:"videos"`
}

// SiteVideo is the shape of a link to one video (kcore.VideoShape): its path, as the URL escapes it, matching Path, and, when Param is set, that query parameter's first value matching Value. Path and Value are regular expressions of the subset kcore.ValidateVideoSite takes.
type SiteVideo struct {
	Path  string `json:"path"`
	Param string `json:"param,omitempty"`
	Value string `json:"value,omitempty"`
}

// VideoSites are the sites of s as kcore.VideoSite.
func (s *Site) VideoSites() []kcore.VideoSite {
	out := make([]kcore.VideoSite, len(s.Sites))
	for i, d := range s.Sites {
		out[i] = kcore.VideoSite{Domain: d.Domain, Not: d.Not}
		for _, v := range d.Videos {
			out[i].Videos = append(out[i].Videos, kcore.VideoShape{Path: v.Path, Param: v.Param, Value: v.Value})
		}
	}
	return out
}

// SiteDomains are the domains of a site pack's sites, in its order; none for another kind.
func (m Manifest) SiteDomains() []string {
	if m.Site == nil {
		return nil
	}
	return m.Site.Domains()
}

// Domains are the domains of s, in its order.
func (s *Site) Domains() []string {
	out := make([]string, len(s.Sites))
	for i, d := range s.Sites {
		out[i] = d.Domain
	}
	return out
}

// The scopes a task pack runs on: the line playing (or selected), the word looked up, and the film as far as it was watched. The app sends only those lines and the ones before them, never one the learner has not reached.
const (
	ScopeLine = "line"
	ScopeWord = "word"
	ScopeFilm = "film-so-far"
)

// The ways a task's reply shows: as headed sections, one a field (one reply object); as a list, a row an item with its fields side by side; or as cards, an item a card with its first field as its title.
const (
	RenderSections = "sections"
	RenderList     = "list"
	RenderCards    = "cards"
)

// Task is a task pack: an instruction (Prompt) the user's AI service is given about the Scope, the fields its reply has (Reply) and how they show (Render). The host adds to the prompt the language to answer in, that nothing after the lines given may be guessed at, and the JSON its reply must be; it caps the reply at Tokens, MaxTokens or, when that is 0, what the reply's shape takes (TaskFieldTokens a value). A task pack spends the user's tokens, so it runs only once the user turned it on, within its daily budget (PluginState.Budget).
type Task struct {
	Scope     string    `json:"scope"`
	Prompt    string    `json:"prompt"`
	Reply     TaskReply `json:"reply"`
	Render    string    `json:"render"`
	MaxTokens int       `json:"maxTokens,omitempty"`
}

// TaskReply is the shape of a task's reply: its Fields, each a string, in the order they show; for a list or cards, at most Items items, DefaultTaskItems when 0.
type TaskReply struct {
	Fields []TaskField `json:"fields"`
	Items  int         `json:"items,omitempty"`
}

// TaskField is a field of a task's reply: the key the reply has it under, and its heading.
type TaskField struct {
	Key   string `json:"key"`
	Label Text   `json:"label"`
}

// DefaultTaskItems is the most items of a list or cards when a task names no Items; TaskFieldTokens the tokens a value of its reply is allowed when it names no MaxTokens, and TaskReplyTokens what the JSON around them takes; MaxTaskTokens the most a reply may take, what Validate holds a task's shape to.
const (
	DefaultTaskItems = 8
	TaskFieldTokens  = 100
	TaskReplyTokens  = 50
	MaxTaskTokens    = 4096
)

// Items is the most items the reply of t has: one for sections, else its Items or DefaultTaskItems.
func (t Task) Items() int {
	switch {
	case t.Render == RenderSections:
		return 1
	case t.Reply.Items > 0:
		return t.Reply.Items
	}
	return DefaultTaskItems
}

// Tokens is the most tokens the reply of t may take: its MaxTokens, or, when 0, TaskFieldTokens for each value its shape has (Items times its fields) and TaskReplyTokens, so a list of many items is not cut off at a fixed limit.
func (t Task) Tokens() int {
	if t.MaxTokens > 0 {
		return t.MaxTokens
	}
	return t.Items()*len(t.Reply.Fields)*TaskFieldTokens + TaskReplyTokens
}

// Style is a translate-style pack: a style instruction Settings › 翻译 offers as a preset of its prompt, with {{language}} for the language translated into. The host still appends the reply rule of a translation (media's subtitleReplyRule), so a style cannot change how a translation is read.
type Style struct {
	Prompt string `json:"prompt"`
}

// The formats an export template writes: Anki's text import, comma-separated or tab-separated values.
const (
	TemplateAnki = "anki"
	TemplateCSV  = "csv"
	TemplateTSV  = "tsv"
)

// TemplatePlaceholders are what a column of an export template may hold, each written {{name}}, and nothing else: the line's original, its translation, the learner's note, the film's name, where the line starts and its sound. The saved lines' export fills them in (saved.Placeholders, the same list).
var TemplatePlaceholders = []string{"original", "translation", "note", "film", "time", "sound"}

// Template is an export-template pack: another way to export the saved lines, offered beside Kando's own Anki notes. Format is what it writes; Columns are a note's fields or a row's columns, in order, each a Value made of text and TemplatePlaceholders, with a Header for the first row of a CSV or TSV file; NoteType is the Anki note type the notes go into, Basic when empty.
type Template struct {
	Format   string           `json:"format"`
	NoteType string           `json:"noteType,omitempty"`
	Columns  []TemplateColumn `json:"columns"`
}

// TemplateColumn is a column of an export template.
type TemplateColumn struct {
	Header string `json:"header,omitempty"`
	Value  string `json:"value"`
}

// Parse reads a manifest in two steps: its header (schema, id, kind) first, and then, for a kind this version implements, the whole of it strictly, a key it does not know at any level being an error. A schema other than those this version reads (dataSchema for the data packs, scriptSchema for script plugins) is ErrSchema, checked before anything else of the manifest, as a newer format may have kinds and keys this version does not know; an unknown kind is ErrUnknownKind; a known kind not implemented yet returns its header alone, for Validate to refuse with ErrUnsupportedKind, as the rest of its fields are only defined in its phase; a kind in the other schema's (a script plugin of schema 1, a data pack of schema 2) is ErrSchema. Parse checks nothing else: that is Validate's.
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
	if head.Schema != dataSchema && head.Schema != scriptSchema {
		return Manifest{}, fmt.Errorf("%w: schema %d", ErrSchema, head.Schema)
	}
	if !head.Kind.known() {
		return Manifest{}, fmt.Errorf("%w: %q", ErrUnknownKind, head.Kind)
	}
	if head.Schema != head.Kind.schema() {
		return Manifest{}, fmt.Errorf("%w: schema %d for a %s pack", ErrSchema, head.Schema, head.Kind)
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
