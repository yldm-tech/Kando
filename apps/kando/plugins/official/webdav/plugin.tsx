// Kando's official WebDAV plugin: a kind of media source (docs/media-sources.md E17, 11.5) for a folder of a WebDAV server. It lists a folder with PROPFIND (Depth: 1) and finds a file or folder with PROPFIND (Depth: 0), reading the multistatus the server answers, and tells Kando to read a file from its own path, which Kando then GETs itself, with a Range, so a video's bytes never pass through this code. It reaches the server only through kando.source.fetch, which Kando sends to the address the user filled in; the password never reaches this code: Kando puts it in each request as HTTP Basic with the user (kando-plugin.json's auth).
import { host } from "nsgo"

// What Kando waits for a request; Kando's own limit is 120 s, so the plugin hears Kando's answer rather than its own timeout.
const WAIT = { timeout: 150000 }

// The most entries Kando takes of a folder.
const LIST_MAX = 5000

// What a PROPFIND asks of each entry.
const PROPS = '<?xml version="1.0" encoding="utf-8"?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/><D:getcontentlength/><D:getlastmodified/><D:getetag/></D:prop></D:propfind>'

// fail rejects with code, which Kando reads (auth, not-found, unavailable, timeout), and message.
function fail(code: string, message: string): never {
  const e: any = new Error(message)
  e.code = code
  throw e
}

// encodePath is p, a path relative to the source's address, with each segment percent-encoded, as kando.source.fetch takes a path.
function encodePath(p: string): string {
  return p.split("/").filter((s) => s !== "").map(encodeURIComponent).join("/")
}

// propfind sends a PROPFIND of depth for p, a folder when folder (its path then ends in a slash, as servers name a collection), and answers the multistatus's text, or null when nothing is there; a server that refuses the password fails as auth.
async function propfind(source: any, p: string, depth: string, folder: boolean): Promise<string | null> {
  const path = encodePath(p) + (folder && p !== "" ? "/" : "")
  let r: any
  try {
    r = await host.call("kando.source.fetch", {
      source: source.id, method: "PROPFIND", path,
      header: { "Depth": depth, "Content-Type": "application/xml; charset=utf-8" }, body: PROPS,
    }, WAIT)
  } catch (e: any) {
    const code = e && e.code
    fail(code === "timeout" ? "timeout" : "unavailable", "the server did not answer: " + String((e && e.message) || code || e))
  }
  if (r.status === 207) return String(r.body || "")
  if (r.status === 404 || r.status === 410) return null
  if (r.status === 401 || r.status === 403) fail("auth", "the server did not accept the user and password")
  if (r.status >= 300 && r.status < 400 && !folder) {
    // A folder named without its slash: some servers redirect to the name with it.
    return propfind(source, p, depth, true)
  }
  if (r.status === 405 || r.status === 501) fail("unavailable", "the address is not a WebDAV folder (" + r.status + ")")
  fail("unavailable", "the server answered " + r.status)
}

// ---- multistatus ----

// Response is what a multistatus says of one href: its path, decoded, and the properties of its propstat of status 200.
type Response = { href: string, dir: boolean, size: string, modified: string, etag: string }

const ENTITIES: Record<string, string> = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'" }

// unescapeXML is the text s of an XML document with its character and entity references replaced.
function unescapeXML(s: string): string {
  return s.replace(/&(#x[0-9a-fA-F]+|#[0-9]+|[a-z]+);/g, (whole, ref: string) => {
    if (ref[0] === "#") {
      const n = ref[1] === "x" ? parseInt(ref.slice(2), 16) : parseInt(ref.slice(1), 10)
      return n > 0 && n <= 0x10ffff ? String.fromCodePoint(n) : whole
    }
    return ENTITIES[ref] !== undefined ? ENTITIES[ref] : whole
  })
}

// parseMultistatus reads the responses of a multistatus (RFC 4918 14.16), whatever prefix the server gives the DAV: namespace, or none: each tag's local name is what counts. CDATA sections are read as text.
export function parseMultistatus(xml: string): Response[] {
  const out: Response[] = []
  const tag = /<!\[CDATA\[([\s\S]*?)\]\]>|<!--[\s\S]*?-->|<(\/?)(?:[A-Za-z_][\w.-]*:)?([A-Za-z_][\w.-]*)([^>]*)>/g
  let cur: Response | null = null
  // props are the properties of the propstat being read, kept once its status says 200.
  let props: Response | null = null
  let status = ""
  let text = ""
  let at = 0
  let m: RegExpExecArray | null
  while ((m = tag.exec(xml)) !== null) {
    text += unescapeXML(xml.slice(at, m.index))
    at = tag.lastIndex
    if (m[1] !== undefined) {
      text += m[1]
      continue
    }
    if (m[3] === undefined) continue
    const closing = m[2] === "/"
    const name = m[3].toLowerCase()
    const selfClosing = /\/\s*$/.test(m[4] || "")
    if (!closing) {
      text = ""
      if (name === "response") cur = { href: "", dir: false, size: "", modified: "", etag: "" }
      else if (name === "propstat" && cur) { props = { href: "", dir: false, size: "", modified: "", etag: "" }; status = "" }
      else if (name === "collection" && props) props.dir = true
      if (!selfClosing) continue
    }
    const value = text.trim()
    text = ""
    if (!cur) continue
    switch (name) {
      case "href":
        if (!props) cur.href = value
        break
      case "status":
        if (props) status = value
        break
      case "getcontentlength":
        if (props) props.size = value
        break
      case "getlastmodified":
        if (props) props.modified = value
        break
      case "getetag":
        if (props) props.etag = value
        break
      case "propstat":
        if (props && /^HTTP\/[\d.]+\s+200\b/.test(status)) {
          cur.dir = cur.dir || props.dir
          cur.size = cur.size || props.size
          cur.modified = cur.modified || props.modified
          cur.etag = cur.etag || props.etag
        }
        props = null
        break
      case "response":
        if (cur.href) out.push(cur)
        cur = null
        break
    }
  }
  return out
}

// hrefPath is the path an href names, an absolute URL's or a path's, percent-decoded where it can be, without its trailing slash.
function hrefPath(href: string): string {
  let p = href.replace(/^[A-Za-z][A-Za-z0-9+.-]*:\/\/[^/]*/, "")
  const q = p.search(/[?#]/)
  if (q >= 0) p = p.slice(0, q)
  try {
    p = decodeURIComponent(p)
  } catch (e) {
    // An href that is not valid percent-encoding is read as it is.
  }
  return p.replace(/\/+$/, "")
}

// lastSegment is the last segment of a path.
function lastSegment(p: string): string {
  return p.slice(p.lastIndexOf("/") + 1)
}

// rfc3339 is a getlastmodified date (RFC 1123) in RFC 3339, "" when it is not a date.
function rfc3339(s: string): string {
  if (!s) return ""
  const t = new Date(s)
  return isNaN(t.getTime()) ? "" : t.toISOString()
}

// entry is the entry Kando takes of response r, named name: a folder, or a file with its size, time and version, its ETag or else its size and time.
function entry(r: Response, name: string): any {
  const e: any = { name, dir: r.dir }
  const modified = rfc3339(r.modified)
  if (modified) e.modified = modified
  if (!r.dir) {
    const size = Number(r.size)
    if (r.size !== "" && Number.isFinite(size) && size >= 0 && Math.floor(size) === size) e.size = size
    e.version = r.etag || (r.size + "@" + modified)
  }
  return e
}

// basePath is the path of the source's address, decoded, without its trailing slash: "" for a server's root.
function basePath(source: any): string {
  const endpoint = String((source && source.config && source.config.endpoint) || "")
  return hrefPath(endpoint.replace(/^\s+|\s+$/g, ""))
}

// ---- the source's exports ----

// list lists the folder dir of the source: the entries of the server's multistatus but the folder's own, named by the last segment of their href. The folder's own is the one whose href is the folder's path; a server that names it otherwise, as behind a proxy that rewrites paths, has its shortest href left out, which every other one is under.
export async function list(source: any, dir: string): Promise<any[]> {
  const xml = await propfind(source, dir, "1", true)
  if (xml === null) fail("not-found", "no folder " + dir)
  const responses = parseMultistatus(xml)
  const self = (basePath(source) + "/" + dir).replace(/\/+/g, "/").replace(/\/$/, "")
  let own = responses.findIndex((r) => hrefPath(r.href) === self)
  if (own < 0 && responses.length > 0) {
    const paths = responses.map((r) => hrefPath(r.href))
    const shortest = paths.reduce((best, p, i) => p.length < paths[best].length ? i : best, 0)
    if (paths.every((p, i) => i === shortest || p.startsWith(paths[shortest] + "/"))) own = shortest
  }
  const out: any[] = []
  const seen: Record<string, boolean> = {}
  responses.forEach((r, i) => {
    if (i === own || out.length >= LIST_MAX) return
    const name = lastSegment(hrefPath(r.href))
    if (name === "" || name === "." || name === ".." || seen[name]) return
    seen[name] = true
    out.push(entry(r, name))
  })
  return out
}

// stat finds p in the source: its entry, or null when nothing is there. The source's root is a folder named by the last segment of its address.
export async function stat(source: any, p: string): Promise<any> {
  const xml = await propfind(source, p, "0", false)
  if (xml === null) return null
  const [r] = parseMultistatus(xml)
  if (!r) fail("unavailable", "the server answered no entry for " + p)
  const name = p === "" ? (lastSegment(basePath(source)) || "WebDAV") : lastSegment(p)
  return entry(r, name)
}

// open tells Kando to read p from its own path under the source's address, with the password Kando puts in.
export async function open(source: any, p: string): Promise<any> {
  return { path: encodePath(p) }
}
