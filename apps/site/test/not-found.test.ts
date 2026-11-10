// TestNotFoundHasNoCurrentPage: 404.html is no language's page, so its switchers mark no link as the current page; it says it in every language, is not indexed, and runs no module script, since GitHub Pages serves it at addresses the router would read as other pages.
import { expect, it } from "vite-plus/test";
import { attr, builtLangs, pageFile, readJSON, readOut, siteDir, tags, text } from "./helpers";
import { join } from "node:path";

it("404.html marks no current page and says it in every language", () => {
  const html = readOut("404.html");
  expect(html).not.toContain("aria-current");
  expect(tags(html, "meta").some((t) => attr(t, "name") === "robots" && attr(t, "content") === "noindex")).toBe(true);
  const body = text(html);
  for (const l of builtLangs()) {
    const strs = readJSON(join(siteDir, "content", l.code + ".json"));
    expect(body, l.code).toContain(strs["notFoundTitle"]);
    expect(body, l.code).toContain(strs["notFoundHome"]);
  }
  expect(tags(html, "script").filter((t) => attr(t, "type") === "module")).toEqual([]);
  expect(tags(html, "link").filter((t) => attr(t, "rel") === "modulepreload")).toEqual([]);
  expect(html).not.toContain("$_TSR");
});

it("marks its own link as the current page twice on every page, in the header and the footer", () => {
  for (const l of builtLangs()) {
    for (const kind of ["home", "sites"] as const) {
      const html = readOut(pageFile(l, kind));
      const current = tags(html, "a").filter((t) => attr(t, "data-lang") === l.code && attr(t, "aria-current") === "page");
      expect(current.length, `${l.code} ${kind}`).toBe(2);
    }
  }
});
