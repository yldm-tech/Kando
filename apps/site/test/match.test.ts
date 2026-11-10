// lib/match.ts, the sites page's search, matches as static/webmcp.js's search_supported_sites does: both are the app's catalog.Site.Matches.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import vm from "node:vm";
import { expect, it } from "vite-plus/test";
import { linkHost, siteMatches, type MatchSite } from "../src/lib/match";
import { readJSON, repoRoot, siteDir } from "./helpers";

// webmcpMatch loads webmcp.js's own matcher: the functions are inside its closure, so their source is cut out and run alone.
function webmcpMatch(): { linkHost: (q: string) => string; siteMatches: (s: MatchSite, q: string) => boolean } {
  const src = readFileSync(join(siteDir, "static/webmcp.js"), "utf8");
  const start = src.indexOf("  function linkHost(query)");
  const end = src.indexOf("  // otherDomains");
  expect(start).toBeGreaterThan(0);
  const ctx = vm.createContext({ URL });
  vm.runInContext(src.slice(start, end) + "\nthis.linkHost = linkHost; this.siteMatches = siteMatches;", ctx);
  return ctx as unknown as ReturnType<typeof webmcpMatch>;
}

it("matches as webmcp.js does", () => {
  const theirs = webmcpMatch();
  const sites = readJSON<{ sites: MatchSite[] }>(join(repoRoot, "apps/kando/sites/catalog/sites.json")).sites;
  const queries = [
    "哔哩",
    "NICONICO",
    "x.com",
    "https://www.bilibili.com/video/BV1xx411c7mD",
    "https://youtu.be/dQw4w9WgXcQ",
    "www.youtube.com/watch?v=1",
    "tv",
    "news bbc",
    "",
    "  ",
    "https://unknown.example/1",
    "vimeo.com",
  ];
  for (const q of queries) {
    expect(linkHost(q), q).toBe(theirs.linkHost(q));
    const ours = sites.filter((s) => siteMatches(s, q)).map((s) => s.domain);
    const want = sites.filter((s) => theirs.siteMatches(s, q)).map((s) => s.domain);
    expect(ours, q).toEqual(want);
  }
});
