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

  // The hero's player plays its demo film, as the app's player plays one: the line and the scene follow the time, and every control does what it does in the app. It starts paused where the reader asked for less motion.
  function setUpPlayer() {
    var root = document.getElementById("player");
    if (!root) {
      return;
    }
    var length = Number(root.getAttribute("data-seconds"));
    var lineSeconds = Number(root.getAttribute("data-line-seconds"));
    var lines = Array.prototype.slice.call(root.querySelectorAll(".kp-line"));
    var cues = lines.map(function (line) {
      var parts = line.querySelectorAll(".kp-text span");
      return { translation: parts[0].textContent, original: parts[1].textContent };
    });
    var scenes = root.querySelectorAll(".kp-scene");
    var clips = root.querySelectorAll(".kp-clip");
    var segments = root.querySelectorAll(".kp-segs i");
    var translation = root.querySelector(".kp-tr");
    var original = root.querySelector(".kp-or");
    var clockNow = root.querySelector(".kp-now");
    var clockLeft = root.querySelector(".kp-left");
    var seek = root.querySelector(".kp-seek");
    var volume = root.querySelector(".kp-volume");
    var playButton = root.querySelector(".kp-play");
    var muteButton = root.querySelector(".kp-mute");
    var loopButton = root.querySelector('[data-act="loop"]');
    var modeLabel = root.querySelector(".kp-mode span");
    var modeLabels = root.getAttribute("data-mode-labels").split("|");
    var still = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    var state = { time: 0, playing: !still, loop: -1, mode: 0, volume: 0.8, muted: false };
    var shown = -1;

    function clock(seconds) {
      seconds = Math.max(0, Math.floor(seconds));
      return Math.floor(seconds / 60) + ":" + ("0" + (seconds % 60)).slice(-2);
    }

    function current() {
      return Math.min(cues.length - 1, Math.floor(state.time / lineSeconds));
    }

    function render() {
      var line = current();
      if (line !== shown) {
        shown = line;
        translation.textContent = cues[line].translation;
        original.textContent = cues[line].original;
        lines.forEach(function (el, i) { el.classList.toggle("on", i === line); });
        Array.prototype.forEach.call(scenes, function (el, i) { el.classList.toggle("on", i === Math.floor(line / 2)); });
        Array.prototype.forEach.call(clips, function (el, i) { el.classList.toggle("on", i === Math.floor(line / 2)); });
        Array.prototype.forEach.call(segments, function (el, i) {
          el.classList.toggle("done", i < line);
          el.classList.toggle("now", i === line);
        });
      }
      var share = (state.time / length) * 100 + "%";
      seek.querySelector(".kp-fill").style.width = share;
      seek.querySelector(".kp-knob").style.left = share;
      seek.setAttribute("aria-valuenow", String(Math.floor(state.time)));
      clockNow.textContent = clock(state.time);
      clockLeft.textContent = "-" + clock(length - state.time);
    }

    function seekTo(seconds) {
      state.time = Math.max(0, Math.min(length - 0.01, seconds));
      if (state.loop >= 0) {
        state.loop = current();
      }
      render();
    }

    function setPlaying(playing) {
      state.playing = playing;
      root.classList.toggle("paused", !playing);
      var label = root.getAttribute(playing ? "data-pause" : "data-play");
      playButton.setAttribute("aria-label", label);
      playButton.title = label;
    }

    function setVolume(level, muted) {
      state.volume = level;
      state.muted = muted || level === 0;
      var share = (state.muted ? 0 : level) * 100 + "%";
      volume.querySelector(".kp-fill").style.width = share;
      volume.querySelector(".kp-knob").style.left = share;
      volume.setAttribute("aria-valuenow", String(Math.round((state.muted ? 0 : level) * 100)));
      root.classList.toggle("muted", state.muted);
      muteButton.setAttribute("aria-pressed", state.muted ? "true" : "false");
      var label = root.getAttribute(state.muted ? "data-unmute" : "data-mute");
      muteButton.setAttribute("aria-label", label);
      muteButton.title = label;
    }

    function setMode(mode) {
      state.mode = mode;
      root.setAttribute("data-mode", String(mode));
      modeLabel.textContent = modeLabels[mode];
      press(root.querySelectorAll("[data-mode]"), "data-mode", String(mode));
    }

    // press marks the button of buttons whose attr is value as pressed and the others as not.
    function press(buttons, attr, value) {
      Array.prototype.forEach.call(buttons, function (b) {
        if (b !== root) {
          b.setAttribute("aria-pressed", b.getAttribute(attr) === value ? "true" : "false");
        }
      });
    }

    function share(event, el) {
      var box = el.getBoundingClientRect();
      return Math.max(0, Math.min(1, (event.clientX - box.left) / box.width));
    }

    root.addEventListener("click", function (event) {
      var line = event.target.closest(".kp-line");
      if (line) {
        seekTo((Number(line.getAttribute("data-line")) - 1) * lineSeconds + 0.05);
        return;
      }
      var clip = event.target.closest(".kp-clip");
      if (clip) {
        seekTo((Number(clip.getAttribute("data-clip")) - 1) * 2 * lineSeconds + 0.05);
        setPlaying(true);
        return;
      }
      var choice = event.target.closest("button[data-size], button[data-order], button[data-mode]");
      if (choice) {
        if (choice.hasAttribute("data-size")) {
          root.setAttribute("data-size", choice.getAttribute("data-size"));
          press(root.querySelectorAll("[data-size]"), "data-size", choice.getAttribute("data-size"));
        } else if (choice.hasAttribute("data-order")) {
          root.setAttribute("data-order", choice.getAttribute("data-order"));
          press(root.querySelectorAll("[data-order]"), "data-order", choice.getAttribute("data-order"));
        } else {
          setMode(Number(choice.getAttribute("data-mode")));
        }
        return;
      }
      var control = event.target.closest("[data-act]");
      if (!control) {
        return;
      }
      switch (control.getAttribute("data-act")) {
        case "toggle":
          setPlaying(!state.playing);
          break;
        case "back":
          seekTo(state.time - 5);
          break;
        case "forward":
          seekTo(state.time + 5);
          break;
        case "seek":
          seekTo(share(event, control) * length);
          break;
        case "volume":
          setVolume(share(event, control), false);
          break;
        case "mute":
          setVolume(state.volume || 0.8, !state.muted);
          break;
        case "loop":
          state.loop = state.loop >= 0 ? -1 : current();
          root.classList.toggle("looping", state.loop >= 0);
          loopButton.setAttribute("aria-pressed", state.loop >= 0 ? "true" : "false");
          break;
        case "mode":
          setMode((state.mode + 1) % modeLabels.length);
          break;
        case "panel":
          var hidden = root.classList.toggle("no-panel");
          Array.prototype.forEach.call(root.querySelectorAll('[data-act="panel"]'), function (b) {
            b.setAttribute("aria-pressed", hidden ? "false" : "true");
          });
          break;
        case "fullscreen":
          var win = root.closest(".win") || root;
          if (document.fullscreenElement) {
            document.exitFullscreen();
          } else if (win.requestFullscreen) {
            win.requestFullscreen();
          }
          break;
      }
    });

    // The arrow keys move the time and the volume, as they do on the app's sliders.
    root.addEventListener("keydown", function (event) {
      var step = event.key === "ArrowRight" || event.key === "ArrowUp" ? 1 : event.key === "ArrowLeft" || event.key === "ArrowDown" ? -1 : 0;
      if (!step) {
        return;
      }
      if (event.target === seek) {
        seekTo(state.time + step * 5);
      } else if (event.target === volume) {
        setVolume(Math.max(0, Math.min(1, (state.muted ? 0 : state.volume) + step * 0.1)), false);
      } else {
        return;
      }
      event.preventDefault();
    });

    var search = root.querySelector(".kp-search");
    search.addEventListener("input", function () {
      var query = search.value.trim().toLowerCase();
      var hits = 0;
      lines.forEach(function (el, i) {
        var hit = !query || (cues[i].translation + " " + cues[i].original).toLowerCase().indexOf(query) >= 0;
        el.hidden = !hit;
        hits += hit ? 1 : 0;
      });
      root.querySelector(".kp-none").hidden = hits > 0;
    });

    window.setInterval(function () {
      if (!state.playing) {
        return;
      }
      var next = state.time + 0.25;
      if (state.loop >= 0 && (next >= (state.loop + 1) * lineSeconds || next < state.loop * lineSeconds)) {
        next = state.loop * lineSeconds;
      }
      state.time = next >= length ? 0 : next;
      render();
    }, 250);

    setPlaying(state.playing);
    render();
  }

  document.addEventListener("DOMContentLoaded", function () {
    setUpTabs();
    setUpHeader();
    setUpQuestions();
    setUpPlayer();
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
