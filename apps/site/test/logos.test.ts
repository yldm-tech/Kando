// TestLogoStripNames and TestSiteNamesRejectUnknownDomain: the names of the logo strip are written in the page's script, and a name override for a domain that is not a featured site fails the build.
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vite-plus/test";
import { loadSiteNames } from "../buildtime/catalog";
import { languages } from "../src/lib/site";
import { attr, builtLangs, pageFile, readOut, section, tags } from "./helpers";

const han = /\p{Script=Han}/u;

describe("logo strip names", () => {
  for (const l of builtLangs()) {
    it(l.code, () => {
      const strip = section(readOut(pageFile(l)), 'class="logos"', "</ul>");
      if (l.code === "zh-TW") {
        for (const m of strip.matchAll(/\s(?:title|alt|aria-label)="([^"]*)"/gi)) {
          expect(/[哔红视爱国优腾讯鱼]/.test(m[1] ?? ""), `Simplified Chinese site name ${m[1]} on the Traditional Chinese page`).toBe(false);
        }
        expect(strip).toContain('title="嗶哩嗶哩"');
      }
      for (const li of tags(strip, "li")) {
        const name = attr(li, "title");
        if (!name) {
          continue;
        }
        const want = han.test(name) && !l.tag.startsWith("zh") ? "zh-Hans" : "";
        expect(attr(li, "lang"), name).toBe(want);
      }
    });
  }
});

it("rejects a site name for a domain that is not a featured site", () => {
  const dir = mkdtempSync(join(tmpdir(), "kando-names-"));
  try {
    writeFileSync(join(dir, "sites.zh-TW.json"), JSON.stringify({ "bilibil.com": "嗶哩嗶哩" }));
    expect(() => loadSiteNames(dir, languages, [{ name: "哔哩哔哩", domain: "bilibili.com" }])).toThrow(/not a featured site/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
