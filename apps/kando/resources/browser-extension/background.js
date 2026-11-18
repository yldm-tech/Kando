// Kando's extension in the background: Chrome's service worker, Firefox's event page. It holds the port to Kando's native messaging host, Kando's own executable, which relays to the running app (apps/kando/browserext), and answers the content script's buttons (content.js), the popup and the menu item on links.
"use strict";

if (typeof KANDO === "undefined" && typeof importScripts === "function") importScripts("sites.js");

const api = globalThis.browser || globalThis.chrome;

// The port to the host, opened at the first call and closed after a minute without one, so neither the host nor this worker stays for nothing. Each call waits for the answer with its ID.
const IDLE = 60 * 1000, WAIT = 40 * 1000;
let port = null, next = 0, idle = 0;
const waiting = new Map();

// unreachable is the error of a call when the host could not be started: Kando is not connected to this browser (Settings › General › Browser Extension), or not installed.
const unreachable = (message) => ({ code: "unreachable", message: message || "" });

const connect = () => {
  const p = api.runtime.connectNative(KANDO.host);
  p.onMessage.addListener((m) => {
    const done = m && waiting.get(m.id);
    if (!done) return;
    waiting.delete(m.id);
    done(m);
  });
  p.onDisconnect.addListener(() => {
    const error = unreachable((api.runtime.lastError && api.runtime.lastError.message) || (p.error && p.error.message));
    if (port === p) port = null;
    for (const done of waiting.values()) done({ error });
    waiting.clear();
  });
  return p;
};

// call asks the app for method with params through the host, and resolves to its answer: { result } or { error: { code, message } }.
const call = (method, params) => new Promise((resolve) => {
  clearTimeout(idle);
  if (!port) {
    try { port = connect(); } catch (e) { resolve({ error: unreachable(String(e)) }); return; }
  }
  const id = ++next;
  const timer = setTimeout(() => { waiting.delete(id); resolve({ error: { code: "timeout", message: "" } }); }, WAIT);
  waiting.set(id, (m) => {
    clearTimeout(timer);
    if (!waiting.size) idle = setTimeout(() => { if (port && !waiting.size) { port.disconnect(); port = null; } }, IDLE);
    resolve(m.error ? { error: m.error } : { result: m.result });
  });
  try { port.postMessage({ id, method, params }); } catch (e) { waiting.delete(id); clearTimeout(timer); resolve({ error: unreachable(String(e)) }); }
});

// warn marks the toolbar button while Kando cannot be reached, so the popup can say why; it clears once a call goes through.
const warn = (error) => {
  const on = !!error && error.code === "unreachable";
  try {
    api.action.setBadgeText({ text: on ? "!" : "" });
    if (on) api.action.setBadgeBackgroundColor({ color: "#d93025" });
  } catch (e) {}
};

// fallbackText is the buttons' words from the extension's own strings, while the app, which says them in its own language, is not running.
const fallbackText = () => ({ add: api.i18n.getMessage("buttonAdd"), queued: api.i18n.getMessage("buttonQueued"), done: api.i18n.getMessage("buttonDone") });

const state = (m) => KANDO.state + JSON.stringify(m);

// ask answers the buttons' question about the videos of a page (KANDO.ask): what the library has of each, and the buttons' words. Without the app, the buttons show to add each, in the extension's words, as a click starts the app.
const ask = async (urls) => {
  const { result, error } = await call("browser.state", { urls });
  warn(error);
  if (result) return state(result);
  if (error && (error.code === KANDO.notRunning || error.code === "unreachable")) return state({ text: fallbackText(), have: {} });
  return null;
};

// add adds one video, as a button of a page, the popup or the menu item asks, and answers with what the library has of it, and what happened in the app's words. at is where the page's player was in it, in seconds, which 下载视频 says, so the film opens there once done; 0 for none.
const add = async (url, title, at) => {
  const { result, error } = await call("browser.add", { url, title: title || "", at: Number.isFinite(at) && at > 0 ? Math.floor(at) : 0 });
  warn(error);
  if (!result) return { have: {}, error };
  return { have: { [url]: result.have || "" }, message: result.message || "", failed: !!result.failed };
};

api.runtime.onMessage.addListener((msg, sender, reply) => {
  if (sender.id !== api.runtime.id) return false;
  // The content script's buttons speak as the built-in browser's pages do (apps/kando/browse/buttons.go).
  if (typeof msg === "string") {
    if (msg.startsWith(KANDO.ask)) {
      let urls;
      try { urls = JSON.parse(msg.slice(KANDO.ask.length)); } catch (e) { return false; }
      if (!Array.isArray(urls)) return false;
      ask(urls.slice(0, KANDO.maxAsk).filter((u) => typeof u === "string" && KANDO.videoOf(u) === u)).then(reply);
      return true;
    }
    if (msg.startsWith(KANDO.add)) {
      let m;
      try { m = JSON.parse(msg.slice(KANDO.add.length)); } catch (e) { return false; }
      if (!m || typeof m.url !== "string" || KANDO.videoOf(m.url) !== m.url) return false;
      add(m.url, typeof m.title === "string" ? m.title : "", typeof m.at === "number" ? m.at : 0).then((r) => reply(state({ have: r.have })));
      return true;
    }
    return false;
  }
  // The popup's requests.
  if (msg && msg.kando === "state" && typeof msg.url === "string") {
    call("browser.state", { urls: [msg.url] }).then(({ result, error }) => { warn(error); reply({ have: result ? result.have[msg.url] || "" : "", text: result ? result.text : fallbackText(), error }); });
    return true;
  }
  if (msg && msg.kando === "add" && typeof msg.url === "string") {
    add(msg.url, msg.title).then(reply);
    return true;
  }
  if (msg && msg.kando === "show" && typeof msg.url === "string") {
    call("browser.show", { url: msg.url }).then(({ error }) => { warn(error); reply({ error }); });
    return true;
  }
  return false;
});

// The menu item on links: Add to Kando, on the links of the sites the buttons know whose shape is a video's (KANDO.menu), and only for a link that is one video, as Firefox can hide it for the others before it shows.
const MENU = "kando-add";
const menus = api.menus || api.contextMenus;
api.runtime.onInstalled.addListener(() => {
  menus.create({ id: MENU, title: api.i18n.getMessage("menuAdd"), contexts: ["link"], targetUrlPatterns: KANDO.menu });
});
menus.onClicked.addListener((info) => {
  if (info.menuItemId !== MENU) return;
  const url = KANDO.videoOf(info.linkUrl || "");
  if (url) add(url, info.linkText || "");
});
if (menus.onShown) {
  menus.onShown.addListener((info) => {
    // A hidden item is not among info.menuIds, so every menu on a link sets it again.
    if (!info.contexts.includes("link")) return;
    menus.update(MENU, { visible: !!KANDO.videoOf(info.linkUrl || "") });
    menus.refresh();
  });
}
