package browse

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The browser's own buttons in the pages of the sites it knows (videoSites): 下载视频 on a page of one video, in the site's row of buttons under it or over the corner of its player, and + over the thumbnail of each video a page lists, on hover and keyboard focus, which turns into a mark once the library has the video. The page asks the app what the library has of the videos it shows (AskMessage), the app answers with the buttons' words and their states (StateMessage) and tells it again as they change, and a click adds the video (AddMessage), as 下载视频 in the toolbar does.

// AskMessage starts what the page posts to ask what the library has of the videos it shows: a JSON array of their links follows it, at most MaxAsk of them.
const AskMessage = "kando-ask:"

// AddMessage starts what the page posts when the user clicks one of its buttons: a JSON object follows it, the video's link (url) and its title as the page names it, "" when it does not (title).
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
func ParseAsk(data string) ([]string, bool) {
	rest, ok := strings.CutPrefix(data, AskMessage)
	if !ok {
		return nil, false
	}
	var links []string
	if json.Unmarshal([]byte(rest), &links) != nil {
		return nil, false
	}
	var out []string
	seen := map[string]bool{}
	for _, u := range links[:min(len(links), MaxAsk)] {
		if !seen[u] && VideoLink(u) {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out, true
}

// ParseAdd reads an AddMessage: the link to one video it adds (VideoLink), and the title the page gave it, on one line, without control characters and at most maxTitle characters long; false when data is no AddMessage or names no video.
func ParseAdd(data string) (link, title string, ok bool) {
	rest, ok := strings.CutPrefix(data, AddMessage)
	if !ok {
		return "", "", false
	}
	var add struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(rest), &add) != nil || !VideoLink(add.URL) {
		return "", "", false
	}
	title = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, add.Title)), " ")
	if utf8.RuneCountInString(title) > maxTitle {
		title = strings.TrimSpace(string([]rune(title)[:maxTitle])) + "…"
	}
	return add.URL, title, true
}

// buttonsScript is the part of Script that draws the page's buttons. It reads videoSites as VideoLink does, finds the videos of a page by its links (a[href]) that are videos' and hold a picture, rather than by the sites' own components, and looks again as the page changes, a site that is one page as much as any other: once things settle after a change of the page or its address. Its buttons never move what the page lays out: + lies over the thumbnail's corner without taking room, marked so another look does not add it again, and 下载视频 goes into the site's row of buttons or floats over the player. They live in a closed shadow root, drawn with inline styles only, so the page's styles and its content security policy leave them be, and they keep to themselves only the events of their own clicks, keys and the app's StateMessage. A click adds only when the user made it (event.isTrusted); the app checks again what it adds (the app's browsePageAdd).
func buttonsScript() string {
	return `(() => {
  const ASK = "` + AskMessage + `", ADD = "` + AddMessage + `", STATE = "` + StateMessage + `";
  const MAX_URL = ` + strconv.Itoa(MaxVideoURL) + `, MAX_ASK = ` + strconv.Itoa(MaxAsk) + `, KEEP = ` + strconv.Itoa(PageVideos) + `;
  const MARK = "data-kando-add", PILL = "data-kando-video";
  const bridge = window.nsgo, send = bridge && bridge.postMessage;
  if (typeof send !== "function") return;
  const tell = (data) => { try { send.call(bridge, data); } catch (e) {} };
  const sites = (` + videoSitesJSON() + `).map((s) => ({
    domain: s.domain, not: s.not || [], rows: s.rows || [],
    videos: s.videos.map((v) => ({ path: new RegExp(v.path), param: v.param || "", value: v.param ? new RegExp(v.value) : null })),
  }));
  const siteOf = (hostname) => {
    const host = hostname.toLowerCase().replace(/\.$/, "");
    const bare = host.replace(/^www\./, "").replace(/^m\./, "");
    for (const s of sites) {
      if (host !== s.domain && !host.endsWith("." + s.domain)) continue;
      return s.not.includes(bare) ? null : s;
    }
    return null;
  };
  // videoOf is the link raw resolves to when it is one video's, as VideoLink decides it, and "" otherwise.
  const videoOf = (raw) => {
    if (!raw || raw.length > MAX_URL) return "";
    let u;
    try { u = new URL(raw, document.baseURI); } catch (e) { return ""; }
    if ((u.protocol !== "http:" && u.protocol !== "https:") || u.href.length > MAX_URL) return "";
    const s = siteOf(u.hostname);
    if (!s) return "";
    for (const v of s.videos) {
      if (v.path.test(u.pathname) && (!v.param || v.value.test(u.searchParams.get(v.param) || ""))) return u.href;
    }
    return "";
  };

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
  addEventListener("message", (e) => {
    if (typeof e.data !== "string" || !e.data.startsWith(STATE)) return;
    e.stopImmediatePropagation();
    let m;
    try { m = JSON.parse(e.data.slice(STATE.length)); } catch (err) { return; }
    if (!m || typeof m !== "object") return;
    if (m.text && typeof m.text === "object") text = m.text;
    for (const [url, state] of Object.entries(m.have || {})) if (have.has(url)) have.set(url, state === "queued" || state === "done" ? state : "");
    paint();
  }, true);

  // The buttons, each a host element whose closed shadow root holds the button.
  const SVG = "http://www.w3.org/2000/svg";
  const ICONS = {
    plus: ["M12 5v14", "M5 12h14"],
    download: ["M12 15V3", "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4", "m7 10 5 5 5-5"],
    queued: ["M22 12a10 10 0 1 1-20 0 10 10 0 1 1 20 0", "M12 6v6l4 2"],
    done: ["M20 6 9 17l-5-5"],
  };
  const put = (style, props) => { for (const [k, v] of Object.entries(props)) style.setProperty(k, v, "important"); };
  const hosts = new WeakMap();
  const make = (kind) => {
    const host = document.createElement("span");
    host.setAttribute(kind, "");
    const root = host.attachShadow({ mode: "closed" });
    const button = document.createElement("button");
    button.type = "button";
    const icon = document.createElementNS(SVG, "svg");
    for (const [k, v] of Object.entries({ viewBox: "0 0 24 24", width: "16", height: "16", fill: "none", stroke: "currentColor", "stroke-width": "2.25", "stroke-linecap": "round", "stroke-linejoin": "round", "aria-hidden": "true" })) icon.setAttribute(k, v);
    button.append(icon);
    const entry = { kind, host, button, icon, label: null, url: "", link: null, state: null, text: null, glyph: "", floating: false, hover: false };
    put(button.style, { all: "initial", "box-sizing": "border-box", display: "flex", "align-items": "center", "justify-content": "center", cursor: "pointer", color: "inherit", "-webkit-tap-highlight-color": "transparent" });
    if (kind === PILL) {
      entry.label = document.createElement("span");
      button.append(entry.label);
      put(button.style, { gap: "6px", height: "32px", padding: "0 14px", "border-radius": "16px", "white-space": "nowrap", font: "500 14px/1 system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Hiragino Sans', 'Microsoft YaHei', sans-serif" });
      put(host.style, { display: "none", "flex": "none", "align-self": "center", padding: "0", border: "0", "pointer-events": "auto" });
    } else {
      put(button.style, { width: "28px", height: "28px", "border-radius": "14px", padding: "0", background: "rgba(0, 0, 0, 0.72)", color: "#fff" });
      put(host.style, { display: "none", position: "absolute", "z-index": "2147483646", top: "auto", left: "auto", right: "auto", bottom: "auto", margin: "6px", padding: "0", border: "0", width: "28px", height: "28px", float: "none", transform: "none", opacity: "0", transition: "opacity 0.15s", "pointer-events": "auto" });
    }
    button.addEventListener("pointerenter", () => { entry.hover = true; fill(entry); });
    button.addEventListener("pointerleave", () => { entry.hover = false; fill(entry); });
    button.addEventListener("focus", () => put(button.style, { outline: "2px solid #3ea6ff", "outline-offset": "2px" }));
    button.addEventListener("blur", () => put(button.style, { outline: "none" }));
    root.append(button);
    hosts.set(host, entry);
    return entry;
  };
  // fill gives a button its background for where it lies and whether the pointer is on it.
  const fill = (e) => {
    if (e.kind !== PILL) {
      put(e.button.style, { background: e.hover ? "rgba(0, 0, 0, 0.88)" : "rgba(0, 0, 0, 0.72)" });
    } else if (e.floating) {
      put(e.button.style, { background: e.hover ? "rgba(0, 0, 0, 0.88)" : "rgba(0, 0, 0, 0.72)" });
    } else {
      put(e.button.style, { background: e.hover ? "rgba(128, 128, 128, 0.28)" : "rgba(128, 128, 128, 0.16)" });
      e.button.style.setProperty("background", "color-mix(in srgb, currentColor " + (e.hover ? "18%" : "10%") + ", transparent)", "important");
    }
  };
  const glyph = (e, name) => {
    if (e.glyph === name) return;
    e.glyph = name;
    e.icon.replaceChildren(...ICONS[name].map((d) => { const p = document.createElementNS(SVG, "path"); p.setAttribute("d", d); return p; }));
  };

  // Where the pointer and the keyboard focus are, which show a thumbnail's +.
  let pointed = null, focused = null;
  const thumbOf = (node) => {
    const a = node instanceof Element ? node.closest("a[href]") : null;
    const seen = a && links.get(a);
    return seen && seen.mark && seen.mark.host.parentNode === a ? seen.mark : null;
  };
  const look = (e) => {
    if (!e || e.kind === PILL) return;
    const shown = text && (e.state || e === pointed || e === focused);
    put(e.host.style, { opacity: shown ? "1" : "0" });
  };
  const point = (e) => { if (pointed === e) return; const was = pointed; pointed = e; look(was); look(e); };
  addEventListener("pointerover", (ev) => point(thumbOf(ev.target)), { capture: true, passive: true });
  addEventListener("pointerout", (ev) => { if (!ev.relatedTarget) point(null); }, { capture: true, passive: true });
  addEventListener("focusin", (ev) => { const was = focused; focused = thumbOf(ev.target); look(was); look(focused); }, true);
  addEventListener("focusout", (ev) => { if (!ev.relatedTarget) { const was = focused; focused = null; look(was); } }, true);

  // A press on a button is the button's alone: the page under it neither follows the link nor hears it.
  const ours = (target) => target instanceof Element && hosts.has(target);
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
  // titleOf names a thumbnail's video as the page does, by a link to it with a title, or the words of one without a picture; the page's own video the app names itself.
  const titleOf = (e) => {
    if (e.kind === PILL) return "";
    let words = "";
    for (const a of document.querySelectorAll("a[href]")) {
      if (videoOf(a.getAttribute("href")) !== e.url) continue;
      const t = clean(a.getAttribute("title") || (a.parentElement && a.parentElement.getAttribute("title")));
      if (t) return t;
      if (!words && !a.querySelector("img, picture, video")) words = clean(a.textContent);
    }
    return words;
  };
  const add = (e) => { if (e && e.url) tell(ADD + JSON.stringify({ url: e.url, title: titleOf(e).slice(0, 300) })); };

  // paint gives every button its words, its glyph and its look for what the library has of its video.
  const marks = new Set();
  let pill = null;
  const paintOne = (e) => {
    const state = have.get(e.url) || "";
    if (!text) { put(e.host.style, { display: "none" }); return; }
    if (e.state !== state || e.text !== text) {
      e.state = state;
      e.text = text;
      const words = String((state === "done" ? text.done : state === "queued" ? text.queued : text.add) || "");
      e.button.setAttribute("aria-label", words);
      if (e.kind === PILL) {
        e.label.textContent = words;
      } else {
        e.button.title = words;
        if (state) e.host.setAttribute("data-kando-have", state); else e.host.removeAttribute("data-kando-have");
      }
      glyph(e, state || (e.kind === PILL ? "download" : "plus"));
    }
    if (e.kind === PILL) {
      if (!e.floating) put(e.host.style, { display: "inline-flex" });
      fill(e);
      place();
    } else {
      put(e.host.style, { display: "block" });
      look(e);
    }
  };
  const paint = () => {
    for (const e of marks) {
      if (!e.host.isConnected) { marks.delete(e); continue; }
      paintOne(e);
    }
    if (pill && pill.url) paintOne(pill);
  };

  // place puts 下载视频 over the top corner of the largest video showing, when it is not in the site's row.
  let placing = 0;
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
  const placeSoon = () => { if (!placing) placing = requestAnimationFrame(() => { placing = 0; place(); }); };
  addEventListener("scroll", placeSoon, { capture: true, passive: true });
  addEventListener("resize", placeSoon, { passive: true });

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
      if (el && el.getClientRects().length) { row = el; end = !!r.end; break; }
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

  // scan finds the page's videos, gives each thumbnail its + and asks the app what the library has of them.
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
        if (mark) { mark.host.remove(); marks.delete(mark); }
        links.set(a, { raw, url, mark: null });
        continue;
      }
      if (!mark) mark = make(MARK);
      if (mark.host.parentNode !== a) a.insertBefore(mark.host, a.firstChild);
      mark.url = url;
      mark.link = a;
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
