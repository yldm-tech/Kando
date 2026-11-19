package plugins

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"kando/apps/kando/sites/catalog"
	kcore "kando/packages/core"

	"golang.org/x/net/publicsuffix"
	"golang.org/x/text/language"
)

// Pack is a pack that passed Validate: its manifest, with its texts cleaned (cleanText), the folder it is in and the hash of its files (hashFiles), which tells one content of an id from another. A script plugin's Source and Signer are set once its signature was verified (plugins/script), never from its manifest: where it comes from by the root its certificate chains to, and the ID of the release key that signed it.
type Pack struct {
	Manifest
	Dir, Hash string
	Source    Source
	Signer    string
}

// Source is where a script plugin comes from, by the root its certificate chains to: Kando's official root, or the developer key of this computer (developer mode). Data packs are not signed, and come from a file on this computer (SourceLocal).
type Source string

const (
	SourceLocal     Source = ""
	SourceOfficial  Source = "official"
	SourceDeveloper Source = "developer"
)

// Issue is a folder of the plugins folder that is not a valid pack, and why.
type Issue struct {
	Dir string
	Err error
}

// dataSchema is the schema of the data packs, scriptSchema that of the script plugins: a Kando or a phone that runs no script refuses a script plugin by its schema alone.
const (
	dataSchema   = 1
	scriptSchema = 2
)

// schema is the schema of the manifests of kind k.
func (k Kind) schema() int {
	if k == KindScript {
		return scriptSchema
	}
	return dataSchema
}

// idMax is the most bytes an id may have.
const idMax = 64

// idPattern is an id's shape: lowercase words of letters and digits joined by single dots or hyphens, which leaves out ".", "..", "a..b" and an id that starts or ends with either.
var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

// reservedPublisher is the publisher of Kando's own packs, which no data pack may use; a script plugin may only when Kando's official release key signed it (plugins/script).
const reservedPublisher = "kando."

// The limits of a pack (limitsOf).
type limits struct {
	// zip is the size of the largest zip Stage takes.
	zip int64
	// entries is the most files a pack may have, those inside a dictionary's zips counted too.
	entries int
	// bytes is the most its files may unpack to, those inside a dictionary's zips counted too.
	bytes int64
}

// limitsOf is the limits of a pack of kind k: a dictionary's are wider, as dictionaries are large.
func limitsOf(k Kind) limits {
	switch k {
	case KindDictionary:
		return limits{zip: 200 << 20, entries: 64, bytes: 1 << 30}
	case KindScript:
		return limits{zip: scriptMax, entries: 8, bytes: scriptMax}
	}
	return limits{zip: 32 << 20, entries: 16, bytes: 64 << 20}
}

// The most a glossary pack, a word list or a field may hold.
const (
	termsMax   = 5000
	termMax    = 200
	tiersMax   = 32
	versionMax = 32
	licenseMax = 64
	modelMax   = 200
	urlMax     = 2048
	langMax    = 35
	// sitesMax is the most sites a site pack may name, siteShapesMax the most shapes of video links a site may have, and siteNotMax the most hosts it may leave out.
	sitesMax      = 8
	siteShapesMax = 16
	siteNotMax    = 16
)

var (
	versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]*$`)
	tierPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
)

// Validate checks the pack in the folder dir, the same way for the app's scan, for `kando plugin validate` and for a pack Stage has just copied: its manifest (Parse) and every field of it, its id, the shape of the folder (only the files its manifest names, plus .md and .txt notes, no folder, link or special file, names in plain ASCII), and its limits, counting the bytes actually read, those inside a dictionary's zips included. It returns the pack with its texts cleaned of invisible and bidirectional characters, and the hash of its files.
func Validate(dir string) (Pack, error) {
	return validate(dir, nil)
}

// validate is Validate, with the hashes of the packs a scan before read (cache, nil for none): a pack whose files are just as they were then keeps its hash, and its dictionary's zips are not read again, while its manifest, its fields and the shape of its folder are checked as Validate checks them. The hash of a pack read in full goes into cache.
func validate(dir string, cache *hashCache) (Pack, error) {
	names, err := listFiles(dir)
	if err != nil {
		return Pack{}, err
	}
	if !slices.Contains(names, ManifestName) {
		return Pack{}, fmt.Errorf("%w: no %s", ErrBadManifest, ManifestName)
	}
	data, err := readSmall(filepath.Join(dir, ManifestName), manifestMax)
	if err != nil {
		return Pack{}, err
	}
	m, err := Parse(data)
	if err != nil {
		return Pack{}, err
	}
	if err := checkManifest(&m); err != nil {
		return Pack{}, err
	}
	if err := checkFiles(m, names); err != nil {
		return Pack{}, err
	}
	lim := limitsOf(m.Kind)
	b := &budget{bytes: lim.bytes, entries: lim.entries}
	if err := b.take(len(names)); err != nil {
		return Pack{}, err
	}
	// The files are stamped before they are read, so one that changes while it is read leaves a stamp the next scan does not find again.
	var stamps []fileStamp
	if cache != nil {
		if stamps, err = stampFiles(dir, names); err != nil {
			return Pack{}, err
		}
		if hash, ok := cache.get(dir, stamps); ok {
			return Pack{Manifest: m, Dir: dir, Hash: hash}, nil
		}
	}
	hash, err := readFiles(dir, names, m, b)
	if err != nil {
		return Pack{}, err
	}
	if m.Script != nil {
		if err := checkScriptPackage(dir, m.Script); err != nil {
			return Pack{}, err
		}
	}
	if cache != nil {
		cache.put(dir, stamps, hash)
	}
	return Pack{Manifest: m, Dir: dir, Hash: hash}, nil
}

// readFiles reads the files names of the pack m in dir: it hashes them all (hashFiles) and checks a Yomitan dictionary's zips (checkNested). The bytes the files unpack to come out of b once: a Yomitan zip's are those its entries unpack to, which checkNested counts, so the zip itself, read to be hashed, takes from a budget of its own, as large as b, which only bounds how much is read.
func readFiles(dir string, names []string, m Manifest, b *budget) (string, error) {
	var nested []string
	if m.Kind == KindDictionary && m.Dictionary.Format == FormatYomitan {
		nested = m.Dictionary.Files
	}
	raw := &budget{bytes: b.bytes}
	hash, err := hashFiles(dir, names, func(name string) *budget {
		if slices.Contains(nested, name) {
			return raw
		}
		return b
	})
	if err != nil {
		return "", err
	}
	for _, name := range nested {
		if err := checkNested(filepath.Join(dir, name), b); err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
	}
	return hash, nil
}

// listFiles is the names of the files in dir, sorted, each checked by checkFileName; a folder, a link or a special file in it is an error. The files a file manager writes into a folder it shows (systemMetadata) are left out, so they are neither checked, counted nor hashed: opening an installed pack's folder in Finder or Explorer leaves the pack as it was.
func listFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if systemMetadata(e.Name()) {
			continue
		}
		switch {
		case e.IsDir():
			return nil, fmt.Errorf("%w: %s is a folder", ErrBadShape, e.Name())
		case !e.Type().IsRegular():
			return nil, fmt.Errorf("%w: %s is a link or a special file", ErrUnsafePath, e.Name())
		}
		if err := checkFileName(e.Name()); err != nil {
			return nil, err
		}
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names, nil
}

// readSmall reads the regular file at name, which may be at most max bytes.
func readSmall(name string, max int64) ([]byte, error) {
	f, err := openRegular(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%w: %s is over %d KiB", ErrTooLarge, filepath.Base(name), max>>10)
	}
	return data, nil
}

// openRegular opens the file at name for reading, refusing one that is a link or not a regular file, or that was replaced between looking at it and opening it.
func openRegular(name string) (*os.File, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is a link or a special file", ErrUnsafePath, filepath.Base(name))
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err == nil && !os.SameFile(before, after) {
		err = fmt.Errorf("%w: %s changed while it was read", ErrUnsafePath, filepath.Base(name))
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// windowsReserved are the names Windows keeps for devices, which no file of a pack may have before its first dot and no part of an id between its dots may be, whatever the system, so a pack valid on one is valid on all.
var windowsReserved = []string{"con", "prn", "aux", "nul", "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9", "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9"}

// checkFileName checks the name of a file of a pack, which is a bare name: one that would reach into another folder or outside the pack is ErrUnsafePath; one that is not letters, digits, dots, hyphens and underscores of ASCII, starts with a dot, ends with one, is over 100 bytes or is a Windows device's is ErrBadShape.
func checkFileName(name string) error {
	if strings.ContainsAny(name, `/\:`) || name == ".." {
		return fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	if name == "" || len(name) > 100 || name[0] == '.' || name[len(name)-1] == '.' {
		return fmt.Errorf("%w: file name %q", ErrBadShape, name)
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return fmt.Errorf("%w: file name %q is not plain ASCII", ErrBadShape, name)
		}
	}
	stem, _, _ := strings.Cut(strings.ToLower(name), ".")
	if slices.Contains(windowsReserved, stem) {
		return fmt.Errorf("%w: file name %q", ErrBadShape, name)
	}
	if !filepath.IsLocal(name) {
		return fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	return nil
}

// checkFiles checks that the files of a pack are those its manifest names, plus the manifest and .md and .txt notes, and that every file it names is there.
func checkFiles(m Manifest, names []string) error {
	refs := m.files()
	for _, ref := range refs {
		if !slices.Contains(names, ref) {
			return fmt.Errorf("%w: %s is missing", ErrBadShape, ref)
		}
	}
	for _, name := range names {
		ext := strings.ToLower(path.Ext(name))
		if name == ManifestName || ext == ".md" || ext == ".txt" || slices.Contains(refs, name) {
			continue
		}
		return fmt.Errorf("%w: the manifest does not name %s", ErrBadShape, name)
	}
	return nil
}

// files is the files the manifest names.
func (m Manifest) files() []string {
	switch {
	case m.Dictionary != nil:
		return m.Dictionary.Files
	case m.Wordlist != nil:
		return []string{m.Wordlist.File}
	case m.Script != nil:
		return []string{m.Script.Package}
	}
	return nil
}

// checkManifest checks every field of m for its kind, cleaning its texts in place.
func checkManifest(m *Manifest) error {
	if !m.Kind.implemented() {
		return fmt.Errorf("%w: %s", ErrUnsupportedKind, m.Kind)
	}
	if m.Schema != m.Kind.schema() {
		return fmt.Errorf("%w: schema %d", ErrSchema, m.Schema)
	}
	if err := checkID(m.ID, m.Kind); err != nil {
		return err
	}
	if len(m.Version) > versionMax || !versionPattern.MatchString(m.Version) {
		return fmt.Errorf("%w: version %q", ErrBadField, m.Version)
	}
	m.License = cleanText(m.License, false)
	if m.License == "" || utf8.RuneCountInString(m.License) > licenseMax {
		return fmt.Errorf("%w: license %q", ErrBadField, m.License)
	}
	m.Author = cleanText(m.Author, false)
	if utf8.RuneCountInString(m.Author) > nameMax*2 {
		return fmt.Errorf("%w: author", ErrBadField)
	}
	if m.Homepage != "" {
		if err := checkURL(m.Homepage); err != nil {
			return fmt.Errorf("homepage: %w", err)
		}
	}
	if err := checkText("name", m.Name, nameMax, true, false); err != nil {
		return err
	}
	for _, name := range m.Name {
		if reserved(name) {
			return fmt.Errorf("%w: %q", ErrReservedName, name)
		}
	}
	if err := checkText("description", m.Description, textMax, false, true); err != nil {
		return err
	}
	// Exactly the field of its kind is set: Parse leaves another kind's field set when the manifest has it.
	set := 0
	for _, isSet := range []bool{m.Dictionary != nil, m.Wordlist != nil, m.Glossary != nil, m.Service != nil, m.Site != nil, m.Script != nil} {
		if isSet {
			set++
		}
	}
	// The fields of a script plugin's besides its script are only a script plugin's.
	if m.Kind != KindScript && (m.Kando != "" || m.Capabilities != nil || m.Optional != nil || m.Contributes != nil || m.Settings != nil || m.Events != nil) {
		return fmt.Errorf("%w: a %s pack has a script plugin's fields", ErrBadField, m.Kind)
	}
	var err error
	switch {
	case m.Kind == KindDictionary && m.Dictionary != nil && set == 1:
		err = checkDictionary(m.Dictionary)
	case m.Kind == KindWordlist && m.Wordlist != nil && set == 1:
		err = checkWordlist(m.Wordlist)
	case m.Kind == KindGlossary && m.Glossary != nil && set == 1:
		err = checkGlossary(m.Glossary)
	case m.Kind == KindAIService && m.Service != nil && set == 1:
		err = checkService(m.Service)
	case m.Kind == KindSite && m.Site != nil && set == 1:
		err = checkSite(m.Site)
	case m.Kind == KindScript && m.Script != nil && set == 1:
		err = checkScript(m)
	default:
		err = fmt.Errorf("%w: a %s pack needs its %s field and no other kind's", ErrBadField, m.Kind, kindField(m.Kind))
	}
	return err
}

// kindField is the manifest's key for the fields of kind k.
func kindField(k Kind) string {
	if k == KindAIService {
		return "service"
	}
	return string(k)
}

// checkID checks that id is <publisher>.<name> (idPattern with at least one dot, at most idMax bytes, no part between its dots a Windows device's name) and, for a data pack of kind, is not under the publisher kando., which is Kando's own. A script plugin's publisher is the one its certificate covers, which the app checks once it verified its signature (plugins/script): only Kando's official release key may sign kando. plugins.
func checkID(id string, kind Kind) error {
	if !validID(id) {
		return fmt.Errorf("%w: %q", ErrBadID, id)
	}
	if kind != KindScript && strings.HasPrefix(id, reservedPublisher) {
		return fmt.Errorf("%w: %q", ErrReservedID, id)
	}
	return nil
}

func validID(id string) bool {
	if len(id) > idMax || !strings.Contains(id, ".") || !idPattern.MatchString(id) || !filepath.IsLocal(id) {
		return false
	}
	for part := range strings.SplitSeq(id, ".") {
		if slices.Contains(windowsReserved, part) {
			return false
		}
	}
	return true
}

// checkText checks the text t a pack supplies for what: its languages are language tags in their canonical form, its English is there when required, and each, cleaned (cleanText), is at most max characters and not empty. It cleans t in place.
func checkText(what string, t Text, max int, required, lines bool) error {
	if len(t) == 0 && !required {
		return nil
	}
	if _, ok := t["en"]; !ok {
		return fmt.Errorf("%w: %s has no English (en)", ErrBadName, what)
	}
	for lang, s := range t {
		if err := checkLanguage(lang); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		s = cleanText(s, lines)
		if s == "" || utf8.RuneCountInString(s) > max {
			return fmt.Errorf("%w: %s in %s is empty or over %d characters", ErrBadName, what, lang, max)
		}
		t[lang] = s
	}
	return nil
}

// checkLanguage checks that lang is a language tag in its canonical form, such as "zh-Hans", so Text.In finds it by the tag the app has.
func checkLanguage(lang string) error {
	tag, err := language.Parse(lang)
	if err != nil || len(lang) > langMax || tag.String() != lang {
		return fmt.Errorf("%w: language %q", ErrBadField, lang)
	}
	return nil
}

func checkLanguages(what string, langs []string) error {
	if len(langs) == 0 {
		return fmt.Errorf("%w: %s has no language", ErrBadField, what)
	}
	for _, lang := range langs {
		if err := checkLanguage(lang); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
	}
	return nil
}

func checkDictionary(d *Dictionary) error {
	switch d.Format {
	case FormatYomitan, FormatCCCEDICT, FormatTSV:
	default:
		return fmt.Errorf("%w: dictionary format %q", ErrBadField, d.Format)
	}
	if len(d.Files) == 0 {
		return fmt.Errorf("%w: the dictionary names no file", ErrBadField)
	}
	for i, name := range d.Files {
		if err := checkFileName(name); err != nil {
			return err
		}
		if name == ManifestName || slices.Contains(d.Files[:i], name) {
			return fmt.Errorf("%w: dictionary file %q", ErrBadField, name)
		}
		// A Yomitan dictionary is a zip, and only a Yomitan dictionary's file is read as one.
		if isZip := strings.EqualFold(path.Ext(name), ".zip"); isZip != (d.Format == FormatYomitan) {
			return fmt.Errorf("%w: dictionary file %q for the format %s", ErrBadField, name, d.Format)
		}
	}
	if err := checkLanguages("from", d.From); err != nil {
		return err
	}
	if err := checkLanguages("to", d.To); err != nil {
		return err
	}
	return checkText("attribution", d.Attribution, textMax, false, true)
}

func checkWordlist(w *Wordlist) error {
	if err := checkLanguage(w.Language); err != nil {
		return err
	}
	if err := checkFileName(w.File); err != nil {
		return err
	}
	if w.File == ManifestName {
		return fmt.Errorf("%w: word list file %q", ErrBadField, w.File)
	}
	if len(w.Tiers) == 0 || len(w.Tiers) > tiersMax {
		return fmt.Errorf("%w: a word list has 1 to %d tiers", ErrBadField, tiersMax)
	}
	for i, tier := range w.Tiers {
		if !tierPattern.MatchString(tier.ID) || slices.ContainsFunc(w.Tiers[:i], func(t Tier) bool { return t.ID == tier.ID }) {
			return fmt.Errorf("%w: tier id %q", ErrBadField, tier.ID)
		}
		switch tier.Tone {
		case ToneAccent, ToneWarn, ToneSuccess, ToneMuted:
		default:
			return fmt.Errorf("%w: tier tone %q", ErrBadField, tier.Tone)
		}
		if err := checkText("tier label", tier.Label, nameMax, true, false); err != nil {
			return err
		}
	}
	return nil
}

func checkGlossary(g *Glossary) error {
	if err := checkLanguage(g.From); err != nil {
		return fmt.Errorf("from: %w", err)
	}
	if err := checkLanguage(g.To); err != nil {
		return fmt.Errorf("to: %w", err)
	}
	if len(g.Terms) == 0 || len(g.Terms) > termsMax {
		return fmt.Errorf("%w: a glossary has 1 to %d terms", ErrBadField, termsMax)
	}
	for i := range g.Terms {
		t := &g.Terms[i]
		t.Source, t.Target = cleanText(t.Source, false), cleanText(t.Target, false)
		if t.Source == "" || t.Target == "" || utf8.RuneCountInString(t.Source) > termMax || utf8.RuneCountInString(t.Target) > termMax {
			return fmt.Errorf("%w: term %d is empty or over %d characters", ErrBadField, i+1, termMax)
		}
	}
	return nil
}

func checkService(s *Service) error {
	if s.BaseURL == "" {
		return fmt.Errorf("%w: the service has no baseURL", ErrBadField)
	}
	if err := checkURL(s.BaseURL); err != nil {
		return fmt.Errorf("baseURL: %w", err)
	}
	if s.KeyURL != "" {
		if err := checkURL(s.KeyURL); err != nil {
			return fmt.Errorf("keyURL: %w", err)
		}
	}
	if cleanText(s.Model, false) != s.Model || utf8.RuneCountInString(s.Model) > modelMax {
		return fmt.Errorf("%w: model %q", ErrBadField, s.Model)
	}
	switch s.Group {
	case GroupCloud, GroupLocal:
	default:
		return fmt.Errorf("%w: service group %q", ErrBadField, s.Group)
	}
	return nil
}

// checkSite checks a site pack: 1 to sitesMax sites of at most siteShapesMax shapes and siteNotMax hosts left out each, which kcore.ValidateVideoSites takes (domains that do not overlap, patterns Go and JavaScript read alike that run in time), whose domains are each a registrable domain or a host under one, not a public suffix every site under it shares (publicsuffix), and none of which is, or is under or over, a site Kando knows itself (siteKnown).
func checkSite(s *Site) error {
	if len(s.Sites) == 0 || len(s.Sites) > sitesMax {
		return fmt.Errorf("%w: a site pack has 1 to %d sites", ErrBadField, sitesMax)
	}
	for _, d := range s.Sites {
		if len(d.Videos) > siteShapesMax || len(d.Not) > siteNotMax {
			return fmt.Errorf("%w: %s has over %d shapes or %d hosts left out", ErrBadField, d.Domain, siteShapesMax, siteNotMax)
		}
	}
	if err := kcore.ValidateVideoSites(s.VideoSites()); err != nil {
		return fmt.Errorf("%w: %v", ErrBadSite, err)
	}
	for _, d := range s.Sites {
		if _, err := publicsuffix.EffectiveTLDPlusOne(d.Domain); err != nil {
			return fmt.Errorf("%w: %s is a public suffix", ErrBadSite, d.Domain)
		}
		if siteKnown(d.Domain) {
			return fmt.Errorf("%w: %s", ErrSiteKnown, d.Domain)
		}
	}
	return nil
}

// siteKnown reports whether domain is, or is under or over, a site Kando knows itself: one of kcore.VideoSites, whose videos Kando's rules decide, or one of the catalog's (sites/catalog, its domain or an alias that is a domain), which yt-dlp downloads from and the sign-in recipes, the extension and the remote API trust, so no pack decides how Kando reads such a site or keeps its downloads from the sign-in.
func siteKnown(domain string) bool {
	overlaps := func(known string) bool {
		return domain == known || strings.HasSuffix(domain, "."+known) || strings.HasSuffix(known, "."+domain)
	}
	for _, own := range kcore.VideoSites {
		if overlaps(own.Domain) {
			return true
		}
	}
	for _, s := range catalog.Load().Sites {
		for _, known := range append([]string{s.Domain}, s.Aliases...) {
			if strings.Contains(known, ".") && overlaps(strings.ToLower(known)) {
				return true
			}
		}
	}
	return false
}

// checkURL checks that raw is an absolute https address with a host name and no user name or password, or an http one on this computer (loopback), the only addresses a pack may name. An address with a character cleanText takes out, such as a bidirectional control or a zero-width space, is refused, as the address is shown to the user before Kando opens it and must read as what it is.
func checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > urlMax || cleanText(raw, false) != raw || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return fmt.Errorf("%w: %q", ErrBadField, raw)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if loopback(u.Hostname()) {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrInsecureURL, raw)
}

// loopback reports whether host is this computer: localhost, or a loopback address such as 127.0.0.1 or ::1.
func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// checkNested checks a zip inside a pack, a Yomitan dictionary, without unpacking it: its entries' names stay inside it, none is a link or another zip (one level of nesting only), and its entries, folders included, and the bytes its files unpack to, counted as they are read, come out of the pack's budget b. A file whose bytes do not match its checksum or cannot be decompressed is ErrBadArchive.
func checkNested(name string, b *budget) error {
	f, err := openRegular(name)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	zr, err := openZip(f, info.Size())
	if err != nil {
		return err
	}
	for _, zf := range zr.File {
		if strings.Contains(zf.Name, `\`) || !filepath.IsLocal(zf.Name) {
			return fmt.Errorf("%w: %q", ErrUnsafePath, zf.Name)
		}
		if err := b.take(1); err != nil {
			return err
		}
		mode := zf.Mode()
		if mode.IsDir() {
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("%w: %q is a link or a special file", ErrUnsafePath, zf.Name)
		}
		if strings.EqualFold(path.Ext(zf.Name), ".zip") {
			return fmt.Errorf("%w: %q is a zip inside a zip", ErrBadShape, zf.Name)
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrBadArchive, err)
		}
		_, err = b.copy(io.Discard, rc)
		rc.Close()
		if err != nil && !errors.Is(err, ErrTooLarge) {
			err = fmt.Errorf("%w: %s: %v", ErrBadArchive, zf.Name, err)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// zipHeadersMax is the most bytes openZip lets archive/zip read of a zip while it reads the zip's central directory. A pack's zip has at most 64 entries, whose records and end of directory (with a comment of up to 64 KiB, which archive/zip looks for in the last 65 KiB) take far less.
const zipHeadersMax = 1 << 20

// openZip reads the central directory of the zip r, size bytes long. archive/zip reads every record of the directory before it returns, whatever count the zip declares, so a zip of millions of records would have it allocate gigabytes before any limit of the pack is checked; openZip stops it once it has read zipHeadersMax bytes, which is ErrTooLarge. A zip that cannot be read is ErrBadArchive, and one with an entry whose name reaches outside it ErrUnsafePath.
func openZip(r io.ReaderAt, size int64) (*zip.Reader, error) {
	hr := &headerReader{r: r, left: zipHeadersMax, reading: true}
	zr, err := zip.NewReader(hr, size)
	hr.reading = false
	switch {
	case errors.Is(err, ErrTooLarge):
		return nil, err
	case errors.Is(err, zip.ErrInsecurePath):
		return nil, fmt.Errorf("%w: an entry of the zip", ErrUnsafePath)
	case err != nil:
		return nil, fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	return zr, nil
}

// headerReader is the io.ReaderAt openZip hands archive/zip: while reading is set, it reads at most left bytes in all, and after that it reads r as it is, as the zip's files are read through it too.
type headerReader struct {
	r       io.ReaderAt
	left    int64
	reading bool
}

func (h *headerReader) ReadAt(p []byte, off int64) (int, error) {
	if h.reading {
		if int64(len(p)) > h.left {
			return 0, fmt.Errorf("%w: the zip's directory is over %d KiB", ErrTooLarge, zipHeadersMax>>10)
		}
		h.left -= int64(len(p))
	}
	return h.r.ReadAt(p, off)
}
