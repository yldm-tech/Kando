package widgets

import (
	"net/url"
	"strings"
)

// Scheme is the app's own URL scheme, nsgo.yaml's url_schemes, which a widget's link opens the app with: Kando on a computer has kando://, and the phone's is another, which does not clash with it on a Mac that runs the iPad app.
const Scheme = "kando-mobile"

// Link is what a tap on a widget opens: the review, or the film kept under Film, as Continue Watching opens it.
type Link struct {
	Review bool
	Film   string
}

// URL is the link that opens l.
func (l Link) URL() string {
	if l.Review {
		return Scheme + "://review"
	}
	return Scheme + "://continue?" + url.Values{"film": {l.Film}}.Encode()
}

// ReadLink is what a link of the app's scheme opens, false for any other link. Anyone can write such a link, so it opens only what a widget does, and the app finds the film by its key among those it lists.
func ReadLink(link string) (Link, bool) {
	u, err := url.Parse(link)
	if err != nil || !strings.EqualFold(u.Scheme, Scheme) {
		return Link{}, false
	}
	switch u.Host {
	case "review":
		return Link{Review: true}, true
	case "continue":
		if key := u.Query().Get("film"); key != "" {
			return Link{Film: key}, true
		}
	}
	return Link{}, false
}
