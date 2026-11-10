// Three.js stays out of what a page loads first: the scripts a page names and preloads hold no WebGL renderer, so the hero's backdrop can only arrive in a chunk loaded later.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { attr, dist, htmlFiles, readOut, tags } from "./helpers";

it("keeps Three.js out of the scripts a page loads first", () => {
  for (const file of htmlFiles()) {
    const html = readOut(file);
    const initial = [
      ...tags(html, "script").map((t) => attr(t, "src")),
      ...tags(html, "link")
        .filter((t) => attr(t, "rel") === "modulepreload")
        .map((t) => attr(t, "href")),
    ].filter((src) => src.startsWith("/assets/"));
    for (const src of initial) {
      expect(readFileSync(join(dist(), src.slice(1)), "utf8").includes("WebGLRenderer"), `${file}: ${src}`).toBe(false);
    }
  }
});
