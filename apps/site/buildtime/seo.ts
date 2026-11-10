// sitemap.xml, robots.txt, CNAME and the web app manifest, site.webmanifest.
import { spawnSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { pageURL, siteURL, sourceLang, themeColor, type Language } from "../src/lib/site";
import { manifestIconSizes } from "./icons";

// pageGroups are the kinds of page every language has; each lists its pages in every language as alternates of one another.
export const pageGroups = ["home", "sites"] as const;

// lastModified is the date of the commit the site is built from, as YYYY-MM-DD: the site is built on a push to main that changes what it shows, or after a release, so the commit's date is when its pages last changed. Where git cannot tell, as in a copy without its history, it is today.
export function lastModified(root: string): string {
  const git = spawnSync("git", ["-C", root, "log", "-1", "--format=%cs"], { encoding: "utf8" });
  const date = git.status === 0 ? git.stdout.trim() : "";
  return /^\d{4}-\d{2}-\d{2}$/.test(date) ? date : new Date().toISOString().slice(0, 10);
}

// sitemap lists every page with its alternates in the other languages and x-default, the English page, and the date it last changed.
export function sitemap(langs: readonly Language[], lastmod: string): string {
  let xml = '<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">\n';
  for (const group of pageGroups) {
    for (const l of langs) {
      xml += `  <url>\n    <loc>${pageURL(l.code, group)}</loc>\n    <lastmod>${lastmod}</lastmod>\n`;
      for (const alt of langs) {
        xml += `    <xhtml:link rel="alternate" hreflang="${alt.tag}" href="${pageURL(alt.code, group)}"/>\n`;
      }
      xml += `    <xhtml:link rel="alternate" hreflang="x-default" href="${pageURL(sourceLang, group)}"/>\n  </url>\n`;
    }
  }
  return xml + "</urlset>\n";
}

// webManifest is site.webmanifest: the site as an installable app named Kando, opening on the English page, which sends a reader on to their language's page as the root page does, with the icons made from the app icon and the page's paper as its colors.
export function webManifest(description: string): string {
  const manifest = {
    id: "/",
    name: "Kando",
    short_name: "Kando",
    description,
    lang: sourceLang,
    dir: "ltr",
    start_url: "/",
    scope: "/",
    display: "standalone",
    theme_color: themeColor,
    background_color: themeColor,
    icons: manifestIconSizes.map((size) => ({ src: `/icon-${size}.png`, sizes: `${size}x${size}`, type: "image/png", purpose: "any" })),
  };
  return JSON.stringify(manifest, null, 2) + "\n";
}

export function writeSEOFiles(out: string, langs: readonly Language[], opts: { lastmod: string; description: string }): void {
  writeFileSync(join(out, "sitemap.xml"), sitemap(langs, opts.lastmod));
  writeFileSync(join(out, "robots.txt"), `User-agent: *\nAllow: /\n\nSitemap: ${siteURL}/sitemap.xml\n`);
  writeFileSync(join(out, "CNAME"), siteURL.replace(/^https:\/\//, "") + "\n");
  writeFileSync(join(out, "site.webmanifest"), webManifest(opts.description));
}
