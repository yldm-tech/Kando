// The last step of the build, on the prerendered HTML. React's hydration needs inline scripts (TanStack Start's dehydrated router state), so the pages cannot do without them as the Go site did; instead each page carries a Content Security Policy that allows scripts from the site itself and those inline scripts by their hashes only, and no style sheets but the site's own. 404.html loses its module scripts: GitHub Pages serves it at any missing address, where the router would match another route than the one it was rendered for, so it stays the static page it was rendered as.
import { createHash } from "node:crypto";
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import { otherLangCodes, sourceLang, type LangCode } from "../src/lib/site";

// scriptRe finds a script element: its attributes and its text.
const scriptRe = /<script\b([^>]*)>([\s\S]*?)<\/script>/gi;

// inlineScripts are the texts of the inline scripts of html that run: those without src that are not data blocks.
export function inlineScripts(html: string): string[] {
  const out: string[] = [];
  for (const m of html.matchAll(scriptRe)) {
    const attrs = m[1] ?? "";
    if (/\ssrc\s*=/i.test(attrs) || /\stype\s*=\s*"application\/(ld\+)?json"/i.test(attrs)) {
      continue;
    }
    out.push(m[2] ?? "");
  }
  return out;
}

export function scriptHash(text: string): string {
  return "'sha256-" + createHash("sha256").update(text, "utf8").digest("base64") + "'";
}

// policy is the Content Security Policy of a page whose inline scripts are scripts.
export function policy(scripts: readonly string[]): string {
  const hashes = [...new Set(scripts.map(scriptHash))];
  return [
    "default-src 'self'",
    ["script-src 'self'", ...hashes].join(" "),
    "style-src 'self'",
    "img-src 'self'",
    "connect-src 'self'",
    "font-src 'self'",
    "object-src 'none'",
    "base-uri 'none'",
    "form-action 'none'",
  ].join("; ");
}

// stripModules takes the module scripts, their preloads and the inline scripts out of html, leaving the classic scripts it loads by src and its data blocks.
export function stripModules(html: string): string {
  return html
    .replace(scriptRe, (whole, attrs: string) => {
      if (/\stype\s*=\s*"module"/i.test(attrs)) {
        return "";
      }
      if (/\ssrc\s*=/i.test(attrs) || /\stype\s*=\s*"application\/(ld\+)?json"/i.test(attrs)) {
        return whole;
      }
      return "";
    })
    .replace(/<link\b[^>]*\brel="modulepreload"[^>]*>/gi, "");
}

// withPolicy puts the page's policy right after its charset declaration, before anything it governs.
export function withPolicy(html: string): string {
  const csp = `<meta http-equiv="Content-Security-Policy" content="${policy(inlineScripts(html))}"/>`;
  const charset = html.match(/<meta\s+charset="utf-8"\s*\/?>/i);
  if (!charset || charset.index === undefined) {
    throw new Error("the page declares no charset");
  }
  const at = charset.index + charset[0].length;
  return html.slice(0, at) + csp + html.slice(at);
}

// pageLang is the language of the page at rel, a path in the built site: the language whose folder it is in, or the source language at the root.
export function pageLang(rel: string): LangCode {
  const first = rel.split(/[\\/]/)[0];
  return otherLangCodes.find((c) => c === first) ?? sourceLang;
}

// withPreload adds a modulepreload of href to html's head, after its other preloads, so that the browser fetches the page's content chunk with the scripts that import it rather than one round trip after them.
export function withPreload(html: string, href: string): string {
  const link = `<link rel="modulepreload" href="${href}"/>`;
  const preloads = [...html.matchAll(/<link\b[^>]*\brel="modulepreload"[^>]*>/gi)];
  const last = preloads.at(-1);
  if (last?.index !== undefined) {
    const at = last.index + last[0].length;
    return html.slice(0, at) + link + html.slice(at);
  }
  const head = html.indexOf("</head>");
  if (head < 0) {
    throw new Error("the page has no </head>");
  }
  return html.slice(0, head) + link + html.slice(head);
}

function htmlFiles(dir: string): string[] {
  return readdirSync(dir, { recursive: true, encoding: "utf8" })
    .filter((f) => f.endsWith(".html"))
    .map((f) => join(dir, f));
}

// withStructuredData puts json, a page's JSON-LD, at the end of html's head as a data block, which no policy needs to allow because it never runs.
export function withStructuredData(html: string, json: string): string {
  const head = html.indexOf("</head>");
  if (head < 0) {
    throw new Error("the page has no </head>");
  }
  return html.slice(0, head) + `<script type="application/ld+json">${json}</script>` + html.slice(head);
}

// pagePathOf is the address path of the page at rel, a path in the built site: zh-CN/sites/index.html is /zh-CN/sites/.
export function pagePathOf(rel: string): string {
  return (
    "/" +
    rel
      .split(/[\\/]/)
      .join("/")
      .replace(/index\.html$/, "")
  );
}

// finishHTML gives every page of the built site in dir its policy, strips 404.html's module scripts first, and preloads every other page's content chunk, from chunks by language, and puts in its structured data, from structured by the page's path.
export function finishHTML(dir: string, chunks: ReadonlyMap<LangCode, string>, structured: ReadonlyMap<string, string>): void {
  const files = htmlFiles(dir);
  if (files.length === 0) {
    throw new Error(`${dir} holds no pages; the prerender wrote nothing`);
  }
  for (const file of files) {
    let html = readFileSync(file, "utf8");
    if (html.includes('http-equiv="Content-Security-Policy"')) {
      continue;
    }
    if (file === join(dir, "404.html")) {
      html = stripModules(html);
    } else {
      const code = pageLang(relative(dir, file));
      const chunk = chunks.get(code);
      if (!chunk) {
        throw new Error(`${file}: the client build made no chunk of the ${code} content`);
      }
      html = withPreload(html, chunk);
      const json = structured.get(pagePathOf(relative(dir, file)));
      if (!json) {
        throw new Error(`${file}: no structured data for ${pagePathOf(relative(dir, file))}`);
      }
      html = withStructuredData(html, json);
    }
    writeFileSync(file, withPolicy(html));
  }
}
