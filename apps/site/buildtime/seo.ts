// sitemap.xml, robots.txt and CNAME.
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { pageURL, siteURL, sourceLang, type Language } from "../src/lib/site";

// pageGroups are the kinds of page every language has; each lists its pages in every language as alternates of one another.
export const pageGroups = ["home", "sites"] as const;

// sitemap lists every page with its alternates in the other languages and x-default, the English page.
export function sitemap(langs: readonly Language[]): string {
  let xml = '<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">\n';
  for (const group of pageGroups) {
    for (const l of langs) {
      xml += `  <url>\n    <loc>${pageURL(l.code, group)}</loc>\n`;
      for (const alt of langs) {
        xml += `    <xhtml:link rel="alternate" hreflang="${alt.tag}" href="${pageURL(alt.code, group)}"/>\n`;
      }
      xml += `    <xhtml:link rel="alternate" hreflang="x-default" href="${pageURL(sourceLang, group)}"/>\n  </url>\n`;
    }
  }
  return xml + "</urlset>\n";
}

export function writeSEOFiles(out: string, langs: readonly Language[]): void {
  writeFileSync(join(out, "sitemap.xml"), sitemap(langs));
  writeFileSync(join(out, "robots.txt"), `User-agent: *\nAllow: /\n\nSitemap: ${siteURL}/sitemap.xml\n`);
  writeFileSync(join(out, "CNAME"), siteURL.replace(/^https:\/\//, "") + "\n");
}
