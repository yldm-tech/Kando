// Reading the built site in the tests. React writes some attribute names in camel case (hrefLang, charSet), so attributes are matched ignoring case, and their values are decoded from HTML entities before they are compared.
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import { inject } from "vite-plus/test";
import { languages, pagePath, type Language } from "../src/lib/site";

// repoRoot is the repository root as seen from the site's folder, where the tests run.
export const repoRoot = join(process.cwd(), "../..");
export const siteDir = process.cwd();

export function dist(): string {
  return inject("dist");
}

// readOut is a file of the built site.
export function readOut(path: string): string {
  return readFileSync(join(dist(), path), "utf8");
}

export function readJSON<T = Record<string, string>>(path: string): T {
  return JSON.parse(readFileSync(path, "utf8")) as T;
}

// pageFile is the file a language's page of a kind is written to.
export function pageFile(l: Language, kind: "home" | "sites" = "home"): string {
  return pagePath(l.code, kind).slice(1) + "index.html";
}

// builtLangs are the languages with a content file, in the switcher's order: the languages that have pages.
export function builtLangs(): Language[] {
  const codes = new Set(readdirSync(join(siteDir, "content")).map((f) => f.replace(/\.json$/, "")));
  return languages.filter((l) => codes.has(l.code));
}

// htmlFiles are the built site's HTML files, relative to its root.
export function htmlFiles(): string[] {
  return readdirSync(dist(), { recursive: true, encoding: "utf8" })
    .filter((f) => f.endsWith(".html"))
    .map((f) => relative(dist(), join(dist(), f)).split("\\").join("/"));
}

const entities: Record<string, string> = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", nbsp: "\u00a0" };

// decode is text with its HTML entities replaced by the characters they stand for.
export function decode(s: string): string {
  return s.replace(/&(#x[0-9a-f]+|#\d+|[a-z]+);/gi, (whole, name: string) => {
    if (name.startsWith("#x") || name.startsWith("#X")) {
      return String.fromCodePoint(parseInt(name.slice(2), 16));
    }
    if (name.startsWith("#")) {
      return String.fromCodePoint(parseInt(name.slice(1), 10));
    }
    return entities[name.toLowerCase()] ?? whole;
  });
}

// text is the text of an HTML fragment: its tags and React's text separators taken out, its entities decoded.
export function text(html: string): string {
  return decode(html.replace(/<!--.*?-->/gs, "").replace(/<[^>]+>/g, ""));
}

export interface Tag {
  raw: string;
  index: number;
  attrs: Map<string, string>;
}

const attrRe = /\s([a-zA-Z_:][-a-zA-Z0-9_:.]*)(?:="([^"]*)")?/g;

// parseTag is a start tag's attributes, by lower-case name.
export function parseTag(raw: string, index = 0): Tag {
  const attrs = new Map<string, string>();
  const inner = raw.replace(/^<[a-zA-Z0-9]+/, "").replace(/\/?>$/, "");
  for (const m of inner.matchAll(attrRe)) {
    attrs.set((m[1] ?? "").toLowerCase(), decode(m[2] ?? ""));
  }
  return { raw, index, attrs };
}

// tags are the start tags of element name in html.
export function tags(html: string, name: string): Tag[] {
  const re = new RegExp(`<${name}(?=[\\s>/])[^>]*>`, "gi");
  return [...html.matchAll(re)].map((m) => parseTag(m[0], m.index));
}

// attr is a tag's attribute, or "" when it has none.
export function attr(tag: Tag, name: string): string {
  return tag.attrs.get(name.toLowerCase()) ?? "";
}

// section is html from the first start tag with the class to the end of the element given by end.
export function section(html: string, marker: string, end: string): string {
  const i = html.indexOf(marker);
  if (i < 0) {
    throw new Error(`no ${marker} in the page`);
  }
  const rest = html.slice(i);
  const j = rest.indexOf(end);
  return j < 0 ? rest : rest.slice(0, j);
}

// isExternal reports whether ref leaves the site.
export function isExternal(ref: string): boolean {
  return /^(https?:)?\/\//.test(ref);
}

// simplified are the Simplified Chinese forms in the catalog's site names that Traditional Chinese writes otherwise, which the Traditional Chinese pages must not show.
export const simplified = /[哔红视频爱国优腾讯斗鱼乐网头条马场云]/;
