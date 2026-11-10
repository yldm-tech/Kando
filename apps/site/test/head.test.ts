// TestPageHead: each page says its language, names its canonical address, lists every page of its kind as an alternate with x-default for the English one, has the social preview tags, and links each other page from its switcher.
import { describe, expect, it } from "vite-plus/test";
import { pageURL, siteURL, sourceLang } from "../src/lib/site";
import { attr, builtLangs, pageFile, readOut, section, tags } from "./helpers";

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
      for (const key of ["description", "og:title", "og:description", "og:image", "og:image:alt", "og:url", "og:locale", "twitter:card", "twitter:image"]) {
        expect(metas.get(key), key).toBeTruthy();
      }
      expect(metas.get("og:url")).toBe(pageURL(l.code, kind));
      const image = `${siteURL}/shots/${l.code}/library.png`;
      expect(metas.get("og:image")).toBe(image);
      expect(metas.get("twitter:image")).toBe(image);
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
