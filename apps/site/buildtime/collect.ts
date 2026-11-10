// collect reads everything the site shows from the repository, checks it, and writes the files the pages link into the public folder: the static scripts, the icons, the screenshots and link preview images, the marks and logos, the WebMCP files, the sitemap, robots.txt, CNAME, the web app manifest and llms.txt; and it makes each page's structured data. Any source that is missing or changed shape fails the build.
import { copyFileSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { minifySync } from "vite";
import { languages, pagePath, sourceLang, type LangCode } from "../src/lib/site";
import { appVersion, loadAppStrings, readModelNames, readModels, readSurface } from "./app";
import { catalogRows, featuredTiles, loadSiteNames, localTiles, readCatalog } from "./catalog";
import { loadContent, loadNotes } from "./content";
import { pngSize, writeIcons } from "./icons";
import { aiServices, claudeCode, Marks } from "./marks";
import { mediaCommand, mediaDir, ogFile, ogSize, shotFile, shotFormats, shotWidths } from "./media";
import { siteDataSchema, type CatalogRow, type ContentModule, type SiteData } from "./schema";
import { jsonForScript } from "../src/lib/json";
import { structuredData } from "./jsonld";
import { writeLLMsFiles } from "./llms";
import { lastModified, pageGroups, writeSEOFiles } from "./seo";
import { hashLink, newAgentContent, writeAgentFiles } from "./webmcp";

export interface Collected {
  site: SiteData;
  content: Map<LangCode, ContentModule>;
  catalog: CatalogRow[];
  // structured is each page's JSON-LD (buildtime/jsonld.ts), by the page's path.
  structured: Map<string, string>;
}

// screenshots are the pictures of the app each page shows, as docs/assets/<language>/<name>.png: the library beside the Library section. The hero draws its player itself.
export const screenshots = ["library"] as const;

// collect builds the data from the repository at root, for the site at siteDir with its content files in contentDir, writing its files into pub, which it empties first. A language has pages only when it has a content file.
export function collect(root: string, siteDir: string, contentDir: string, pub: string): Collected {
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
  // The static scripts are not bundled, so they are minified here as Vite minifies the bundles: site.js blocks the first paint, and the smaller it is the sooner the page shows.
  for (const name of ["site.js", "webmcp.js"]) {
    const min = minifySync(name, readFileSync(join(siteDir, "static", name), "utf8"));
    if (min.errors.length > 0) {
      throw new Error(`static/${name}: ${min.errors.map((e) => e.message).join("; ")}`);
    }
    writeFileSync(join(pub, "static", name), min.code);
    statics[name] = hashLink("/static/" + name, min.code);
  }

  writeIcons(join(root, "apps/kando/resources/icon.png"), pub);

  // The images scripts/media.ts makes (buildtime/media.ts) are copied from media/, linked with a hash of their content so that a new one is fetched again. A missing one fails the build, except in the build the script makes for itself before it has made them.
  const pending = process.env["KANDO_SITE_MEDIA_PENDING"] === "1";
  const copyMedia = (rel: string, to: string): string => {
    const src = join(siteDir, mediaDir, rel);
    if (!existsSync(src)) {
      if (pending) {
        return "/" + to;
      }
      throw new Error(`${mediaDir}/${rel} is missing; run ${mediaCommand} in apps/site`);
    }
    const data = readFileSync(src);
    mkdirSync(dirname(join(pub, to)), { recursive: true });
    writeFileSync(join(pub, to), data);
    return hashLink("/" + to, data);
  };

  const shots = {} as SiteData["shots"];
  const og = {} as SiteData["og"];
  for (const l of langs) {
    const src = join(root, "docs/assets", l.code, "library.png");
    const size = pngSize(src);
    mkdirSync(join(pub, "shots", l.code), { recursive: true });
    copyFileSync(src, join(pub, "shots", l.code, "library.png"));
    const sources = shotFormats.map((format) => ({
      type: "image/" + format,
      srcset: shotWidths.map((w) => `${copyMedia(shotFile(l.code, w, format), shotFile(l.code, w, format))} ${w}w`).join(", "),
    }));
    shots[l.code] = { library: { src: `/shots/${l.code}/library.png`, ...size, sources } };

    const card = ogFile(l.code);
    if (existsSync(join(siteDir, mediaDir, card))) {
      const got = pngSize(join(siteDir, mediaDir, card));
      if (got.w !== ogSize.w || got.h !== ogSize.h) {
        throw new Error(`${mediaDir}/${card} is ${got.w}×${got.h}, not ${ogSize.w}×${ogSize.h}; run ${mediaCommand} in apps/site`);
      }
    }
    og[l.code] = { src: copyMedia(card, card), w: ogSize.w, h: ogSize.h };
  }

  const marks = new Marks(pub);
  const catalog = readCatalog(root);
  const tiles = featuredTiles(root, pub, catalog, marks);
  const names = loadSiteNames(contentDir, langs, catalog.sites);
  const rows = catalogRows(root, pub, catalog);

  const version = appVersion(root);
  const surface = readSurface(root);
  const models = readModels(root);
  const modelNames = readModelNames(root, models);

  const modules = new Map<LangCode, ContentModule>();
  for (const l of langs) {
    const strings = content.get(l.code) ?? {};
    const app = loadAppStrings(root, l, modelNames);
    modules.set(l.code, { code: l.code, strings, ...app });
  }

  const agentModels = new Map(langs.map((l) => [l.code, models.map((m) => ({ name: modules.get(l.code)?.modelNames[m.id] ?? m.id, mb: m.mb }))]));
  const agent = writeAgentFiles(pub, langs, content, catalog, version, surface, agentModels);

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
    og,
    assets: { siteJS: statics["site.js"], webmcpJS: statics["webmcp.js"], sitesJSON: agent.sites },
    webmcp: Object.fromEntries(agent.data),
    models,
    missing: langs.map((l) => {
      const s = content.get(l.code) ?? {};
      return { code: l.code, tag: l.tag, title: s["notFoundTitle"], body: s["notFoundBody"], home: s["notFoundHome"] };
    }),
  });

  writeSEOFiles(pub, langs, { lastmod: lastModified(root), description: en["metaDescription"] ?? "" });
  const enLang = langs[0];
  if (enLang?.code !== sourceLang) {
    throw new Error("the source language has no pages");
  }
  writeLLMsFiles(pub, {
    langs,
    content,
    version,
    total: catalog.sites.length,
    agent: newAgentContent(enLang, en, catalog.sites.length, surface, agentModels.get(sourceLang) ?? []),
  });

  // The structured data of each page, by its path, which the post-build step puts into the page (buildtime/csp.ts).
  const structured = new Map<string, string>();
  for (const l of langs) {
    const shot = shots[l.code]?.library;
    if (!shot) {
      throw new Error(`no library screenshot for ${l.code}`);
    }
    for (const kind of pageGroups) {
      const graph = structuredData({ code: l.code, kind, strings: content.get(l.code) ?? {}, version, total: catalog.sites.length, shot });
      structured.set(pagePath(l.code, kind), jsonForScript(graph));
    }
  }
  return { site, content: modules, catalog: rows, structured };
}
