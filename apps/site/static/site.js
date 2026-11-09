// Kando's landing page. Loaded in <head> without defer, so that a first-time reader of the English page who reads Chinese, Japanese or Korean is sent to their page before this one paints.
(function () {
  "use strict";

  var STORAGE_KEY = "kando-lang";

  // localStorage throws when storage is disabled, as in some private windows; the page then simply does not remember the choice.
  function storedLanguage() {
    try {
      return window.localStorage.getItem(STORAGE_KEY);
    } catch (e) {
      return null;
    }
  }

  function storeLanguage(code) {
    try {
      window.localStorage.setItem(STORAGE_KEY, code);
    } catch (e) {
      // Not remembered; see storedLanguage.
    }
  }

  // pageFor maps a language tag of navigator.languages to the code of a page, or "" when there is no page in that language.
  function pageFor(tag) {
    var t = String(tag).toLowerCase();
    var is = function (prefix) {
      return t === prefix || t.indexOf(prefix + "-") === 0;
    };
    if (is("zh-hant") || is("zh-tw") || is("zh-hk") || is("zh-mo")) {
      return "zh-TW";
    }
    if (is("zh")) {
      return "zh-CN";
    }
    if (is("ja")) {
      return "ja";
    }
    if (is("ko")) {
      return "ko";
    }
    if (is("en")) {
      return "en";
    }
    return "";
  }

  // sameOriginReferrer reports whether the reader came to this page from another page of the site.
  function sameOriginReferrer() {
    try {
      return document.referrer !== "" && new URL(document.referrer).origin === window.location.origin;
    } catch (e) {
      return false;
    }
  }

  // On the root page only, with no language chosen before, no ?lang= in the address and no arrival from another page of the site, follow the first of the reader's languages that has a page. A move to / within the site (the switcher, the logo, the 404 page's home link) is never redirected, so that choosing English holds even where storage is unavailable and the choice cannot be remembered. The pages come from the hreflang alternates above this script, so a language without a page is never a target.
  var root = document.documentElement;
  if (root.hasAttribute("data-root") && !storedLanguage() && !/[?&]lang=/.test(window.location.search) && !sameOriginReferrer()) {
    var pages = {};
    var links = document.querySelectorAll('link[rel="alternate"][data-lang]');
    for (var i = 0; i < links.length; i++) {
      // The alternates are absolute production addresses; follow only their path, so that the redirect stays on whatever host serves the page, such as a local preview.
      pages[links[i].getAttribute("data-lang")] = new URL(links[i].getAttribute("href")).pathname;
    }
    var wanted = navigator.languages && navigator.languages.length ? navigator.languages : [navigator.language || ""];
    for (var j = 0; j < wanted.length; j++) {
      var code = pageFor(wanted[j]);
      if (code && pages[code]) {
        if (code !== "en") {
          window.location.replace(pages[code] + window.location.hash);
        }
        break;
      }
    }
  }

  // A language picked in a switcher is remembered, so that the root page no longer redirects.
  document.addEventListener("click", function (event) {
    var link = event.target.closest && event.target.closest("a[data-lang]");
    if (link) {
      storeLanguage(link.getAttribute("data-lang"));
    }
    // A click outside the open language menu closes it.
    var open = document.querySelector("details.lang[open]");
    if (open && !open.contains(event.target)) {
      open.removeAttribute("open");
    }
  });

  document.addEventListener("keydown", function (event) {
    if (event.key !== "Escape") {
      return;
    }
    var open = document.querySelector("details.lang[open]");
    if (open) {
      open.removeAttribute("open");
      open.querySelector("summary").focus();
    }
  });

  // Copy buttons stay hidden without script; with it, each copies its command.
  document.addEventListener("DOMContentLoaded", function () {
    if (!navigator.clipboard) {
      return;
    }
    var buttons = document.querySelectorAll("button.copy");
    for (var i = 0; i < buttons.length; i++) {
      var button = buttons[i];
      button.hidden = false;
      button.addEventListener("click", function () {
        var b = this;
        var label = b.getAttribute("data-label") || b.textContent;
        b.setAttribute("data-label", label);
        var text = b.parentNode.querySelector("code").textContent;
        navigator.clipboard.writeText(text).then(function () {
          b.textContent = b.getAttribute("data-copied");
          b.classList.add("done");
          setTimeout(function () {
            b.textContent = label;
            b.classList.remove("done");
          }, 1600);
        });
      });
    }
  });
})();
