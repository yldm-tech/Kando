// The content files: content/<code>.json per language, with exactly en.json's keys, and notes.json, a translator's note for every key.
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { z } from "zod";
import { languages, sourceLang, type LangCode } from "../src/lib/site";
import { contentFileSchema, contentSchema } from "./schema";

export type Strings = Record<string, string>;

// issues formats a zod error as the lines a build failure prints.
export function issues(file: string, err: z.ZodError): Error {
  return new Error(err.issues.map((i) => `${file}: ${i.path.join(".") || "(file)"} ${i.message}`).join("\n"));
}

export function readJSON(path: string): unknown {
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch (err) {
    throw new Error(`${path}: ${(err as Error).message}`, { cause: err });
  }
}

// parseContent checks the content file of code against the source language's, and returns it.
export function parseContent(code: string, source: Strings, raw: unknown): Strings {
  const file = `content/${code}.json`;
  const flat = contentFileSchema.safeParse(raw);
  if (!flat.success) {
    throw issues(file, flat.error);
  }
  const res = contentSchema(source).safeParse(flat.data);
  if (!res.success) {
    throw issues(file, res.error);
  }
  return res.data;
}

// loadContent reads every content file there is, keyed by language code. en.json must exist; every other file is checked against it (parseContent). The languages are in the order of languages, English first.
export function loadContent(dir: string): Map<LangCode, Strings> {
  const srcRaw = readJSON(join(dir, sourceLang + ".json"));
  const flat = contentFileSchema.safeParse(srcRaw);
  if (!flat.success) {
    throw issues(`content/${sourceLang}.json`, flat.error);
  }
  const source = parseContent(sourceLang, flat.data, flat.data);
  const content = new Map<LangCode, Strings>();
  for (const l of languages) {
    const path = join(dir, l.code + ".json");
    if (l.code !== sourceLang && !existsSync(path)) {
      continue;
    }
    content.set(l.code, l.code === sourceLang ? source : parseContent(l.code, source, readJSON(path)));
  }
  return content;
}

// loadNotes reads notes.json and checks that it has a note, not empty, for every key of the source language and none for a key that is gone.
export function loadNotes(dir: string, source: Strings): Strings {
  const raw = readJSON(join(dir, "notes.json"));
  const flat = contentFileSchema.safeParse(raw);
  if (!flat.success) {
    throw issues("content/notes.json", flat.error);
  }
  const shape: Record<string, z.ZodString> = {};
  for (const key of Object.keys(source)) {
    shape[key] = z.string();
  }
  const res = z.strictObject(shape).safeParse(flat.data);
  if (!res.success) {
    throw issues("content/notes.json", res.error);
  }
  return flat.data;
}

// fill puts each value of values in place of {name} in s, and fails when one is left unfilled.
export function fill(key: string, s: string, values: Record<string, string | number>): string {
  let out = s;
  for (const [name, value] of Object.entries(values)) {
    out = out.replaceAll("{" + name + "}", String(value));
  }
  const left = out.match(/\{[a-zA-Z]+\}/);
  if (left) {
    throw new Error(`${JSON.stringify(key)}: ${left[0]} is not filled in`);
  }
  return out;
}
