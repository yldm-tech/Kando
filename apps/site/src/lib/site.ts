// The site's constants and its languages, shared by the build (buildtime/) and the pages (src/). Nothing here touches the file system, so the pages can import it.

// siteURL is where the site is served, the address of canonical links, Open Graph tags and the sitemap. CNAME binds the GitHub Pages site to it.
export const siteURL = "https://kando.yldm.ai";

// themeColor is the page's paper, the browser's color around it and the web app manifest's theme and background.
export const themeColor = "#fbf8f3";

export const repoURL = "https://github.com/yldm-tech/Kando";
export const releasesURL = repoURL + "/releases";
// latestURL redirects to the newest published release, so the Download buttons reach the current files even when the site was built before a release came out. Drafts and prereleases are not "latest", so it never lands on a release that 404s for visitors.
export const latestURL = releasesURL + "/latest";
export const licenseURL = repoURL + "/blob/main/LICENSE";
export const issuesURL = repoURL + "/issues";
export const everyAPIURL = "https://everyapi.ai";
// readmeBuildURL is the README's Build from source section, where get_download sends a visitor whose system has no build.
export const readmeBuildURL = repoURL + "#build-from-source";

// buildCommands build Kando from source, as the download panel's Windows tab shows them and get_started returns them. go tool mygo build is what make build runs, and works where make is not installed, as on Windows.
export const buildCommands = "git clone https://github.com/yldm-tech/Kando.git\ncd Kando\ngo tool mygo build apps/kando";

// installCommand installs the newest release for the current user with the install.sh mygo build makes, as the Linux tab shows it.
export const installCommand = "curl -fsSL " + latestURL + "/download/install.sh | sh";

// Language is one page of the site: the interface languages Kando ships, in the order the switcher lists them.
export interface Language {
  // code names the content file, the screenshot folder under docs/assets and the page's folder.
  code: LangCode;
  // tag is the BCP 47 tag of the page, for <html lang> and hreflang: a script rather than a region for Chinese, so that the Simplified page also serves Singapore and the Traditional one Hong Kong and Macau.
  tag: string;
  // locale is the Open Graph locale.
  locale: string;
  // name is the language named in itself, as the switcher shows it.
  name: string;
  // readme is the README file written in this language.
  readme: string;
  // app is the app's locale of this language, the catalog in apps/kando/locales the hero's player takes its words from.
  app: string;
}

export const langCodes = ["en", "zh-CN", "zh-TW", "ja", "ko"] as const;
export type LangCode = (typeof langCodes)[number];

// otherLangCodes are the languages whose pages live in a folder of their own, the values of the $lang route segment.
export const otherLangCodes = ["zh-CN", "zh-TW", "ja", "ko"] as const;
export type OtherLangCode = (typeof otherLangCodes)[number];

export const languages: readonly Language[] = [
  { code: "en", tag: "en", locale: "en_US", name: "English", readme: "README.md", app: "en" },
  { code: "zh-CN", tag: "zh-Hans", locale: "zh_CN", name: "简体中文", readme: "README.zh-CN.md", app: "zh-Hans" },
  { code: "zh-TW", tag: "zh-Hant", locale: "zh_TW", name: "繁體中文", readme: "README.zh-TW.md", app: "zh-Hant" },
  { code: "ja", tag: "ja", locale: "ja_JP", name: "日本語", readme: "README.ja.md", app: "ja" },
  { code: "ko", tag: "ko", locale: "ko_KR", name: "한국어", readme: "README.ko.md", app: "ko" },
];

// sourceLang is the language the other content files are translated from, and the page at the root.
export const sourceLang: LangCode = "en";

export function language(code: LangCode): Language {
  const l = languages.find((x) => x.code === code);
  if (!l) {
    throw new Error("no language " + code);
  }
  return l;
}

// pagePath is the path of a language's page: the root for English, a folder named after the language for the others; page "sites" is the catalog page below it.
export function pagePath(code: LangCode, page: "home" | "sites" = "home"): string {
  const base = code === sourceLang ? "/" : "/" + code + "/";
  return page === "sites" ? base + "sites/" : base;
}

// pageURL is a page's absolute address.
export function pageURL(code: LangCode, page: "home" | "sites" = "home"): string {
  return siteURL + pagePath(code, page);
}

// readmeURL is the README in this language on GitHub.
export function readmeURL(l: Language): string {
  return repoURL + "/blob/main/" + l.readme;
}

// faqPairs are the questions of the FAQ section in its order, each a question's content key and its answer's; the home pages' structured data (buildtime/jsonld.ts) lists the same.
export const faqPairs = [
  ["faqPriceQ", "faqPriceA"],
  ["faqSitesQ", "faqSitesA"],
  ["faqSpeechQ", "faqSpeechA"],
  ["faqTranslateQ", "faqTranslateA"],
  ["faqSystemsQ", "faqSystemsA"],
  ["faqLoginQ", "faqLoginA"],
  ["faqExportQ", "faqExportA"],
] as const;

// demoLines are the English lines of the film the hero's player plays, How Rivers Shape Landscapes, the first lines of the film of the app's screenshots (protoCues in apps/kando/screenshot_prototype_test.go). The content files have their translations as demoCue1 to demoCue8.
export const demoLines = [
  "Every river begins as a trickle high in the mountains.",
  "As water flows downhill, it picks up sand and stones.",
  "Those particles act like sandpaper on the riverbed.",
  "Over thousands of years, the channel cuts deeper.",
  "That is how steep V-shaped valleys are formed.",
  "Lower down, the land flattens and the river slows.",
  "Instead of cutting down, it starts to swing sideways.",
  "These sweeping curves are called meanders.",
] as const;

// demoLineSeconds is how long each line of the demo film shows; two lines make a scene and a clip.
export const demoLineSeconds = 10;

// demoSeconds is the demo film's length.
export const demoSeconds = demoLines.length * demoLineSeconds;

// clock is seconds as the player writes a time: 1:05.
export function clock(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
}

// initial is the letter a site without a logo shows, as apps/kando/sites.Initial picks it: the first letter or digit of its name, upper case.
export function initial(name: string): string {
  for (const ch of name) {
    if (/[\p{L}\p{N}]/u.test(ch)) {
      return ch.toUpperCase();
    }
  }
  const first = name.codePointAt(0);
  return first === undefined ? "?" : String.fromCodePoint(first);
}

// plateCount is the number of plate colors under initials: the app's eight (apps/kando/sites.plates), as .plate-0 to .plate-7 in the stylesheet.
export const plateCount = 8;

// ratingSteps is the top of the app's scales for a whisper model's speed and accuracy (packages/media/whisper_model.go), which Settings › Speech Recognition draws as that many stars.
export const ratingSteps = 5;
