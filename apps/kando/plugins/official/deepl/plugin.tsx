// Kando's official DeepL plugin: a translation source (docs/plugin-system.md E14) that sends the subtitles to the DeepL API with the user's own key. The key never reaches this code: Kando keeps it and puts it in the Authorization header of the requests this plugin sends through kando.net.credentialFetch, only to DeepL's two API hosts and only to /v2/translate and /v2/usage.
import { host } from "nsgo"
import manifest from "./kando-plugin.json"

// kandoManifest says what the plugin may reach and adds, as its kando-plugin.json does, which Kando compares key for key with the signed code's answer.
export function kandoManifest() {
  return { capabilities: manifest.capabilities, contributes: manifest.contributes, credentials: manifest.credentials }
}

// DeepL serves a Free key, which ends in ":fx", from api-free.deepl.com and a Pro key from api.deepl.com, and refuses a key at the other host with 403. This code never sees the key, so it asks /v2/usage at the free host first and at the other one after a 403, and keeps the host that took the key; the subtitles are only ever sent to that one.
const FREE = "api-free.deepl.com"
const PRO = "api.deepl.com"
let apiHost: string | null = null

// The most texts DeepL takes in one request, and the most Kando gives at once anyway.
const BATCH = 50

// What Kando waits for a request; Kando's own limit is 120 s, so the plugin hears Kando's answer rather than its own timeout.
const WAIT = { timeout: 150000 }

// fail rejects with code, which Kando's failure card words for the user (quota, auth, no-credential, language, rate-limit, unavailable), and message, which it shows for a code it does not know.
function fail(code: string, message: string): never {
  const e: any = new Error(message)
  e.code = code
  throw e
}

// targetCode is DeepL's target_lang for a Kando language tag: Chinese names its script, English defaults to American and Portuguese to Brazilian, as DeepL asks for one of each.
const TARGETS: Record<string, string> = {
  "zh": "ZH-HANS", "zh-hans": "ZH-HANS", "zh-hant": "ZH-HANT",
  "en": "EN-US", "en-us": "EN-US", "en-gb": "EN-GB",
  "pt": "PT-BR", "pt-br": "PT-BR", "pt-pt": "PT-PT",
  "nb": "NB", "no": "NB",
}
export function targetCode(tag: string): string {
  const t = String(tag || "").toLowerCase()
  return TARGETS[t] || t.split("-")[0].toUpperCase()
}

// sourceCode is DeepL's source_lang for the language a film is heard in, whisper's code or a tag, "" to let DeepL tell it.
export function sourceCode(tag: string): string {
  const t = String(tag || "").toLowerCase().split("-")[0]
  if (t === "no") return "NB"
  return t.toUpperCase()
}

// send POSTs body as JSON to path at host h with the key Kando keeps, and answers {status, body}; Kando's own refusals become this plugin's codes.
async function send(h: string, path: string, body: any): Promise<{ status: number, body: string }> {
  try {
    return await host.call("kando.net.credentialFetch", {
      credential: "deepl", method: "POST", url: "https://" + h + path,
      header: { "Content-Type": "application/json" }, body: JSON.stringify(body),
    }, WAIT)
  } catch (e: any) {
    const code = e && e.code
    if (code === "no-credential") fail("no-credential", "no DeepL API key is set")
    if (code === "denied") fail("unavailable", "Kando did not let the request go (offline mode, or the plugin's permission is off)")
    fail("unavailable", "DeepL did not answer: " + String((e && e.message) || code || e))
  }
}

// apiHostOf finds the host that takes the key (/v2/usage), and checks that the account has characters left.
async function apiHostOf(): Promise<string> {
  const hosts = apiHost ? [apiHost, apiHost === FREE ? PRO : FREE] : [FREE, PRO]
  for (const h of hosts) {
    const r = await send(h, "/v2/usage", {})
    if (r.status === 403) continue
    check(r)
    apiHost = h
    try {
      const u = JSON.parse(r.body)
      if (typeof u.character_limit === "number" && u.character_limit > 0 && u.character_count >= u.character_limit) fail("quota", "the DeepL character limit is used up")
    } catch (e: any) {
      if (e && e.code) throw e
    }
    return h
  }
  apiHost = null
  fail("auth", "DeepL did not accept the key")
}

// check fails for a DeepL reply that is not a success, with the code its status means.
function check(r: { status: number, body: string }) {
  if (r.status >= 200 && r.status < 300) return
  const said = String(r.body || "").slice(0, 200)
  if (r.status === 456) fail("quota", "the DeepL quota is exceeded")
  if (r.status === 403 || r.status === 401) fail("auth", "DeepL did not accept the key")
  if (r.status === 429) fail("rate-limit", "DeepL asked to slow down")
  if (r.status >= 500) fail("unavailable", "DeepL answered " + r.status)
  if (r.status === 400 && /target_lang|source_lang/.test(said)) fail("language", said)
  fail("failed", "DeepL answered " + r.status + ": " + said)
}

// request sends texts to /v2/translate and answers their translations, as many and in their order.
async function request(texts: string[], from: string, to: string, extra: any): Promise<string[]> {
  let h = apiHost || await apiHostOf()
  const body: any = Object.assign({ text: texts, target_lang: targetCode(to), split_sentences: "nonewlines", preserve_formatting: true }, extra)
  const source = sourceCode(from)
  if (source) body.source_lang = source
  let r = await send(h, "/v2/translate", body)
  if (r.status === 403) {
    // The key changed since the host was found: find it again, once.
    apiHost = null
    h = await apiHostOf()
    r = await send(h, "/v2/translate", body)
  }
  check(r)
  let got: any
  try {
    got = JSON.parse(r.body)
  } catch (e) {
    fail("bad-reply", "DeepL's answer is not JSON")
  }
  const out = (got && got.translations || []).map((t: any) => String(t && t.text || ""))
  if (out.length !== texts.length) fail("bad-reply", "DeepL answered " + out.length + " of " + texts.length + " lines")
  return out
}

// translate translates lines from the language the film is heard in (from, "" to let DeepL tell it) into to, a Kando language tag, BATCH lines a request, and answers as many lines in their order; a blank line stays blank and is not sent.
export async function translate(lines: string[], from: string, to: string, context: any): Promise<string[]> {
  const out: string[] = lines.map(() => "")
  const at: number[] = []
  lines.forEach((l, i) => { if (String(l).trim() !== "") at.push(i) })
  for (let start = 0; start < at.length; start += BATCH) {
    const part = at.slice(start, start + BATCH)
    const got = await request(part.map((i) => lines[i]), from, to, {})
    part.forEach((i, k) => { out[i] = got[k] })
  }
  return out
}

// candidates offers other translations of line, the one the viewer says is wrong: DeepL's in a more and a less formal register, with the lines around it as context, which DeepL reads and does not translate. Kando drops the line's current translation and repeats.
export async function candidates(line: string, from: string, to: string, context: any): Promise<string[]> {
  const around = ((context && context.before) || []).concat((context && context.after) || []).join("\n")
  const extra: any = around ? { context: around } : {}
  const out: string[] = []
  for (const formality of ["prefer_more", "prefer_less"]) {
    const [t] = await request([line], from, to, Object.assign({ formality }, extra))
    if (t && !out.includes(t)) out.push(t)
  }
  return out
}
