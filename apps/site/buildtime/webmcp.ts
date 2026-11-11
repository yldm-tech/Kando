// The page offers WebMCP tools to AI agents in the browser (static/webmcp.js): whether Kando downloads from a website, the download for the visitor's system, and the getting-started steps and features in any of the page's languages. What the tools answer from is written here, as files the script fetches only when an agent calls a tool, so that the page itself grows only by the download table it embeds (AgentData).
import { createHash } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import {
  everyAPIURL,
  installCommand,
  debPackage,
  linuxArchive,
  macDisk,
  pageURL,
  pagePath,
  readmeBuildURL,
  readmeURL,
  releaseFileURL,
  releasesURL,
  windowsSetup,
  type Language,
  type LangCode,
} from "../src/lib/site";
import type { Catalog } from "./catalog";
import type { Strings } from "./content";
import type { AgentData, AgentRelease } from "./schema";
import type { Surface } from "./app";

// agentDir is the folder of the built site the tools' files are in.
export const agentDir = "webmcp";

// hashLink is the path a file is linked by, with a hash of its content in the query, so that a changed file is fetched again.
export function hashLink(path: string, data: string | Uint8Array): string {
  return path + "?v=" + createHash("sha256").update(data).digest("hex").slice(0, 10);
}

// releaseDownloads are the packages the Release workflow publishes for version, under the names mygo build gives them (.github/workflows/package.yml): a disk image for each Mac, an installer for each Windows architecture, and for each Linux architecture a Debian package and an archive that the release's install.sh installs.
export function releaseDownloads(version: string): AgentRelease {
  const tag = "v" + version;
  const base = releaseFileURL(version, "");
  const downloads: AgentRelease["downloads"] = [];
  const add = (platform: string, arch: string, kind: string, name: string) =>
    downloads.push({ platform, arch, kind, name, url: releaseFileURL(version, name) });
  for (const arch of ["arm64", "amd64"]) {
    add("macos", arch, "dmg", macDisk(version, arch));
  }
  for (const arch of ["amd64", "arm64"]) {
    add("linux", arch, "deb", debPackage(version, arch));
    add("linux", arch, "tar.gz", linuxArchive(version, arch));
  }
  for (const arch of ["amd64", "arm64"]) {
    add("windows", arch, "exe", windowsSetup(version, arch));
  }
  return {
    version,
    page: releasesURL + "/tag/" + tag,
    checksums: base + "checksums.txt",
    source: readmeBuildURL,
    downloads,
    install: {
      dmg: "Open the disk image and drag Kando to Applications. Releases newer than 0.5.1 are signed with a Developer ID and notarized, so macOS opens them without a warning. 0.5.1 and earlier are not: on macOS 15 and later open it once, then choose System Settings › Privacy & Security › Open Anyway; on macOS 14 and earlier Control-click Kando in Applications and choose Open; or run xattr -dr com.apple.quarantine /Applications/Kando.app in Terminal.",
      deb: "On Debian, Ubuntu and their derivatives, run sudo apt install ./<file name> in the folder it was downloaded to.",
      "tar.gz":
        "On other distributions, download " +
        base +
        "install.sh into the same folder as the archive and run sh install.sh there. It installs Kando into ~/.local for the current user, without root, and adds it to the applications menu; sh install.sh --uninstall removes it.",
      exe: "Run the installer. It is not signed yet, so SmartScreen may warn before it runs: choose More info, then Run anyway. On Windows Kando does not download yt-dlp, ffmpeg and whisper-cli yet: install them with a package manager and put them on PATH.",
    },
  };
}

interface AgentLink {
  text: string;
  url: string;
}

// AgentStep is a card of the Get started section, in the order the page shows its parts: text, then tools or command, then notes and link.
interface AgentStep {
  // number is the card's number on the page.
  number?: number;
  title: string;
  text: string[];
  tools?: { name: string; does: string }[];
  command?: string;
  notes?: string[];
  link?: AgentLink;
}

interface AgentFeature {
  title: string;
  text: string;
  link?: AgentLink;
}

// AgentContent is a language's content file: the Get started section and the features, in that language's words.
export interface AgentContent {
  lang: string;
  name: string;
  page: string;
  start: { title: string; lead: string; steps: AgentStep[]; readme: AgentLink };
  features: { title: string; lead: string; items: AgentFeature[] };
}

// detailKeys are the cards of the details section in its order, by the prefix of their Title and Body keys.
export const detailKeys = [
  "detailReplay",
  "detailModes",
  "detailSearch",
  "detailWhisper",
  "detailAI",
  "detailBrowser",
  "detailUpload",
  "detailShortcuts",
  "detailLanguages",
] as const;

// plainLine is s with its line breaks taken out: nothing after a Chinese character, kana or CJK punctuation mark, as Chinese and Japanese put no space between words, and a space after anything else, Korean among it.
export function plainLine(s: string): string {
  let out = "";
  let prev = "";
  for (const ch of s) {
    if (ch === "\n") {
      const cp = prev.codePointAt(0) ?? 0;
      const unspaced = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u.test(prev) || (cp >= 0x3000 && cp <= 0x303f) || (cp >= 0xff00 && cp <= 0xffef);
      if (!unspaced) {
        out += " ";
      }
      continue;
    }
    out += ch;
    prev = ch;
  }
  return out;
}

// agentKeys are the content keys the WebMCP content files are made from; the tests compare them with the keys the page's Get started and features sections show.
export const agentKeys = new Set<string>();

// AgentModel is a whisper model as the models card names it in a language, the recommended one marked with the app's word for it, and its stats as the app's Settings › Speech Recognition writes them under its name: speed and accuracy out of 5 and the download size.
export interface AgentModel {
  name: string;
  stats: string;
}

// newAgentContent is l's content file for release version, made from the same content strings as the page's download panel (start) and its Library, Command line and details sections (features), the whisper models' card among the details.
export function newAgentContent(l: Language, strs: Strings, version: string, total: number, surf: Surface, models: readonly AgentModel[]): AgentContent {
  const t = (key: string, values: Record<string, number> = {}): string => {
    agentKeys.add(key);
    let s = strs[key];
    if (s === undefined) {
      throw new Error(`no content key ${key}`);
    }
    for (const [name, n] of Object.entries({ count: total, commands: surf.commands, tools: surf.tools.length, ...values })) {
      s = s.replaceAll("{" + name + "}", String(n));
    }
    // The page sets the names between ` in code and breaks some headings in two; the tools return plain text on one line.
    return plainLine(s.replaceAll("`", ""));
  };
  const join = (...keys: string[]) => keys.map((key) => t(key)).join(" ");
  const c: AgentContent = {
    lang: l.code,
    name: l.name,
    page: pageURL(l.code),
    start: {
      title: t("openTitle"),
      lead: t("openBody"),
      steps: [
        {
          number: 1,
          title: "macOS",
          text: [t("dlMacBody")],
          notes: [t("dlFirstRun")],
          link: { text: t("dlMacButton"), url: releaseFileURL(version, macDisk(version, "arm64")) },
        },
        {
          number: 2,
          title: "Linux",
          text: [t("dlLinuxBody")],
          command: installCommand,
          notes: [t("dlLinuxNote"), t("dlFirstRun")],
          link: { text: t("dlLinuxButton"), url: releaseFileURL(version, debPackage(version, "amd64")) },
        },
        {
          number: 3,
          title: "Windows",
          text: [t("dlWinBody")],
          notes: [t("dlWinNote")],
          link: { text: t("dlWinButton"), url: releaseFileURL(version, windowsSetup(version, "amd64")) },
        },
      ],
      readme: { text: t("dlReadme"), url: readmeURL(l) },
    },
    features: {
      title: t("detailsTitle"),
      lead: t("heroLead"),
      items: [
        { title: t("libraryTitle"), text: join("libraryBody", "libraryPoint1", "libraryPoint2", "libraryPoint3") },
        { title: t("cliTitle"), text: join("cliBody", "cliPoint1", "cliPoint2", "cliPoint3", "mcpOther") },
      ],
    },
  };
  for (const key of detailKeys) {
    const f: AgentFeature = { title: t(key + "Title"), text: t(key + "Body") };
    if (key === "detailAI") {
      f.link = { text: t("detailAILink"), url: everyAPIURL };
    }
    c.features.items.push(f);
    if (key === "detailWhisper") {
      // The models card sits under the whisper card's row on the page; the tools give what its chart and table show as one line, each model's name with its speed, accuracy and size in the app's words.
      const stats = models.map((m) => `${m.name}: ${m.stats.replace(/\s+/g, " ")}`).join("; ");
      c.features.items.push({ title: t("modelsTitle", { count: models.length }), text: t("modelsLead") + " " + stats });
    }
  }
  return c;
}

// writeAgentJSON writes v as name in dir and returns the path the page fetches it by.
function writeAgentJSON(dir: string, name: string, v: unknown): string {
  const data = JSON.stringify(v);
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, name), data);
  return hashLink("/" + agentDir + "/" + name, data);
}

// writeAgentFiles writes the tools' files into out's webmcp folder, the sites file and one content file per language, and returns the data each page embeds, by language code, and the sites file's link.
export function writeAgentFiles(
  out: string,
  langs: readonly Language[],
  content: ReadonlyMap<LangCode, Strings>,
  catalog: Catalog,
  version: string,
  surf: Surface,
  models: ReadonlyMap<LangCode, readonly AgentModel[]>,
): { data: Map<LangCode, AgentData>; sites: string } {
  const dir = join(out, agentDir);
  // The catalog's sites without their logos and featured marks, as search_supported_sites matches them: by name, domain and aliases.
  const sites = writeAgentJSON(dir, "sites.json", {
    ytdlp: catalog.ytdlp,
    sites: catalog.sites.map((s) =>
      s.aliases && s.aliases.length > 0 ? { name: s.name, domain: s.domain, aliases: s.aliases } : { name: s.name, domain: s.domain },
    ),
  });
  const all = langs.map((l) => {
    const strs = content.get(l.code);
    if (!strs) {
      throw new Error(`no content for ${l.code}`);
    }
    const link = writeAgentJSON(dir, l.code + ".json", newAgentContent(l, strs, version, catalog.sites.length, surf, models.get(l.code) ?? []));
    return { code: l.code, tag: l.tag, name: l.name, page: pagePath(l.code), content: link };
  });
  const release = releaseDownloads(version);
  const data = new Map<LangCode, AgentData>();
  for (const l of langs) {
    data.set(l.code, { lang: l.code, langs: all, sites, release });
  }
  return { data, sites };
}
