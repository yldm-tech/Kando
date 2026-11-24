package plugins

import "errors"

// The errors Parse, Validate, Stage, Swap, Remove and the state file return, wrapped with what was wrong (errors.Is finds them). They are English and never shown as they are: the app maps each to a message of its catalogs, and the command line prints them.
var (
	// ErrBadManifest is a kando-plugin.json that is missing, is not JSON, has a key this version does not know, or has a value of the wrong type.
	ErrBadManifest = errors.New("the manifest kando-plugin.json is missing or cannot be read")
	// ErrUnknownKind is a manifest whose kind is none Kando knows.
	ErrUnknownKind = errors.New("the kind of pack is unknown")
	// ErrUnsupportedKind is a kind Kando knows but this version cannot install yet.
	ErrUnsupportedKind = errors.New("this version of Kando cannot install this kind of pack")
	// ErrSchema is a manifest made for another version of the format, usually a newer Kando.
	ErrSchema = errors.New("the pack is made for another version of Kando")
	// ErrBadID is an id that is not <publisher>.<name> in lowercase letters, digits, dots and hyphens, at most 64 bytes.
	ErrBadID = errors.New("the pack's id is not valid")
	// ErrReservedID is an id under kando., which only Kando's own packs may use.
	ErrReservedID = errors.New("the pack's id is reserved for Kando")
	// ErrBadName is a name, description or label that is empty where it is required, has no English text, or is too long.
	ErrBadName = errors.New("a name or label of the pack is not valid")
	// ErrReservedName is a name that reads as Kando's own or as a built-in AI service's.
	ErrReservedName = errors.New("the pack's name is reserved")
	// ErrBadField is any other value of the manifest that is not valid, such as a language that is not a language tag.
	ErrBadField = errors.New("a field of the manifest is not valid")
	// ErrBadSite is a site of a site pack Kando's browser cannot recognize safely: a domain that is not a host name, or is a public suffix such as com or co.uk, which every site under it shares, or a pattern outside the regular expressions Go and JavaScript read alike, or one that could take a page's script exponential time (kcore.ValidateVideoSite).
	ErrBadSite = errors.New("a site of the pack is not one Kando can recognize safely")
	// ErrSiteKnown is a site of a site pack that is, or is under or over, a site Kando knows itself: one of kcore.VideoSites, whose videos only Kando's own rules decide, or one of the sites yt-dlp downloads from that Kando lists (sites/catalog).
	ErrSiteKnown = errors.New("the pack names a site Kando knows itself")
	// ErrInsecureURL is an address that is not https, other than one on this computer (loopback).
	ErrInsecureURL = errors.New("an address of the pack is not https")
	// ErrBadShape is a pack whose files are not what its manifest says: a folder in it, a file name that is not plain ASCII, a file it names that is missing, or a file it does not name.
	ErrBadShape = errors.New("the pack's files do not match its manifest")
	// ErrUnsafePath is a file that is a link or a special file, or whose name would reach outside the pack.
	ErrUnsafePath = errors.New("the pack holds a link or a path outside it")
	// ErrBadArchive is a file that is not a zip archive or is damaged.
	ErrBadArchive = errors.New("the file is not a zip archive or is damaged")
	// ErrTooLarge is a pack over one of its limits: the zip's size, the number of files, or the bytes they unpack to.
	ErrTooLarge = errors.New("the pack is too large")
	// ErrMismatch is an installed pack whose folder is not named after its kind and id.
	ErrMismatch = errors.New("the pack's folder does not match its id or kind")
	// ErrIDTaken is a pack whose id an installed pack of another kind already has: an id is one pack, whatever its kind.
	ErrIDTaken = errors.New("a pack of another kind already has this id")
	// ErrNotInstalled is an id no installed pack has.
	ErrNotInstalled = errors.New("no pack with this id is installed")
	// ErrStateFormat is a state.json written by a newer Kando, which this one reads as every pack turned off and does not write over.
	ErrStateFormat = errors.New("the plugins' state was written by a newer version of Kando")
	// ErrBadState is a state.json that cannot be read, is not JSON or has no format; every pack reads as turned off until it is written again.
	ErrBadState = errors.New("the plugins' state cannot be read")
	// ErrUninstallRefused is an uninstall refused before anything was removed, wrapped with why, as while state.json is a newer Kando's (ErrStateFormat), which this one does not write over.
	ErrUninstallRefused = errors.New("the pack was not uninstalled")
	// ErrUnknownPerm is a script plugin that asks for a permission of Kando's this version does not have, or that is not a permission at all.
	ErrUnknownPerm = errors.New("the plugin asks for a permission this version of Kando does not have")
	// ErrBadCapabilities is a script plugin that asks for a capability Kando never grants a plugin: anything but its own permissions (host) and HTTP requests each limited to methods and paths (network).
	ErrBadCapabilities = errors.New("the plugin asks for a capability Kando does not grant plugins")
	// ErrSourceNetwork is a script plugin that contributes media sources and also asks for another way out of Kando, network rules, a credential, the AI service or a page in the browser, or for the lines seen (docs/media-sources.md 11.1): a source's plugin reaches only the user's server, through kando.source.fetch.
	ErrSourceNetwork = errors.New("a plugin with media sources may reach only their servers")
	// ErrTampered is a script plugin whose parts do not agree: its package is not the one its manifest names by hash, or what its signed code says of itself (kandoManifest, its capabilities, its id or version) is not what its manifest says.
	ErrTampered = errors.New("the plugin was changed after it was signed")
)

// StateUnsaved is state.json not saved after an install or a removal that was done: the pack is installed or removed, and Err is why which packs are on was not saved. The app says the work was done and why the state was not saved, rather than that it failed.
type StateUnsaved struct{ Err error }

func (e StateUnsaved) Error() string { return "the plugins' state was not saved: " + e.Err.Error() }

func (e StateUnsaved) Unwrap() error { return e.Err }
