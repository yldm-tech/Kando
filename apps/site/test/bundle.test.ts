// Three.js and the chart stay out of what a page loads first: the scripts a page names and preloads hold no WebGL renderer and no chart surface, so the hero's backdrop and the models' chart can only arrive in chunks loaded later.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { attr, dist, htmlFiles, readOut, tags } from "./helpers";

it("keeps Three.js and the chart out of the scripts a page loads first", () => {
  for (const file of htmlFiles()) {
    const html = readOut(file);
    const initial = [
      ...tags(html, "script").map((t) => attr(t, "src")),
      ...tags(html, "link")
        .filter((t) => attr(t, "rel") === "modulepreload")
        .map((t) => attr(t, "href")),
    ].filter((src) => src.startsWith("/assets/"));
    for (const src of initial) {
      const code = readFileSync(join(dist(), src.slice(1)), "utf8");
      expect(code.includes("WebGLRenderer"), `${file}: ${src} has Three.js`).toBe(false);
      expect(code.includes("ts-chart-surface"), `${file}: ${src} has the chart`).toBe(false);
    }
  }
});
