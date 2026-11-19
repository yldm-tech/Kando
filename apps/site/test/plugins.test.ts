// lib/plugins.ts, the plugin page's reading of the plugin index: it reads the envelope and its base64 body, refuses an index of another format or version, leaves out an entry that is not valid, links only https homepages, picks a text in the page's language with its fallbacks, and searches and filters as the page does.
import { describe, expect, it } from "vite-plus/test";
import {
  decodeIndex,
  filterPlugins,
  formatSize,
  httpsURL,
  IndexError,
  installLink,
  localText,
  parseEntry,
  pluginMatches,
  tagParent,
  type PluginEntry,
} from "../src/lib/plugins";

function entry(over: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: "ada.oxford-words",
    kind: "wordlist",
    name: { en: "Oxford words", "zh-Hant": "牛津詞表", ja: "オックスフォード単語" },
    summary: { en: "The 3000 most useful English words" },
    version: "1.2.0",
    publisher: "ada",
    license: "CC-BY-4.0",
    homepage: "https://example.org/words",
    size: 123456,
    sha256: "a".repeat(64),
    url: "https://example.org/words.zip",
    languages: ["en"],
    ...over,
  };
}

function envelope(body: unknown, over: Record<string, unknown> = {}): Record<string, unknown> {
  const bytes = new TextEncoder().encode(JSON.stringify(body));
  let bin = "";
  for (const b of bytes) {
    bin += String.fromCharCode(b);
  }
  return { format: "kando-plugins-index", version: 1, signer: "key", signature: "c2ln", body: btoa(bin), ...over };
}

function index(entries: unknown[], over: Record<string, unknown> = {}): Record<string, unknown> {
  return { format: "kando-plugins-index", version: 1, generated: "2026-11-19T00:00:00Z", entries, ...over };
}

function must(e: PluginEntry | undefined): PluginEntry {
  if (!e) {
    throw new Error("the entry was refused");
  }
  return e;
}

describe("decodeIndex", () => {
  it("reads the envelope's base64 body, UTF-8 text included", () => {
    const got = decodeIndex(
      envelope(index([entry(), entry({ id: "bo.style", publisher: "bo", kind: "translate-style", name: { en: "Plain", ko: "평이체" } })])),
    );
    expect(got.entries.map((e) => e.id)).toEqual(["ada.oxford-words", "bo.style"]);
    expect(got.dropped).toBe(0);
    expect(localText(must(got.entries[0]).name, "zh-Hant")?.text).toBe("牛津詞表");
    expect(localText(must(got.entries[1]).name, "ko")?.text).toBe("평이체");
  });

  it("refuses an envelope or body of another format or version, or not an index", () => {
    const bad: unknown[] = [
      null,
      [],
      "x",
      envelope(index([]), { format: "other" }),
      envelope(index([]), { version: 2 }),
      envelope(index([]), { body: 42 }),
      envelope(index([]), { signature: undefined }),
      envelope(index([]), { body: "not base64!" }),
      envelope(index([]), { body: btoa("{not json") }),
      envelope(index([]), { body: btoa(String.fromCharCode(0xff, 0xfe)) }),
      envelope(index([], { format: "other" })),
      envelope(index([], { version: 2 })),
      envelope(index([], { generated: "yesterday" })),
      envelope(index([], { entries: {} })),
      envelope(index(Array.from({ length: 5001 }, () => ({})))),
    ];
    for (const raw of bad) {
      expect(() => decodeIndex(raw), JSON.stringify(raw)).toThrow(IndexError);
    }
  });

  it("leaves out entries that are not valid or list an id again, and counts them", () => {
    const got = decodeIndex(envelope(index([entry(), entry(), "x", entry({ kind: "theme" })])));
    expect(got.entries.length).toBe(1);
    expect(got.dropped).toBe(3);
  });

  it("reads an index with no entries", () => {
    expect(decodeIndex(envelope(index([]))).entries).toEqual([]);
  });
});

describe("parseEntry", () => {
  it("accepts a valid entry, and a script plugin with its source", () => {
    expect(parseEntry(entry())?.homepage).toBe("https://example.org/words");
    const script = must(
      parseEntry(entry({ id: "kando.lookup", publisher: "kando", kind: "script", source: "official", minKando: "0.34.0", homepage: undefined })),
    );
    expect(script.source).toBe("official");
    expect(script.minKando).toBe("0.34.0");
    expect(script.homepage).toBeUndefined();
  });

  const bad: Record<string, Record<string, unknown>> = {
    "an id with no publisher": { id: "words" },
    "an id with capitals": { id: "Ada.Words", publisher: "Ada" },
    "an id over 64 characters": { id: "ada." + "w".repeat(64) },
    "a publisher other than the id's": { publisher: "bob" },
    "an unknown kind": { kind: "theme" },
    "no English name": { name: { ja: "単語" } },
    "a name that is not text": { name: { en: 3 } },
    "an empty name": { name: { en: "  " } },
    "a name too long": { name: { en: "w".repeat(121) } },
    "a name with a control character": { name: { en: "a\u0007b" } },
    "a name with a bidirectional override": { name: { en: "abc\u202etxt.exe" } },
    "a name in a tag that is not one": { name: { en: "Words", "<b>": "x" } },
    "a summary that is not an object": { summary: "Words" },
    "a homepage over http": { homepage: "http://example.org" },
    "a javascript homepage": { homepage: "javascript:alert(1)" },
    "a homepage with a password": { homepage: "https://u:p@example.org" },
    "a homepage that is no address": { homepage: "example.org" },
    "a download over http": { url: "http://example.org/w.zip" },
    "no size": { size: undefined },
    "a size of 0": { size: 0 },
    "a fractional size": { size: 1.5 },
    "a size as text": { size: "100" },
    "a bad SHA-256": { sha256: "A".repeat(64) },
    "no version": { version: undefined },
    "a version with spaces": { version: "1 2" },
    "no license": { license: "" },
    "languages that are not a list": { languages: "en" },
    "a language twice": { languages: ["en", "en"] },
    "a language that is no tag": { languages: ["english language"] },
    "too many languages": { languages: Array.from({ length: 17 }, (_, i) => "x" + String.fromCharCode(97 + i)) },
    "minKando on a data pack": { minKando: "0.34.0" },
    "a source on a data pack": { source: "official" },
    "a script plugin with no source": { id: "ada.s", kind: "script" },
    "a script plugin from a developer": { id: "ada.s", kind: "script", source: "developer" },
  };
  for (const [name, over] of Object.entries(bad)) {
    it(`refuses ${name}`, () => {
      expect(parseEntry(entry(over))).toBeUndefined();
    });
  }

  it("refuses what is not an object, and reads only the entry's own keys", () => {
    expect(parseEntry(null)).toBeUndefined();
    expect(parseEntry([entry()])).toBeUndefined();
    const inherited = Object.create(entry()) as Record<string, unknown>;
    expect(parseEntry(inherited)).toBeUndefined();
  });

  it("makes one space of runs of white space in a text", () => {
    expect(localText(must(parseEntry(entry({ summary: { en: " two\n\tlines " } }))).summary, "en")?.text).toBe("two lines");
  });
});

it("links only https addresses", () => {
  expect(httpsURL("https://example.org/a b")).toBe("https://example.org/a%20b");
  for (const s of [
    "http://example.org",
    "javascript:alert(1)",
    "data:text/html,x",
    "//example.org",
    "https://",
    "",
    42,
    undefined,
    "https://" + "a".repeat(2050),
  ]) {
    expect(httpsURL(s), String(s)).toBeUndefined();
  }
});

describe("localText", () => {
  const name = must(parseEntry(entry({ name: { en: "English", "zh-Hant": "繁體", "zh-Hans": "简体", pt: "Português" } }))).name;
  it("falls back through the tag's parents to English", () => {
    expect(localText(name, "zh-Hant")?.text).toBe("繁體");
    expect(localText(name, "zh-TW")?.text).toBe("繁體");
    expect(localText(name, "zh-HK")?.text).toBe("繁體");
    expect(localText(name, "zh-CN")?.text).toBe("简体");
    expect(localText(name, "pt-BR")?.text).toBe("Português");
    expect(localText(name, "ja")).toEqual({ tag: "en", text: "English" });
    expect(localText(name, "ZH-hant")?.text).toBe("繁體");
  });
  it("is undefined for a summary in no language", () => {
    expect(localText(must(parseEntry(entry({ summary: undefined }))).summary, "en")).toBeUndefined();
  });
  it("names the parents as Kando does", () => {
    expect(tagParent("zh-TW")).toBe("zh-Hant");
    expect(tagParent("zh-Hant")).toBe("zh");
    expect(tagParent("en")).toBe("");
  });
});

describe("search and kind", () => {
  const entries = decodeIndex(
    envelope(
      index([
        entry(),
        entry({
          id: "bo.plain",
          publisher: "bo",
          kind: "translate-style",
          name: { en: "Plain style", ja: "やさしい日本語" },
          summary: { en: "Short sentences" },
        }),
        entry({ id: "kando.lookup", publisher: "kando", kind: "script", source: "verified", name: { en: "Lookup" } }),
      ]),
    ),
  ).entries;
  const ids = (query: string, kind: Parameters<typeof filterPlugins>[2] = "") => filterPlugins(entries, query, kind).map((e) => e.id);

  it("matches the id, the publisher, and the name and summary in any language, ignoring case", () => {
    expect(ids("")).toEqual(["ada.oxford-words", "bo.plain", "kando.lookup"]);
    expect(ids("OXFORD")).toEqual(["ada.oxford-words"]);
    expect(ids("bo")).toEqual(["bo.plain"]);
    expect(ids("やさしい")).toEqual(["bo.plain"]);
    expect(ids("牛津")).toEqual(["ada.oxford-words"]);
    expect(ids("short plain")).toEqual(["bo.plain"]);
    expect(ids("kando.look")).toEqual(["kando.lookup"]);
    expect(ids("nothing")).toEqual([]);
  });

  it("filters by kind", () => {
    expect(ids("", "script")).toEqual(["kando.lookup"]);
    expect(ids("plain", "script")).toEqual([]);
    expect(ids("", "dictionary")).toEqual([]);
  });

  it("is the same match for one entry", () => {
    expect(pluginMatches(must(entries[0]), "  ada  words ")).toBe(true);
  });
});

it("opens an entry in Kando with its id escaped", () => {
  expect(installLink("ada.oxford-words")).toBe("kando://plugin/install?id=ada.oxford-words");
  expect(installLink("a&b=c")).toBe("kando://plugin/install?id=a%26b%3Dc");
});

it("writes a size in bytes, kilobytes or megabytes of 1000", () => {
  expect(formatSize(512, "en")).toMatch(/^512 /);
  expect(formatSize(123_456, "en")).toBe("123.5 kB");
  expect(formatSize(4_200_000, "en")).toBe("4.2 MB");
});
