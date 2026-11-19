// The plugin page's reading of Kando's community plugin index, the file Kando downloads (apps/kando/plugins/index): an envelope with the index's JSON in base64 and its signature. The page does not check the signature; it only lists what the index says, and Kando checks the signature with its own key, and the pack's size and SHA-256, before it installs anything. So everything in the file is treated as untrusted: each field is checked for its type, shape and length, an entry that fails is left out as Kando leaves it out, the page shows the fields as text only, and links to a homepage only when it is an https address.

import { pluginIndexPath } from "./site";

export const indexFormat = "kando-plugins-index";
export const indexVersion = 1;

// The limits of the index, as Kando reads it (index.MaxBytes, MaxEntries, MaxLanguages): the file's size in characters, its base64 body grown by a third, the number of entries and the languages of one.
const maxFileChars = 6 << 20;
const maxEntries = 5000;
const maxLanguages = 16;
// The longest texts the page shows: a name, a summary, a license, a version and an address.
const maxName = 120;
const maxSummary = 600;
const maxLicense = 64;
const maxVersion = 64;
const maxURL = 2048;

export const pluginKinds = ["dictionary", "wordlist", "glossary", "ai-service", "translate-style", "task", "export-template", "site", "script"] as const;
export type PluginKind = (typeof pluginKinds)[number];

// PluginSource is where a script plugin's signature comes from: Kando's own key, or a publisher's key Kando verified.
export type PluginSource = "official" | "verified";

// Localized is a text of a pack's in each language it has, by the tag in lower case, with the tag as the pack writes it.
export type Localized = ReadonlyMap<string, { tag: string; text: string }>;

export interface PluginEntry {
  id: string;
  kind: PluginKind;
  name: Localized;
  summary: Localized;
  version: string;
  publisher: string;
  license: string;
  // homepage is the pack's homepage as a parsed https address, or undefined.
  homepage?: string;
  size: number;
  languages: string[];
  minKando?: string;
  source?: PluginSource;
  // search is the text a search looks in, in lower case: the id, the publisher, and the name and summary in every language.
  search: string;
}

export interface PluginIndex {
  generated: string;
  entries: PluginEntry[];
  // dropped counts the entries left out because they were not valid or listed an id again.
  dropped: number;
}

// IndexError is an index the page cannot read: not JSON, of another format or version, or not shaped as an index.
export class IndexError extends Error {
  override name = "IndexError";
}

// OfflineError is a fetch that did not reach the server: the browser is offline, or the network failed.
export class OfflineError extends Error {
  override name = "OfflineError";
}

type Obj = Record<string, unknown>;

function isObj(v: unknown): v is Obj {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

// own is a key of o that o has itself, never one it inherits, such as constructor.
function own(o: Obj, key: string): unknown {
  return Object.hasOwn(o, key) ? o[key] : undefined;
}

const idRe = /^[a-z0-9]+(?:[.-][a-z0-9]+)*$/;
const tagRe = /^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8}){0,3}$/;
const versionRe = /^[0-9A-Za-z][0-9A-Za-z.+-]*$/;
const sha256Re = /^[0-9a-f]{64}$/;
const minKandoRe = /^\d+\.\d+\.\d+$/;

// codePoints counts the characters of s as Kando counts a text's length, by code point.
function codePoints(s: string): number {
  let n = 0;
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    // The second half of a surrogate pair is the same character as the first.
    if (c < 0xdc00 || c > 0xdfff) {
      n++;
    }
  }
  return n;
}

// hasControl reports whether s has a control character or a bidirectional override or isolate, which could make a text read as another; no text the page shows may have one once its runs of white space are single spaces.
function hasControl(s: string): boolean {
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    if (c < 0x20 || (c >= 0x7f && c <= 0x9f) || (c >= 0x202a && c <= 0x202e) || (c >= 0x2066 && c <= 0x2069)) {
      return true;
    }
  }
  return false;
}

// cleanText is s with its runs of white space made one space and trimmed, or undefined when it is not a string, is empty, is over max characters or has a control character.
function cleanText(s: unknown, max: number): string | undefined {
  if (typeof s !== "string" || s.length > max * 4) {
    return undefined;
  }
  const t = s.replace(/\s+/g, " ").trim();
  if (t === "" || codePoints(t) > max || hasControl(t)) {
    return undefined;
  }
  return t;
}

// localized reads a pack's text by language; undefined when it is not an object of tags to texts, or when required and it has no English.
function localized(v: unknown, max: number, required: boolean): Localized | undefined {
  if (v === undefined && !required) {
    return new Map();
  }
  if (!isObj(v)) {
    return undefined;
  }
  const out = new Map<string, { tag: string; text: string }>();
  const tags = Object.keys(v);
  if (tags.length > 64) {
    return undefined;
  }
  for (const tag of tags) {
    const text = cleanText(v[tag], max);
    if (!tagRe.test(tag) || text === undefined) {
      return undefined;
    }
    out.set(tag.toLowerCase(), { tag, text });
  }
  if (required && !out.has("en")) {
    return undefined;
  }
  return out;
}

// httpsURL is s as a parsed https address without a user or password, or undefined.
export function httpsURL(s: unknown): string | undefined {
  if (typeof s !== "string" || s.length > maxURL) {
    return undefined;
  }
  let u: URL;
  try {
    u = new URL(s);
  } catch {
    return undefined;
  }
  if (u.protocol !== "https:" || u.username !== "" || u.password !== "" || u.hostname === "") {
    return undefined;
  }
  return u.href;
}

// parseEntry is one entry of the index, or undefined when any of its fields is not valid as Kando checks it (index.Entry.Check) where the page can tell.
export function parseEntry(v: unknown): PluginEntry | undefined {
  if (!isObj(v)) {
    return undefined;
  }
  const id = own(v, "id");
  const kind = own(v, "kind");
  if (typeof id !== "string" || id.length > 64 || !id.includes(".") || !idRe.test(id)) {
    return undefined;
  }
  if (typeof kind !== "string" || !(pluginKinds as readonly string[]).includes(kind)) {
    return undefined;
  }
  const publisher = own(v, "publisher");
  if (typeof publisher !== "string" || publisher !== id.slice(0, id.indexOf("."))) {
    return undefined;
  }
  const name = localized(own(v, "name"), maxName, true);
  const summary = localized(own(v, "summary"), maxSummary, false);
  const version = cleanText(own(v, "version"), maxVersion);
  const license = cleanText(own(v, "license"), maxLicense);
  if (!name || !summary || version === undefined || !versionRe.test(version) || license === undefined) {
    return undefined;
  }
  const rawHomepage = own(v, "homepage");
  const homepage = rawHomepage === undefined || rawHomepage === "" ? undefined : httpsURL(rawHomepage);
  if (rawHomepage !== undefined && rawHomepage !== "" && homepage === undefined) {
    return undefined;
  }
  const size = own(v, "size");
  if (typeof size !== "number" || !Number.isSafeInteger(size) || size < 1) {
    return undefined;
  }
  const sha256 = own(v, "sha256");
  if (typeof sha256 !== "string" || !sha256Re.test(sha256) || httpsURL(own(v, "url")) === undefined) {
    return undefined;
  }
  const rawLanguages = own(v, "languages");
  const languages: string[] = [];
  if (rawLanguages !== undefined) {
    if (!Array.isArray(rawLanguages) || rawLanguages.length > maxLanguages) {
      return undefined;
    }
    for (const lang of rawLanguages as unknown[]) {
      if (typeof lang !== "string" || !tagRe.test(lang) || languages.includes(lang)) {
        return undefined;
      }
      languages.push(lang);
    }
  }
  const minKando = own(v, "minKando");
  if (minKando !== undefined && minKando !== "" && (typeof minKando !== "string" || !minKandoRe.test(minKando) || kind !== "script")) {
    return undefined;
  }
  const source = own(v, "source");
  if (kind === "script" ? source !== "official" && source !== "verified" : source !== undefined && source !== "") {
    return undefined;
  }
  const texts = [...name.values(), ...summary.values()].map((t) => t.text);
  return {
    id,
    kind: kind as PluginKind,
    name,
    summary,
    version,
    publisher,
    license,
    homepage,
    size,
    languages,
    minKando: typeof minKando === "string" && minKando !== "" ? minKando : undefined,
    source: source === "official" || source === "verified" ? source : undefined,
    search: [id, publisher, ...texts].join("\n").toLowerCase(),
  };
}

// decodeBase64 is the UTF-8 text of the standard base64 s, or an IndexError.
function decodeBase64(s: string): string {
  if (s.length % 4 !== 0 || !/^[A-Za-z0-9+/]*={0,2}$/.test(s)) {
    throw new IndexError("the body is not base64");
  }
  const bin = atob(s);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) {
    bytes[i] = bin.charCodeAt(i);
  }
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    throw new IndexError("the body is not UTF-8");
  }
}

// decodeIndex reads an index file already parsed from JSON: the envelope's format and version, its body, and the body's format, version, time and entries. A file of another format or version, or one not shaped as an index, is an IndexError; an entry that is not valid, or lists an id again, is left out and counted.
export function decodeIndex(raw: unknown): PluginIndex {
  if (!isObj(raw)) {
    throw new IndexError("the index is not an object");
  }
  if (own(raw, "format") !== indexFormat) {
    throw new IndexError("the file is not a plugin index");
  }
  if (own(raw, "version") !== indexVersion) {
    throw new IndexError("the index is of another version");
  }
  const body = own(raw, "body");
  if (typeof own(raw, "signer") !== "string" || typeof own(raw, "signature") !== "string" || typeof body !== "string" || body.length > maxFileChars) {
    throw new IndexError("the index has no body or signature");
  }
  let index: unknown;
  try {
    index = JSON.parse(decodeBase64(body));
  } catch (err) {
    throw err instanceof IndexError ? err : new IndexError("the body is not JSON");
  }
  if (!isObj(index) || own(index, "format") !== indexFormat || own(index, "version") !== indexVersion) {
    throw new IndexError("the body is not a plugin index of this version");
  }
  const generated = own(index, "generated");
  if (typeof generated !== "string" || Number.isNaN(Date.parse(generated))) {
    throw new IndexError("the index has no time it was made");
  }
  const list = own(index, "entries");
  if (!Array.isArray(list) || list.length > maxEntries) {
    throw new IndexError("the index has no list of entries, or too many");
  }
  const entries: PluginEntry[] = [];
  const seen = new Set<string>();
  let dropped = 0;
  for (const item of list as unknown[]) {
    const e = parseEntry(item);
    if (!e || seen.has(e.id)) {
      dropped++;
      continue;
    }
    seen.add(e.id);
    entries.push(e);
  }
  return { generated, entries, dropped };
}

// fetchIndex fetches the index from the site and reads it (decodeIndex): an OfflineError when the browser is offline or the request never reached the server, an IndexError for an answer other than the file or a file the page cannot read.
export async function fetchIndex(signal?: AbortSignal): Promise<PluginIndex> {
  if (typeof navigator !== "undefined" && navigator.onLine === false) {
    throw new OfflineError("the browser is offline");
  }
  let res: Response;
  try {
    res = await fetch(pluginIndexPath, { signal, cache: "no-cache", credentials: "omit", headers: { Accept: "application/json" } });
  } catch (err) {
    if (signal?.aborted) {
      throw err;
    }
    throw new OfflineError("the request did not reach the server", { cause: err });
  }
  if (!res.ok) {
    throw new IndexError(`HTTP ${res.status}`);
  }
  const text = await res.text();
  if (text.length > maxFileChars) {
    throw new IndexError("the index is too large");
  }
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    throw new IndexError("the index is not JSON");
  }
  return decodeIndex(raw);
}

// tagParent is the language tag tag falls back to when a text is not in it, as Kando's i18n.Parent: the script of a Chinese region (zh-TW is zh-Hant, zh-CN zh-Hans), else the tag without its last part; "" after the last.
export function tagParent(tag: string): string {
  const scripts: Record<string, string> = {
    "zh-tw": "zh-Hant",
    "zh-hk": "zh-Hant",
    "zh-mo": "zh-Hant",
    "zh-cn": "zh-Hans",
    "zh-sg": "zh-Hans",
    "zh-my": "zh-Hans",
  };
  const script = scripts[tag.toLowerCase()];
  if (script) {
    return script;
  }
  const at = tag.lastIndexOf("-");
  return at > 0 ? tag.slice(0, at) : "";
}

// localText is t in the language tag, else in its parents (tagParent), else in English; with the tag it is written in, for the lang attribute. Undefined when t has none of them, as a summary may have none.
export function localText(t: Localized, tag: string): { tag: string; text: string } | undefined {
  for (let l = tag; l !== ""; l = tagParent(l)) {
    const found = t.get(l.toLowerCase());
    if (found) {
      return found;
    }
  }
  return t.get("en");
}

// pluginMatches reports whether the entry e matches query: every word of it, ignoring case, is part of its id, its publisher, or its name or summary in any language. An empty query matches every entry.
export function pluginMatches(e: PluginEntry, query: string): boolean {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  return words.every((w) => e.search.includes(w));
}

// filterPlugins are the entries of the kind ("" for every kind) that match query, in the index's order.
export function filterPlugins(entries: readonly PluginEntry[], query: string, kind: PluginKind | ""): PluginEntry[] {
  return entries.filter((e) => (kind === "" || e.kind === kind) && pluginMatches(e, query));
}

// installLink is the address that opens the entry in Kando, which shows it in Settings › Plugins and asks before it installs anything.
export function installLink(id: string): string {
  return "kando://plugin/install?id=" + encodeURIComponent(id);
}

// formatSize is bytes in the page's language, in bytes, kilobytes or megabytes of 1000 as the systems' file browsers count them.
export function formatSize(bytes: number, tag: string): string {
  const format = (value: number, unit: string) =>
    new Intl.NumberFormat(tag, { style: "unit", unit, unitDisplay: "short", maximumFractionDigits: 1 }).format(value);
  if (bytes < 1000) {
    return format(bytes, "byte");
  }
  return bytes < 1_000_000 ? format(bytes / 1000, "kilobyte") : format(bytes / 1_000_000, "megabyte");
}

// languageName is the language tag named in the page's language tag, or the tag itself where the browser has no name for it.
export function languageName(lang: string, tag: string): string {
  try {
    return new Intl.DisplayNames([tag], { type: "language" }).of(lang) ?? lang;
  } catch {
    return lang;
  }
}
