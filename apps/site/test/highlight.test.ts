// The download panel's commands are coloured as the design's terminals are, and putting the tokens back together gives the command unchanged.
import { expect, it } from "vite-plus/test";
import { highlight } from "../src/lib/highlight";
import { builtLangs, decode, pageFile, readOut } from "./helpers";

it("colours the command, subcommand, flags, links, placeholders and operators", () => {
  const kinds = (line: string) =>
    highlight(line)
      .filter((t) => t.kind !== "")
      .map((t) => `${t.kind}:${t.text}`);
  expect(kinds("curl -fsSL https://github.com/yldm-tech/Kando/releases/latest/download/install.sh | sh")).toEqual([
    "k:curl",
    "f:-fsSL",
    "u:https://github.com/yldm-tech/Kando/releases/latest/download/install.sh",
    "o:|",
    "k:sh",
  ]);
  expect(kinds("go tool mygo build apps/kando")).toEqual(["k:go", "s:tool", "u:apps/kando"]);
  expect(kinds("kando subtitles <film-id> --format text")).toEqual(["k:kando", "s:subtitles", "h:<film-id>", "f:--format"]);
  expect(kinds("# a comment")).toEqual(["c:# a comment"]);
  for (const line of ["curl -fsSL https://x.test/a | sh", "  cd  Kando ", "# note", ""]) {
    expect(
      highlight(line)
        .map((t) => t.text)
        .join(""),
    ).toBe(line);
  }
});

it("shows the download panel's commands coloured, each line after a prompt", () => {
  for (const l of builtLangs()) {
    const html = readOut(pageFile(l));
    const blocks = [...html.matchAll(/<div class="code"><pre><code>([\s\S]*?)<\/code><\/pre>/g)].map((m) => m[1] ?? "");
    expect(blocks.length, l.code).toBeGreaterThanOrEqual(2);
    for (const block of blocks) {
      expect(block, l.code).toMatch(/<span class="k">/);
      const text = decode(block.replace(/<!-- -->/g, "").replace(/<[^>]+>/g, ""));
      for (const line of text.split("\n")) {
        expect(line.startsWith("$ "), `${l.code}: ${line}`).toBe(true);
      }
    }
    const linux = decode((blocks[0] ?? "").replace(/<!-- -->/g, "").replace(/<[^>]+>/g, "")).replace(/^\$ /, "");
    expect(linux, l.code).toMatch(/^curl -fsSL https:\/\/github\.com\/yldm-tech\/Kando\/releases\/latest\/download\/install\.sh \| sh$/);
  }
});
