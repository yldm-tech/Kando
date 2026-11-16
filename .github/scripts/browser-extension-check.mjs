// browser-extension-check.mjs <folder>
//
// Checks Kando's browser extension (apps/kando/resources/browser-extension) before it is zipped: its manifest.json is a Manifest V3 manifest whose every file is there, its strings are in each of the five languages with the same names, and every script parses. It uses Node.js alone, with no package.
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const dir = process.argv[2];
const errors = [];
const fail = (msg) => errors.push(msg);
const json = (path) => JSON.parse(readFileSync(join(dir, path), "utf8"));

const m = json("manifest.json");
if (m.manifest_version !== 3) fail(`manifest_version is ${m.manifest_version}, not 3`);
if (!/^\d+(\.\d+){0,3}$/.test(m.version || "")) fail(`version ${JSON.stringify(m.version)} is not one the browsers take`);
if (typeof m.key !== "string" || !/^[A-Za-z0-9+/]+=*$/.test(m.key)) fail("key is not a public key in base64");
const gecko = (m.browser_specific_settings || {}).gecko || {};
if (!/^[a-zA-Z0-9-._]*@[a-zA-Z0-9-._]+$/.test(gecko.id || "")) fail(`browser_specific_settings.gecko.id ${JSON.stringify(gecko.id)} is not an add-on ID`);
const allowed = new Set(["nativeMessaging", "contextMenus", "activeTab"]);
for (const p of m.permissions || []) if (!allowed.has(p)) fail(`permission ${p} is not one Kando's extension asks for`);
if (m.host_permissions) fail("host_permissions: the extension reaches pages through its content script's matches alone");

// Every file the manifest names, and the scripts popup.html loads.
const files = [
  ...Object.values(m.icons || {}),
  ...Object.values((m.action || {}).default_icon || {}),
  (m.action || {}).default_popup,
  (m.background || {}).service_worker,
  ...((m.background || {}).scripts || []),
  ...(m.content_scripts || []).flatMap((c) => [...(c.js || []), ...(c.css || [])]),
].filter(Boolean);
if (m.action && m.action.default_popup) {
  const html = readFileSync(join(dir, m.action.default_popup), "utf8");
  for (const [, src] of html.matchAll(/<script src="([^"]+)"/g)) files.push(src);
  if (/<script>/.test(html)) fail(`${m.action.default_popup} has an inline script, which Manifest V3 refuses`);
}
for (const f of files) if (!existsSync(join(dir, f))) fail(`${f} is named but not there`);
const pattern = /^(\*|https?):\/\/(\*|\*\.[^/*]+|[^/*]+)\/.*$/;
for (const c of m.content_scripts || []) for (const p of c.matches || []) if (!pattern.test(p)) fail(`match pattern ${p}`);

// The strings: the five languages of Kando's catalogs, each with every name the others have, and every __MSG_name__ the manifest uses.
const locales = readdirSync(join(dir, "_locales")).sort();
const want = ["en", "ja", "ko", "zh_CN", "zh_TW"];
if (locales.join() !== want.join()) fail(`_locales has ${locales.join(", ")}, not ${want.join(", ")}`);
if (!locales.includes(m.default_locale)) fail(`default_locale ${m.default_locale} has no strings`);
const names = (loc) => Object.keys(json(join("_locales", loc, "messages.json"))).sort().join();
for (const loc of locales) if (names(loc) !== names(m.default_locale)) fail(`_locales/${loc} does not name the strings ${m.default_locale} does`);
const source = json(join("_locales", m.default_locale || "en", "messages.json"));
for (const [, name] of JSON.stringify(m).matchAll(/__MSG_(\w+)__/g)) if (!source[name]) fail(`__MSG_${name}__ has no string`);
for (const f of readdirSync(dir).filter((f) => f.endsWith(".js"))) {
  for (const [, name] of readFileSync(join(dir, f), "utf8").matchAll(/(?:getMessage|\bt)\("(\w+)"\)/g)) if (!source[name]) fail(`${f} uses the string ${name}, which is not there`);
}

// Every script parses.
const scripts = [];
const walk = (d) => {
  for (const f of readdirSync(d)) {
    const p = join(d, f);
    if (statSync(p).isDirectory()) walk(p);
    else if (f.endsWith(".js") || f.endsWith(".mjs")) scripts.push(p);
  }
};
walk(dir);
for (const s of scripts) {
  try { execFileSync(process.execPath, ["--check", s], { stdio: "pipe" }); } catch (e) { fail(`${s} does not parse: ${e.stderr}`); }
}

if (errors.length) {
  for (const e of errors) console.error(`::error::${e}`);
  process.exit(1);
}
console.log(`browser extension ${m.version}: ${files.length} files, ${locales.length} languages, ${scripts.length} scripts`);
