// The Vite plugins that build the site from the repository: kandoSite collects the data (collect.ts) and serves it to the pages as virtual modules, and kandoPostBuild finishes the prerendered HTML.
import { existsSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import type { Plugin } from "vite";
import { langCodes, sourceLang, type LangCode } from "../src/lib/site";
import { collect, type Collected } from "./collect";
import { finishHTML } from "./csp";

export const siteModule = "virtual:kando/site";
export const catalogModule = "virtual:kando/catalog";
export const contentPrefix = "virtual:kando/content/";

export interface KandoSiteOptions {
  // repoRoot is the repository root, relative to the site's folder.
  repoRoot: string;
  // outDir is the build's output folder, relative to the site's folder; the pages are in its client folder.
  outDir: string;
  // publicDir is the folder the collected files are written to, served and copied as Vite's public folder.
  publicDir: string;
}

// checkOutDir refuses an output folder that holds anything but a previous build (one with client/CNAME), so that a mistyped outDir cannot wipe a folder of other files when the build empties it.
export function checkOutDir(out: string): void {
  if (!existsSync(out)) {
    return;
  }
  if (readdirSync(out).length > 0 && !existsSync(join(out, "client", "CNAME"))) {
    throw new Error(`${out} is not empty and holds no previous build (no client/CNAME); choose another output folder`);
  }
}

// collected is the data of this process, collected once: Vite loads the config again for the prerender's preview server, and the files are the same.
const cacheKey = Symbol.for("kando-site.collected");
function collectOnce(root: string, siteDir: string, pub: string): Collected {
  const g = globalThis as { [cacheKey]?: Collected };
  g[cacheKey] ??= collect(root, siteDir, pub);
  return g[cacheKey];
}

// prerenderPages are the pages TanStack Start renders to static HTML: each language's home page and sites page, and 404.html.
export function prerenderPages(): { path: string; prerender?: { outputPath: string } }[] {
  const home = (code: LangCode) => (code === sourceLang ? "/" : `/${code}/`);
  return [
    ...langCodes.map((code) => ({ path: home(code) })),
    ...langCodes.map((code) => ({ path: home(code) + "sites/" })),
    { path: "/404", prerender: { outputPath: "/404.html" } },
  ];
}

export function kandoSite(opts: KandoSiteOptions): Plugin {
  let data: Collected | undefined;
  const need = (): Collected => {
    if (!data) {
      throw new Error("kando-site: the data was not collected");
    }
    return data;
  };
  return {
    name: "kando-site",
    enforce: "pre",
    config(_config, env) {
      const siteDir = process.cwd();
      if (env.command === "build") {
        checkOutDir(resolve(siteDir, opts.outDir));
      }
      data = collectOnce(resolve(siteDir, opts.repoRoot), siteDir, resolve(siteDir, opts.publicDir));
    },
    resolveId(id) {
      if (id === siteModule || id === catalogModule || id.startsWith(contentPrefix)) {
        return "\0" + id;
      }
      return undefined;
    },
    load(id) {
      if (!id.startsWith("\0virtual:kando/")) {
        return undefined;
      }
      const name = id.slice(1);
      const json = (v: unknown) => `export default JSON.parse(${JSON.stringify(JSON.stringify(v))});\n`;
      if (name === siteModule) {
        return json(need().site);
      }
      if (name === catalogModule) {
        return json(need().catalog);
      }
      const code = name.slice(contentPrefix.length) as LangCode;
      const content = need().content.get(code);
      if (!content) {
        throw new Error(`kando-site: no content for ${JSON.stringify(code)}`);
      }
      return json(content);
    },
  };
}

// kandoPostBuild runs after TanStack Start has prerendered the pages: it gives each page its Content Security Policy and takes the scripts out of 404.html (finishHTML).
export function kandoPostBuild(opts: { outDir: string }): Plugin {
  return {
    name: "kando-site-post-build",
    enforce: "post",
    apply: "build",
    buildApp: {
      order: "post",
      handler() {
        finishHTML(resolve(process.cwd(), opts.outDir, "client"));
        return Promise.resolve();
      },
    },
  };
}
