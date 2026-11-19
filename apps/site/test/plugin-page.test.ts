// The plugin page of each language is rendered with its heading, the list loading and the way to add a plugin; the list itself is fetched once the page runs, so no entry of an index is in the HTML. The footer of every page links the plugin page of its language.
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { pagePath, pluginsGuideURL, pluginsRepoURL } from "../src/lib/site";
import { attr, builtLangs, pageFile, readJSON, readOut, section, siteDir, tags, text } from "./helpers";

it("renders the plugin page's shell, loading, without the index", () => {
  for (const l of builtLangs()) {
    const html = readOut(pageFile(l, "plugin"));
    const strs = readJSON(join(siteDir, "content", l.code + ".json"));
    const body = text(html);
    const h1 = section(html, 'id="plugin-title"', "</h1>");
    expect(text(h1.slice(h1.indexOf(">") + 1)), l.code).toBe(strs["pluginPageTitle"]);
    expect(body, l.code).toContain(strs["pluginLoading"]);
    expect(body, l.code).toContain(strs["pluginContributeTitle"]);
    const links = tags(html, "a").map((t) => attr(t, "href"));
    expect(links, l.code).toContain(pluginsGuideURL);
    expect(links, l.code).toContain(pluginsRepoURL);
    // No entry is rendered: no card, no link to open one in Kando, and nothing of the index's file.
    expect(html).not.toContain("kando://");
    expect(html).not.toContain("plugin-card");
    expect(html).not.toContain("kando-plugins-index");
  }
});

it("links the plugin page of its language from every page's footer", () => {
  for (const l of builtLangs()) {
    for (const kind of ["home", "sites", "plugin"] as const) {
      const foot = section(readOut(pageFile(l, kind)), 'class="foot-links"', "</nav>");
      const link = tags(foot, "a").find((t) => attr(t, "href") === pagePath(l.code, "plugin"));
      expect(link, `${l.code} ${kind}`).toBeDefined();
      expect(link && attr(link, "aria-current"), `${l.code} ${kind}`).toBe(kind === "plugin" ? "page" : "");
    }
  }
});
