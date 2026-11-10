// TestContentFiles' other half: a language has pages only when it has a content file, as the Go site's loadContent skipped a language without one. The site is built again from a copy of the content folder without one language that is not the source, and that language is left out of the pages, the language switcher, the sitemap and the WebMCP files while the others are built as before.
import { spawnSync } from "node:child_process";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { join, resolve } from "node:path";
import { afterAll, beforeAll, expect, it } from "vite-plus/test";
import { builtCodes, loadersSource, prerenderPages } from "../buildtime/plugin";
import { siteDir } from "./helpers";

const dropped = "ko";
let tmp = "";
let pages = "";

beforeAll(() => {
  mkdirSync(join(siteDir, ".generated"), { recursive: true });
  tmp = mkdtempSync(join(resolve(siteDir, ".generated"), "test-langs-"));
  const content = join(tmp, "content");
  cpSync(join(siteDir, "content"), content, { recursive: true });
  rmSync(join(content, dropped + ".json"));
  rmSync(join(content, `sites.${dropped}.json`), { force: true });
  const out = join(tmp, "out");
  const build = spawnSync("vp", ["build"], {
    cwd: siteDir,
    env: { ...process.env, NODE_ENV: "production", KANDO_SITE_OUT: out, KANDO_SITE_CONTENT: content, KANDO_SITE_PUBLIC: join(tmp, "public") },
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    timeout: 280_000,
  });
  if (build.status !== 0) {
    throw new Error(`vp build without ${dropped}.json failed (${build.status ?? build.signal}):\n${build.stdout}\n${build.stderr}`);
  }
  pages = join(out, "client");
});

afterAll(() => {
  if (tmp) {
    rmSync(tmp, { recursive: true, force: true });
  }
});

it("lists only the languages with a content file to prerender and to load", () => {
  const content = join(tmp, "content");
  expect(builtCodes(content)).toEqual(["en", "zh-CN", "zh-TW", "ja"]);
  const paths = prerenderPages(content).map((p) => p.path);
  expect(paths).not.toContain(`/${dropped}/`);
  expect(paths).not.toContain(`/${dropped}/sites/`);
  expect(paths).toContain("/ja/sites/");
  expect(loadersSource(builtCodes(content))).not.toContain(`content/${dropped}`);
});

it("builds the other languages and leaves the one without content out", () => {
  for (const path of ["index.html", "sites/index.html", "zh-CN/index.html", "zh-TW/sites/index.html", "ja/index.html", "404.html", "webmcp/ja.json"]) {
    expect(existsSync(join(pages, path)), path).toBe(true);
  }
  for (const path of [`${dropped}/index.html`, `${dropped}/sites/index.html`, `webmcp/${dropped}.json`, `shots/${dropped}`]) {
    expect(existsSync(join(pages, path)), path).toBe(false);
  }
  const home = readFileSync(join(pages, "index.html"), "utf8");
  expect(home).not.toContain(`href="/${dropped}/"`);
  expect(home).not.toMatch(/hreflang="ko"/i);
  expect(home).toContain('href="/ja/"');
  expect(readFileSync(join(pages, "sitemap.xml"), "utf8")).not.toContain(`/${dropped}/`);
});
