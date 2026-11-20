package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// The limits of a script plugin: the bytes its pack and its package may hold, its commands, settings and HTTP rules, a choice setting's choices and a text setting's characters.
const (
	scriptMax    = 16 << 20
	PackageMax   = 8 << 20
	commandsMax  = 16
	settingsMax  = 16
	rulesMax     = 16
	choicesMax   = 32
	textValueMax = 1000
)

// Script is a script plugin's code: the file of its signed package (.nsgopkg) and the package's SHA-256 in hex, so the manifest names the one package it was written for.
type Script struct {
	Package string `json:"package"`
	SHA256  string `json:"sha256"`
}

// Contributes is what a script plugin adds to Kando (docs/plugin-system.md 2.2, 2.3): commands (E9), sources of word lookups (E10), subtitle formats (E11), targets of the saved lines' export (E12), annotators of the lines (E13), sources of machine translation (E14) and panels of the player's 扩展 tab (E15). Each but a translator names the function of the plugin that does it (Export), which the plugin's code must export; translators call translate, and candidates when the plugin exports it (TranslateExport, CandidatesExport).
type Contributes struct {
	Commands        []Command        `json:"commands,omitempty"`
	Lookups         []Lookup         `json:"lookups,omitempty"`
	SubtitleFormats []SubtitleFormat `json:"subtitleFormats,omitempty"`
	Exporters       []Exporter       `json:"exporters,omitempty"`
	Annotators      []Annotator      `json:"annotators,omitempty"`
	Translators     []Translator     `json:"translators,omitempty"`
	Panels          []Panel          `json:"panels,omitempty"`
}

// The moments a command applies (Command.When).
const (
	WhenAlways = "always"
	WhenFilm   = "film"
)

// Command is a command a script plugin adds to the menu bar, the command palette and, with a key, the shortcuts: its id, the function of the plugin it calls with that id (Export), its key in NSGo's form ("mod+shift+k", "" for none), when it applies (WhenAlways, the default, or WhenFilm while a film is open) and its title.
type Command struct {
	ID     string `json:"id"`
	Export string `json:"export"`
	Key    string `json:"key,omitempty"`
	When   string `json:"when,omitempty"`
	Title  Text   `json:"title"`
}

// The types of a script plugin's settings (Setting.Type).
const (
	SettingBool   = "bool"
	SettingChoice = "choice"
	SettingNumber = "number"
	SettingText   = "text"
)

// Setting is a setting a script plugin declares, which Kando draws on its details and keeps for it: a switch (bool), one of Choices (choice), a number between Min and Max (number) or a text of at most Max characters (text). Default is its value until the user changes it, of the setting's type. A secret is not a setting: it is a credential (Manifest.Credentials), which Kando keeps in the system's secure store and only the host uses (kando.net.credentialFetch, docs/plugin-system.md 6.3).
type Setting struct {
	Key     string   `json:"key"`
	Type    string   `json:"type"`
	Title   Text     `json:"title"`
	Default any      `json:"default,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Choices []Choice `json:"choices,omitempty"`
}

// Choice is one choice of a choice setting.
type Choice struct {
	ID    string `json:"id"`
	Label Text   `json:"label"`
}

var (
	kandoPattern   = regexp.MustCompile(`^>=[0-9]+\.[0-9]+\.[0-9]+$`)
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	commandPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	exportPattern  = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)
	settingPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
)

// checkScript checks the fields of a script plugin m (docs/plugin-system.md 4.2): its package, the capabilities it asks for, which are its own permissions (host) and HTTP requests each limited to methods and paths (network) and nothing else, the permissions it can do without (optional), what it contributes, its credentials, settings and events. Its texts are cleaned in place.
func checkScript(m *Manifest) error {
	if m.Kando != "" && !kandoPattern.MatchString(m.Kando) {
		return fmt.Errorf("%w: kando %q is not >=MAJOR.MINOR.PATCH", ErrBadField, m.Kando)
	}
	s := m.Script
	if err := checkFileName(s.Package); err != nil {
		return err
	}
	if s.Package == ManifestName || !strings.EqualFold(filepath.Ext(s.Package), ".nsgopkg") {
		return fmt.Errorf("%w: script package %q is not an .nsgopkg", ErrBadField, s.Package)
	}
	if !sha256Pattern.MatchString(s.SHA256) {
		return fmt.Errorf("%w: script sha256 %q", ErrBadField, s.SHA256)
	}
	if err := checkCapabilities(m.Capabilities); err != nil {
		return err
	}
	var host []string
	if m.Capabilities != nil {
		host = m.Capabilities.Host
	}
	for i, p := range m.Optional {
		if !p.Known() {
			return fmt.Errorf("%w: optional %q", ErrUnknownPerm, p)
		}
		if slices.Contains(host, string(p)) || slices.Contains(m.Optional[:i], p) {
			return fmt.Errorf("%w: optional %q is asked for twice", ErrBadField, p)
		}
	}
	if m.Contributes != nil {
		if err := checkCommands(m.Contributes.Commands); err != nil {
			return err
		}
		if err := checkContributes(m.Contributes); err != nil {
			return err
		}
	}
	if err := checkTranslators(m); err != nil {
		return err
	}
	if err := checkCredentials(m); err != nil {
		return err
	}
	if err := checkSettings(m.Settings); err != nil {
		return err
	}
	for i, e := range m.Events {
		if !slices.Contains(Events, e) || slices.Contains(m.Events[:i], e) {
			return fmt.Errorf("%w: event %q", ErrBadField, e)
		}
	}
	return nil
}

// checkCapabilities checks that c asks for no capability but host and network, its host names are permissions this version has, each once, and its network is HTTP rules alone, at most rulesMax, each limited to methods and to paths or path prefixes, which the signing tool's own check takes too (pkg.ParseCapabilities). A plugin that asks for nothing has none.
func checkCapabilities(c *pkg.Capabilities) error {
	if c == nil {
		return nil
	}
	rest := *c
	rest.Host, rest.Network = nil, nil
	if !reflect.ValueOf(rest).IsZero() {
		return fmt.Errorf("%w: only host and network", ErrBadCapabilities)
	}
	for i, h := range c.Host {
		if !Perm(h).Known() {
			return fmt.Errorf("%w: %q", ErrUnknownPerm, h)
		}
		if slices.Contains(c.Host[:i], h) {
			return fmt.Errorf("%w: %q is asked for twice", ErrBadField, h)
		}
	}
	if n := c.Network; n != nil {
		if len(n.Hosts) > 0 {
			return fmt.Errorf("%w: network.hosts allows any request; name http rules", ErrBadCapabilities)
		}
		if len(n.HTTP) == 0 || len(n.HTTP) > rulesMax {
			return fmt.Errorf("%w: network has 1 to %d http rules", ErrBadCapabilities, rulesMax)
		}
		for _, r := range n.HTTP {
			if len(r.Methods) == 0 || len(r.Paths) == 0 && len(r.PathPrefixes) == 0 || strings.Contains(r.Host, "*") {
				return fmt.Errorf("%w: an http rule names one host, its methods and its paths or path prefixes", ErrBadCapabilities)
			}
		}
	}
	data, err := json.Marshal(c)
	if err == nil {
		_, _, err = pkg.ParseCapabilities(data, true)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadCapabilities, err)
	}
	return nil
}

func checkCommands(commands []Command) error {
	if len(commands) > commandsMax {
		return fmt.Errorf("%w: at most %d commands", ErrBadField, commandsMax)
	}
	for i := range commands {
		c := &commands[i]
		if !commandPattern.MatchString(c.ID) || slices.ContainsFunc(commands[:i], func(o Command) bool { return o.ID == c.ID }) {
			return fmt.Errorf("%w: command id %q", ErrBadField, c.ID)
		}
		if !exportPattern.MatchString(c.Export) {
			return fmt.Errorf("%w: command export %q", ErrBadField, c.Export)
		}
		if c.Key != "" && !validKey(c.Key) {
			return fmt.Errorf("%w: command key %q", ErrBadField, c.Key)
		}
		switch c.When {
		case "", WhenAlways, WhenFilm:
		default:
			return fmt.Errorf("%w: command when %q", ErrBadField, c.When)
		}
		if err := checkText("command title", c.Title, nameMax*2, true, false); err != nil {
			return err
		}
	}
	return nil
}

// validKey reports whether key is a key in NSGo's form a plugin may take: modifiers among mod, ctrl, alt and shift, each once, at least one of mod, ctrl and alt, so a plugin's key never takes a key a text field or the player uses alone, and a letter, a digit or a function key.
func validKey(key string) bool {
	parts := strings.Split(key, "+")
	last := parts[len(parts)-1]
	held := false
	for i, m := range parts[:len(parts)-1] {
		if !slices.Contains([]string{"mod", "ctrl", "alt", "shift"}, m) || slices.Contains(parts[:i], m) {
			return false
		}
		held = held || m != "shift"
	}
	if !held {
		return false
	}
	if len(last) == 1 {
		return last[0] >= 'a' && last[0] <= 'z' || last[0] >= '0' && last[0] <= '9'
	}
	n, ok := strings.CutPrefix(last, "f")
	return ok && slices.Contains(strings.Fields("1 2 3 4 5 6 7 8 9 10 11 12"), n)
}

func checkSettings(settings []Setting) error {
	if len(settings) > settingsMax {
		return fmt.Errorf("%w: at most %d settings", ErrBadField, settingsMax)
	}
	for i := range settings {
		s := &settings[i]
		if !settingPattern.MatchString(s.Key) || slices.ContainsFunc(settings[:i], func(o Setting) bool { return o.Key == s.Key }) {
			return fmt.Errorf("%w: setting key %q", ErrBadField, s.Key)
		}
		if err := checkText("setting title", s.Title, nameMax*2, true, false); err != nil {
			return err
		}
		if err := checkSetting(s); err != nil {
			return fmt.Errorf("setting %s: %w", s.Key, err)
		}
	}
	return nil
}

// settingSecret is the type of a setting the first script plugins could declare, which no host call ever used, as a credential (Manifest.Credentials) does what it was for: Validate refuses it with a reason that names credentials.
const settingSecret = "secret"

// checkSetting checks the setting s of its type: what bounds it, that only its type's fields are set, and that its default is a value of it.
func checkSetting(s *Setting) error {
	bad := func(what string) error { return fmt.Errorf("%w: %s", ErrBadField, what) }
	if s.Type == settingSecret {
		return bad("a secret is not a setting: declare it under credentials and ask for kando.net.credentialFetch, so Kando keeps it and sends it only to the hosts the credential names")
	}
	if s.Type != SettingChoice && s.Choices != nil {
		return bad("choices of a setting that is not a choice")
	}
	if s.Type != SettingNumber && s.Type != SettingText && (s.Min != nil || s.Max != nil) {
		return bad("bounds of a setting that is not a number or a text")
	}
	switch s.Type {
	case SettingBool:
	case SettingChoice:
		if len(s.Choices) == 0 || len(s.Choices) > choicesMax {
			return bad(fmt.Sprintf("a choice has 1 to %d choices", choicesMax))
		}
		for i := range s.Choices {
			c := &s.Choices[i]
			if !commandPattern.MatchString(c.ID) || slices.ContainsFunc(s.Choices[:i], func(o Choice) bool { return o.ID == c.ID }) {
				return bad(fmt.Sprintf("choice id %q", c.ID))
			}
			if err := checkText("choice label", c.Label, nameMax, true, false); err != nil {
				return err
			}
		}
	case SettingNumber:
		if s.Min != nil && s.Max != nil && *s.Min > *s.Max {
			return bad("min over max")
		}
		for _, b := range []*float64{s.Min, s.Max} {
			if b != nil && (math.IsInf(*b, 0) || math.IsNaN(*b)) {
				return bad("a bound that is not a number")
			}
		}
	case SettingText:
		if s.Min != nil || s.Max != nil && (*s.Max < 1 || *s.Max > textValueMax || *s.Max != math.Trunc(*s.Max)) {
			return bad(fmt.Sprintf("a text's max is 1 to %d", textValueMax))
		}
	default:
		return bad(fmt.Sprintf("setting type %q", s.Type))
	}
	if s.Default != nil {
		v, ok := s.Value(s.Default)
		if !ok {
			return bad("its default is not a value of its type")
		}
		s.Default = v
	}
	return nil
}

// Value is v as a value of the setting s, and whether it is one: a bool of a switch, the id of one of a choice's choices, a number within a number's bounds, a text of at most a text's characters, its line breaks and invisible characters taken out (cleanText).
func (s Setting) Value(v any) (any, bool) {
	switch s.Type {
	case SettingBool:
		b, ok := v.(bool)
		return b, ok
	case SettingChoice:
		id, ok := v.(string)
		return id, ok && slices.ContainsFunc(s.Choices, func(c Choice) bool { return c.ID == id })
	case SettingNumber:
		n, ok := v.(float64)
		if !ok || math.IsInf(n, 0) || math.IsNaN(n) || s.Min != nil && n < *s.Min || s.Max != nil && n > *s.Max {
			return nil, false
		}
		return n, true
	case SettingText:
		text, ok := v.(string)
		if !ok {
			return nil, false
		}
		text = cleanText(text, false)
		return text, utf8.RuneCountInString(text) <= s.textMax()
	}
	return nil, false
}

// textMax is the most characters a text setting holds: its Max, else 200.
func (s Setting) textMax() int {
	if s.Max != nil {
		return int(*s.Max)
	}
	return 200
}

// Zero is the setting's value when the user never set it: its default, else false, its first choice, its lower bound or 0, or "".
func (s Setting) Zero() any {
	if s.Default != nil {
		return s.Default
	}
	switch s.Type {
	case SettingBool:
		return false
	case SettingChoice:
		return s.Choices[0].ID
	case SettingNumber:
		if s.Min != nil && *s.Min > 0 {
			return *s.Min
		}
		if s.Max != nil && *s.Max < 0 {
			return *s.Max
		}
		return float64(0)
	case SettingText:
		return ""
	}
	return nil
}

// checkScriptPackage checks that the package of the script plugin in dir is the one its manifest names, by its SHA-256, and at most PackageMax bytes.
func checkScriptPackage(dir string, s *Script) error {
	data, err := readSmall(filepath.Join(dir, s.Package), PackageMax)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != s.SHA256 {
		return fmt.Errorf("%w: %s is not the package the manifest names", ErrTampered, s.Package)
	}
	return nil
}

// RunsOn reports whether the script plugin m runs on Kando version, a MAJOR.MINOR.PATCH with an optional v before it and a suffix after it: always when m names no oldest version (Kando), and for a version that is not one, as a build of the source; else when version is that one or newer.
func (m Manifest) RunsOn(version string) bool {
	need, ok := strings.CutPrefix(m.Kando, ">=")
	if !ok {
		return true
	}
	have, ok := versionNumbers(strings.TrimPrefix(version, "v"))
	if !ok {
		return true
	}
	want, _ := versionNumbers(need)
	return slices.Compare(have, want) >= 0
}

// versionNumbers is MAJOR.MINOR.PATCH of v, a suffix after a hyphen or a plus left out.
func versionNumbers(v string) ([]int, bool) {
	v, _, _ = strings.Cut(v, "+")
	v, _, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil, false
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}
