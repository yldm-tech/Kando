// TestPageBody: each page shows the screenshot in its own language, its images keep their space as they load, the works-with strip names the catalog's count and its featured sites, and the Download buttons and the footer link the releases.
import { join } from "node:path";
import { describe, expect, it } from "vite-plus/test";
import { latestURL, readmeURL, releasesURL } from "../src/lib/site";
import { attr, builtLangs, pageFile, readJSON, readOut, repoRoot, section, tags } from "./helpers";

const catalog = readJSON<{ sites: { featured?: boolean }[] }>(join(repoRoot, "apps/kando/sites/catalog/sites.json"));
const featured = catalog.sites.filter((s) => s.featured).length;

describe("page body", () => {
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

      // The Download buttons (the hero's, the macOS tab's and the closing one) open the newest published release, which stays current when a release comes out after the site was built; the footer lists every release.
      const downloads = tags(html, "a").filter((t) => ` ${attr(t, "class")} `.includes(" primary "));
      expect(downloads.length).toBeGreaterThanOrEqual(2);
      for (const d of downloads) {
        expect(attr(d, "href")).toBe(latestURL);
      }
      const foot = section(html, 'class="foot-links"', "</nav>");
      expect(tags(foot, "a").some((t) => attr(t, "href") === releasesURL)).toBe(true);
    });
  }
});
