// TestWebMCPPageData, TestReleaseDownloads, TestWebMCPSites, TestWebMCPContentCoversPage and TestWebMCPScript: the WebMCP tools' data, in the page and in the files they fetch, and static/webmcp.js answering each tool through testdata/webmcp_harness.cjs.
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { describe, expect, it } from "vite-plus/test";
import { appVersion, readSurface } from "../buildtime/app";
import { loadContent } from "../buildtime/content";
import { agentKeys, newAgentContent, releaseDownloads, type AgentContent } from "../buildtime/webmcp";
import type { AgentData } from "../buildtime/schema";
import { language, releasesURL } from "../src/lib/site";
import { attr, builtLangs, dist, pageFile, readJSON, readOut, repoRoot, siteDir, tags } from "./helpers";

const block = /<script type="application\/json" id="webmcp-data">([^<]*)<\/script>/;

function pageAgentData(file: string): AgentData {
  const m = readOut(file).match(block);
  if (!m) {
    throw new Error(`${file} has no webmcp-data block`);
  }
  return JSON.parse(m[1] ?? "") as AgentData;
}

// outFile is the built file a link the page fetches names, without its query.
function outFile(link: string): string {
  return join(dist(), (link.split("?")[0] ?? "").slice(1));
}

describe("page data", () => {
  const version = appVersion(repoRoot);
  for (const l of builtLangs()) {
    it(l.code, () => {
      const html = readOut(pageFile(l));
      expect(tags(html, "script").some((t) => /^\/static\/webmcp\.js\?v=[0-9a-f]+$/.test(attr(t, "src")) && t.attrs.has("defer"))).toBe(true);
      const data = pageAgentData(pageFile(l));
      expect(data.lang).toBe(l.code);
      expect(data.langs.length).toBe(builtLangs().length);
      for (const al of data.langs) {
        const c = JSON.parse(readFileSync(outFile(al.content), "utf8")) as AgentContent;
        // get_started's links download this release's files, as the page's buttons do: the macOS one the disk image for Apple silicon.
        expect(c.start.steps[0]?.link?.url, al.code).toBe(`${releasesURL}/download/v${version}/kando-${version}-arm64.dmg`);
      }
      expect(existsSync(outFile(data.sites))).toBe(true);
      const rel = data.release;
      expect(rel.version).toBe(version);
      expect(rel.page).toBe(`${releasesURL}/tag/v${version}`);
      expect(rel.checksums).toBe(`${releasesURL}/download/v${version}/checksums.txt`);
      const kinds = new Set<string>();
      for (const d of rel.downloads) {
        expect(d.url).toBe(`${releasesURL}/download/v${version}/${d.name}`);
        kinds.add(d.kind);
      }
      for (const kind of kinds) {
        expect(rel.install[kind], kind).toBeTruthy();
      }
    });
  }
});

it("offers the files the Release workflow publishes", () => {
  const rel = releaseDownloads("0.1.0");
  expect(rel.downloads.map((d) => `${d.platform}/${d.arch} ${d.name}`)).toEqual([
    "macos/arm64 kando-0.1.0-arm64.dmg",
    "macos/amd64 kando-0.1.0-amd64.dmg",
    "linux/amd64 kando_0.1.0_amd64.deb",
    "linux/amd64 kando-0.1.0-linux-amd64.tar.gz",
    "linux/arm64 kando_0.1.0_arm64.deb",
    "linux/arm64 kando-0.1.0-linux-arm64.tar.gz",
    "windows/amd64 kando-setup-0.1.0-amd64.exe",
    "windows/arm64 kando-setup-0.1.0-arm64.exe",
  ]);
  expect(rel.install["tar.gz"]).toContain(`${releasesURL}/download/v0.1.0/install.sh`);
});

it("writes the app's whole catalog to webmcp/sites.json", () => {
  type Sites = { ytdlp: string; sites: { name: string; domain: string; aliases?: string[] }[] };
  const want = readJSON<Sites>(join(repoRoot, "apps/kando/sites/catalog/sites.json"));
  const first = builtLangs()[0];
  if (!first) {
    throw new Error("no languages");
  }
  const got = JSON.parse(readFileSync(outFile(pageAgentData(pageFile(first)).sites), "utf8")) as Sites;
  expect(got.ytdlp).toBe(want.ytdlp);
  expect(got.sites.length).toBe(want.sites.length);
  want.sites.forEach((s, i) => {
    const g = got.sites[i];
    expect({ name: g?.name, domain: g?.domain, aliases: g?.aliases ?? [] }).toEqual({ name: s.name, domain: s.domain, aliases: s.aliases ?? [] });
  });
  expect(readOut("webmcp/sites.json")).toContain('"domain":"youtube.com","aliases":["youtu.be"');
});

// rendered are the component files a section's component renders: its own file and every component file it imports, statically or lazily, followed through their imports in turn.
function rendered(file: string): string[] {
  const dir = join(siteDir, "src/components");
  const seen = new Set<string>();
  const visit = (path: string) => {
    if (seen.has(path)) {
      return;
    }
    seen.add(path);
    const src = readFileSync(path, "utf8");
    for (const m of src.matchAll(/(?:from\s+|import\()\s*"(\.{1,2}\/[^"]+)"/g)) {
      const base = resolve(dirname(path), m[1] ?? "");
      const found = [".tsx", ".ts"].map((ext) => base + ext).find((f) => f.startsWith(dir) && existsSync(f));
      if (found) {
        visit(found);
      }
    }
  };
  visit(join(dir, file));
  return [...seen];
}

// The tools' content files say everything the page's Get started and features sections say, so that get_started and list_features do not fall behind a card added to the page. Each section is a component; the keys it and the components it renders name are compared with the keys newAgentContent reads.
it("covers in the WebMCP content files what the page's sections say", () => {
  const content = loadContent(join(siteDir, "content"));
  const en = content.get("en") ?? {};
  newAgentContent(language("en"), en, "0.1.0", 1, readSurface(repoRoot), [{ name: "Tiny", stats: "Speed 5/5  Accuracy 2/5  77.7 MB" }]);
  // Labels of controls, the small lines above headings, a screenshot's description, the comments of the terminal example, the open-source panel's own links and the models table's caption and column headings and the chart's description and caption, which the tools have no use for: the models item gives each model's speed, accuracy and size in the app's words instead.
  const skip = new Set([
    "copyLabel",
    "copiedLabel",
    "dlTabsLabel",
    "cliTabsLabel",
    "cliTabCommands",
    "libraryEyebrow",
    "cliEyebrow",
    "detailsEyebrow",
    "modelsEyebrow",
    "shotLibraryAlt",
    "cliCommentAdd",
    "cliCommentList",
    "cliCommentSubtitles",
    "cliCommentStatus",
    "mcpCommentAdd",
    "mcpCommentTools",
    "detailModesMark",
    "detailLanguagesMark",
    "modelsTableCaption",
    "modelsColumnModel",
    "modelsAxis",
    "modelsChartLabel",
    "modelsChartCaption",
    "openEyebrow",
    "openGitHub",
    "openReleases",
    // The buttons for each tab's second architecture; get_started links each system's main file, and get_download gives every file by architecture.
    "dlMacIntel",
    "dlLinuxArm",
    "dlWinArm",
  ]);
  const sections = { library: "library/Library.tsx", cli: "cli/Cli.tsx", features: "details/Details.tsx", start: "open-source/OpenSource.tsx" };
  const missing: string[] = [];
  for (const [id, file] of Object.entries(sections)) {
    const files = rendered(file);
    if (id === "features") {
      expect(
        files.some((f) => f.endsWith("models/Models.tsx")),
        "the features section's files include the models card",
      ).toBe(true);
    }
    for (const path of files) {
      for (const m of readFileSync(path, "utf8").matchAll(/"(\w+)"/g)) {
        const key = m[1] ?? "";
        if (key in en && !skip.has(key) && !agentKeys.has(key)) {
          missing.push(`#${id} shows ${key} (${relative(siteDir, path)}), which the WebMCP content files leave out (newAgentContent)`);
        }
      }
    }
  }
  expect([...new Set(missing)]).toEqual([]);
});

describe("webmcp.js", () => {
  for (const l of builtLangs()) {
    it(`answers every tool on the ${l.code} page`, () => {
      const run = spawnSync(
        process.execPath,
        [join(siteDir, "testdata/webmcp_harness.cjs"), join(dist(), "static/webmcp.js"), dist(), pageFile(l), join(siteDir, "content")],
        {
          encoding: "utf8",
          timeout: 30_000,
        },
      );
      expect(run.status, run.stderr + run.stdout).toBe(0);
    });
  }
});
