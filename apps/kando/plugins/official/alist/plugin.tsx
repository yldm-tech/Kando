// Kando's official Alist plugin: a kind of media source (docs/media-sources.md E17, 11.5) for an Alist server (and OpenList, which keeps its API), and so for the cloud drives it mounts. It logs in with POST /api/auth/login, whose password Kando writes into the request's body (kando-plugin.json's auth: login), so the password never reaches this code; only the session token it answers does, which goes back to the same server alone, as kando.source.fetch sends nowhere else. With no user it browses as Alist's guest. It lists a folder with POST /api/fs/list and finds a file with POST /api/fs/get, the only paths it may POST to, and tells Kando to read a file from the raw_url fs/get answers when that is on the server itself, else from the server's /d/ link with the file's signature, which Alist redirects to the drive; Kando then reads it itself, so a video's bytes never pass through this code.
import { host } from "nsgo"

// What Kando waits for a request; Kando's own limit is 120 s, so the plugin hears Kando's answer rather than its own timeout.
const WAIT = { timeout: 150000 }

// The most entries Kando takes of a folder, and the most Alist answers a page.
const LIST_MAX = 5000
const PAGE = 500

// tokens are the session tokens Alist answered each source's login with, by the id Kando knows the source by to this plugin.
const tokens: Record<string, string> = {}

// fail rejects with code, which Kando reads (auth, not-found, unavailable, timeout, rate-limit), and message.
function fail(code: string, message: string): never {
  const e: any = new Error(message)
  e.code = code
  throw e
}

// post POSTs body as JSON to path under the source's address, with the session token when there is one, and answers Alist's {code, message, data}; login marks the login request, whose body Kando writes the password into.
async function post(source: any, path: string, body: string, token: string, login: boolean): Promise<any> {
  const header: Record<string, string> = { "Content-Type": "application/json" }
  if (token) header["Authorization"] = token
  let r: any
  try {
    r = await host.call("kando.source.fetch", { source: source.id, method: "POST", path, header, body, login }, WAIT)
  } catch (e: any) {
    const code = e && e.code
    fail(code === "timeout" ? "timeout" : "unavailable", "Alist did not answer: " + String((e && e.message) || code || e))
  }
  if (r.status === 401 || r.status === 403) fail("auth", "Alist answered " + r.status)
  if (r.status === 404) fail("unavailable", "the address is not an Alist server (" + path + " answered 404)")
  if (r.status !== 200) fail("unavailable", "Alist answered " + r.status)
  let got: any
  try {
    got = JSON.parse(r.body)
  } catch (e) {
    fail("unavailable", "the address is not an Alist server: its answer is not JSON")
  }
  if (!got || typeof got.code !== "number") fail("unavailable", "the address is not an Alist server")
  return got
}

// userOf is the user the source signs in as, "" for Alist's guest.
function userOf(source: any): string {
  return String((source && source.config && source.config.user) || "").trim()
}

// login signs the source's user in and keeps the token Alist answers.
async function login(source: any): Promise<string> {
  const body = JSON.stringify({ username: userOf(source), password: "{{secret:password}}" })
  const got = await post(source, "api/auth/login", body, "", true)
  if (got.code === 200 && got.data && typeof got.data.token === "string" && got.data.token) {
    tokens[source.id] = got.data.token
    return got.data.token
  }
  if (got.code === 429) fail("rate-limit", String(got.message || "too many sign-in attempts"))
  fail("auth", String(got.message || "Alist did not accept the user and password"))
}

// api POSTs payload to path of Alist's API for the source and answers its data: signed in as its user first, and once more if Alist says the token expired; as the guest when it has no user. Alist answers a failure in its body's code, 401 for a token or a guest it does not take, 403 for a path the user may not read, 500 with "not found" for nothing there.
async function api(source: any, path: string, payload: any): Promise<any> {
  const user = userOf(source)
  const body = JSON.stringify(payload)
  let token = user ? (tokens[source.id] || await login(source)) : ""
  let got = await post(source, path, body, token, false)
  if (got.code === 401 && user) {
    delete tokens[source.id]
    token = await login(source)
    got = await post(source, path, body, token, false)
  }
  if (got.code === 200) return got.data
  const message = String(got.message || "")
  if (got.code === 401 || got.code === 403) fail("auth", message || "Alist refused the request")
  if (/not found/i.test(message)) fail("not-found", message)
  fail("unavailable", "Alist answered " + got.code + ": " + message)
}

// alistPath is p, a path relative to the source's root, as Alist names it.
function alistPath(p: string): string {
  return "/" + p.split("/").filter((s) => s !== "").join("/")
}

// encodePath is p with each segment percent-encoded, as kando.source.fetch takes a path.
function encodePath(p: string): string {
  return p.split("/").filter((s) => s !== "").map(encodeURIComponent).join("/")
}

// entry is the entry Kando takes of an object of Alist's, named name: a folder, or a file with its size, time and version, its size and time, as Alist has no ETag.
function entry(o: any, name: string): any {
  const e: any = { name, dir: !!o.is_dir }
  const modified = typeof o.modified === "string" && !o.modified.startsWith("0001-") ? o.modified : ""
  if (modified) e.modified = modified
  if (!e.dir) {
    const size = Number(o.size)
    if (Number.isFinite(size) && size >= 0 && Math.floor(size) === size) e.size = size
    e.version = String(o.size) + "@" + modified
  }
  return e
}

// ---- the source's exports ----

// list lists the folder dir of the source, page by page.
export async function list(source: any, dir: string): Promise<any[]> {
  const out: any[] = []
  const seen: Record<string, boolean> = {}
  for (let page = 1; out.length < LIST_MAX; page++) {
    const data = await api(source, "api/fs/list", { path: alistPath(dir), password: "", page, per_page: PAGE, refresh: false })
    const content: any[] = (data && data.content) || []
    for (const o of content) {
      const name = String((o && o.name) || "")
      if (name === "" || seen[name] || out.length >= LIST_MAX) continue
      seen[name] = true
      out.push(entry(o, name))
    }
    const total = Number(data && data.total) || 0
    if (content.length === 0 || page * PAGE >= total) break
  }
  return out
}

// stat finds p in the source: its entry, or null when nothing is there. The root is a folder.
export async function stat(source: any, p: string): Promise<any> {
  if (alistPath(p) === "/") return { name: "Alist", dir: true }
  try {
    const o = await api(source, "api/fs/get", { path: alistPath(p), password: "" })
    return entry(o, p.slice(p.lastIndexOf("/") + 1))
  } catch (e: any) {
    if (e && e.code === "not-found") return null
    throw e
  }
}

// sameServer reports whether url is on the source's address: its scheme, host and port, and under its path.
function sameServer(source: any, url: string): boolean {
  const endpoint = String((source && source.config && source.config.endpoint) || "").trim().replace(/\/*$/, "/")
  const origin = (u: string) => {
    const m = /^(https?):\/\/([^/?#]+)/i.exec(u)
    if (!m) return ""
    const scheme = m[1].toLowerCase()
    let authority = m[2].toLowerCase()
    if (scheme === "http") authority = authority.replace(/:80$/, "")
    if (scheme === "https") authority = authority.replace(/:443$/, "")
    return scheme + "://" + authority
  }
  const o = origin(endpoint)
  if (!o || origin(url) !== o) return false
  const path = (s: string) => s.replace(/^[a-z]+:\/\/[^/?#]+/i, "").replace(/[?#].*$/, "") || "/"
  return path(url).startsWith(path(endpoint))
}

// open tells Kando where to read p from: the raw_url fs/get answers when it is on the source's server, as a local storage's or a proxied one's (/d/ or /p/ with its signature); else the server's /d/ link of the file, with its signature when Alist signs it, which Alist redirects to the drive's own link, which Kando follows only by https and without the token.
export async function open(source: any, p: string): Promise<any> {
  const o = await api(source, "api/fs/get", { path: alistPath(p), password: "" })
  if (o.is_dir) fail("not-found", p + " is a folder")
  const raw = typeof o.raw_url === "string" ? o.raw_url : ""
  if (raw && sameServer(source, raw)) return { url: raw }
  // /d/ takes the file's whole path in Alist, which is the user's own base path and p; fs/get answers it as virtual_path, which older versions of Alist leave out, p alone standing in for it then.
  const whole = typeof o.virtual_path === "string" && o.virtual_path ? o.virtual_path : alistPath(p)
  const query: Record<string, string> = {}
  if (o.sign) query.sign = String(o.sign)
  return { path: "d/" + encodePath(whole), query }
}
