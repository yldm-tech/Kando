// TestNoExternalHosts: the pages load nothing from elsewhere and link nowhere but the repository, EveryAPI and the site's own address: no fonts, scripts, analytics or CDNs, in the HTML, the site's own scripts and styles, or the bundled libraries. The Go site also had no inline script or style at all; React's hydration needs the router's inline scripts, so here each page carries a Content Security Policy that allows the site's own files and those scripts by their hashes only, and the HTML still has no style attribute, no <style> and no event handler attribute.
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vite-plus/test";
import { inlineScripts, scriptHash } from "../buildtime/csp";
import { everyAPIURL, repoURL, siteURL } from "../src/lib/site";
import { attr, decode, dist, htmlFiles, isExternal, readOut, siteDir, tags } from "./helpers";

const allowed = [repoURL, everyAPIURL, siteURL];

function allowedRef(ref: string): boolean {
  return allowed.some((base) => ref === base || (ref.startsWith(base) && /^[/?#]/.test(ref.slice(base.length))));
}

// shownOnly are addresses the page shows as text and never loads or links: the clone command of the Windows tab and the link of the command-line example.
const shownOnly = new Set([repoURL + ".git", "https://www.youtube.com/watch?v=…"]);

// vocabulary reports whether ref names a vocabulary rather than a resource: an XML or SVG namespace, or schema.org, the context of the pages' JSON-LD.
function vocabulary(ref: string): boolean {
  return /^http:\/\/www\.(w3|sitemaps)\.org\//.test(ref) || ref === "https://schema.org";
}

const urlRe = /(?:https?:)?\/\/[A-Za-z0-9.-]+\.[A-Za-z]{2,}[^\s"'`)<>\\]*/g;

function files(dir: string, ext: RegExp): string[] {
  return readdirSync(dir, { recursive: true, encoding: "utf8" })
    .filter((f) => ext.test(f))
    .map((f) => join(dir, f));
}

describe("no external hosts", () => {
  it("links and loads only the allowed hosts from the pages", () => {
    const bad: string[] = [];
    for (const file of htmlFiles()) {
      for (const m of readOut(file).matchAll(/\s(href|src|content|action|srcset|poster)="([^"]*)"/gi)) {
        const ref = decode(m[2] ?? "");
        if (isExternal(ref) && !allowedRef(ref)) {
          bad.push(`${file}: ${m[1]}="${ref}"`);
        }
      }
    }
    expect(bad).toEqual([]);
  });

  it("names only the allowed hosts in the site's own scripts, styles and source", () => {
    const sources = [
      ...files(join(dist(), "static"), /\.js$/),
      ...files(join(dist(), "assets"), /\.css$/),
      ...files(join(siteDir, "src"), /\.(ts|tsx|css)$/),
      ...files(join(siteDir, "buildtime"), /\.ts$/),
    ];
    expect(sources.length).toBeGreaterThan(10);
    const bad: string[] = [];
    for (const file of sources) {
      // Comments, such as Tailwind's license line, name no address the page uses.
      const code = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
      for (const ref of code.match(urlRe) ?? []) {
        // XML and SVG namespaces and the JSON-LD context name a vocabulary; nothing is fetched from them.
        if (!allowedRef(ref) && !shownOnly.has(ref) && !vocabulary(ref)) {
          bad.push(`${file}: ${ref}`);
        }
      }
    }
    expect(bad).toEqual([]);
  });

  // The bundled scripts are mostly the libraries'. Addresses they name only in error messages and package metadata are listed here by name, so that a library update that starts to name another address, such as a font or decoder fetched from a CDN, fails the test until someone looks at it.
  const libraryText = new Set([
    // React's error decoder, in its minified error messages.
    "https://react.dev/errors/",
    // @react-three/fiber's package.json, which it bundles for its version, and the links of its error messages.
    "https://github.com/drcmda",
    "https://github.com/joshuaellis",
    "https://github.com/codyjasonbennett",
    "https://github.com/krispya",
    "https://github.com/pmndrs/react-three-fiber/issues",
    "https://github.com/pmndrs/react-three-fiber#readme",
    "https://github.com/pmndrs/react-three-fiber.git",
    "https://opencollective.com/react-three-fiber",
    "https://docs.pmnd.rs/react-three-fiber/api/objects#using-3rd-party-objects-declaratively",
    // react-use-measure's error message when the browser has no ResizeObserver.
    "https://github.com/react-spring/react-use-measure/#resize-observer-polyfills",
  ]);

  it("names only the allowed hosts and the libraries' known texts in the bundled scripts", () => {
    const chunks = files(join(dist(), "assets"), /\.js$/);
    expect(chunks.length).toBeGreaterThan(5);
    const bad: string[] = [];
    for (const file of chunks) {
      const code = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
      for (const ref of code.match(urlRe) ?? []) {
        // A regular expression literal's end and its flags, such as /^\/\//i.test(s), look like an address without a scheme.
        if (/^\/\/[a-z]+\.test\(/.test(ref)) {
          continue;
        }
        if (!allowedRef(ref) && !shownOnly.has(ref) && !libraryText.has(ref) && !vocabulary(ref)) {
          bad.push(`${file}: ${ref}`);
        }
      }
    }
    expect(bad).toEqual([]);
  });

  it("has no url() to another host in the stylesheet", () => {
    for (const file of files(join(dist(), "assets"), /\.css$/)) {
      for (const m of readFileSync(file, "utf8").matchAll(/url\(\s*["']?([^"')]+)/g)) {
        const ref = m[1] ?? "";
        expect(isExternal(ref) || ref.startsWith("data:"), `${file}: url(${ref})`).toBe(false);
      }
    }
  });

  it("has no style attribute, <style> or event handler attribute in the HTML", () => {
    for (const file of htmlFiles()) {
      const html = readOut(file);
      for (const re of [/<style[\s>]/i, /\sstyle="/i, /\son[a-z]+="/i]) {
        expect(re.test(html), `${file} has ${re}`).toBe(false);
      }
    }
  });

  it("gives every page a policy that allows its own inline scripts only", () => {
    for (const file of htmlFiles()) {
      const html = readOut(file);
      const meta = tags(html, "meta").find((t) => attr(t, "http-equiv") === "Content-Security-Policy");
      expect(meta, file).toBeDefined();
      const csp = meta ? attr(meta, "content") : "";
      for (const directive of [
        "default-src 'self'",
        "style-src 'self'",
        "img-src 'self'",
        "connect-src 'self'",
        "object-src 'none'",
        "base-uri 'none'",
        "form-action 'none'",
      ]) {
        expect(csp, file).toContain(directive);
      }
      const scriptSrc = csp.split(";").find((d) => d.trim().startsWith("script-src")) ?? "";
      const hashes = scriptSrc.trim().split(/\s+/).slice(1);
      const want = ["'self'", ...new Set(inlineScripts(html).map(scriptHash))];
      expect(hashes.sort(), file).toEqual(want.sort());
      expect(html.indexOf("Content-Security-Policy"), `${file}: the policy comes before every script`).toBeLessThan(html.indexOf("<script"));
    }
  });
});
