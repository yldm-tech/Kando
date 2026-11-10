// The sites page of each language lists the app's whole catalog without script.
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { attr, builtLangs, pageFile, readJSON, readOut, repoRoot, section, tags } from "./helpers";

it("server-renders every site of the catalog on each language's sites page", () => {
  const catalog = readJSON<{ sites: { domain: string }[] }>(join(repoRoot, "apps/kando/sites/catalog/sites.json"));
  for (const l of builtLangs()) {
    const html = readOut(pageFile(l, "sites"));
    const body = section(html, 'class="sites-table"', "</table>");
    const rows = tags(body, "tr").length - 1;
    expect(rows, l.code).toBe(catalog.sites.length);
    expect(html).toContain(String(catalog.sites.length));
    if (!l.tag.startsWith("zh")) {
      const bili = section(body, "哔哩哔哩", "</th>");
      expect(bili.length).toBeGreaterThan(0);
    }
    const names = tags(body, "th").filter((t) => attr(t, "scope") === "row");
    expect(names.length).toBe(catalog.sites.length);
  }
});
