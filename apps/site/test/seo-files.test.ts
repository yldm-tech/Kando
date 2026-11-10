// TestSitemapRobotsCNAME: the sitemap lists each page with all of its alternates, and robots.txt and CNAME point at the site's address.
import { expect, it } from "vite-plus/test";
import { pageURL, siteURL, sourceLang } from "../src/lib/site";
import { builtLangs, readOut } from "./helpers";

it("lists every page in the sitemap with its alternates", () => {
  const sitemap = readOut("sitemap.xml");
  const langs = builtLangs();
  expect((sitemap.match(/<loc>/g) ?? []).length).toBe(2 * langs.length);
  for (const kind of ["home", "sites"] as const) {
    for (const l of langs) {
      expect(sitemap).toContain(`<loc>${pageURL(l.code, kind)}</loc>`);
      const alt = `<xhtml:link rel="alternate" hreflang="${l.tag}" href="${pageURL(l.code, kind)}"/>`;
      expect(sitemap.split(alt).length - 1, alt).toBe(langs.length);
    }
    const xDefault = `hreflang="x-default" href="${pageURL(sourceLang, kind)}"`;
    expect(sitemap.split(xDefault).length - 1).toBe(langs.length);
  }
});

it("writes CNAME and robots.txt", () => {
  expect(readOut("CNAME")).toBe("kando.yldm.ai\n");
  expect(readOut("robots.txt")).toContain(`Sitemap: ${siteURL}/sitemap.xml`);
});
