// TestPageBody: each page shows the screenshot in its own language, its images keep their space as they load, the works-with strip names the catalog's count and its featured sites, and the Download buttons and the footer link the releases.
import { join } from "node:path";
import { describe, expect, it } from "vite-plus/test";
import { appVersion } from "../buildtime/app";
import { readmeURL, releasesURL } from "../src/lib/site";
import { attr, builtLangs, pageFile, readJSON, readOut, repoRoot, section, tags } from "./helpers";

const catalog = readJSON<{ sites: { featured?: boolean }[] }>(join(repoRoot, "apps/kando/sites/catalog/sites.json"));
const featured = catalog.sites.filter((s) => s.featured).length;

describe("page body", () => {
  const version = appVersion(repoRoot);
  for (const l of builtLangs()) {
    it(l.code, () => {
      const html = readOut(pageFile(l));
      expect(tags(html, "img").some((t) => attr(t, "src") === `/shots/${l.code}/library.png`)).toBe(true);
      for (const img of [...tags(html, "img"), ...tags(readOut(pageFile(l, "sites")), "img")]) {
        expect(attr(img, "width") && attr(img, "height"), `image without width and height, which shifts the layout as it loads: ${img.raw}`).toBeTruthy();
        expect(img.attrs.has("alt"), `image without alt: ${img.raw}`).toBe(true);
      }
      const strip = section(html, 'class="logos"', "</ul>");
      expect((strip.match(/<li title=/g) ?? []).length).toBe(featured);
      expect(html).toContain(String(catalog.sites.length));
      expect(tags(html, "a").some((t) => attr(t, "href") === readmeURL(l))).toBe(true);

      // The Download buttons fetch this release's files rather than opening its page: the hero's and the closing one the disk image for Apple silicon, and each tab of the download panel its system's packages, one per architecture. The footer lists every release.
      const files = `${releasesURL}/download/v${version}/`;
      const downloads = tags(html, "a")
        .map((t) => attr(t, "href"))
        .filter((href) => href.startsWith(files))
        .map((href) => href.slice(files.length));
      const mac = `kando-${version}-arm64.dmg`;
      expect(downloads).toEqual([
        mac,
        mac,
        `kando-${version}-amd64.dmg`,
        `kando_${version}_amd64.deb`,
        `kando_${version}_arm64.deb`,
        `kando-setup-${version}-amd64.exe`,
        `kando-setup-${version}-arm64.exe`,
        mac,
      ]);
      expect(tags(html, "a").some((t) => attr(t, "href").endsWith("/releases/latest"))).toBe(false);
      const foot = section(html, 'class="foot-links"', "</nav>");
      expect(tags(foot, "a").some((t) => attr(t, "href") === releasesURL)).toBe(true);
    });
  }
});
