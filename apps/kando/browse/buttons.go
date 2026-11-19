package browse

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The browser's own buttons in the pages of the sites it knows (videoSites): 下载视频 on a page of one video, in the site's row of buttons under it or over the corner of its player, and + over the thumbnail of each video a page lists, on hover and keyboard focus, which turns into a mark once the library has the video. The page asks the app what the library has of the videos it shows (AskMessage), the app answers with the buttons' words and their states (StateMessage) and tells it again as they change, and a click adds the video (AddMessage), as 下载视频 in the toolbar does.

// AskMessage starts what the page posts to ask what the library has of the videos it shows: a JSON array of their links follows it, at most MaxAsk of them.
const AskMessage = "kando-ask:"

// AddMessage starts what the page posts when the user clicks one of its buttons: a JSON object follows it, the video's link (url), its title as the page names it, "" when it does not (title), and for the page's own video, 下载视频's, where its player was, in whole seconds (at), 0 when it was not playing it.
const AddMessage = "kando-add:"

// StateMessage starts what the app posts to the page (core.Ref.PostMessage): a JSON PageState follows it.
const StateMessage = "kando-state:"

// MaxAsk is how many links one AskMessage may ask about; PageVideos is how many videos of a page the page and the app follow at most, the oldest dropped first.
const (
	MaxAsk     = 100
	PageVideos = 500
)

// maxTitle is how many characters of a title the page gives AddMessage are kept.
const maxTitle = 200

// PageText is the words of the page's buttons in the interface's language: 下载视频, 已在队列中 and 已在资料库中, as the toolbar's pill says them.
type PageText struct {
	Add    string `json:"add"`
	Queued string `json:"queued"`
	Done   string `json:"done"`
}

// PageState is what StateMessage carries: the buttons' words, and what the library has of the videos by their links as the page asked about them: "" nothing, HaveQueued or HaveDone.
type PageState struct {
	Text PageText          `json:"text"`
	Have map[string]string `json:"have"`
}

// StateData is s as the app posts it to the page.
func StateData(s PageState) string {
	data, err := json.Marshal(s)
	if err != nil {
		panic("browse: PageState: " + err.Error())
	}
	return StateMessage + string(data)
}

// ParseAsk reads an AskMessage: the links to one video it asks about (VideoLink), each once, at most MaxAsk; false when data is no AskMessage.
func ParseAsk(data string) ([]string, bool) { return ParseAskWith(data, VideoLink) }

// ParseAskWith is ParseAsk keeping the links video takes for one video's, as the app's pages ask about the videos of the site packs the user allowed too.
func ParseAskWith(data string, video func(string) bool) ([]string, bool) {
	rest, ok := strings.CutPrefix(data, AskMessage)
	if !ok {
		return nil, false
	}
	var links []string
	if json.Unmarshal([]byte(rest), &links) != nil {
		return nil, false
	}
	return AskLinksWith(links, video), true
}

// AskLinks are the links among the first MaxAsk of links that are to one video (VideoLink), each once, as an AskMessage or the browser extension asks about them.
func AskLinks(links []string) []string { return AskLinksWith(links, VideoLink) }

// AskLinksWith is AskLinks keeping the links video takes for one video's.
func AskLinksWith(links []string, video func(string) bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, u := range links[:min(len(links), MaxAsk)] {
		if !seen[u] && video(u) {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// ParseAdd reads an AddMessage: the link to one video it adds (VideoLink), and the title the page gave it, on one line, without control characters and at most maxTitle characters long; false when data is no AddMessage or names no video.
func ParseAdd(data string) (link, title string, ok bool) { return ParseAddWith(data, VideoLink) }

// ParseAddWith is ParseAdd taking the links video takes for one video's.
func ParseAddWith(data string, video func(string) bool) (link, title string, ok bool) {
	rest, ok := strings.CutPrefix(data, AddMessage)
	if !ok {
		return "", "", false
	}
	var add struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(rest), &add) != nil {
		return "", "", false
	}
	return AddVideoWith(add.URL, add.Title, video)
}

// AddedAt is where the player of the page that posted an AddMessage was in its video (PlayedTo), 0 when data says nowhere or is no AddMessage.
func AddedAt(data string) time.Duration {
	rest, ok := strings.CutPrefix(data, AddMessage)
	if !ok {
		return 0
	}
	var add struct {
		At float64 `json:"at"`
	}
	if json.Unmarshal([]byte(rest), &add) != nil {
		return 0
	}
	return PlayedTo(add.At)
}

// PlayedTo is a page's player's time, in seconds as an AddMessage or the browser extension says it, as a time in the video to open the film at, to the second: 0 for none, and for anything that is no time in a video, as a negative one or one longer than any video.
func PlayedTo(seconds float64) time.Duration {
	if math.IsNaN(seconds) || seconds < 1 || seconds > maxPlayedTo.Seconds() {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// maxPlayedTo is the longest time a page's player may say it was at, longer than any video a site serves.
const maxPlayedTo = 100 * time.Hour

// AddVideo reads what an AddMessage or the browser extension adds: the link to one video (VideoLink), and the title the page gave it, on one line, without control characters and at most maxTitle characters long; false when link names no video.
func AddVideo(link, title string) (string, string, bool) { return AddVideoWith(link, title, VideoLink) }

// AddVideoWith is AddVideo taking the links video takes for one video's.
func AddVideoWith(link, title string, video func(string) bool) (string, string, bool) {
	if !video(link) {
		return "", "", false
	}
	title = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)), " ")
	if utf8.RuneCountInString(title) > maxTitle {
		title = strings.TrimSpace(string([]rune(title)[:maxTitle])) + "…"
	}
	return link, title, true
}

// webViewBridge is how the buttons of the built-in browser's pages speak to the app: tell posts to the web view (window.nsgo.postMessage), and hear takes the app's StateMessage, which it posts to the page (core.Ref.PostMessage), keeping it from the page's own listeners. Without the web view's postMessage the page gets no buttons.
const webViewBridge = `  const host = window.nsgo, post = host && host.postMessage;
  if (typeof post !== "function") return;
  const tell = (data) => { try { post.call(host, data); } catch (e) {} };
  const hear = (fn) => addEventListener("message", (e) => {
    if (typeof e.data !== "string" || !e.data.startsWith(STATE)) return;
    e.stopImmediatePropagation();
    fn(e.data);
  }, true);
`

// extensionBridge is how the buttons of the browser extension's pages (ExtensionFiles' content.js) speak to Kando: tell sends to the extension's background (runtime.sendMessage), which relays to Kando through its native messaging host, and hear takes the StateMessage the background answers with. Kando does not tell the page as things change, as the app tells its own browser's pages (the app's browsePagePush), so the bridge asks again: every five seconds about the videos Kando has queued, until they are done, while the page shows, and about every video the page asked about once the page shows again, which also brings the buttons' words in Kando's language as it is now.
const extensionBridge = `  const api = globalThis.browser || globalThis.chrome;
  if (!api || !api.runtime || typeof api.runtime.sendMessage !== "function") return;
  let heard = null;
  const asked = new Set(), queued = new Set();
  const deliver = (answer) => {
    if (typeof answer !== "string" || !answer.startsWith(STATE) || !heard) return;
    try {
      const m = JSON.parse(answer.slice(STATE.length));
      for (const [url, state] of Object.entries((m && m.have) || {})) { if (state === "queued") queued.add(url); else queued.delete(url); }
    } catch (e) { return; }
    heard(answer);
  };
  const tell = (data) => {
    if (data.startsWith(ASK)) {
      try { for (const url of JSON.parse(data.slice(ASK.length))) { asked.delete(url); asked.add(url); } } catch (e) {}
      while (asked.size > KEEP) asked.delete(asked.values().next().value);
    }
    let sent;
    try { sent = api.runtime.sendMessage(data); } catch (e) { return; }
    if (sent && typeof sent.then === "function") sent.then(deliver, () => {});
  };
  const again = (urls) => { const all = [...urls]; while (all.length) tell(ASK + JSON.stringify(all.splice(0, MAX_ASK))); };
  setInterval(() => { if (queued.size && document.visibilityState === "visible") again(queued); }, 5000);
  document.addEventListener("visibilitychange", () => { if (document.visibilityState === "visible") again(asked); });
  const hear = (fn) => { heard = fn; };
`

// buttonsScript is the part of Script, and of the browser extension's content script (ExtensionScript), that draws the page's buttons; bridge is the JavaScript that declares how they speak to Kando, tell(data) and hear(fn) (webViewBridge, extensionBridge), and may return when the page cannot. It reads videoSites as VideoLink does (sitesScript), finds the videos of a page by its links (a[href]) that are videos' and hold a picture, rather than by the sites' own components, and looks again as the page changes, a site that is one page as much as any other: once things settle after a change of the page or its address. Its buttons never move what the page lays out: + and the marks of the videos the library has float over the top-left corner of the thumbnails, above whatever the site draws over them, and each thumbnail's link holds only a button that takes no room, for the keyboard, marked so another look does not add it again; 下载视频 goes into the site's row of buttons or floats over the player. They live in a closed shadow root, drawn with inline styles only, so the page's styles and its content security policy leave them be, and they keep to themselves only the events of their own clicks, keys and the app's StateMessage. A click adds only when the user made it (event.isTrusted); the app checks again what it adds (the app's browsePageAdd).
func buttonsScript(bridge string) string { return buttonsScriptOf(bridge, videoSitesJSON()) }

// buttonsScriptOf is buttonsScript knowing the sites of sitesJSON (sitesJSON).
func buttonsScriptOf(bridge, sitesJSON string) string {
	return `(() => {
  const ASK = "` + AskMessage + `", ADD = "` + AddMessage + `", STATE = "` + StateMessage + `";
  const MAX_URL = ` + strconv.Itoa(MaxVideoURL) + `, MAX_ASK = ` + strconv.Itoa(MaxAsk) + `, KEEP = ` + strconv.Itoa(PageVideos) + `;
  const MARK = "data-kando-add", PILL = "data-kando-video";
` + sitesScriptOf("document.baseURI", sitesJSON) + bridge + `

  // What the library has of the videos asked about, oldest first, and the words of the buttons, which the app sends with its first answer.
  let text = null;
  const have = new Map();
  let pending = [], flushing = 0;
  const ask = (url) => {
    if (have.has(url)) return;
    have.set(url, "");
    while (have.size > KEEP) have.delete(have.keys().next().value);
    pending.push(url);
    if (!flushing) flushing = setTimeout(() => {
      flushing = 0;
      while (pending.length) tell(ASK + JSON.stringify(pending.splice(0, MAX_ASK)));
    }, 150);
  };
  hear((data) => {
    let m;
    try { m = JSON.parse(data.slice(STATE.length)); } catch (err) { return; }
    if (!m || typeof m !== "object") return;
    if (m.text && typeof m.text === "object") text = m.text;
    for (const [url, state] of Object.entries(m.have || {})) if (have.has(url)) have.set(url, state === "queued" || state === "done" ? state : "");
    paint();
  });

  // The buttons, each a host element whose closed shadow root holds the button: 下载视频 (PILL), the + over the thumbnail under the pointer or the keyboard (PLUS), a mark over each thumbnail of a video the library has (BADGE), and, inside each thumbnail's link, a button the keyboard reaches it by, which shows nothing itself (MARK).
  const PLUS = "data-kando-plus", BADGE = "data-kando-mark";
  const SVG = "http://www.w3.org/2000/svg";
  const ICONS = {
    plus: ["M12 5v14", "M5 12h14"],
    download: ["M12 15V3", "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4", "m7 10 5 5 5-5"],
    queued: ["M22 12a10 10 0 1 1-20 0 10 10 0 1 1 20 0", "M12 6v6l4 2"],
    done: ["M20 6 9 17l-5-5"],
  };
  const DARK = "rgba(0, 0, 0, 0.72)", DARKER = "rgba(0, 0, 0, 0.88)";
  const put = (style, props) => { for (const [k, v] of Object.entries(props)) style.setProperty(k, v, "important"); };
  const hosts = new WeakMap();
  const make = (kind) => {
    const host = document.createElement("span");
    host.setAttribute(kind, "");
    const root = host.attachShadow({ mode: "closed" });
    const button = document.createElement(kind === BADGE ? "span" : "button");
    if (kind !== BADGE) button.type = "button";
    const icon = document.createElementNS(SVG, "svg");
    for (const [k, v] of Object.entries({ viewBox: "0 0 24 24", width: "16", height: "16", fill: "none", stroke: "currentColor", "stroke-width": "2.25", "stroke-linecap": "round", "stroke-linejoin": "round", "aria-hidden": "true" })) icon.setAttribute(k, v);
    button.append(icon);
    const entry = { kind, host, button, icon, label: null, url: "", link: null, state: null, text: null, glyph: "", floating: false, hover: false, badge: null };
    put(button.style, { all: "initial", "box-sizing": "border-box", display: "flex", "align-items": "center", "justify-content": "center", cursor: "pointer", color: "inherit", "-webkit-tap-highlight-color": "transparent" });
    const floating = { position: "fixed", "z-index": "2147483646", top: "0", left: "0", right: "auto", bottom: "auto", margin: "0", padding: "0", border: "0", float: "none", transform: "none", opacity: "1" };
    if (kind === PILL) {
      entry.label = document.createElement("span");
      button.append(entry.label);
      put(button.style, { gap: "6px", height: "32px", padding: "0 14px", "border-radius": "16px", "white-space": "nowrap", font: "500 14px/1 system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Hiragino Sans', 'Microsoft YaHei', sans-serif" });
      put(host.style, { display: "none", "flex": "none", "align-self": "center", padding: "0", border: "0", "pointer-events": "auto" });
    } else if (kind === MARK) {
      // It takes no room and draws nothing; with the keyboard on it, PLUS shows over the thumbnail.
      put(host.style, { display: "block", position: "absolute", width: "1px", height: "1px", margin: "0", padding: "0", border: "0", overflow: "hidden", opacity: "0", "pointer-events": "none", "clip-path": "inset(50%)" });
    } else {
      put(button.style, { width: "28px", height: "28px", "border-radius": "14px", padding: "0", background: DARK, color: "#fff" });
      put(host.style, { ...floating, display: "none", width: "28px", height: "28px", "pointer-events": kind === PLUS ? "auto" : "none" });
    }
    if (kind === PLUS || kind === PILL) {
      button.addEventListener("pointerenter", () => { entry.hover = true; fill(entry); });
      button.addEventListener("pointerleave", () => { entry.hover = false; fill(entry); });
    }
    if (kind === PILL) {
      button.addEventListener("focus", () => put(button.style, { outline: "2px solid #3ea6ff", "outline-offset": "2px" }));
      button.addEventListener("blur", () => put(button.style, { outline: "none" }));
    }
    root.append(button);
    hosts.set(host, entry);
    return entry;
  };
  // fill gives a button its background for where it lies and whether the pointer is on it.
  const fill = (e) => {
    if (e.kind !== PILL || e.floating) {
      put(e.button.style, { background: e.hover ? DARKER : DARK });
    } else {
      put(e.button.style, { background: e.hover ? "rgba(128, 128, 128, 0.28)" : "rgba(128, 128, 128, 0.16)" });
      e.button.style.setProperty("background", "color-mix(in srgb, currentColor " + (e.hover ? "18%" : "10%") + ", transparent)", "important");
    }
  };
  const glyph = (e, name) => {
    if (e.glyph === name) return;
    e.glyph = name;
    e.icon.replaceChildren(...(ICONS[name] || ICONS.plus).map((d) => { const p = document.createElementNS(SVG, "path"); p.setAttribute("d", d); return p; }));
  };
  // look gives a button the words and the glyph of what the library has of its video.
  const look = (e, url, plain) => {
    const state = have.get(url) || "";
    if (e.state === state && e.text === text) return;
    e.state = state;
    e.text = text;
    const words = String((state === "done" ? text.done : state === "queued" ? text.queued : text.add) || "");
    if (e.kind === BADGE) {
      e.host.setAttribute("data-kando-have", state);
    } else {
      e.button.setAttribute("aria-label", words);
      if (e.kind === PILL) e.label.textContent = words; else e.button.title = words;
    }
    glyph(e, state || plain);
  };

  // The + shows over the thumbnail of the link the pointer is on, or the keyboard, and stays while the pointer is on the + itself; it floats over the page, above whatever the site draws over its thumbnails. The link under the pointer is the first marked one of all the elements there, not only the topmost, as a site may lay its own layer over a thumbnail the pointer rests on, as YouTube's preview that plays it.
  const ours = (target) => target instanceof Element && hosts.has(target);
  let plus = null, target = null, pointed = null, focused = null;
  const markOf = (node) => {
    const a = node instanceof Element ? node.closest("a[href]") : null;
    const seen = a && links.get(a);
    return seen && seen.mark && seen.mark.host.parentNode === a ? seen.mark : null;
  };
  const aim = () => {
    const now = pointed || focused;
    if (now === target) return;
    target = now;
    if (target) {
      if (!plus) { plus = make(PLUS); document.documentElement.append(plus.host); }
      plus.url = target.url;
      plus.link = target.link;
    }
    layoutSoon();
  };
  let pointer = null, under = new Set();
  const hit = () => {
    if (!pointer) return;
    const all = document.elementsFromPoint(pointer.x, pointer.y);
    under = new Set(all);
    if (plus && all[0] === plus.host) return;
    let found = null;
    for (const el of all) {
      if (ours(el)) continue;
      if ((found = markOf(el))) break;
    }
    pointed = found;
  };
  let hitting = 0;
  addEventListener("pointermove", (ev) => {
    pointer = { x: ev.clientX, y: ev.clientY };
    if (!hitting) hitting = requestAnimationFrame(() => { hitting = 0; hit(); aim(); layoutSoon(); });
  }, { capture: true, passive: true });
  addEventListener("pointerout", (ev) => { if (!ev.relatedTarget) { pointer = null; under = new Set(); pointed = null; aim(); } }, { capture: true, passive: true });
  addEventListener("focusin", (ev) => { focused = markOf(ev.target); aim(); }, true);
  addEventListener("focusout", (ev) => { if (!ev.relatedTarget) { focused = null; aim(); } }, true);

  // corner is where a mark of the link a goes: the top-left corner of its thumbnail as it shows, the largest picture in it, inside the link; null while it shows none.
  const corner = (a) => {
    const box = a.getBoundingClientRect();
    let best = null, area = 0;
    for (const el of a.querySelectorAll("img, picture, video, [style*='background-image']")) {
      const r = el.getBoundingClientRect();
      if (r.width * r.height > area) { best = r; area = r.width * r.height; }
    }
    if (!best || best.width < 40 || best.height < 30) best = box;
    if (best.width < 40 || best.height < 30) return null;
    return { x: Math.round(Math.max(best.left, box.left) + 6), y: Math.round(Math.max(best.top, box.top) + 6) };
  };
  // shows reports whether the corner p of the link a is on the screen and not under something else of the page, as a bar over the thumbnail; what lies over the pointer too, as a preview over the whole thumbnail, does not count.
  const shows = (a, p) => {
    if (p.x < 0 || p.y < 0 || p.x + 28 > innerWidth || p.y + 28 > innerHeight) return false;
    for (const el of document.elementsFromPoint(p.x + 14, p.y + 14)) {
      if (ours(el)) continue;
      return a.contains(el) || under.has(el);
    }
    return false;
  };
  let laying = 0;
  const layoutSoon = () => { if (!laying) laying = requestAnimationFrame(layout); };
  const layout = () => {
    laying = 0;
    hit();
    aim();
    if (plus) {
      const at = text && target && target.host.isConnected ? corner(target.link) : null;
      if (at && (target === focused || shows(target.link, at))) {
        look(plus, target.url, "plus");
        put(plus.host.style, { display: "block", left: at.x + "px", top: at.y + "px" });
        put(plus.button.style, { outline: target === focused && document.activeElement === target.host ? "2px solid #3ea6ff" : "none", "outline-offset": "2px" });
      } else {
        put(plus.host.style, { display: "none" });
      }
    }
    let shown = 0;
    for (const e of marks) {
      if (!e.host.isConnected) { marks.delete(e); if (e.badge) e.badge.host.remove(); continue; }
      const state = text ? have.get(e.url) || "" : "";
      if (!state) { if (e.badge) put(e.badge.host.style, { display: "none" }); continue; }
      if (!e.badge) { e.badge = make(BADGE); document.documentElement.append(e.badge.host); }
      look(e.badge, e.url, "plus");
      const at = e !== target && shown < 60 ? corner(e.link) : null;
      if (at && shows(e.link, at)) {
        shown++;
        put(e.badge.host.style, { display: "block", left: at.x + "px", top: at.y + "px" });
      } else {
        put(e.badge.host.style, { display: "none" });
      }
    }
    place();
  };
  addEventListener("scroll", layoutSoon, { capture: true, passive: true });
  addEventListener("resize", layoutSoon, { passive: true });

  // A press on a button is the button's alone: the page under it neither follows the link nor hears it.
  for (const type of ["pointerdown", "mousedown", "pointerup", "mouseup", "click", "auxclick", "dblclick"]) {
    addEventListener(type, (ev) => {
      if (!ours(ev.target)) return;
      ev.stopImmediatePropagation();
      if (type.startsWith("pointer")) return;
      ev.preventDefault();
      if (type === "click" && ev.isTrusted && ev.button === 0) add(hosts.get(ev.target));
    }, true);
  }
  for (const type of ["keydown", "keyup", "keypress"]) {
    addEventListener(type, (ev) => { if (ours(ev.target) && (ev.key === "Enter" || ev.key === " ")) ev.stopImmediatePropagation(); }, true);
  }
  const clean = (s) => (s || "").replace(/\s+/g, " ").trim();
  const visible = (el) => el.getClientRects().length > 0;
  // titleOf names the video as the page does: a thumbnail's by a link to it with a title, or the words of one without a picture, and the page's own by its heading, or its document's title without the site's name.
  const titleOf = (e) => {
    if (e.kind === PILL) {
      const site = siteOf(new URL(e.url).hostname);
      for (const sel of site.titles) {
        for (const el of document.querySelectorAll(sel)) {
          const t = visible(el) && clean(el.textContent);
          if (t) return t;
        }
      }
      let t = clean(document.title).replace(/^\(\d+\+?\) /, "");
      for (const cut of site.cut) if (t.endsWith(cut)) t = t.slice(0, -cut.length).trim();
      return t;
    }
    let words = "";
    for (const a of document.querySelectorAll("a[href]")) {
      if (videoOf(a.getAttribute("href")) !== e.url) continue;
      const t = clean(a.getAttribute("title") || (a.parentElement && a.parentElement.getAttribute("title")));
      if (t) return t;
      if (!words && !a.querySelector("img, picture, video")) words = clean(a.textContent);
    }
    return words;
  };
  // playedTo is where the largest video of the page is, in whole seconds, for 下载视频, which adds the page's own video, so its film opens where the user was; 0 for a thumbnail's +, and when the page plays nothing.
  const playedTo = (e) => {
    if (e.kind !== PILL) return 0;
    let best = null, area = 0;
    for (const v of document.querySelectorAll("video")) {
      const r = v.getBoundingClientRect();
      if (r.width * r.height > area) { best = v; area = r.width * r.height; }
    }
    const t = best ? best.currentTime : 0;
    return Number.isFinite(t) && t >= 1 ? Math.floor(t) : 0;
  };
  const add = (e) => { if (e && e.url) tell(ADD + JSON.stringify({ url: e.url, title: titleOf(e).slice(0, 300), at: playedTo(e) })); };

  // paint gives every button its words and its glyph for what the library has of its video.
  const marks = new Set();
  let pill = null;
  const paint = () => {
    if (!text) return;
    for (const e of marks) if (e.host.isConnected) look(e, e.url, "plus");
    if (pill && pill.url) {
      look(pill, pill.url, "download");
      if (!pill.floating) put(pill.host.style, { display: "inline-flex" });
      fill(pill);
    }
    layoutSoon();
  };

  // place puts 下载视频 over the top corner of the largest video showing, when it is not in the site's row.
  const place = () => {
    if (!pill || !pill.floating || !pill.host.isConnected) return;
    let best = null, area = 0;
    for (const v of document.querySelectorAll("video")) {
      const r = v.getBoundingClientRect();
      if (r.width >= 200 && r.height >= 120 && r.bottom > 40 && r.top < innerHeight - 40 && r.width * r.height > area) { best = r; area = r.width * r.height; }
    }
    if (!best || !text) { put(pill.host.style, { display: "none" }); return; }
    put(pill.host.style, { display: "inline-flex", top: Math.round(Math.max(best.top, 0) + 12) + "px", left: Math.round(best.left + 12) + "px" });
  };

  // pagePill shows 下载视频 on a page of one video, in the first of the site's rows the page shows, or else floating over its player.
  const pagePill = () => {
    const url = videoOf(location.href);
    if (!url) {
      if (pill) { pill.host.remove(); pill.url = ""; }
      return;
    }
    if (!pill) pill = make(PILL);
    pill.url = url;
    ask(url);
    const site = siteOf(new URL(url).hostname);
    let row = null, end = false;
    for (const r of site.rows) {
      const el = document.querySelector(r.selector);
      if (el && visible(el)) { row = el; end = !!r.end; break; }
    }
    const h = pill.host;
    if (row) {
      const there = end ? row.lastElementChild === h : row.firstElementChild === h;
      if (!there) { if (end) row.append(h); else row.prepend(h); }
      pill.floating = false;
      put(h.style, { position: "static", margin: end ? "0 0 0 8px" : "0 8px 0 0", color: getComputedStyle(row).color, "z-index": "auto" });
    } else {
      if (h.parentNode !== document.documentElement) document.documentElement.append(h);
      pill.floating = true;
      put(h.style, { position: "fixed", margin: "0", color: "#fff", "z-index": "2147483646" });
    }
  };

  // scan finds the page's videos, gives each thumbnail's link its MARK and asks the app what the library has of them.
  const links = new WeakMap();
  const scan = () => {
    timer = 0;
    if (!document.body) return;
    for (const a of document.body.querySelectorAll("a[href]")) {
      const raw = a.getAttribute("href");
      const seen = links.get(a);
      if (seen && seen.raw === raw && (!seen.url || (seen.mark && seen.mark.host.parentNode === a))) continue;
      const url = seen && seen.raw === raw ? seen.url : videoOf(raw);
      let mark = seen && seen.mark;
      if (!url || !a.querySelector("img, picture, video, [style*='background-image']")) {
        if (mark) { mark.host.remove(); marks.delete(mark); if (mark.badge) mark.badge.host.remove(); }
        links.set(a, { raw, url, mark: null });
        continue;
      }
      if (!mark) mark = make(MARK);
      if (mark.host.parentNode !== a) a.insertBefore(mark.host, a.firstChild);
      mark.url = url;
      mark.link = a;
      if (target === mark && plus) plus.url = url;
      marks.add(mark);
      links.set(a, { raw, url, mark });
      ask(url);
    }
    pagePill();
    paint();
  };
  let timer = 0;
  const soon = () => { if (!timer) timer = setTimeout(scan, 200); };
  for (const name of ["pushState", "replaceState"]) {
    const original = history[name];
    history[name] = function (...args) {
      const result = original.apply(this, args);
      soon();
      return result;
    };
  }
  addEventListener("popstate", soon);
  const watch = () => {
    new MutationObserver(soon).observe(document.documentElement, { childList: true, subtree: true, attributes: true, attributeFilter: ["href"] });
    soon();
  };
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", watch); else watch();
})();`
}
