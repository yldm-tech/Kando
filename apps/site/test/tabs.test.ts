// TestTabStacks: choosing another tab never changes a section's height. Each tab group's panels are the children of one stack element and nothing else is, the first panel is the one shown and the others are marked data-off, and the stylesheet lays every panel of a stack in one grid cell once the page runs script, so that the stack is as tall as its tallest panel; the panel that arrives comes in with the design's k-panel, and none of it runs where the reader asked for less motion.
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vite-plus/test";
import { attr, builtLangs, dist, pageFile, parseTag, readOut, tags } from "./helpers";

// stacks are the start tags of the page's tab stacks with the start tags of their child elements. The children are found by counting the div elements the stack holds, as the stack and its panels are divs.
function stacks(html: string): { stack: string; children: ReturnType<typeof parseTag>[] }[] {
  const out: { stack: string; children: ReturnType<typeof parseTag>[] }[] = [];
  for (const m of html.matchAll(/<div class="tab-stack"[^>]*>/g)) {
    const children: ReturnType<typeof parseTag>[] = [];
    const re = /<(\/?)div\b[^>]*>/g;
    re.lastIndex = (m.index ?? 0) + m[0].length;
    let depth = 1;
    for (let t = re.exec(html); t && depth > 0; t = re.exec(html)) {
      if (t[1] === "/") {
        depth--;
        continue;
      }
      if (depth === 1) {
        children.push(parseTag(t[0], t.index));
      }
      depth++;
    }
    out.push({ stack: m[0], children });
  }
  return out;
}

// css is the built stylesheet, with its whitespace taken out.
function css(): string {
  const dir = join(dist(), "assets");
  return readdirSync(dir)
    .filter((f) => f.endsWith(".css"))
    .map((f) => readFileSync(join(dir, f), "utf8"))
    .join("\n")
    .replace(/\s+/g, "");
}

describe("tab stacks", () => {
  for (const l of builtLangs()) {
    it(l.code, () => {
      const html = readOut(pageFile(l));
      const found = stacks(html);
      const lists = tags(html, "div").filter((t) => attr(t, "role") === "tablist");
      expect(lists.length, "the player's side panel, the command line's terminal and the downloads").toBe(3);
      expect(found.length).toBe(lists.length);
      const tabs = tags(html, "button").filter((t) => attr(t, "role") === "tab");
      for (const { stack, children } of found) {
        expect(stack, "a stack is not marked moved before the reader chooses a tab").not.toContain("data-moved");
        expect(children.length).toBeGreaterThan(1);
        for (const [i, child] of children.entries()) {
          expect(attr(child, "role"), `a stack holds tab panels only: ${child.raw}`).toBe("tabpanel");
          expect(child.attrs.has("hidden"), `a panel kept out of the layout would let the stack shrink: ${child.raw}`).toBe(false);
          expect(child.attrs.has("data-off"), `only the first panel is shown: ${child.raw}`).toBe(i > 0);
          expect(tabs.some((t) => attr(t, "aria-controls") === attr(child, "id"))).toBe(true);
        }
      }
      // Every panel a tab controls is in a stack.
      const stacked = new Set(found.flatMap((s) => s.children.map((c) => attr(c, "id"))));
      for (const t of tabs) {
        expect(stacked.has(attr(t, "aria-controls")), `${attr(t, "id")}'s panel is not in a stack`).toBe(true);
      }
    });
  }

  it("lays a stack's panels in one cell", () => {
    const sheet = css();
    expect(sheet).toContain(".js.tab-stack{display:grid}");
    expect(sheet).toMatch(/\.js\.tab-stack>\[role=("?)tabpanel\1\]\{grid-area:1\/1[;}]/);
    expect(sheet).toMatch(/\.js\.tab-stack>\[data-off\]\{[^}]*visibility:hidden/);
    expect(sheet).toMatch(/\.js\.tab-stack\[data-moved\]>\[role=("?)tabpanel\1\]:not\(\[data-off\]\)\{animation:k-panel\.45svar\(--ease\)both\}/);
    const from = /@keyframesk-panel\{(?:from|0%)\{([^}]*)\}/.exec(sheet)?.[1] ?? "";
    for (const decl of ["opacity:0", "transform:translateY(12px)", "filter:blur(6px)"]) {
      expect(from.split(";"), `k-panel starts with ${decl}`).toContain(decl);
    }
    // The stack must not be taken out of the layout by a display rule on its panels.
    expect(sheet).not.toMatch(/\[role=("?)tabpanel\1\]\[hidden\]/);
  });
});
