// The app's catalog of websites (apps/kando/sites/catalog/sites.json): its featured sites in the works-with strip, every site on the sites page, and their logos.
import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { initial, plateCount, type Language, type LangCode } from "../src/lib/site";
import { issues, readJSON } from "./content";
import { Marks, siteMarks } from "./marks";
import { catalogSchema, siteNamesSchema, type CatalogRow, type CatalogSite, type Tile } from "./schema";

export interface Catalog {
  ytdlp: string;
  sites: CatalogSite[];
}

export function readCatalog(root: string): Catalog {
  const file = "apps/kando/sites/catalog/sites.json";
  const res = catalogSchema.safeParse(readJSON(join(root, file)));
  if (!res.success) {
    throw issues(file, res.error);
  }
  return res.data;
}

// fnv1a32 is the 32-bit FNV-1a hash of s's UTF-8 bytes, Go's hash/fnv New32a.
export function fnv1a32(s: string): number {
  let h = 0x811c9dc5;
  for (const b of new TextEncoder().encode(s)) {
    h ^= b;
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h >>> 0;
}

// plate is the index of domain's plate color, the one the app gives it.
export function plate(domain: string): number {
  return fnv1a32(domain) % plateCount;
}

const hanRe = /\p{Script=Han}/u;

// logoFile copies a site's logo from the catalog into the built site's logos folder and returns its path there.
function logoFile(root: string, out: string, logo: string): string {
  const src = join(root, "apps/kando/sites/catalog/logos", logo);
  if (!existsSync(src)) {
    throw new Error(`apps/kando/sites/catalog/logos/${logo} does not exist`);
  }
  mkdirSync(join(out, "logos"), { recursive: true });
  copyFileSync(src, join(out, "logos", logo));
  return "/logos/" + logo;
}

// featuredTiles are the catalog's featured sites with their marks written into the site: AI Logo's (marks), the catalog's own logos, or an initial on a plate.
export function featuredTiles(root: string, out: string, catalog: Catalog, marks: Marks): Tile[] {
  const tiles: Tile[] = [];
  for (const s of catalog.sites) {
    if (!s.featured) {
      continue;
    }
    const tile: Tile = { name: s.name, domain: s.domain };
    const id = siteMarks[s.domain];
    if (id !== undefined) {
      tile.logo = marks.src(id);
    } else if (s.logo) {
      tile.logo = logoFile(root, out, s.logo);
    } else {
      tile.initial = initial(s.name);
      tile.plate = plate(s.domain);
    }
    tiles.push(tile);
  }
  if (tiles.length === 0) {
    throw new Error("sites.json lists no featured site");
  }
  return tiles;
}

// catalogRows are every site of the catalog as the sites page lists it, with the catalog's logos written into the site.
export function catalogRows(root: string, out: string, catalog: Catalog): CatalogRow[] {
  return catalog.sites.map((s) => {
    const row: CatalogRow = { name: s.name, domain: s.domain, featured: s.featured === true };
    if (s.logo) {
      row.logo = logoFile(root, out, s.logo);
    } else {
      row.initial = initial(s.name);
      row.plate = plate(s.domain);
    }
    return row;
  });
}

// loadSiteNames reads content/sites.<code>.json, where a language has one: the names of the catalog's sites as that language writes them, by domain, where they differ from the catalog's, such as the Traditional Chinese forms of names the catalog writes in Simplified Chinese. The logo strip shows the featured ones and the sites page all of them. The catalog is the app's and stays as it is. A domain that is not in the catalog fails the build, so that a typo does not silently leave the catalog's name in place.
export function loadSiteNames(dir: string, langs: readonly Language[], sites: readonly { domain: string }[]): Map<LangCode, Record<string, string>> {
  const known = new Set(sites.map((s) => s.domain));
  const names = new Map<LangCode, Record<string, string>>();
  for (const l of langs) {
    const file = `sites.${l.code}.json`;
    const path = join(dir, file);
    if (!existsSync(path)) {
      continue;
    }
    const res = siteNamesSchema.safeParse(readJSON(path));
    if (!res.success) {
      throw issues("content/" + file, res.error);
    }
    for (const domain of Object.keys(res.data)) {
      if (!known.has(domain)) {
        throw new Error(`content/${file}: ${domain} is not a site of sites.json`);
      }
    }
    names.set(l.code, res.data);
  }
  return names;
}

// localTiles are the tiles as the page in l shows them: with the names l writes differently, and a name in Chinese characters on a page that is not in Chinese marked as Simplified Chinese.
export function localTiles(tiles: readonly Tile[], l: Language, names: Record<string, string> = {}): Tile[] {
  return tiles.map((t) => {
    const tile = { ...t };
    const name = names[tile.domain];
    if (name !== undefined) {
      tile.name = name;
      if (!tile.logo) {
        tile.initial = initial(name);
      }
    }
    if (!l.tag.startsWith("zh") && hanRe.test(tile.name)) {
      tile.lang = "zh-Hans";
    }
    return tile;
  });
}
