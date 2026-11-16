// The toolbar button's popup: what Kando has of the video of the current tab, with Add to Kando or Show in Kando; on a page of no video, or while Kando cannot be reached, one line that says so.
"use strict";

const api = globalThis.browser || globalThis.chrome;
const t = (name) => api.i18n.getMessage(name);
const $ = (id) => document.getElementById(id);

const say = (text) => { $("line").textContent = text; };

// show draws what Kando has of the video: have is "", "queued" or "done", and text the buttons' words in Kando's language.
const show = (url, have, text) => {
  say(have === "done" ? text.done : have === "queued" ? text.queued : "");
  $("add").hidden = !!have;
  $("show").hidden = !have;
};

// unreachable says to connect Kando to this browser, when the host could not be started.
const unreachable = (error) => !!error && error.code === "unreachable";

const main = async () => {
  $("add").textContent = t("popupAdd");
  $("show").textContent = t("popupShow");
  const [tab] = await api.tabs.query({ active: true, currentWindow: true });
  const url = tab && KANDO.videoOf(tab.url || "");
  if (!url) { say(t("popupNoVideo")); return; }
  const title = KANDO.title(url, tab.title);
  $("title").textContent = title;
  const answer = await api.runtime.sendMessage({ kando: "state", url });
  if (unreachable(answer.error)) { say(t("popupConnect")); return; }
  show(url, answer.have, answer.text);
  $("add").addEventListener("click", async () => {
    $("add").disabled = true;
    const r = await api.runtime.sendMessage({ kando: "add", url, title });
    $("add").disabled = false;
    if (unreachable(r.error)) { say(t("popupConnect")); return; }
    if (r.error) { say(r.error.message || ""); return; }
    const have = r.have[url] || "";
    show(url, have, answer.text);
    if (r.message) say(r.message);
  });
  $("show").addEventListener("click", async () => {
    const r = await api.runtime.sendMessage({ kando: "show", url });
    if (unreachable(r.error)) { say(t("popupConnect")); return; }
    window.close();
  });
};

main();
