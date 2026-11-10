// TestEveryKeyUsed: every key of en.json is shown on a page, so that no string is translated for nothing. The pages look keys up through a typed t(), so a key that does not exist fails vp check; this test checks the other way, that each key is named in the pages' source. As in the Go site, where the templates' lookups (Used) and the WebMCP files' (AgentUsed) were counted apart, the keys only buildtime/webmcp.ts reads do not count: a string the page stops showing fails here even if the agents still read it.
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { readJSON, siteDir } from "./helpers";

function sources(dir: string): string {
  return readdirSync(join(siteDir, dir), { recursive: true, encoding: "utf8" })
    .filter((f) => /\.(ts|tsx)$/.test(f) && !f.endsWith("routeTree.gen.ts"))
    .map((f) => readFileSync(join(siteDir, dir, f), "utf8"))
    .join("\n");
}

it("uses every key of en.json on the pages", () => {
  const en = readJSON(join(siteDir, "content", "en.json"));
  // The page components, and collect.ts, which puts 404.html's strings of every language into the site module its component reads.
  const code = sources("src") + readFileSync(join(siteDir, "buildtime", "collect.ts"), "utf8");
  const unused = Object.keys(en).filter((key) => !code.includes(`"${key}"`));
  expect(unused).toEqual([]);
});
