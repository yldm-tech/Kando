// The header's section links follow the page's order: each one's section comes after the one before it, so the pill that marks the section being read moves left to right as the page scrolls down, and a link's section is never shadowed by an earlier link's section placed after it.
import { expect, it } from "vite-plus/test";
import { builtLangs, decode, pageFile, readOut } from "./helpers";

it("lists the header's sections in the page's order", () => {
  for (const l of builtLangs()) {
    const html = readOut(pageFile(l));
    const header = html.slice(html.indexOf("<header"), html.indexOf("</header>"));
    const ids = [...header.matchAll(/<a [^>]*href="#([^"]+)"/g)].map((m) => decode(m[1] ?? ""));
    expect(ids.length, l.code).toBeGreaterThan(1);
    const at = ids.map((id) => html.indexOf(` id="${id}"`, html.indexOf("</header>")));
    for (const [i, pos] of at.entries()) {
      expect(pos, `${l.code}: no section #${ids[i]}`).toBeGreaterThan(0);
    }
    expect(at, `${l.code}: ${ids.join(", ")}`).toEqual([...at].sort((a, b) => a - b));
  }
});
