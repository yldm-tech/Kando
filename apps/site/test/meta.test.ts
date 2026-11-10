// TestMetaLengths: every page's title and description are its own, none shared with another page, and as long as search results show them whole: notes.json asks for 150 to 160 characters of description in English and 80 to 90 in Chinese, Japanese and Korean, whose characters are about twice as wide, and the titles stay under the about 60 Latin or 30 CJK characters a result shows.
import { describe, expect, it } from "vite-plus/test";
import { attr, builtLangs, decode, pageFile, readOut, tags } from "./helpers";

// limits are the lengths, in characters, that a page's title and description keep within, by whether its language is written in Latin letters.
const limits = {
  latin: { title: [20, 60], description: [120, 160] },
  cjk: { title: [10, 32], description: [70, 90] },
} as const;

function head(file: string): { title: string; description: string } {
  const html = readOut(file);
  const title = decode(html.match(/<title>([^<]*)<\/title>/)?.[1] ?? "");
  const meta = tags(html, "meta").find((t) => attr(t, "name") === "description");
  return { title, description: meta ? attr(meta, "content") : "" };
}

const pages = builtLangs().flatMap((l) => (["home", "sites"] as const).map((kind) => ({ l, kind, file: pageFile(l, kind) })));

describe("titles and descriptions", () => {
  for (const { l, kind, file } of pages) {
    it(`${l.code} ${kind}`, () => {
      const { title, description } = head(file);
      const limit = l.code === "en" ? limits.latin : limits.cjk;
      // Characters as a reader counts them, one per grapheme.
      const segmenter = new Intl.Segmenter(l.tag, { granularity: "grapheme" });
      const len = (s: string) => [...segmenter.segment(s)].length;
      expect(len(title), `title ${JSON.stringify(title)}`).toBeGreaterThanOrEqual(limit.title[0]);
      expect(len(title), `title ${JSON.stringify(title)}`).toBeLessThanOrEqual(limit.title[1]);
      expect(len(description), `description ${JSON.stringify(description)}`).toBeGreaterThanOrEqual(limit.description[0]);
      expect(len(description), `description ${JSON.stringify(description)}`).toBeLessThanOrEqual(limit.description[1]);
    });
  }

  it("gives every page a title and a description of its own", () => {
    const heads = pages.map((p) => head(p.file));
    expect(new Set(heads.map((h) => h.title)).size).toBe(pages.length);
    expect(new Set(heads.map((h) => h.description)).size).toBe(pages.length);
  });
});
