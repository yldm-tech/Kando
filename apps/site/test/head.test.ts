// TestPageHead: each page says its language, names its canonical address, lists every page of its kind as an alternate with x-default for the English one, has the social preview tags with its language's link preview image, links the web app manifest, and links each other page from its switcher.
import { describe, expect, it } from "vite-plus/test";
import { join } from "node:path";
import { pngSize } from "../buildtime/icons";
import { ogSize } from "../buildtime/media";
import { pageURL, siteURL, sourceLang } from "../src/lib/site";
import { attr, builtLangs, dist, pageFile, readJSON, readOut, section, siteDir, tags } from "./helpers";

describe.each(["home", "sites"] as const)("%s pages", (kind) => {
  for (const l of builtLangs()) {
    it(l.code, () => {
      const html = readOut(pageFile(l, kind));
      const htmlTag = tags(html, "html")[0];
      expect(htmlTag && attr(htmlTag, "lang")).toBe(l.tag);
      let canonical = "";
      const alternates = new Map<string, string>();
      for (const tag of tags(html, "link")) {
        if (attr(tag, "rel") === "canonical") {
          canonical = attr(tag, "href");
        } else if (attr(tag, "rel") === "alternate") {
          alternates.set(attr(tag, "hreflang"), attr(tag, "href"));
        }
      }
      expect(canonical).toBe(pageURL(l.code, kind));
      for (const o of builtLangs()) {
        expect(alternates.get(o.tag), o.tag).toBe(pageURL(o.code, kind));
      }
      expect(alternates.get("x-default")).toBe(pageURL(sourceLang, kind));
      expect(alternates.size).toBe(builtLangs().length + 1);

      const metas = new Map<string, string>();
      for (const tag of tags(html, "meta")) {
        metas.set(attr(tag, "property") || attr(tag, "name"), attr(tag, "content"));
      }
      for (const key of [
        "description",
        "og:title",
        "og:description",
        "og:image",
        "og:image:alt",
        "og:url",
        "og:locale",
        "twitter:card",
        "twitter:image",
        "twitter:image:alt",
      ]) {
        expect(metas.get(key), key).toBeTruthy();
      }
      expect(metas.get("og:url")).toBe(pageURL(l.code, kind));
      // The link preview image is the language's own, on the sites page too, linked with a hash of its content; the file is there in the size the tags give.
      const image = metas.get("og:image") ?? "";
      expect(image).toMatch(new RegExp(`^${siteURL}/og/${l.code}\\.png\\?v=[0-9a-f]{10}$`));
      expect(metas.get("twitter:image")).toBe(image);
      expect(metas.get("twitter:card")).toBe("summary_large_image");
      expect(metas.get("og:image:type")).toBe("image/png");
      expect([metas.get("og:image:width"), metas.get("og:image:height")]).toEqual([String(ogSize.w), String(ogSize.h)]);
      expect(pngSize(join(dist(), new URL(image).pathname))).toEqual(ogSize);
      expect(metas.get("twitter:image:alt")).toBe(metas.get("og:image:alt"));
      expect(metas.get("og:image:alt")).toBe(readJSON(join(siteDir, "content", l.code + ".json"))["metaImageAlt"]);
      expect(tags(html, "link").some((t) => attr(t, "rel") === "manifest" && attr(t, "href") === "/site.webmanifest")).toBe(true);
      const locales = tags(html, "meta")
        .filter((t) => attr(t, "property") === "og:locale:alternate")
        .map((t) => attr(t, "content"));
      expect(locales.sort()).toEqual(
        builtLangs()
          .filter((o) => o.code !== l.code)
          .map((o) => o.locale)
          .sort(),
      );

      const menu = section(html, 'class="lang-menu"', "</ul>");
      for (const o of builtLangs()) {
        const link = tags(menu, "a").find((t) => attr(t, "data-lang") === o.code);
        expect(link, `switcher link to ${o.code}`).toBeDefined();
        if (link) {
          expect(attr(link, "href")).toBe(kind === "sites" ? pageURL(o.code, kind).slice(siteURL.length) : pageURL(o.code).slice(siteURL.length));
          expect(attr(link, "hreflang")).toBe(o.tag);
          expect(attr(link, "lang")).toBe(o.tag);
        }
        expect(menu).toContain(">" + o.name + "</a>");
      }
      expect(htmlTag?.attrs.has("data-root"), "data-root, which turns on the redirect by browser language, is on the English home page only").toBe(
        l.code === sourceLang && kind === "home",
      );
    });
  }
});
