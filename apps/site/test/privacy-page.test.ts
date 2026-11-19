// The privacy policy of each language, which the App Store listing links (apps/mobile/listing/appstore/*/privacy_url.txt): it renders every section and the contact without script, and every page's footer links it.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { pagePath, pageURL, privacyContact } from "../src/lib/site";
import { attr, builtLangs, pageFile, readJSON, readOut, repoRoot, section, siteDir, tags, text } from "./helpers";

it("server-renders every section of the policy and the contact on each language's privacy page", () => {
  for (const l of builtLangs()) {
    const strs = readJSON(join(siteDir, "content", l.code + ".json"));
    const html = readOut(pageFile(l, "privacy"));
    const body = text(section(html, "<main", "</main>"));
    for (const [key, s] of Object.entries(strs)) {
      if (key.startsWith("privacy") && !key.startsWith("privacyMeta")) {
        for (const line of s.split("\n")) {
          expect(body, `${l.code}: ${key}`).toContain(line.replace(/^- /, "").replaceAll("`", ""));
        }
      }
    }
    expect(
      tags(html, "a").some((a) => attr(a, "href") === privacyContact),
      l.code,
    ).toBe(true);
  }
});

it("links each language's privacy page from the footer of all its pages", () => {
  for (const l of builtLangs()) {
    for (const kind of ["home", "sites", "privacy", "plugin"] as const) {
      const foot = section(readOut(pageFile(l, kind)), 'class="foot"', "</footer>");
      expect(
        tags(foot, "a").some((a) => attr(a, "href") === pagePath(l.code, "privacy")),
        `${l.code} ${kind}`,
      ).toBe(true);
    }
  }
});

it("is the page the App Store listing of each language gives as its privacy policy", () => {
  const listings: Record<string, string> = { "en-US": "en", "zh-Hans": "zh-CN", "zh-Hant": "zh-TW", ja: "ja", ko: "ko" };
  for (const [dir, code] of Object.entries(listings)) {
    const l = builtLangs().find((b) => b.code === code);
    if (!l) {
      continue;
    }
    const url = readFileSync(join(repoRoot, "apps/mobile/listing/appstore", dir, "privacy_url.txt"), "utf8").trim();
    expect(url, dir).toBe(pageURL(l.code, "privacy"));
  }
});
