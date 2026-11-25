// Kando's official Jellyfin and Emby plugins (kando.jellyfin, kando.emby) run this same code, as the two servers share their API: a kind of media source (docs/media-sources.md E17, 11.5) that shows a media server's libraries as folders, a TV library's series, seasons and episodes as folders and films, a film library's films and any other video library's folders and videos as they are, and tells Kando to read a video's original file (/Videos/<id>/stream?static=true), never one the server transcodes; Kando then reads it itself, with a Range, so a video's bytes never pass through this code. It reaches the server only through kando.source.fetch, which Kando sends to the address the user filled in, and never sees the API key: Kando puts it in each request's header, as kando-plugin.json's auth says (Jellyfin's Authorization, Emby's X-Emby-Token). The two folders' plugin.tsx are the same file, which their tests check.
import { host } from "nsgo"

// What Kando waits for a request; Kando's own limit is 120 s, so the plugin hears Kando's answer rather than its own timeout.
const WAIT = { timeout: 150000 }

// How many items are named in one piece of JS (children), few enough that a piece of a folder of thousands stays well under Kando's half second, under the race detector on a busy runner too.
const NAMED_AT_ONCE = 200

// The most entries Kando takes of a folder, and how many the server is asked for at once, few enough that a page stays well under the 4 MiB Kando takes of a reply.
const LIST_MAX = 5000
const PAGE = 500

// The fields asked of an item besides the default ones: its file's path, for its extension, the time it was added and its version.
const FIELDS = "Path,DateCreated,Etag"

// The kinds of library that hold no video, or only what other libraries hold (collections, playlists), which are not shown.
const SKIPPED = ["music", "books", "audiobooks", "photos", "playlists", "livetv", "boxsets"]

// Node is what a path of the source names: the root, a library (lib, of its collection type), a series, a season (of its series), another folder, or a video.
type Node = { kind: "root" | "lib" | "series" | "season" | "folder" | "video", id: string, type?: string, series?: string, item?: any }

// nodes are what each path listed so far names, by the source's id and the path, so a stat or an open need not walk the path again; it is emptied once it holds too many.
let nodes: Record<string, Node> = {}
let held = 0
const NODES_MAX = 50000

// turn waits for a timer, so what follows runs as a piece of JS of its own, under a time budget of its own (Kando's half second a piece), rather than as the rest of the piece that was running.
// TODO(nsgo): a piece of JS cut at its budget while tsx settles a host.call's answer is lost: tsx/host.go drops the interrupt that settling the promise returns, so the call never ends and holds Kando's runner until its deadline, a minute, rather than failing at once; one cut in a timer's piece tsx does report. get takes a turn after each answer, so what follows it never runs as part of settling it; drop that once tsx reports the cut.
function turn(): Promise<void> {
  return new Promise((done) => setTimeout(done, 0))
}

// fail rejects with code, which Kando reads (auth, not-found, unavailable, timeout), and message.
function fail(code: string, message: string): never {
  const e: any = new Error(message)
  e.code = code
  throw e
}

// get GETs path with query under the source's address and answers its JSON, the API key put in by Kando; null for 404.
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
  if (r.status === 401 || r.status === 403) fail("auth", "the server did not accept the API key")
  if (r.status === 404) return null
  if (r.status !== 200) fail("unavailable", "the server answered " + r.status)
  try {
    return JSON.parse(r.body)
  } catch (e) {
    fail("unavailable", "the address is not a media server: its answer is not JSON")
  }
}

// items GETs path's items, page by page, LIST_MAX at most.
async function items(source: any, path: string, query: Record<string, string>): Promise<any[]> {
  const out: any[] = []
  for (let start = 0; out.length < LIST_MAX; start += PAGE) {
    const q: Record<string, string> = Object.assign({ Fields: FIELDS, EnableImages: "false", EnableUserData: "false", StartIndex: String(start), Limit: String(PAGE) }, query)
    const got = await get(source, path, q)
    if (got === null) fail("not-found", "the server has no " + path)
    const page: any[] = (got && Array.isArray(got.Items)) ? got.Items : []
    out.push(...page)
    const total = Number(got && got.TotalRecordCount)
    if (page.length < PAGE || (Number.isFinite(total) && start + page.length >= total)) break
  }
  return out.slice(0, LIST_MAX)
}

// clean is s as one segment of a path: no slash, backslash or control character, and neither empty nor . or ..; fallback when nothing is left.
function clean(s: any, fallback: string): string {
  const t = String(s == null ? "" : s).replace(/[\/\\\u0000-\u001f\u007f]+/g, " ").replace(/\s+/g, " ").trim()
  return t === "" || t === "." || t === ".." ? fallback : t
}

// extension is a video item's file extension, with its dot: its file's, else its container's, ".mkv" when it says neither.
function extension(item: any): string {
  const path = String(item.Path || "")
  const m = /\.([A-Za-z0-9]{1,5})$/.exec(path)
  if (m) return "." + m[1].toLowerCase()
  const containers = String(item.Container || "").toLowerCase().split(",").map((c) => c.trim()).filter((c) => c)
  if (containers.includes("mp4")) return ".mp4"
  return containers.length ? "." + containers[0] : ".mkv"
}

// pad is n with two digits at least.
function pad(n: number): string {
  return n < 10 ? "0" + n : String(n)
}

// videoName is the name of a video item: an episode's season and number before its title (S01E02 Title), a film's year after it (Title (2024)), and its extension.
function videoName(item: any): string {
  let base = clean(item.Name, String(item.Id))
  if (item.Type === "Episode" && typeof item.IndexNumber === "number") {
    let tag = "E" + pad(item.IndexNumber)
    if (typeof item.IndexNumberEnd === "number" && item.IndexNumberEnd > item.IndexNumber) tag += "-E" + pad(item.IndexNumberEnd)
    if (typeof item.ParentIndexNumber === "number") tag = "S" + pad(item.ParentIndexNumber) + tag
    base = tag + " " + base
  } else if (item.Type === "Movie" && typeof item.ProductionYear === "number" && !base.includes("(" + item.ProductionYear + ")")) {
    base += " (" + item.ProductionYear + ")"
  }
  return base + extension(item)
}

// isVideo reports whether an item is a video Kando can read, not a folder of them.
function isVideo(item: any): boolean {
  return !item.IsFolder && (item.MediaType === "Video" || ["Movie", "Episode", "Video", "MusicVideo"].includes(item.Type))
}

// nodeOf is what a child item names: a series or a season, a folder, or a video; null for what is neither, such as a song or a photo.
function nodeOf(item: any, parent: Node): Node | null {
  const id = String(item.Id || "")
  if (!id) return null
  if (item.Type === "Series") return { kind: "series", id }
  if (item.Type === "Season") return { kind: "season", id, series: String(item.SeriesId || parent.id) }
  if (item.IsFolder) return { kind: "folder", id }
  if (isVideo(item)) return { kind: "video", id, item }
  return null
}

// children lists what node holds, as [name, node, item] in the server's order, each name once: two of the same name are told apart by a number before the extension, in the order of their ids.
async function children(source: any, node: Node): Promise<Array<[string, Node, any]>> {
  let list: any[]
  switch (node.kind) {
    case "root": {
      const got = await get(source, "Library/MediaFolders", {})
      if (got === null) fail("unavailable", "the address is not a Jellyfin or Emby server")
      list = ((got && got.Items) || []).filter((l: any) => !SKIPPED.includes(String(l.CollectionType || "").toLowerCase()))
      break
    }
    case "lib":
      if (node.type === "movies") list = await items(source, "Items", { ParentId: node.id, Recursive: "true", IncludeItemTypes: "Movie", SortBy: "SortName,ProductionYear", SortOrder: "Ascending" })
      else if (node.type === "tvshows") list = await items(source, "Items", { ParentId: node.id, Recursive: "true", IncludeItemTypes: "Series", SortBy: "SortName", SortOrder: "Ascending" })
      else list = await items(source, "Items", { ParentId: node.id, SortBy: "IsFolder,SortName", SortOrder: "Ascending" })
      break
    case "series":
      list = await items(source, "Shows/" + encodeURIComponent(node.id) + "/Seasons", {})
      break
    case "season":
      list = await items(source, "Shows/" + encodeURIComponent(node.series || "") + "/Episodes", { SeasonId: node.id })
      break
    case "folder":
      list = await items(source, "Items", { ParentId: node.id, SortBy: "IsFolder,SortName", SortOrder: "Ascending" })
      break
    default:
      fail("not-found", "not a folder")
  }
  const named: Array<[string, Node, any]> = []
  for (let i = 0; i < list.length; i++) {
    if (i > 0 && i % NAMED_AT_ONCE === 0) await turn()
    const item = list[i]
    const child: Node | null = node.kind === "root" ? (item && item.Id ? { kind: "lib", id: String(item.Id), type: String(item.CollectionType || "").toLowerCase() } : null) : nodeOf(item, node)
    if (!child) continue
    const name = child.kind === "video" ? videoName(item) : clean(item.Name, child.id)
    named.push([name, child, item])
  }
  // Names met more than once are numbered in the order of their ids, so each keeps its number whatever else the folder holds.
  const count: Record<string, number> = {}
  for (const [name] of named) count[name] = (count[name] || 0) + 1
  const twins = named.filter(([name]) => count[name] > 1).sort((a, b) => a[1].id < b[1].id ? -1 : a[1].id > b[1].id ? 1 : 0)
  const nth: Record<string, number> = {}
  for (const t of twins) {
    const n = (nth[t[0]] = (nth[t[0]] || 0) + 1)
    if (n === 1) continue
    const dot = t[1].kind === "video" ? t[0].lastIndexOf(".") : -1
    t[0] = dot > 0 ? t[0].slice(0, dot) + " (" + n + ")" + t[0].slice(dot) : t[0] + " (" + n + ")"
  }
  return named
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
  const named = await children(source, node)
  for (const [name, child] of named) remember(source, dir === "" ? name : dir + "/" + name, child)
  return named
}

// modified is an item's time in RFC 3339: when it was added, which the servers say in RFC 3339 already.
function modified(item: any): string {
  const t = String((item && item.DateCreated) || "")
  return /^\d{4}-\d{2}-\d{2}T/.test(t) && !t.startsWith("0001-") ? t : ""
}

// entry is the entry Kando takes of an item named name: a folder, or a video with its time, its version (its ETag, else its time) and, when size is not null, its size.
function entry(name: string, node: Node, item: any, size: number | null): any {
  if (node.kind !== "video") return { name, dir: true }
  const e: any = { name, dir: false }
  const t = modified(item)
  if (t) e.modified = t
  e.version = String((item && item.Etag) || t || node.id)
  if (size !== null) e.size = size
  return e
}

// ---- the source's exports ----

// list lists the folder dir of the source. A video's size is not asked for each of them, which would take the server's description of every stream of every file; stat gives it.
export async function list(source: any, dir: string): Promise<any[]> {
  const node = await resolve(source, dir)
  if (!node || node.kind === "video") fail("not-found", "no folder " + dir)
  const named = await listed(source, segments(dir).join("/"), node)
  return named.map(([name, child, item]) => entry(name, child, item, null))
}

// stat finds p in the source: its entry, a video's with its original file's size, or null when nothing is there.
export async function stat(source: any, p: string): Promise<any> {
  const parts = segments(p)
  if (parts.length === 0) return { name: "Media", dir: true }
  const node = await resolve(source, p)
  if (!node) return null
  const name = parts[parts.length - 1]
  if (node.kind !== "video") return entry(name, node, null, null)
  const got = await get(source, "Items", { Ids: node.id, Fields: FIELDS + ",MediaSources", EnableImages: "false", EnableUserData: "false" })
  const item = got && Array.isArray(got.Items) && got.Items[0]
  if (!item) return null
  const sources = Array.isArray(item.MediaSources) ? item.MediaSources : []
  const size = Number(sources.length ? sources[0].Size : NaN)
  return entry(name, node, item, Number.isFinite(size) && size >= 0 && Math.floor(size) === size ? size : null)
}

// open tells Kando to read p's original file, as it is on the server: static=true, so the server sends the file and never transcodes it.
export async function open(source: any, p: string): Promise<any> {
  const node = await resolve(source, p)
  if (!node || node.kind !== "video") fail("not-found", "no video " + p)
  return { path: "Videos/" + encodeURIComponent(node.id) + "/stream", query: { static: "true" } }
}
