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

  // With script the tab panels become tabs and the header follows the page; without it every panel shows and the header stays as it is.
  document.documentElement.classList.add("js");

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

  // selectTab shows the panel of tab in its group and hides the others.
  function selectTab(tab) {
    var group = tab.closest("[data-tabs]");
    var tabs = group.querySelectorAll('[role="tab"]');
    for (var i = 0; i < tabs.length; i++) {
      var on = tabs[i] === tab;
      tabs[i].setAttribute("aria-selected", on ? "true" : "false");
      tabs[i].tabIndex = on ? 0 : -1;
      document.getElementById(tabs[i].getAttribute("aria-controls")).hidden = !on;
    }
  }

  function setUpTabs() {
    var groups = document.querySelectorAll("[data-tabs]");
    for (var i = 0; i < groups.length; i++) {
      var first = groups[i].querySelector('[role="tab"][aria-selected="true"]');
      if (first) {
        selectTab(first);
      }
    }
    document.addEventListener("click", function (event) {
      var tab = event.target.closest && event.target.closest('[role="tab"]');
      if (tab) {
        selectTab(tab);
        return;
      }
      // A link to the download panel that names a system, as Windows and Linux under the headline, opens that system's tab.
      var link = event.target.closest && event.target.closest("a[data-os]");
      if (link) {
        var want = document.querySelector('[role="tab"][data-os="' + link.getAttribute("data-os") + '"]');
        if (want) {
          selectTab(want);
        }
      }
    });
    // The arrow keys move between the tabs of a group, as a tab list's keys do.
    document.addEventListener("keydown", function (event) {
      var tab = event.target.closest && event.target.closest('[role="tab"]');
      if (!tab || (event.key !== "ArrowRight" && event.key !== "ArrowLeft")) {
        return;
      }
      var tabs = Array.prototype.slice.call(tab.parentNode.querySelectorAll('[role="tab"]'));
      var next = tabs[(tabs.indexOf(tab) + (event.key === "ArrowRight" ? 1 : tabs.length - 1)) % tabs.length];
      selectTab(next);
      next.focus();
      event.preventDefault();
    });
  }

  // The header turns solid once the page scrolls, and its pill marks the section being read.
  function setUpHeader() {
    var top = document.querySelector(".top");
    var links = document.querySelectorAll(".nav a");
    if (!top) {
      return;
    }
    var sections = [];
    for (var i = 0; i < links.length; i++) {
      var section = document.getElementById(links[i].getAttribute("href").slice(1));
      if (section) {
        sections.push({ link: links[i], section: section });
      }
    }
    var pending = false;
    function update() {
      pending = false;
      top.classList.toggle("scrolled", window.scrollY > 12);
      var current = null;
      for (var j = 0; j < sections.length; j++) {
        if (sections[j].section.getBoundingClientRect().top < window.innerHeight * 0.4) {
          current = sections[j].link;
        }
      }
      for (var k = 0; k < sections.length; k++) {
        sections[k].link.classList.toggle("on", sections[k].link === current);
      }
    }
    window.addEventListener("scroll", function () {
      if (!pending) {
        pending = true;
        window.requestAnimationFrame(update);
      }
    }, { passive: true });
    update();
  }

  // One question of the FAQ is open at a time.
  function setUpQuestions() {
    var questions = document.querySelectorAll("details.qa");
    for (var i = 0; i < questions.length; i++) {
      questions[i].addEventListener("toggle", function () {
        if (!this.open) {
          return;
        }
        for (var j = 0; j < questions.length; j++) {
          if (questions[j] !== this) {
            questions[j].open = false;
          }
        }
      });
    }
  }

  document.addEventListener("DOMContentLoaded", function () {
    setUpTabs();
    setUpHeader();
    setUpQuestions();
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
