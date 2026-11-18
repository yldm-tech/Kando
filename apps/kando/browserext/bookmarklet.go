package browserext

import (
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// The bookmarklet sends the video a page plays to Kando from any browser, without the extension: a bookmark whose address is a javascript: link, which opens a link of Kando's own scheme (nsgo.yaml's url_schemes) naming the page, its title and where its largest video is. The system hands that link to Kando (desktop.OnOpenLinks), which reads it with ReadLink and asks on the Add sheet before it adds anything, as any page could make such a link.

// Scheme is Kando's own URL scheme, which nsgo.yaml's url_schemes declares.
const Scheme = "kando"

// maxAt is the longest time ReadLink takes, longer than any video a site serves.
const maxAt = 100 * time.Hour

// Bookmarklet is the bookmarklet's address: it opens kando://add with the page's address (url), its title (title) and, when it plays a video, where the largest one is, in whole seconds (t).
func Bookmarklet() string {
	return `javascript:(()=>{let v=null,a=0;for(const e of document.querySelectorAll("video")){const r=e.getBoundingClientRect();if(r.width*r.height>a){v=e;a=r.width*r.height}}const t=v&&Number.isFinite(v.currentTime)?Math.floor(v.currentTime):0;location.href="` + Scheme + `://add?url="+encodeURIComponent(location.href)+"&title="+encodeURIComponent(document.title.slice(0,300))+(t>0?"&t="+t:"")})()`
}

// ReadLink reads a link the bookmarklet opened: the page's address, an http or https one, its title on one line (oneLine), and where its video was, 0 for nowhere or a time that is none; false for any other link.
func ReadLink(link string) (page, title string, at time.Duration, ok bool) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !strings.EqualFold(u.Scheme, Scheme) || !strings.EqualFold(strings.Trim(u.Host+u.Opaque+u.Path, "/"), "add") {
		return "", "", 0, false
	}
	query := u.Query()
	page = strings.TrimSpace(query.Get("url"))
	p, err := url.Parse(page)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
		return "", "", 0, false
	}
	if seconds, err := strconv.Atoi(query.Get("t")); err == nil && seconds > 0 && time.Duration(seconds)*time.Second <= maxAt {
		at = time.Duration(seconds) * time.Second
	}
	return page, oneLine(query.Get("title")), at, true
}

// maxTitle is how many characters of the page's title ReadLink keeps.
const maxTitle = 300

// oneLine is title on one line, without control characters, at most maxTitle characters long.
func oneLine(title string) string {
	title = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)), " ")
	if runes := []rune(title); len(runes) > maxTitle {
		title = strings.TrimSpace(string(runes[:maxTitle])) + "…"
	}
	return title
}
