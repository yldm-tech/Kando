// TestLocalLinksResolve: every local href and src names a file of the built site, and every #fragment an element of the page it points into. Links are absolute paths, so that 404.html works at any address.
import { existsSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { decode, dist, htmlFiles, isExternal, readOut } from "./helpers";

const refRe = /\s(href|src|action|srcset|poster)="([^"]*)"/gi;

it("resolves every local link", () => {
  const ids = new Map<string, Set<string>>();
  for (const file of htmlFiles()) {
    ids.set(file, new Set([...readOut(file).matchAll(/\sid="([^"]+)"/g)].map((m) => decode(m[1] ?? ""))));
  }
  const bad: string[] = [];
  let checked = 0;
  for (const file of htmlFiles()) {
    for (const m of readOut(file).matchAll(refRe)) {
      const name = m[1] ?? "";
      const raw = decode(m[2] ?? "");
      if (isExternal(raw) || raw.startsWith("mailto:")) {
        continue;
      }
      checked++;
      const [beforeHash = "", fragment = ""] = raw.split("#", 2);
      const ref = beforeHash.split("?")[0] ?? "";
      let target = file;
      if (ref !== "") {
        if (!ref.startsWith("/")) {
          bad.push(`${file}: ${name}="${raw}" is relative`);
          continue;
        }
        target = ref.slice(1);
        if (target === "" || target.endsWith("/")) {
          target += "index.html";
        }
        if (!existsSync(join(dist(), target))) {
          bad.push(`${file}: ${name}="${raw}" names no file of the site`);
          continue;
        }
      }
      if (fragment !== "" && !ids.get(target)?.has(fragment)) {
        bad.push(`${file}: ${name}="${raw}": ${target} has no element with id ${fragment}`);
      }
    }
  }
  expect(bad).toEqual([]);
  expect(checked).toBeGreaterThan(100);
});
