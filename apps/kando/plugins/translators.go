package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	kcore "kando/packages/core"
)

// The functions a script plugin's translators call (docs/plugin-system.md E14): translate(lines, from, to, context), which every plugin with a translator exports, and candidates(line, from, to, context), which one may, to offer other translations of a line the viewer says is wrong.
const (
	TranslateExport  = "translate"
	CandidatesExport = "candidates"
)

// TranslateBatch is the most lines Kando gives a translator's translate at once.
const TranslateBatch = 50

// translateLineMax is the most characters of a line a translator may answer with, so a plugin's answer cannot make a line no subtitle shows.
const translateLineMax = 2000

// AnyLanguage is what a translator's languages say for one that translates from and into any language.
const AnyLanguage = "any"

// Translator is a source of machine translation a script plugin adds (docs/plugin-system.md E14), which Settings › 翻译 offers in place of Kando's AI service: the films' lines go to the plugin's translate export, at most TranslateBatch at a time, and the one the viewer says is wrong to its candidates export when it has one. Languages are the languages it translates from and into; Local says it runs on this computer, so offline mode keeps it, which only a plugin that reaches no network and no credential may say.
type Translator struct {
	ID        string              `json:"id"`
	Title     Text                `json:"title"`
	Languages TranslatorLanguages `json:"languages"`
	Local     bool                `json:"local,omitempty"`
}

// TranslatorLanguages are the languages a translator translates from (From) and into (To), as language tags, or every language (Any), which the manifest writes as "any".
type TranslatorLanguages struct {
	Any      bool
	From, To []string
}

// translatorLanguages is TranslatorLanguages as the manifest writes it when it names them.
type translatorLanguages struct {
	From []string `json:"from"`
	To   []string `json:"to"`
}

// MarshalJSON writes l as the manifest does: "any", or {from, to}.
func (l TranslatorLanguages) MarshalJSON() ([]byte, error) {
	if l.Any {
		return json.Marshal(AnyLanguage)
	}
	return json.Marshal(translatorLanguages{From: l.From, To: l.To})
}

// UnmarshalJSON reads l as the manifest writes it: "any", or {from, to} and no other key.
func (l *TranslatorLanguages) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s != AnyLanguage {
			return fmt.Errorf("%w: translator languages %q", ErrBadField, s)
		}
		*l = TranslatorLanguages{Any: true}
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var named translatorLanguages
	if err := dec.Decode(&named); err != nil {
		return fmt.Errorf("%w: translator languages: %v", ErrBadField, err)
	}
	*l = TranslatorLanguages{From: named.From, To: named.To}
	return nil
}

// checkTranslators checks the translators of the script plugin m: at most contribMax, each with an id unique among them, a title, the languages it translates from and into or "any", and, for one that says it runs on this computer, a plugin that reaches no network and no credential.
func checkTranslators(m *Manifest) error {
	if m.Contributes == nil {
		return nil
	}
	ts := m.Contributes.Translators
	if len(ts) > contribMax {
		return fmt.Errorf("%w: at most %d translators", ErrBadField, contribMax)
	}
	for i := range ts {
		t := &ts[i]
		if !commandPattern.MatchString(t.ID) || slices.ContainsFunc(ts[:i], func(o Translator) bool { return o.ID == t.ID }) {
			return fmt.Errorf("%w: translator id %q", ErrBadField, t.ID)
		}
		if err := checkText("translator title", t.Title, nameMax*2, true, false); err != nil {
			return err
		}
		if !t.Languages.Any {
			if err := checkLanguages("translator from", t.Languages.From); err != nil {
				return err
			}
			if err := checkLanguages("translator to", t.Languages.To); err != nil {
				return err
			}
		}
		if t.Local && (m.Capabilities != nil && m.Capabilities.Network != nil || len(m.Credentials) > 0) {
			return fmt.Errorf("%w: translator %s says it runs on this computer and reaches the network", ErrBadField, t.ID)
		}
	}
	return nil
}

// Takes reports whether t translates from, a language tag or whisper's code, "" when it is not known, into to, a language tag, "" for a language Kando has no tag for: every pair for a translator of any language; else from among its From, or not known, which it is left to tell, and to among its To. A language is among them when it is one of them or one is the other's language without its script or region, so zh-Hans is among zh and pt among pt-BR.
func (t Translator) Takes(from, to string) bool {
	if t.Languages.Any {
		return true
	}
	among := func(tag string, list []string) bool {
		return slices.ContainsFunc(list, func(l string) bool {
			l, tag := strings.ToLower(l), strings.ToLower(tag)
			return l == tag || kcore.BaseLanguage(l) == tag || kcore.BaseLanguage(tag) == l
		})
	}
	return (from == "" || among(from, t.Languages.From)) && to != "" && among(to, t.Languages.To)
}

// spokenTags are whisper's codes (media.SpeechLanguageOf) that are not the language tag translators know their language by: Norwegian is no to whisper and nb (Bokmål) to them, Javanese jw and jv.
var spokenTags = map[string]string{"no": "nb", "jw": "jv"}

// SpokenTag is the language tag of whisper's code, what a translator's from names: code itself, in lower case, but for the codes whisper writes otherwise (spokenTags); "" for "".
func SpokenTag(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if tag, ok := spokenTags[code]; ok {
		return tag
	}
	return code
}

// Candidater reports whether the script plugin m offers other translations of a line, which it does by exporting candidates (CandidatesExport), with its translators: exports are the functions its code exports.
func (m Manifest) Candidater(exports []string) bool {
	return m.Contributes != nil && len(m.Contributes.Translators) > 0 && slices.Contains(exports, CandidatesExport)
}

// TranslatedLines are the lines a translator's translate answered as v for n lines, and whether it answered them: an array of exactly n strings, in their order, each on one line (its line breaks spaces) and cleaned of invisible characters (oneLine) and at most translateLineMax characters.
func TranslatedLines(v any, n int) ([]string, bool) {
	items, ok := v.([]any)
	if !ok || len(items) != n {
		return nil, false
	}
	out := make([]string, n)
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		s = oneLine(s)
		if utf8.RuneCountInString(s) > translateLineMax {
			return nil, false
		}
		out[i] = s
	}
	return out, true
}

// CandidateLines are the translations a translator's candidates answered as v, and whether it answered some: an array of strings, of which the first 8 are kept, cleaned as TranslatedLines cleans a line; null is none.
func CandidateLines(v any) ([]string, bool) {
	if v == nil {
		return nil, true
	}
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	var out []string
	for _, item := range items[:min(len(items), 8)] {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		if s = oneLine(s); s != "" && utf8.RuneCountInString(s) <= translateLineMax {
			out = append(out, s)
		}
	}
	return out, true
}

// oneLine is s on one line: its line breaks spaces, then cleaned as a text without lines (cleanText).
func oneLine(s string) string {
	return cleanText(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s), false)
}

// The codes a translator's failure is known by (TranslateError), which the failure card words for the user (jobfailure): the service's quota is used up, it refused the key, the plugin has no key yet, it does not translate into the language asked for, it asks to wait (rate limit), it is not there, the plugin is not running, and an answer that is not the lines asked for. A plugin rejects with the first five as an error's code; Kando gives the last two.
const (
	TranslateQuota        = "quota"
	TranslateAuth         = "auth"
	TranslateNoCredential = "no-credential"
	TranslateLanguage     = "language"
	TranslateRateLimit    = "rate-limit"
	TranslateUnavailable  = "unavailable"
	TranslateNotRunning   = "not-running"
	TranslateBadReply     = "bad-reply"
)

// translateCodes are the codes TranslateError keeps; another a plugin rejects with is a failure of no known kind.
var translateCodes = []string{TranslateQuota, TranslateAuth, TranslateNoCredential, TranslateLanguage, TranslateRateLimit, TranslateUnavailable, TranslateNotRunning, TranslateBadReply}

// translateDetailMax is the most characters of what a plugin said of its failure that Kando keeps.
const translateDetailMax = 200

// TranslateError is a translator's failure: its code, one of the Translate… codes or "" for none known, and what the plugin said of it, cleaned and cut to translateDetailMax characters. Its text, which library.json keeps, is "<code>: <detail>", so TranslateCode reads the code back from a failure kept from before.
type TranslateError struct {
	Code, Detail string
}

// NewTranslateError is the failure of a translator that rejected with code and said message; a code that is not one of the known ones is kept as none.
func NewTranslateError(code, message string) *TranslateError {
	if !slices.Contains(translateCodes, code) {
		code = ""
	}
	message = oneLine(message)
	if r := []rune(message); len(r) > translateDetailMax {
		message = string(r[:translateDetailMax-1]) + "…"
	}
	return &TranslateError{Code: code, Detail: message}
}

func (e *TranslateError) Error() string {
	switch {
	case e.Code == "":
		return e.Detail
	case e.Detail == "":
		return e.Code
	}
	return e.Code + ": " + e.Detail
}

// TranslateCode is the code of a translator's failure as its text says it (TranslateError.Error), one of the Translate… codes, and the rest of the text; "" and the whole text for a text that starts with none.
func TranslateCode(text string) (code, detail string) {
	for _, c := range translateCodes {
		if text == c {
			return c, ""
		}
		if rest, ok := strings.CutPrefix(text, c+": "); ok {
			return c, rest
		}
	}
	return "", text
}
