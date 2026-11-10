// TestSitemapRobotsCNAME: the sitemap lists each page with all of its alternates and the date it last changed, robots.txt and CNAME point at the site's address, the web app manifest names the app and its icons, and llms.txt and llms-full.txt describe Kando with links to every page, the READMEs and the release.
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { pngSize } from "../buildtime/icons";
import { appVersion } from "../buildtime/app";
import { faqPairs, latestURL, pageURL, readmeURL, releasesURL, siteURL, sourceLang, themeColor } from "../src/lib/site";
import { builtLangs, dist, readJSON, readOut, repoRoot, siteDir } from "./helpers";

it("lists every page in the sitemap with its alternates and when it last changed", () => {
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
  const urls = sitemap.split("<url>").slice(1);
  for (const url of urls) {
    const lastmod = url.match(/<lastmod>([^<]*)<\/lastmod>/)?.[1] ?? "";
    expect(lastmod, url).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(Date.parse(lastmod)).toBeLessThanOrEqual(Date.now());
  }
});

it("writes CNAME and robots.txt", () => {
  expect(readOut("CNAME")).toBe("kando.yldm.ai\n");
  expect(readOut("robots.txt")).toContain(`Sitemap: ${siteURL}/sitemap.xml`);
});

it("writes a web app manifest with the app's name, colors and icons", () => {
  const manifest = JSON.parse(readOut("site.webmanifest")) as Record<string, unknown> & { icons: { src: string; sizes: string; type: string }[] };
  expect(manifest["name"]).toBe("Kando");
  expect(manifest["short_name"]).toBe("Kando");
  expect(manifest["start_url"]).toBe("/");
  expect(manifest["display"]).toBe("standalone");
  expect(manifest["theme_color"]).toBe(themeColor);
  expect(manifest["background_color"]).toBe(themeColor);
  expect(manifest["description"]).toBe(readJSON(join(siteDir, "content", "en.json"))["metaDescription"]);
  expect(manifest.icons.map((i) => i.sizes).sort()).toEqual(["192x192", "512x512"]);
  for (const icon of manifest.icons) {
    expect(icon.type).toBe("image/png");
    const side = Number(icon.sizes.split("x")[0]);
    expect(pngSize(join(dist(), icon.src)), icon.src).toEqual({ w: side, h: side });
  }
});

// The links of a Markdown list, by their address.
function links(md: string): Map<string, string> {
  return new Map([...md.matchAll(/\[([^\]]+)\]\(([^)]+)\)/g)].map((m) => [m[2] ?? "", m[1] ?? ""]));
}

it("writes llms.txt as the llms.txt proposal lays it out, linking every page, the READMEs and the release", () => {
  const txt = readOut("llms.txt");
  const en = readJSON(join(siteDir, "content", "en.json"));
  // A title, then the summary as a quote, then sections of links.
  expect(txt.startsWith("# Kando\n\n> " + en["metaDescription"] + "\n")).toBe(true);
  expect(txt).not.toMatch(/\{[a-zA-Z]+\}|`/);
  expect([...txt.matchAll(/^## (.+)$/gm)].map((m) => m[1])).toEqual(["Pages", "Docs", "Optional"]);
  const linked = links(txt);
  for (const l of builtLangs()) {
    expect(linked.has(pageURL(l.code)), pageURL(l.code)).toBe(true);
    expect(linked.has(pageURL(l.code, "sites")), pageURL(l.code, "sites")).toBe(true);
    expect(linked.has(readmeURL(l)), readmeURL(l)).toBe(true);
  }
  const version = appVersion(repoRoot);
  expect(linked.get(`${releasesURL}/tag/v${version}`)).toBe(`Kando ${version}`);
  expect(linked.has(siteURL + "/llms-full.txt")).toBe(true);
});

it("writes llms-full.txt with the English page's text and its questions", () => {
  const txt = readOut("llms-full.txt");
  const en = readJSON(join(siteDir, "content", "en.json"));
  expect(txt.startsWith("# Kando\n\n> " + en["metaDescription"] + "\n")).toBe(true);
  expect(txt).not.toMatch(/\{[a-zA-Z]+\}/);
  for (const [q] of faqPairs) {
    expect(txt).toContain("### " + en[q]);
  }
  expect(txt).toContain(latestURL);
  expect(txt).toContain("curl -fsSL");
  expect(txt.length).toBeGreaterThan(readOut("llms.txt").length);
});
