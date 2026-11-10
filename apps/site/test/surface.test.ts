// TestSurfaceFollowsTheApp: the Command line and MCP section counts the kando command's commands and lists every tool of its MCP server, as the app's source has them.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { builtLangs, pageFile, readJSON, readOut, repoRoot, siteDir, text } from "./helpers";

it("follows the app's commands and tools", () => {
  const cli = readFileSync(join(repoRoot, "apps/kando/cli/cli.go"), "utf8");
  const tools = readFileSync(join(repoRoot, "apps/kando/mcpserver/tools.go"), "utf8");
  const commands = cli.split('{name: "').length - 1;
  const names = [...tools.matchAll(/Name:\s+"(kando_\w+)"/g)].map((m) => m[1] ?? "");
  expect(commands).toBeGreaterThanOrEqual(10);
  expect(names.length).toBeGreaterThanOrEqual(5);
  for (const l of builtLangs()) {
    const html = readOut(pageFile(l));
    const strs = readJSON(join(siteDir, "content", l.code + ".json"));
    const point = (strs["cliPoint1"] ?? "").replaceAll("{commands}", String(commands)).replaceAll("`", "");
    expect(text(html), l.code).toContain(point);
    for (const name of names) {
      expect(html, `${l.code}: the MCP tab lists ${name}`).toContain(`<code>${name}</code>`);
    }
  }
});
