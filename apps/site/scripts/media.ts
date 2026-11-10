// Makes the committed images under media/ (buildtime/media.ts): each language's link preview image and its library screenshot as AVIF and WebP. Run it in apps/site with `vp run media` after changing what an image shows; the tests name the images that are out of date. It makes only those, or every image with --all.
//
// The link preview image is a screenshot of scripts/og.html, filled in with the language's headline and short line and a still of the hero's player, which it takes from a build of the site made for it. Both screenshots come from a headless Chromium (KANDO_CHROME, by default Playwright's chrome-headless-shell), driven over the DevTools protocol with Node's own WebSocket, in a profile of its own that is removed afterwards with the browser. The screenshots take ffmpeg (with SVT-AV1) and cwebp, from Homebrew's ffmpeg and webp.
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import { homedir, tmpdir } from "node:os";
import { dirname, extname, join, resolve } from "node:path";
import {
  inputsHash,
  mediaDir,
  mediaLangs,
  ogFile,
  ogSize,
  ogTemplate,
  readMediaManifest,
  shotEncoder,
  shotFile,
  shotFormats,
  shotWidths,
  staleMedia,
  type MediaLang,
  type MediaManifest,
} from "../buildtime/media";
import { pagePath } from "../src/lib/site";

const siteDir = process.cwd();
const root = resolve(siteDir, "../..");
const contentDir = join(siteDir, "content");
const chromePath =
  process.env["KANDO_CHROME"] ??
  join(homedir(), "Library/Caches/ms-playwright/chromium_headless_shell-1248/chrome-headless-shell-mac-arm64/chrome-headless-shell");

// stillSelector is the part of the hero the link preview image shows: the player's window, without the badges that overlap its edges (stillHidden).
const stillSelector = ".stage .win";
const stillHidden = ".stage .badge";

function need(cmd: string, hint: string): void {
  if (spawnSync("sh", ["-c", `command -v ${cmd}`]).status !== 0) {
    throw new Error(`${cmd} is not installed; ${hint}`);
  }
}

// run runs a command, and prints what it printed only when it fails, as SVT-AV1 logs its settings whatever ffmpeg's log level.
function run(cmd: string, args: string[]): void {
  const res = spawnSync(cmd, args, { stdio: ["ignore", "pipe", "pipe"], encoding: "utf8", timeout: 120_000 });
  if (res.status !== 0) {
    throw new Error(`${cmd} ${args.join(" ")} failed (${res.status ?? res.signal}):\n${res.stdout}${res.stderr}`);
  }
}

// ---- the library screenshots ----

function makeShots(m: MediaLang, manifest: MediaManifest): void {
  const code = m.lang.code;
  const src = join(root, "docs/assets", code, "library.png");
  for (const format of shotFormats) {
    for (const width of shotWidths) {
      const out = join(siteDir, mediaDir, shotFile(code, width, format));
      mkdirSync(dirname(out), { recursive: true });
      const enc = shotEncoder(format, src, width, out);
      run(enc.cmd, enc.args);
      console.log("made", join(mediaDir, shotFile(code, width, format)));
    }
    (manifest.shots[code] ??= {})[format] = inputsHash(m.shots[format]);
  }
}

// ---- the browser ----

// CDP is a connection to a page target of the browser over the DevTools protocol.
class CDP {
  private next = 0;
  private pending = new Map<number, { resolve: (v: unknown) => void; reject: (e: Error) => void }>();
  private waiters = new Map<string, (() => void)[]>();

  private constructor(private ws: WebSocket) {
    ws.addEventListener("message", (event) => {
      const msg = JSON.parse(String(event.data)) as { id?: number; method?: string; result?: unknown; error?: { message: string } };
      if (msg.id !== undefined) {
        const p = this.pending.get(msg.id);
        this.pending.delete(msg.id);
        if (msg.error) {
          p?.reject(new Error(msg.error.message));
        } else {
          p?.resolve(msg.result);
        }
      } else if (msg.method) {
        const list = this.waiters.get(msg.method) ?? [];
        this.waiters.delete(msg.method);
        for (const w of list) {
          w();
        }
      }
    });
  }

  static open(url: string): Promise<CDP> {
    return new Promise((ok, fail) => {
      const ws = new WebSocket(url);
      ws.addEventListener("open", () => ok(new CDP(ws)), { once: true });
      ws.addEventListener("error", () => fail(new Error("cannot connect to " + url)), { once: true });
    });
  }

  send<T = Record<string, unknown>>(method: string, params: Record<string, unknown> = {}): Promise<T> {
    const id = ++this.next;
    this.ws.send(JSON.stringify({ id, method, params }));
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve: resolve as (v: unknown) => void, reject });
    });
  }

  event(method: string): Promise<void> {
    return new Promise((resolve) => {
      this.waiters.set(method, [...(this.waiters.get(method) ?? []), resolve]);
    });
  }

  close(): void {
    this.ws.close();
  }
}

interface Browser {
  page: CDP;
  close: () => Promise<void>;
}

// abort kills the browser and removes its profile when the script is interrupted, as the finally blocks do not run then.
let abort: (() => void) | undefined;
for (const signal of ["SIGINT", "SIGTERM"] as const) {
  process.on(signal, () => {
    abort?.();
    process.exit(130);
  });
}

async function waitFor<T>(what: string, get: () => T | undefined, ms = 20_000): Promise<T> {
  const end = Date.now() + ms;
  for (;;) {
    const v = get();
    if (v !== undefined) {
      return v;
    }
    if (Date.now() > end) {
      throw new Error("timed out waiting for " + what);
    }
    await new Promise((r) => setTimeout(r, 100));
  }
}

// launch starts the browser in a new profile and opens a page. close quits it, kills it if it does not quit, and removes the profile.
async function launch(): Promise<Browser> {
  if (!existsSync(chromePath)) {
    throw new Error(`no browser at ${chromePath}; install it with npx playwright install chromium-headless-shell, or point KANDO_CHROME at a Chromium`);
  }
  const profile = mkdtempSync(join(tmpdir(), "kando-media-chrome-"));
  const chrome: ChildProcess = spawn(
    chromePath,
    [
      `--user-data-dir=${profile}`,
      "--remote-debugging-port=0",
      "--headless",
      "--hide-scrollbars",
      "--no-first-run",
      "--no-default-browser-check",
      "--force-color-profile=srgb",
      "--lang=en-US",
      "about:blank",
    ],
    { stdio: "ignore" },
  );
  let exited = false;
  chrome.on("exit", () => {
    exited = true;
  });
  abort = () => {
    chrome.kill("SIGKILL");
    rmSync(profile, { recursive: true, force: true });
  };
  const close = async () => {
    abort = undefined;
    if (!exited) {
      chrome.kill("SIGTERM");
      await waitFor("the browser to quit", () => (exited ? true : undefined), 5_000).catch(() => chrome.kill("SIGKILL"));
    }
    rmSync(profile, { recursive: true, force: true });
  };
  try {
    const portFile = join(profile, "DevToolsActivePort");
    const port = await waitFor("the browser's DevTools port", () => (existsSync(portFile) ? readFileSync(portFile, "utf8").split("\n")[0] : undefined));
    const res = await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, { method: "PUT" });
    const target = (await res.json()) as { webSocketDebuggerUrl: string };
    const page = await CDP.open(target.webSocketDebuggerUrl);
    await page.send("Page.enable");
    return {
      page,
      close: async () => {
        page.close();
        await close();
      },
    };
  } catch (err) {
    await close();
    throw err;
  }
}

async function load(page: CDP, url: string, width: number, height: number, scale: number): Promise<void> {
  await page.send("Emulation.setDeviceMetricsOverride", { width, height, deviceScaleFactor: scale, mobile: false });
  await page.send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-reduced-motion", value: "reduce" }] });
  const loaded = page.event("Page.loadEventFired");
  await page.send("Page.navigate", { url });
  await loaded;
  // The fonts, and a moment for hydration and the layout that follows it.
  await page.send("Runtime.evaluate", { expression: "document.fonts.ready.then(() => new Promise((r) => setTimeout(r, 1000)))", awaitPromise: true });
}

async function screenshot(page: CDP, clip?: { x: number; y: number; width: number; height: number }): Promise<Buffer> {
  const params: Record<string, unknown> = { format: "png" };
  if (clip) {
    params["clip"] = { ...clip, scale: 1 };
    params["captureBeyondViewport"] = true;
  }
  const { data } = await page.send<{ data: string }>("Page.captureScreenshot", params);
  return Buffer.from(data, "base64");
}

// ---- the site, served ----

const types: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript",
  ".css": "text/css",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".ico": "image/x-icon",
  ".webp": "image/webp",
  ".avif": "image/avif",
};

// serve serves the built site in dir on a free port of the loopback, and the pages and images of extra by path.
function serve(dir: string, extra: Map<string, { type: string; body: string | Buffer }>): Promise<{ server: Server; base: string }> {
  const server = createServer((req, res) => {
    const path = decodeURIComponent(new URL(req.url ?? "/", "http://localhost").pathname);
    const own = extra.get(path);
    if (own) {
      res.writeHead(200, { "content-type": own.type });
      res.end(own.body);
      return;
    }
    let file = join(dir, path);
    if (path.endsWith("/")) {
      file = join(file, "index.html");
    }
    if (!file.startsWith(dir) || !existsSync(file)) {
      res.writeHead(404);
      res.end();
      return;
    }
    res.writeHead(200, { "content-type": types[extname(file)] ?? "application/octet-stream" });
    res.end(readFileSync(file));
  });
  return new Promise((ok) => {
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address();
      const port = typeof addr === "object" && addr ? addr.port : 0;
      ok({ server, base: `http://127.0.0.1:${port}` });
    });
  });
}

// ---- the link preview images ----

function escapeHTML(s: string): string {
  return s.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}

// titleHTML is the headline as the hero sets it: the mark set apart, and each line break a <br>.
function titleHTML(title: string, mark: string): string {
  const at = title.indexOf(mark);
  const lines = (s: string) => escapeHTML(s).replaceAll("\n", "<br>");
  if (at < 0) {
    return lines(title);
  }
  return lines(title.slice(0, at)) + `<em class="mark">${lines(mark)}</em>` + lines(title.slice(at + mark.length));
}

function card(m: MediaLang, still: string): string {
  const fields: Record<string, string> = {
    lang: m.lang.tag,
    icon: "/logo.png",
    title: titleHTML(m.strings["heroTitle"] ?? "", m.strings["heroTitleMark"] ?? ""),
    tagline: escapeHTML(m.strings["ogTagline"] ?? ""),
    still,
  };
  return readFileSync(join(siteDir, ogTemplate), "utf8").replace(/\{\{(\w+)\}\}/g, (whole, name: string) => fields[name] ?? whole);
}

async function makeOG(langs: readonly MediaLang[], manifest: MediaManifest): Promise<void> {
  mkdirSync(join(siteDir, ".generated"), { recursive: true });
  const out = mkdtempSync(join(siteDir, ".generated", "media-build-"));
  const pub = mkdtempSync(join(siteDir, ".generated", "media-public-"));
  let browser: Browser | undefined;
  let server: Server | undefined;
  try {
    console.log("building the site for the stills of the hero's player");
    const build = spawnSync("vp", ["build"], {
      env: { ...process.env, NODE_ENV: "production", KANDO_SITE_OUT: out, KANDO_SITE_PUBLIC: pub, KANDO_SITE_MEDIA_PENDING: "1" },
      stdio: ["ignore", "pipe", "pipe"],
      encoding: "utf8",
      timeout: 300_000,
    });
    if (build.status !== 0) {
      throw new Error(`vp build failed (${build.status ?? build.signal}):\n${build.stdout}${build.stderr}`);
    }
    const extra = new Map<string, { type: string; body: string | Buffer }>();
    const served = await serve(join(out, "client"), extra);
    server = served.server;
    browser = await launch();
    const page = browser.page;
    for (const m of langs) {
      const code = m.lang.code;
      await load(page, served.base + pagePath(code) + "?lang=" + code, 1440, 1000, 2);
      const rect = (
        await page.send<{ result: { value: { x: number; y: number; width: number; height: number } | null } }>("Runtime.evaluate", {
          expression: `(() => { for (const b of document.querySelectorAll(${JSON.stringify(stillHidden)})) b.style.display = "none"; const el = document.querySelector(${JSON.stringify(stillSelector)}); if (!el) return null; const r = el.getBoundingClientRect(); return { x: r.x + scrollX, y: r.y + scrollY, width: r.width, height: r.height }; })()`,
          returnByValue: true,
        })
      ).result.value;
      if (!rect) {
        throw new Error(`${pagePath(code)} has no ${stillSelector}`);
      }
      const still = await screenshot(page, rect);
      if (process.env["KANDO_MEDIA_KEEP_STILLS"]) {
        writeFileSync(join(siteDir, ".generated", `still-${code}.png`), still);
      }
      extra.set(`/__og/still-${code}.png`, { type: "image/png", body: still });
      extra.set(`/__og/${code}.html`, { type: "text/html; charset=utf-8", body: card(m, `/__og/still-${code}.png`) });
      await load(page, `${served.base}/__og/${code}.html`, ogSize.w, ogSize.h, 1);
      const png = await screenshot(page);
      const file = join(siteDir, mediaDir, ogFile(code));
      mkdirSync(dirname(file), { recursive: true });
      writeFileSync(file, png);
      manifest.og[code] = inputsHash(m.og);
      console.log("made", join(mediaDir, ogFile(code)));
    }
  } finally {
    await browser?.close();
    server?.close();
    rmSync(out, { recursive: true, force: true });
    rmSync(pub, { recursive: true, force: true });
  }
}

async function main(): Promise<void> {
  const all = process.argv.includes("--all");
  const langs = mediaLangs(root, siteDir, contentDir);
  const stale = new Set(staleMedia(siteDir, langs));
  const ogLangs = langs.filter((m) => all || stale.has(join(mediaDir, ogFile(m.lang.code))));
  const shotLangs = langs.filter((m) => all || shotFormats.some((f) => shotWidths.some((w) => stale.has(join(mediaDir, shotFile(m.lang.code, w, f))))));
  if (ogLangs.length === 0 && shotLangs.length === 0) {
    console.log("every image is up to date; --all makes them again anyway");
    return;
  }
  const manifest = readMediaManifest(siteDir);
  if (shotLangs.length > 0) {
    need("ffmpeg", "brew install ffmpeg");
    need("cwebp", "brew install webp");
    for (const m of shotLangs) {
      makeShots(m, manifest);
    }
  }
  if (ogLangs.length > 0) {
    await makeOG(ogLangs, manifest);
  }
  // Languages without a content file any more lose their entries.
  const codes = new Set<string>(langs.map((m) => m.lang.code));
  for (const key of ["og", "shots"] as const) {
    for (const code of Object.keys(manifest[key])) {
      if (!codes.has(code)) {
        delete manifest[key][code as keyof (typeof manifest)[typeof key]];
      }
    }
  }
  writeFileSync(join(siteDir, mediaDir, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
}

let failed = false;
await main().catch((err: unknown) => {
  console.error(err instanceof Error ? err.message : err);
  failed = true;
});
process.exitCode = failed ? 1 : 0;
