// TestEveryKeyUsed: every key of en.json is shown somewhere, so that no string is translated for nothing. The pages look keys up through a typed t(), so a key that does not exist fails vp check; this test checks the other way, that each key is named in the pages' or the build's source.
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

it("uses every key of en.json", () => {
  const en = readJSON(join(siteDir, "content", "en.json"));
  const code = sources("src") + sources("buildtime");
  const unused = Object.keys(en).filter((key) => !code.includes(`"${key}"`));
  expect(unused).toEqual([]);
});
