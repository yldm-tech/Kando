// collect reads everything the site shows from the repository, checks it, and writes the files the pages link into the public folder: the static scripts, the icons, the screenshots, the marks and logos, the WebMCP files, the sitemap, robots.txt and CNAME. Any source that is missing or changed shape fails the build.
import { copyFileSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { languages, type LangCode } from "../src/lib/site";
import { appVersion, loadAppStrings, readModelNames, readModels, readSurface } from "./app";
import { catalogRows, featuredTiles, loadSiteNames, localTiles, readCatalog } from "./catalog";
import { loadContent, loadNotes } from "./content";
import { pngSize, writeIcons } from "./icons";
import { aiServices, claudeCode, Marks } from "./marks";
import { siteDataSchema, type CatalogRow, type ContentModule, type SiteData } from "./schema";
import { writeSEOFiles } from "./seo";
import { hashLink, writeAgentFiles } from "./webmcp";

export interface Collected {
  site: SiteData;
  content: Map<LangCode, ContentModule>;
  catalog: CatalogRow[];
}

// screenshots are the pictures of the app each page shows, as docs/assets/<language>/<name>.png: the library beside the Library section. The hero draws its player itself.
export const screenshots = ["library"] as const;

// collect builds the data from the repository at root, for the site at siteDir, writing its files into pub, which it empties first.
export function collect(root: string, siteDir: string, pub: string): Collected {
  const contentDir = join(siteDir, "content");
  const content = loadContent(contentDir);
  const en = content.get("en");
  if (!en) {
    throw new Error("content/en.json is missing");
  }
  loadNotes(contentDir, en);
  const langs = languages.filter((l) => content.has(l.code));

  rmSync(pub, { recursive: true, force: true });
  mkdirSync(join(pub, "static"), { recursive: true });

  const statics: Record<string, string> = {};
  for (const name of ["site.js", "webmcp.js"]) {
    const data = readFileSync(join(siteDir, "static", name));
    writeFileSync(join(pub, "static", name), data);
    statics[name] = hashLink("/static/" + name, data);
  }

  writeIcons(join(root, "apps/kando/resources/icon.png"), pub);

  const shots = {} as SiteData["shots"];
  for (const l of langs) {
    const src = join(root, "docs/assets", l.code, "library.png");
    const size = pngSize(src);
    mkdirSync(join(pub, "shots", l.code), { recursive: true });
    copyFileSync(src, join(pub, "shots", l.code, "library.png"));
    shots[l.code] = { library: { src: `/shots/${l.code}/library.png`, ...size } };
  }

  const marks = new Marks(pub);
  const catalog = readCatalog(root);
  const tiles = featuredTiles(root, pub, catalog, marks);
  const names = loadSiteNames(contentDir, langs, tiles);
  const rows = catalogRows(root, pub, catalog);

  const version = appVersion(root);
  const surface = readSurface(root);
  const models = readModels(root);
  const modelNames = readModelNames(root, models);

  const agent = writeAgentFiles(pub, langs, content, catalog, version, surface);

  const modules = new Map<LangCode, ContentModule>();
  for (const l of langs) {
    const strings = content.get(l.code) ?? {};
    const app = loadAppStrings(root, l, modelNames);
    modules.set(l.code, { code: l.code, strings, ...app });
  }

  const site = siteDataSchema.parse({
    version,
    surface,
    total: catalog.sites.length,
    ytdlp: catalog.ytdlp,
    tiles: Object.fromEntries(langs.map((l) => [l.code, localTiles(tiles, l, names.get(l.code))])),
    siteNames: Object.fromEntries(langs.map((l) => [l.code, names.get(l.code) ?? {}])),
    aiServices: marks.brands(aiServices),
    claudeCode: marks.brands([claudeCode])[0],
    shots,
    assets: { siteJS: statics["site.js"], webmcpJS: statics["webmcp.js"], sitesJSON: agent.sites },
    webmcp: Object.fromEntries(agent.data),
    models,
    missing: langs.map((l) => {
      const s = content.get(l.code) ?? {};
      return { code: l.code, tag: l.tag, title: s["notFoundTitle"], body: s["notFoundBody"], home: s["notFoundHome"] };
    }),
  });

  writeSEOFiles(pub, langs);
  return { site, content: modules, catalog: rows };
}
