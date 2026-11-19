package index

import (
	"net/url"
	"strings"

	"kando/apps/kando/plugins"
)

// linkScheme is Kando's own URL scheme, which nsgo.yaml's url_schemes declares (browserext.Scheme).
const linkScheme = "kando"

// linkPath is the host and path of the link that asks Kando to install a pack of the index: kando://plugin/install?id=<id>, which the directory's page on Kando's site opens (Open in Kando).
const linkPath = "plugin/install"

// InstallLink is the link that asks Kando to install the pack id of the index.
func InstallLink(id string) string {
	return linkScheme + "://" + linkPath + "?" + url.Values{"id": {id}}.Encode()
}

// ReadInstallLink reads a link that asks Kando to install a pack of the index (InstallLink): ok for one, with the pack's id, "" when the link names no valid id (plugins.CheckID); false for any other link. Any page can open such a link, so it only names the pack: the app installs nothing the signed index does not list, and nothing without the install confirmation.
func ReadInstallLink(link string) (id string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !strings.EqualFold(u.Scheme, linkScheme) || !strings.EqualFold(strings.Trim(u.Host+u.Opaque+u.Path, "/"), linkPath) {
		return "", false
	}
	id = u.Query().Get("id")
	// Script is the kind whose ids the check holds to the syntax alone: whether the index lists it decides the rest.
	if plugins.CheckID(id, plugins.KindScript) != nil {
		return "", true
	}
	return id, true
}
