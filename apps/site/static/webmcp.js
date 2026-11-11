// WebMCP tools of Kando's landing page, for AI agents that browse it: whether Kando downloads from a website, the download for the visitor's system, and the page's Get started steps and features in any of its languages. In a browser without WebMCP it registers nothing and does nothing else.
//
// It follows the WebMCP draft of the W3C Web Machine Learning Community Group, as of November 2026: document.modelContext.registerTool(tool), where tool is {name, description, inputSchema, execute, annotations} and execute(input) returns a promise of the result. Earlier drafts and builds put the same object on navigator, so that is used when document has none. A result is MCP's tool result, {content: [{type: "text", text}], structuredContent}, with isError set when the input cannot be answered, so that the agent can correct it and call again.
//
// The tools answer from files the build writes into /webmcp/ (apps/site/buildtime/webmcp.ts), named with their addresses in the page's <script id="webmcp-data">, and fetched only when a tool is called, so that a visitor without an agent loads nothing more.
(function () {
  "use strict";

  var modelContext = document.modelContext || navigator.modelContext;
  if (!modelContext || typeof modelContext.registerTool !== "function") {
    return;
  }

  var page = JSON.parse(document.getElementById("webmcp-data").textContent);
  var release = page.release;
  var langCodes = page.langs.map(function (l) {
    return l.code;
  });

  function textResult(text, data) {
    return { content: [{ type: "text", text: text }], structuredContent: data };
  }

  function errorResult(text) {
    return { content: [{ type: "text", text: text }], isError: true };
  }

  // fetchJSON is the file at path, fetched once and kept; a failed fetch is forgotten, so that the next call tries again.
  var fetched = {};
  function fetchJSON(path) {
    if (!fetched[path]) {
      fetched[path] = fetch(path).then(function (response) {
        if (!response.ok) {
          throw new Error("HTTP " + response.status + " for " + path);
        }
        return response.json();
      });
      fetched[path].catch(function () {
        delete fetched[path];
      });
    }
    return fetched[path];
  }

  // ---- search_supported_sites ----

  // linkHost is the host of query when it is a link to a page, with or without its scheme (a host then needs a path after it); "" otherwise. It is apps/kando/sites/catalog's linkHost.
  function linkHost(query) {
    var q = String(query).trim().toLowerCase();
    if (/[ \t]/.test(q) || q.indexOf(".") < 0) {
      return "";
    }
    if (q.indexOf("://") < 0) {
      if (q.indexOf("/") < 0) {
        return "";
      }
      q = "https://" + q;
    }
    var url;
    try {
      url = new URL(q);
    } catch (e) {
      return "";
    }
    return url.hostname.replace(/\.$/, "");
  }

  // siteMatches is catalog.Site.Matches: a link finds the site whose domain, or a domain among its aliases, is its host or a parent of it; other text finds a site when every word of it, ignoring case, is part of its name, its domain or one of its aliases; an empty query finds every site.
  function siteMatches(site, query) {
    var domains = [site.domain].concat(site.aliases || []);
    var host = linkHost(query);
    if (host) {
      return domains.some(function (domain) {
        return domain.indexOf(".") >= 0 && (host === domain || host.slice(-(domain.length + 1)) === "." + domain);
      });
    }
    var words = String(query).toLowerCase().split(/\s+/).filter(Boolean);
    var hay = (site.name + " " + domains.join(" ")).toLowerCase();
    return words.every(function (word) {
      return hay.indexOf(word) >= 0;
    });
  }

  // otherDomains are a site's aliases that are domains; the others are yt-dlp's extractor names, which mean nothing to a reader.
  function otherDomains(site) {
    return (site.aliases || []).filter(function (alias) {
      return alias.indexOf(".") >= 0;
    });
  }

  function searchSupportedSites(input) {
    var query = input && input.query;
    var limit = input && input.limit !== undefined ? input.limit : 10;
    if (typeof query !== "string" || query.trim() === "") {
      return Promise.resolve(errorResult("query is required: a website's name, its domain (such as vimeo.com) or a link to a video on it."));
    }
    if (typeof limit !== "number" || limit % 1 !== 0 || limit < 1 || limit > 100) {
      return Promise.resolve(errorResult("limit must be a whole number from 1 to 100, not " + JSON.stringify(limit) + "."));
    }
    return fetchJSON(page.sites).then(
      function (catalog) {
        var found = catalog.sites.filter(function (site) {
          return siteMatches(site, query);
        });
        var host = linkHost(query);
        var shown = found.slice(0, limit).map(function (site) {
          return { name: site.name, domain: site.domain, aliases: site.aliases || [] };
        });
        var data = { query: query, link: host !== "", host: host, supported: found.length > 0, total: found.length, sites: shown, catalogSize: catalog.sites.length, ytdlp: catalog.ytdlp };
        var about = "Kando downloads through yt-dlp; its catalog lists " + catalog.sites.length + " websites (yt-dlp " + catalog.ytdlp + ").";
        if (found.length === 0) {
          var what = host ? "The link's website, " + host + "," : "No website matching " + JSON.stringify(query);
          return textResult(what + " is not in Kando's catalog of supported websites. " + about + " A site not listed may still work when yt-dlp's generic extractor finds a video on the page, but that is not certain. Try another spelling, the site's domain, or a link to one of its videos.", data);
        }
        var lines = shown.map(function (site) {
          var more = otherDomains(site);
          return "- " + site.name + " (" + site.domain + (more.length ? "; also " + more.join(", ") : "") + ")";
        });
        var head;
        if (host) {
          head = "Yes: the link is on " + shown[0].name + " (" + shown[0].domain + "), which Kando supports. Paste the link into Kando's window, or run kando add <link> in a terminal.";
        } else {
          head = found.length + (found.length === 1 ? " supported website matches " : " supported websites match ") + JSON.stringify(query) + (found.length > shown.length ? "; the first " + shown.length + ":" : ":");
        }
        return textResult(head + "\n" + lines.join("\n") + "\n" + about, data);
      },
      function (err) {
        return errorResult("Could not load the list of supported websites (" + err.message + "). Try again, or see Help › Supported Websites… in the Kando app.");
      }
    );
  }

  // ---- get_download ----

  var platformNames = { macos: "macOS", linux: "Linux", windows: "Windows", android: "Android", ios: "iOS", chromeos: "ChromeOS" };

  // archName is how a reader of platform knows arch: by the processor's maker on a Mac, by its instruction set elsewhere.
  function archName(platform, arch) {
    if (platform === "macos") {
      return arch === "arm64" ? "Apple silicon" : "Intel";
    }
    return arch === "arm64" ? "ARM64" : "x86-64";
  }

  // platformOf names the system of a userAgentData.platform or a User-Agent string as get_download does, or "" when it is none of them.
  function platformOf(text) {
    var s = String(text || "");
    if (/android/i.test(s)) {
      return "android";
    }
    if (/iphone|ipad|ipod|^ios$/i.test(s)) {
      return "ios";
    }
    if (/\bcros\b|chrome ?os|chromium ?os/i.test(s)) {
      return "chromeos";
    }
    if (/macintosh|mac ?os/i.test(s)) {
      return "macos";
    }
    if (/windows|win32|win64/i.test(s)) {
      return "windows";
    }
    if (/linux|x11/i.test(s)) {
      return "linux";
    }
    return "";
  }

  // archOfUserAgent is the architecture a User-Agent string names, or "" when it names none. A Mac's says Intel even on Apple silicon, so it is not read on a Mac.
  function archOfUserAgent(ua) {
    if (/aarch64|arm64/i.test(ua)) {
      return "arm64";
    }
    if (/x86_64|x64|win64|wow64|amd64/i.test(ua)) {
      return "amd64";
    }
    return "";
  }

  // detectSystem is the visitor's system as the browser tells it: from User-Agent Client Hints where the browser has them, which also tell a Mac's architecture, and from the User-Agent string otherwise.
  function detectSystem() {
    var ua = navigator.userAgent || "";
    var guess = { platform: platformOf(ua), arch: "", from: "User-Agent" };
    if (guess.platform !== "macos") {
      guess.arch = archOfUserAgent(ua);
    }
    var hints = navigator.userAgentData;
    if (!hints || typeof hints.getHighEntropyValues !== "function") {
      return Promise.resolve(guess);
    }
    return hints.getHighEntropyValues(["architecture", "bitness"]).then(
      function (values) {
        var arch = "";
        if (values.architecture === "arm" && values.bitness === "64") {
          arch = "arm64";
        } else if (values.architecture === "x86" && values.bitness === "64") {
          arch = "amd64";
        }
        return { platform: platformOf(hints.platform) || guess.platform, arch: arch || guess.arch, from: "User-Agent Client Hints" };
      },
      function () {
        return guess;
      }
    );
  }

  function describeDownload(d) {
    return "- " + d.name + " for " + platformNames[d.platform] + " on " + archName(d.platform, d.arch) + ": " + d.url;
  }

  function getDownload(input) {
    var platform = input && input.platform;
    var arch = input && input.arch;
    if (platform !== undefined && !/^(macos|linux|windows)$/.test(platform)) {
      return Promise.resolve(errorResult("platform must be macos, linux or windows, not " + JSON.stringify(platform) + "."));
    }
    if (arch !== undefined && !/^(arm64|amd64)$/.test(arch)) {
      return Promise.resolve(errorResult("arch must be arm64 (Apple silicon, ARM64 Windows and Linux) or amd64 (Intel Macs, x86-64 Windows and Linux), not " + JSON.stringify(arch) + "."));
    }
    return detectSystem().then(function (detected) {
      var system = { platform: platform || detected.platform, arch: arch || (platform && platform !== detected.platform ? "" : detected.arch) };
      var data = { version: release.version, release: release.page, checksums: release.checksums, platform: system.platform, arch: system.arch, detected: platform && arch ? null : detected, downloads: [], buildFromSource: release.source };
      var name = platformNames[system.platform] || "an unknown system";
      if (system.platform !== "macos" && system.platform !== "linux" && system.platform !== "windows") {
        return textResult("Kando is a desktop app for macOS, Linux and Windows; there is no build for " + name + ". Ask for a desktop system with the platform and arch inputs. Packages for macOS, Windows and Linux of version " + release.version + " are at " + release.page + ".", data);
      }
      data.downloads = release.downloads.filter(function (d) {
        return d.platform === system.platform && (!system.arch || d.arch === system.arch);
      });
      data.install = {};
      data.downloads.forEach(function (d) {
        data.install[d.kind] = release.install[d.kind];
      });
      var lines = data.downloads.map(describeDownload);
      lines.push("How to install:");
      Object.keys(data.install).forEach(function (kind) {
        lines.push("- ." + kind + ": " + data.install[kind]);
      });
      var head = "Kando " + release.version + " for " + name + (system.arch ? " on " + archName(system.platform, system.arch) : "") + ".";
      if (!system.arch) {
        head += system.platform === "macos" ? " The browser does not say which processor this Mac has. Use arm64 on a Mac with Apple silicon (M1 or later) and amd64 on an Intel Mac; About This Mac shows which." : system.platform === "windows" ? " The browser does not say which processor this PC has. Use amd64 on an x86-64 PC and arm64 on a Windows on Arm PC; Settings › System › About shows which under System type." : " The browser does not say which processor this computer has. Use amd64 on x86-64 and arm64 on ARM64; uname -m prints x86_64 or aarch64.";
      }
      if (system.platform === "linux") {
        head += " Use the .deb on Debian, Ubuntu and their derivatives, and the .tar.gz anywhere else.";
      }
      var foot = "SHA-256 checksums: " + release.checksums + ". Release page: " + release.page + "." + (system.platform === "windows" ? "" : " On first launch Kando downloads yt-dlp, ffmpeg and whisper-cli when they are missing.");
      return textResult(head + "\n" + lines.join("\n") + "\n" + foot, data);
    });
  }

  // ---- get_started and list_features ----

  // langOf is the code of the page language lang names, by its code or its BCP 47 tag, ignoring case; "" when the site has no page in it.
  function langOf(lang) {
    var wanted = String(lang).toLowerCase();
    for (var i = 0; i < page.langs.length; i++) {
      var l = page.langs[i];
      if (l.code.toLowerCase() === wanted || l.tag.toLowerCase() === wanted) {
        return l.code;
      }
    }
    return "";
  }

  // pageContent is the content file of the language input.lang names, the page's own by default, passed to answer; or an error result for a language the site has no page in.
  function pageContent(input, answer) {
    var lang = input && input.lang !== undefined ? input.lang : page.lang;
    var code = langOf(lang);
    if (!code) {
      return Promise.resolve(errorResult("lang must be one of " + langCodes.join(", ") + ", not " + JSON.stringify(lang) + "."));
    }
    var file = page.langs.filter(function (l) {
      return l.code === code;
    })[0].content;
    return fetchJSON(file).then(answer, function (err) {
      return errorResult("Could not load the page's text in " + code + " (" + err.message + "). Try again.");
    });
  }

  function link(l) {
    return l.text + ": " + l.url;
  }

  function startText(c) {
    var s = c.start;
    var out = [s.title + " (" + c.name + ", " + c.page + "#start)", s.lead];
    s.steps.forEach(function (step) {
      var lines = [(step.number ? step.number + ". " : "") + step.title];
      lines = lines.concat(step.text);
      (step.tools || []).forEach(function (tool) {
        lines.push("- " + tool.name + ": " + tool.does);
      });
      if (step.command) {
        lines.push(
          step.command
            .split("\n")
            .map(function (line) {
              return "    " + line;
            })
            .join("\n")
        );
      }
      lines = lines.concat(step.notes || []);
      if (step.link) {
        lines.push(link(step.link));
      }
      out.push(lines.join("\n"));
    });
    out.push(link(s.readme));
    return out.join("\n\n");
  }

  function getStarted(input) {
    return pageContent(input, function (c) {
      return textResult(startText(c), { lang: c.lang, page: c.page + "#start", start: c.start });
    });
  }

  function listFeatures(input) {
    return pageContent(input, function (c) {
      var f = c.features;
      var lines = f.items.map(function (item) {
        return "- " + item.title + ": " + item.text + (item.link ? " (" + link(item.link) + ")" : "");
      });
      return textResult(f.title + " (" + c.name + ", " + c.page + "#features)\n" + f.lead + "\n\n" + lines.join("\n"), { lang: c.lang, page: c.page + "#features", features: f });
    });
  }

  var langInput = {
    type: "string",
    enum: langCodes,
    description: "The language to answer in, by the code of one of the page's languages: " + page.langs.map(function (l) {
      return l.code + " (" + l.name + ")";
    }).join(", ") + ". Defaults to the language of the page the visitor is on (" + page.lang + ")."
  };

  var tools = [
    {
      name: "search_supported_sites",
      title: "Search the websites Kando downloads from",
      description: "Checks whether Kando, a desktop app that downloads a video, transcribes it and plays it with bilingual subtitles, can download videos from a website. Give a site's name (YouTube, 哔哩哔哩, niconico), its domain (vimeo.com) or a link to a video on it: a link is matched by its host, and other text finds the sites whose name, domain or other names contain every word of it, ignoring case. Answers with the matching sites from Kando's catalog of the websites yt-dlp supports.",
      inputSchema: {
        type: "object",
        properties: {
          query: { type: "string", description: "A website's name, its domain, or a link to a video on it." },
          limit: { type: "integer", minimum: 1, maximum: 100, description: "The most sites to list. Defaults to 10." }
        },
        required: ["query"]
      },
      annotations: { readOnlyHint: true },
      execute: searchSupportedSites
    },
    {
      name: "get_download",
      title: "Find the Kando download for a system",
      description: "Gives the download link of the latest Kando release for a computer, with how to install it. By default it is for the visitor's own system as the browser reports it; give platform, and arch, to ask for another. Kando publishes disk images for macOS (Apple silicon and Intel), installers for Windows (x86-64 and ARM64), and Debian packages and archives for Linux (x86-64 and ARM64).",
      inputSchema: {
        type: "object",
        properties: {
          platform: { type: "string", enum: ["macos", "linux", "windows"], description: "The operating system. Defaults to the visitor's." },
          arch: { type: "string", enum: ["arm64", "amd64"], description: "The processor: arm64 for Apple silicon Macs and ARM64 Linux, amd64 for Intel Macs and x86-64 Linux. Defaults to the visitor's when the browser reports it; otherwise both are listed." }
        }
      },
      annotations: { readOnlyHint: true },
      execute: getDownload
    },
    {
      name: "get_started",
      title: "Get started with Kando",
      description: "Returns the steps of this page's Get started section: downloading Kando, what it sets up the first time it opens, and choosing a whisper model and a translation service. Use it to explain to someone how to install and set up Kando, in their language.",
      inputSchema: { type: "object", properties: { lang: langInput } },
      annotations: { readOnlyHint: true },
      execute: getStarted
    },
    {
      name: "list_features",
      title: "List Kando's features",
      description: "Lists what Kando does, as this page's features section describes it: on-device transcription, translation with the AI model you choose, the bilingual player, libraries, supported websites, the command line and MCP server, interface languages, uploads and shortcuts.",
      inputSchema: { type: "object", properties: { lang: langInput } },
      annotations: { readOnlyHint: true },
      execute: listFeatures
    }
  ];

  // A browser that refuses a tool, such as one that already has a tool of its name, refuses only that tool; the page and the other tools stay as they are.
  tools.forEach(function (tool) {
    Promise.resolve()
      .then(function () {
        return modelContext.registerTool(tool);
      })
      .catch(function (err) {
        console.warn("Kando: WebMCP did not register " + tool.name + ":", err);
      });
  });
})();
