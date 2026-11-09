// Runs static/webmcp.js against a built page with a fake WebMCP, the page's data block and a fetch that serves the built site, and calls each tool. TestWebMCPScript runs it with node:
//
//	node webmcp_harness.js <webmcp.js> <built site> <page file, as ja/index.html> <content folder>
//
// It exits non-zero, with the failed assertion on standard error, when a tool answers wrongly.
"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const [scriptPath, dist, pageFile, contentDir] = process.argv.slice(2);
const script = fs.readFileSync(scriptPath, "utf8");
const html = fs.readFileSync(path.join(dist, pageFile), "utf8");
const block = html.match(/<script type="application\/json" id="webmcp-data">([^<]*)<\/script>/);
assert.ok(block, pageFile + " has no webmcp-data block");
const pageData = JSON.parse(block[1]);
const content = (code) => JSON.parse(fs.readFileSync(path.join(contentDir, code + ".json"), "utf8"));

// load runs the script in a fresh global scope, with WebMCP on document, on navigator or on neither, and returns the tools it registered and the paths it fetched.
// failOnce are paths whose first fetch answers 404, as a deploy in progress can.
async function load({ on = "document", userAgent = "", userAgentData, failOnce = new Set() } = {}) {
  const tools = new Map();
  const fetched = [];
  const warnings = [];
  const modelContext = {
    registerTool(tool) {
      if (tools.has(tool.name)) {
        return Promise.reject(new Error("a tool named " + tool.name + " is registered"));
      }
      tools.set(tool.name, tool);
      return Promise.resolve();
    }
  };
  const document = {
    getElementById(id) {
      assert.equal(id, "webmcp-data");
      return { textContent: block[1] };
    }
  };
  const navigator = { userAgent, userAgentData };
  if (on === "document") {
    document.modelContext = modelContext;
  } else if (on === "navigator") {
    navigator.modelContext = modelContext;
  }
  // fetch serves the built site, as GitHub Pages does: the query is ignored, a missing file is a 404.
  const fetch = (url) => {
    fetched.push(url);
    const pathname = decodeURIComponent(new URL(url, "https://kando.yldm.ai/").pathname);
    const file = path.join(dist, pathname);
    const ok = !failOnce.delete(pathname) && fs.existsSync(file) && fs.statSync(file).isFile();
    return Promise.resolve({
      ok,
      status: ok ? 200 : 404,
      json: () => Promise.resolve(JSON.parse(fs.readFileSync(file, "utf8")))
    });
  };
  const console = { warn: (...args) => warnings.push(args.join(" ")) };
  vm.runInContext(script, vm.createContext({ document, navigator, fetch, URL, console }), { filename: scriptPath });
  // Registration happens on a microtask, after the script returns.
  await new Promise((resolve) => setImmediate(resolve));
  return { tools, fetched, warnings };
}

// call runs a tool and checks that its result is MCP's shape: one text content, and structured data unless it is an error.
async function call(tools, name, input) {
  const result = await tools.get(name).execute(input);
  assert.equal(result.content.length, 1, name + " answered with " + result.content.length + " contents");
  assert.equal(result.content[0].type, "text");
  assert.equal(typeof result.content[0].text, "string");
  assert.ok(result.content[0].text.length > 0, name + " answered with no text");
  if (!result.isError) {
    assert.equal(typeof result.structuredContent, "object", name + " answered with no structured data");
  }
  return { text: result.content[0].text, data: result.structuredContent, isError: result.isError === true };
}

const macUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36";
const linuxArmUA = "Mozilla/5.0 (X11; Linux aarch64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36";
const windowsUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36";
const appleSilicon = {
  platform: "macOS",
  getHighEntropyValues: (hints) => Promise.resolve({ platform: "macOS", architecture: hints.includes("architecture") ? "arm" : undefined, bitness: "64" })
};

async function main() {
  // Without WebMCP the script registers nothing and throws nothing.
  const none = await load({ on: "neither" });
  assert.equal(none.tools.size, 0);
  assert.equal(none.fetched.length, 0);

  // Earlier builds have WebMCP on navigator.
  const legacy = await load({ on: "navigator" });
  assert.equal(legacy.tools.size, 4);

  const { tools, fetched, warnings } = await load({ userAgent: macUA });
  assert.deepEqual([...tools.keys()].sort(), ["get_download", "get_started", "list_features", "search_supported_sites"]);
  assert.equal(warnings.length, 0, warnings.join("\n"));
  assert.equal(fetched.length, 0, "the script fetched " + fetched.join(", ") + " before any tool was called");
  for (const tool of tools.values()) {
    assert.ok(tool.description.length > 80, tool.name + "'s description is too short for an agent to choose it by");
    assert.equal(tool.inputSchema.type, "object");
    assert.equal(tool.annotations.readOnlyHint, true, tool.name + " is not marked read-only");
    assert.equal(typeof tool.execute, "function");
  }

  // ---- search_supported_sites: catalog.Site.Matches, as TestFilterFindsSitesByNameDomainAliasAndLink checks it in the app ----
  const firstDomain = async (query) => {
    const r = await call(tools, "search_supported_sites", { query });
    assert.equal(r.isError, false, r.text);
    return r.data.sites.length ? r.data.sites[0].domain : "";
  };
  for (const [query, want] of [
    ["哔哩", "bilibili.com"],
    ["NICONICO", "nicovideo.jp"],
    ["x.com", "twitter.com"],
    ["https://www.bilibili.com/video/BV1xx411c7mD", "bilibili.com"],
    ["https://youtu.be/dQw4w9WgXcQ", "youtube.com"],
    ["www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube.com"],
    ["https://x.com/user/status/1", "twitter.com"],
    ["https://v.douyin.com/abc/", "douyin.com"]
  ]) {
    assert.equal(await firstDomain(query), want, query);
  }
  const link = await call(tools, "search_supported_sites", { query: "https://www.bilibili.com/video/BV1" });
  assert.equal(link.data.total, 1, "a bilibili link found " + link.data.total + " sites");
  assert.equal(link.data.supported, true);
  assert.equal(link.data.host, "www.bilibili.com");
  assert.match(link.text, /^Yes: the link is on 哔哩哔哩 \(bilibili\.com\)/);
  const unknown = await call(tools, "search_supported_sites", { query: "https://unknown-video-site.example/watch/1" });
  assert.equal(unknown.isError, false);
  assert.equal(unknown.data.supported, false);
  assert.equal(unknown.data.sites.length, 0);
  assert.match(unknown.text, /unknown-video-site\.example, is not in Kando's catalog/);
  const many = await call(tools, "search_supported_sites", { query: "tv", limit: 3 });
  assert.equal(many.data.sites.length, 3);
  assert.ok(many.data.total > 3, "tv found only " + many.data.total + " sites");
  assert.match(many.text, /the first 3:/);
  assert.equal((await call(tools, "search_supported_sites", {})).isError, true);
  assert.equal((await call(tools, "search_supported_sites", { query: "youtube", limit: 0 })).isError, true);
  assert.equal((await call(tools, "search_supported_sites", { query: "youtube", limit: 2.5 })).isError, true);
  const siteFetches = fetched.filter((url) => url.startsWith("/webmcp/sites.json"));
  assert.deepEqual(siteFetches, [pageData.sites], "the catalog was fetched " + siteFetches.length + " times, want once");

  // ---- get_download ----
  const release = pageData.release;
  const names = (r) => r.data.downloads.map((d) => d.name).join(" ");
  const v = release.version;
  // A Mac's User-Agent says Intel on Apple silicon too, so without client hints both disk images are offered.
  let r = await call(tools, "get_download", {});
  assert.equal(r.data.platform, "macos");
  assert.equal(r.data.arch, "");
  assert.equal(names(r), `kando-${v}-arm64.dmg kando-${v}-amd64.dmg`);
  assert.match(r.text, /does not say which processor this Mac has/);
  assert.match(r.text, /Open Anyway/);
  for (const d of r.data.downloads) {
    assert.ok(r.text.includes(d.url), "the text does not give " + d.url);
    assert.equal(d.url, "https://github.com/yldm-tech/Kando/releases/download/v" + v + "/" + d.name);
  }
  // Client hints tell Apple silicon.
  const hinted = await load({ userAgent: macUA, userAgentData: appleSilicon });
  r = await call(hinted.tools, "get_download", {});
  assert.equal(names(r), `kando-${v}-arm64.dmg`);
  assert.equal(r.data.detected.from, "User-Agent Client Hints");
  // An ARM64 Linux's User-Agent says aarch64.
  const linux = await load({ userAgent: linuxArmUA });
  r = await call(linux.tools, "get_download", {});
  assert.equal(names(r), `kando_${v}_arm64.deb kando-${v}-linux-arm64.tar.gz`);
  assert.match(r.text, /sudo apt install/);
  assert.match(r.text, /install\.sh/);
  // Asking for another system overrides the visitor's.
  r = await call(tools, "get_download", { platform: "linux", arch: "amd64" });
  assert.equal(names(r), `kando_${v}_amd64.deb kando-${v}-linux-amd64.tar.gz`);
  r = await call(tools, "get_download", { platform: "linux" });
  assert.equal(r.data.downloads.length, 4, "linux without an arch on a Mac lists " + names(r));
  // Windows has no build: the answer is building from source, and it is not an error.
  const windows = await load({ userAgent: windowsUA });
  r = await call(windows.tools, "get_download", {});
  assert.equal(r.isError, false);
  assert.equal(r.data.platform, "windows");
  assert.equal(r.data.downloads.length, 0);
  assert.match(r.text, /no Windows build/);
  assert.ok(r.text.includes(release.source), "the Windows answer does not link " + release.source);
  assert.equal((await call(tools, "get_download", { platform: "freebsd" })).isError, true);
  assert.equal((await call(tools, "get_download", { arch: "x86" })).isError, true);

  // ---- get_started and list_features: the page's language by default, any other by its code or tag ----
  const own = content(pageData.lang);
  r = await call(tools, "get_started", {});
  assert.equal(r.data.lang, pageData.lang);
  for (const key of ["startTitle", "startDownloadTitle", "startDownloadBody", "startToolsBody", "toolWhisper", "startSetup", "startBuildNote", "startReadme"]) {
    assert.ok(r.text.includes(own[key]), "get_started does not say " + key + ": " + own[key]);
  }
  assert.ok(r.text.includes("    git clone https://github.com/yldm-tech/Kando.git\n    cd Kando\n    make build"), "get_started does not give the build commands");
  r = await call(tools, "get_started", { lang: "zh-Hant" });
  assert.equal(r.data.lang, "zh-TW");
  assert.ok(r.text.startsWith(content("zh-TW").startTitle), r.text.slice(0, 40));
  r = await call(tools, "get_started", { lang: "KO" });
  assert.equal(r.data.lang, "ko");
  assert.equal((await call(tools, "get_started", { lang: "fr" })).isError, true);
  r = await call(tools, "list_features", { lang: "ja" });
  const ja = content("ja");
  for (const key of ["featuresTitle", "featureAgentsTitle", "featureAgentsBody", "featureModelLink"]) {
    assert.ok(r.text.includes(ja[key]), "list_features does not say " + key + ": " + ja[key]);
  }
  assert.ok(r.text.includes("kando mcp"));
  assert.equal(r.data.features.items.length, 9);
  // A language file is fetched once, however often it is asked for.
  await call(tools, "get_started", { lang: "ja" });
  const jaFetches = fetched.filter((url) => url.startsWith("/webmcp/ja.json"));
  assert.equal(jaFetches.length, 1, "ja.json was fetched " + jaFetches.length + " times");

  // A failed fetch is an error the agent can read, and is not kept: the next call fetches again.
  const flaky = await load({ failOnce: new Set(["/webmcp/en.json", "/webmcp/sites.json"]) });
  r = await call(flaky.tools, "get_started", { lang: "en" });
  assert.equal(r.isError, true);
  assert.match(r.text, /HTTP 404/);
  r = await call(flaky.tools, "get_started", { lang: "en" });
  assert.equal(r.isError, false, r.text);
  r = await call(flaky.tools, "search_supported_sites", { query: "vimeo" });
  assert.equal(r.isError, true);
  r = await call(flaky.tools, "search_supported_sites", { query: "vimeo" });
  assert.equal(r.data.sites[0].domain, "vimeo.com");
  assert.equal(flaky.fetched.length, 4);

  console.log("webmcp: " + tools.size + " tools checked on " + pageFile);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
