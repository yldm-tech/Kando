// Kando's official Plex plugin (kando.plex): a kind of media source (docs/media-sources.md E17, 11.5) that shows a Plex Media Server's film and TV libraries as folders, a film library's films as they are, a TV library's shows, seasons and episodes as folders and films, and tells Kando to read a video's original file (its part, /library/parts/<id>/<changestamp>/file.<ext>), never one the server transcodes; Kando then reads it itself, with a Range, so a video's bytes never pass through this code. It reaches the server only through kando.source.fetch, which Kando sends to the address the user filled in, and never sees the token: Kando puts it in each request's X-Plex-Token header, as kando-plugin.json's auth says.
import { host } from "nsgo"

// What Kando waits for a request; Kando's own limit is 120 s, so the plugin hears Kando's answer rather than its own timeout.
const WAIT = { timeout: 150000 }

// How many items are named in one piece of JS (children), few enough that a piece of a folder of thousands stays well under Kando's half second, under the race detector on a busy runner too.
const NAMED_AT_ONCE = 200

// The most entries Kando takes of a folder, and how many the server is asked for at once, few enough that a page, which carries each video's media and parts, stays well under the 4 MiB Kando takes of a reply.
const LIST_MAX = 5000
const PAGE = 300

// The kinds of library shown: films (Plex's "movie", which other-video libraries are too) and TV shows; music and photos hold no video.
const SHOWN = ["movie", "show"]

// The kinds of item that are a video Kando can read.
const VIDEOS = ["movie", "episode", "clip", "video"]

// Node is what a path of the source names: the root, a library (lib, by its key and type), a show or a season (meta, whose children Plex lists), or a video, one part of an item's first media.
type Node = { kind: "root" | "lib" | "meta" | "video", id: string, type?: string, item?: any, part?: any }

// nodes are what each path listed so far names, by the source's id and the path, so a stat or an open need not walk the path again; it is emptied once it holds too many.
let nodes: Record<string, Node> = {}
let held = 0
const NODES_MAX = 50000

// turn waits for a timer, so what follows runs as a piece of JS of its own, under a time budget of its own (Kando's half second a piece), rather than as the rest of the piece that was running.
// TODO(nsgo#2390): a piece of JS cut at its budget while tsx settles a host.call's answer is lost: tsx/host.go drops the interrupt that settling the promise returns, so the call never ends and holds Kando's runner until its deadline, a minute, rather than failing at once; one cut in a timer's piece tsx does report. get takes a turn after each answer, so what follows it never runs as part of settling it; drop that once tsx reports the cut. https://github.com/nsgo-dev/nsgo/issues/2390
function turn(): Promise<void> {
  return new Promise((done) => setTimeout(done, 0))
}

// fail rejects with code, which Kando reads (auth, not-found, unavailable, timeout), and message.
function fail(code: string, message: string): never {
  const e: any = new Error(message)
  e.code = code
  throw e
}

// get GETs path with query under the source's address and answers its MediaContainer, as JSON, the token put in by Kando; null for 404.
async function get(source: any, path: string, query: Record<string, string>): Promise<any> {
  let r: any
  let refused: { e: any } | null = null
  try {
    r = await host.call("kando.source.fetch", { source: source.id, method: "GET", path, query, header: { "Accept": "application/json" } }, WAIT)
  } catch (e: any) {
    refused = { e }
  }
  await turn()
  if (refused) {
    const e = refused.e
    const code = e && e.code
    fail(code === "timeout" ? "timeout" : "unavailable", "the server did not answer: " + String((e && e.message) || code || e))
  }
  if (r.status === 401 || r.status === 403) fail("auth", "the server did not accept the token")
  if (r.status === 404) return null
  if (r.status !== 200) fail("unavailable", "the server answered " + r.status)
  let got: any
  try {
    got = JSON.parse(r.body)
  } catch (e) {
    fail("unavailable", "the address is not a Plex server: its answer is not JSON")
  }
  if (!got || typeof got.MediaContainer !== "object" || got.MediaContainer === null) fail("unavailable", "the address is not a Plex server")
  return got.MediaContainer
}

// items GETs path's Metadata, page by page, LIST_MAX at most.
async function items(source: any, path: string, query: Record<string, string>): Promise<any[]> {
  const out: any[] = []
  for (let start = 0; out.length < LIST_MAX; start += PAGE) {
    const q: Record<string, string> = Object.assign({ "X-Plex-Container-Start": String(start), "X-Plex-Container-Size": String(PAGE) }, query)
    const got = await get(source, path, q)
    if (got === null) fail("not-found", "the server has no " + path)
    const page: any[] = Array.isArray(got.Metadata) ? got.Metadata : []
    out.push(...page)
    const total = Number(got.totalSize)
    if (page.length < PAGE || (Number.isFinite(total) && start + page.length >= total)) break
  }
  return out.slice(0, LIST_MAX)
}

// clean is s as one segment of a path: no slash, backslash or control character, and neither empty nor . or ..; fallback when nothing is left.
function clean(s: any, fallback: string): string {
  const t = String(s == null ? "" : s).replace(/[\/\\\u0000-\u001f\u007f]+/g, " ").replace(/\s+/g, " ").trim()
  return t === "" || t === "." || t === ".." ? fallback : t
}

// extension is a part's file extension, with its dot: its file's, else its container's or its media's, ".mkv" when none says.
function extension(part: any, media: any): string {
  const m = /\.([A-Za-z0-9]{1,5})$/.exec(String(part.file || ""))
  if (m) return "." + m[1].toLowerCase()
  const container = String(part.container || media.container || "").toLowerCase().split(",")[0].trim()
  return /^[a-z0-9]{1,5}$/.test(container) ? "." + container : ".mkv"
}

// pad is n with two digits at least.
function pad(n: number): string {
  return n < 10 ? "0" + n : String(n)
}

// baseName is the name of a video item without its extension: an episode's season and number before its title (S01E02 Title), a film's year after it (Title (2024)).
function baseName(item: any): string {
  let base = clean(item.title, String(item.ratingKey))
  if (item.type === "episode" && typeof item.index === "number") {
    let tag = "E" + pad(item.index)
    if (typeof item.parentIndex === "number") tag = "S" + pad(item.parentIndex) + tag
    base = tag + " " + base
  } else if (item.type === "movie" && typeof item.year === "number" && !base.includes("(" + item.year + ")")) {
    base += " (" + item.year + ")"
  }
  return base
}

// readable reports whether a part is a file Kando can read from the server: its key a path under /library/parts/.
function readable(part: any): boolean {
  const key = String((part && part.key) || "")
  return key.startsWith("/library/parts/") && !/[?#\\]/.test(key)
}

// videos are what a video item names: each part of its first media (a film split over two files has two), "- pt1", "- pt2" after the name when there are more than one.
function videos(item: any): Array<[string, Node, any]> {
  const media = Array.isArray(item.Media) && item.Media.length ? item.Media[0] : null
  const parts: any[] = media && Array.isArray(media.Part) ? media.Part.filter(readable) : []
  const base = baseName(item)
  return parts.map((part, i) => {
    const name = (parts.length > 1 ? base + " - pt" + (i + 1) : base) + extension(part, media)
    return [name, { kind: "video", id: String(item.ratingKey) + "#" + i, item, part }, item]
  })
}

// named is what a child item names: a show or a season (whose children Plex lists, so a show that hides its one season lists its episodes), or its videos; nothing for what is neither, such as a song or a photo.
function named(item: any): Array<[string, Node, any]> {
  const id = String((item && item.ratingKey) || "")
  if (!id) return []
  if (item.type === "show" || item.type === "season") return [[clean(item.title, id), { kind: "meta", id }, item]]
  if (VIDEOS.includes(item.type)) return videos(item)
  return []
}

// children lists what node holds, as [name, node, item] in the server's order, each name once: two of the same name are told apart by a number before the extension, in the order of their ids.
async function children(source: any, node: Node): Promise<Array<[string, Node, any]>> {
  const out: Array<[string, Node, any]> = []
  if (node.kind === "root") {
    const got = await get(source, "library/sections", {})
    if (got === null) fail("unavailable", "the address is not a Plex server")
    for (const d of (Array.isArray(got.Directory) ? got.Directory : [])) {
      if (d && d.key != null && SHOWN.includes(String(d.type))) out.push([clean(d.title, String(d.key)), { kind: "lib", id: String(d.key), type: String(d.type) }, d])
    }
  } else {
    let list: any[]
    if (node.kind === "lib") list = await items(source, "library/sections/" + encodeURIComponent(node.id) + "/all", { type: node.type === "show" ? "2" : "1", sort: "titleSort" })
    else if (node.kind === "meta") list = await items(source, "library/metadata/" + encodeURIComponent(node.id) + "/children", {})
    else fail("not-found", "not a folder")
    for (let i = 0; i < list.length; i++) {
      if (i > 0 && i % NAMED_AT_ONCE === 0) await turn()
      out.push(...named(list[i]))
    }
  }
  // Names met more than once are numbered in the order of their ids, so each keeps its number whatever else the folder holds.
  const count: Record<string, number> = {}
  for (const [name] of out) count[name] = (count[name] || 0) + 1
  const twins = out.filter(([name]) => count[name] > 1).sort((a, b) => a[1].id < b[1].id ? -1 : a[1].id > b[1].id ? 1 : 0)
  const nth: Record<string, number> = {}
  for (const t of twins) {
    const n = (nth[t[0]] = (nth[t[0]] || 0) + 1)
    if (n === 1) continue
    const dot = t[1].kind === "video" ? t[0].lastIndexOf(".") : -1
    t[0] = dot > 0 ? t[0].slice(0, dot) + " (" + n + ")" + t[0].slice(dot) : t[0] + " (" + n + ")"
  }
  return out
}

// key is the key of p of the source in nodes.
function key(source: any, p: string): string {
  return source.id + "\n" + p
}

// remember keeps what the path p names.
function remember(source: any, p: string, node: Node) {
  if (held >= NODES_MAX) {
    nodes = {}
    held = 0
  }
  const k = key(source, p)
  if (!nodes[k]) held++
  nodes[k] = node
}

// segments are the segments of p.
function segments(p: string): string[] {
  return p.split("/").filter((s) => s !== "")
}

// resolve is what p names, from what was listed, else by listing each folder on the way down; null when nothing there.
async function resolve(source: any, p: string): Promise<Node | null> {
  const parts = segments(p)
  if (parts.length === 0) return { kind: "root", id: "" }
  const kept = nodes[key(source, parts.join("/"))]
  if (kept) return kept
  let node: Node = { kind: "root", id: "" }
  for (let i = 0; i < parts.length; i++) {
    if (node.kind === "video") return null
    const dir = parts.slice(0, i).join("/")
    const found = (await listed(source, dir, node)).find(([name]) => name === parts[i])
    if (!found) return null
    node = found[1]
  }
  return node
}

// listed lists node, at dir, remembering what each child's path names.
async function listed(source: any, dir: string, node: Node): Promise<Array<[string, Node, any]>> {
  const out = await children(source, node)
  for (const [name, child] of out) remember(source, dir === "" ? name : dir + "/" + name, child)
  return out
}

// modified is an item's time in RFC 3339: when it was added to the library, which Plex says in seconds since 1970.
function modified(item: any): string {
  const s = Number(item && item.addedAt)
  return Number.isFinite(s) && s > 0 ? new Date(s * 1000).toISOString() : ""
}

// entry is the entry Kando takes of what node names, named name: a folder, or a video with its size, its time and its version, its part's key, which holds the file's changestamp, and its size.
function entry(name: string, node: Node): any {
  if (node.kind !== "video") return { name, dir: true }
  const e: any = { name, dir: false }
  const t = modified(node.item)
  if (t) e.modified = t
  const size = Number(node.part.size)
  if (Number.isFinite(size) && size >= 0 && Math.floor(size) === size) e.size = size
  e.version = String(node.part.key) + ":" + (e.size === undefined ? "" : String(e.size))
  return e
}

// ---- the source's exports ----

// list lists the folder dir of the source, each video with its size, which Plex lists with it.
export async function list(source: any, dir: string): Promise<any[]> {
  const node = await resolve(source, dir)
  if (!node || node.kind === "video") fail("not-found", "no folder " + dir)
  const out = await listed(source, segments(dir).join("/"), node)
  return out.map(([name, child]) => entry(name, child))
}

// stat finds p in the source: its entry, or null when nothing is there.
export async function stat(source: any, p: string): Promise<any> {
  const parts = segments(p)
  if (parts.length === 0) return { name: "Media", dir: true }
  const node = await resolve(source, p)
  if (!node) return null
  return entry(parts[parts.length - 1], node)
}

// open tells Kando to read p's original file, its part as it is on the server, which Plex sends without transcoding it.
export async function open(source: any, p: string): Promise<any> {
  const node = await resolve(source, p)
  if (!node || node.kind !== "video") fail("not-found", "no video " + p)
  return { path: String(node.part.key).slice(1) }
}
