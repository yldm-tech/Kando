// TestMedia: the images scripts/media.ts commits under media/ are there for every language and were made from what each shows now, so that a changed headline, template or screenshot cannot reach the site with an old picture; and each page offers its library screenshot as AVIF and WebP before the PNG, with files that exist in the widths its srcset names.
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vite-plus/test";
import { pngSize } from "../buildtime/icons";
import {
  inputsHash,
  mediaCommand,
  mediaDir,
  mediaLangs,
  ogFile,
  ogSize,
  readMediaManifest,
  shotFile,
  shotFormats,
  shotWidths,
  staleMedia,
} from "../buildtime/media";
import { siteURL } from "../src/lib/site";
import { attr, builtLangs, dist, pageFile, readOut, repoRoot, section, siteDir, tags } from "./helpers";

const langs = mediaLangs(repoRoot, siteDir, join(siteDir, "content"));

describe("media", () => {
  it("has every image, made from the inputs it has now", () => {
    const stale = staleMedia(siteDir, langs);
    expect(stale, `out of date: run ${mediaCommand} in apps/site to make these again`).toEqual([]);
  });

  it("records in its manifest the languages that have pages, and no others", () => {
    const manifest = readMediaManifest(siteDir);
    const codes = builtLangs()
      .map((l) => l.code)
      .sort();
    expect(Object.keys(manifest.og).sort()).toEqual(codes);
    expect(Object.keys(manifest.shots).sort()).toEqual(codes);
  });

  it("makes the link preview images 1200×630", () => {
    for (const l of builtLangs()) {
      expect(pngSize(join(siteDir, mediaDir, ogFile(l.code))), l.code).toEqual(ogSize);
    }
  });

  it("finds an image stale when the content it shows changes", () => {
    const m = langs[0];
    if (!m) {
      throw new Error("no languages");
    }
    const changed = { ...m, og: { ...m.og, heroTitle: m.og["heroTitle"] + " again" } };
    expect(inputsHash(changed.og)).not.toBe(inputsHash(m.og));
    expect(staleMedia(siteDir, [changed])).toEqual([join(mediaDir, ogFile(m.lang.code))]);
    const png = { ...m, shots: { ...m.shots, webp: { ...m.shots.webp, png: "0".repeat(64) } } };
    expect(staleMedia(siteDir, [png])).toEqual(shotWidths.map((w) => join(mediaDir, shotFile(m.lang.code, w, "webp"))));
  });

  it("encodes the screenshots in their formats", () => {
    for (const l of builtLangs()) {
      for (const w of shotWidths) {
        const avif = readFileSync(join(siteDir, mediaDir, shotFile(l.code, w, "avif")));
        expect(avif.toString("latin1", 4, 12), `${l.code} ${w} avif`).toBe("ftypavif");
        const webp = readFileSync(join(siteDir, mediaDir, shotFile(l.code, w, "webp")));
        expect([webp.toString("latin1", 0, 4), webp.toString("latin1", 8, 12)], `${l.code} ${w} webp`).toEqual(["RIFF", "WEBP"]);
      }
    }
  });
});

describe("library picture", () => {
  for (const l of builtLangs()) {
    it(l.code, () => {
      const picture = section(readOut(pageFile(l)), "<picture>", "</picture>");
      const sources = tags(picture, "source");
      expect(sources.map((s) => attr(s, "type"))).toEqual(shotFormats.map((f) => "image/" + f));
      for (const s of sources) {
        expect(attr(s, "sizes")).toBeTruthy();
        const candidates = attr(s, "srcset")
          .split(",")
          .map((c) => c.trim().split(/\s+/));
        expect(candidates.map((c) => c[1])).toEqual(shotWidths.map((w) => w + "w"));
        for (const [ref] of candidates) {
          const path = new URL(ref ?? "", siteURL).pathname;
          expect(existsSync(join(dist(), path)), ref).toBe(true);
        }
      }
      const img = tags(picture, "img")[0];
      expect(img && attr(img, "src")).toBe(`/shots/${l.code}/library.png`);
      const size = pngSize(join(repoRoot, "docs/assets", l.code, "library.png"));
      expect(img && [attr(img, "width"), attr(img, "height")]).toEqual([String(size.w), String(size.h)]);
      expect(img && attr(img, "loading")).toBe("lazy");
    });
  }
});
