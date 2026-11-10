// TestContentFiles, TestNotes and TestContentValidation: the content files and the build's checks of them.
import { existsSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vite-plus/test";
import { loadContent, loadNotes, parseContent } from "../buildtime/content";
import { languages, sourceLang } from "../src/lib/site";
import { builtLangs, dist, pageFile, readJSON, siteDir } from "./helpers";

const contentDir = join(siteDir, "content");
const en = readJSON(join(contentDir, "en.json"));

describe("content files", () => {
  it("has a page for each content file, English first, and each file has exactly en.json's keys, none empty", () => {
    const content = loadContent(contentDir);
    const want = languages.filter((l) => existsSync(join(contentDir, l.code + ".json"))).map((l) => l.code);
    expect([...content.keys()]).toEqual(want);
    expect(want[0]).toBe(sourceLang);
    for (const [code, strs] of content) {
      expect(Object.keys(strs).sort(), code).toEqual(Object.keys(en).sort());
      for (const [key, s] of Object.entries(strs)) {
        expect(s.trim(), `${code}.json: ${key}`).not.toBe("");
      }
    }
    for (const l of builtLangs()) {
      expect(existsSync(join(dist(), pageFile(l))), pageFile(l)).toBe(true);
    }
  });

  it("notes.json has a note, not empty, for every key and none for a key that is gone", () => {
    const notes = loadNotes(contentDir, en);
    expect(Object.keys(notes).sort()).toEqual(Object.keys(en).sort());
  });
});

describe("content validation", () => {
  const cases: Record<string, (m: Record<string, string>) => void> = {
    "missing key": (m) => {
      delete m["heroTitle"];
    },
    "extra key": (m) => {
      m["heroTitleTypo"] = "x";
    },
    "empty string": (m) => {
      m["heroTitle"] = " ";
    },
    "missing placeholder": (m) => {
      m["sitesLead"] = "and more";
    },
    "extra placeholder": (m) => {
      m["heroLead"] = "{count} more";
    },
    "odd backtick": (m) => {
      m["libraryBody"] = "a `.kando folder";
    },
    "mark not in the title": (m) => {
      m["heroTitleMark"] = "nowhere";
    },
  };
  for (const [name, change] of Object.entries(cases)) {
    it(`fails on a ${name}`, () => {
      const ja = { ...en };
      change(ja);
      expect(() => parseContent("ja", en, ja)).toThrow();
    });
  }

  it("accepts en.json itself", () => {
    expect(() => parseContent("en", en, en)).not.toThrow();
  });
});
