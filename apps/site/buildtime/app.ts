// What the page says about the app, read from the app's own source so that the page follows it: the version, the commands and MCP tools, the whisper models, and the app's strings the hero's player shows. Each reader fails the build when its source no longer has the shape it reads.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { IntlMessageFormat } from "intl-messageformat";
import { parse } from "yaml";
import { clock, demoLineSeconds, demoLines, ratingSteps, type Language } from "../src/lib/site";
import { issues } from "./content";
import { appCatalogSchema, appKeys, appStringsSchema, manifestSchema, modelSchema, type AppKey, type Model } from "./schema";

// readYAML reads the YAML file at path: the app's nsgo.yaml or one of its catalogs.
function readYAML(path: string): unknown {
  try {
    return parse(readFileSync(path, "utf8"));
  } catch (err) {
    throw new Error(`${path}: ${(err as Error).message}`, { cause: err });
  }
}

// appVersion is the version in apps/kando/nsgo.yaml, which the Release workflow tags and publishes.
export function appVersion(root: string): string {
  const res = manifestSchema.safeParse(readYAML(join(root, "apps/kando/nsgo.yaml")));
  if (!res.success) {
    throw issues("apps/kando/nsgo.yaml", res.error);
  }
  return res.data.version;
}

const commandRe = /^\s*\{name: "[a-z]+"/gm;
const toolRe = /^\s*Name:\s+"(kando_[a-z_]+)"/gm;

export interface Surface {
  commands: number;
  tools: string[];
}

// readSurface reads the commands of apps/kando/cli/cli.go and the tools of apps/kando/mcpserver/tools.go.
export function readSurface(root: string): Surface {
  const cli = readFileSync(join(root, "apps/kando/cli/cli.go"), "utf8");
  const tools = readFileSync(join(root, "apps/kando/mcpserver/tools.go"), "utf8");
  const commands = cli.match(commandRe)?.length ?? 0;
  const names = [...tools.matchAll(toolRe)].map((m) => m[1] ?? "");
  if (commands === 0 || names.length === 0) {
    throw new Error(
      `found ${commands} commands in apps/kando/cli/cli.go and ${names.length} tools in apps/kando/mcpserver/tools.go; the patterns no longer match the code`,
    );
  }
  return { commands, tools: names };
}

const modelRe = /\{ID: "([a-z0-9-]+)", Speed: (\d), Accuracy: (\d), SizeLabel: "([\d.]+) (MB|GB)", File: "([^"]+)"(, Recommended: true)?\}/g;

// readModels reads the whisper models of packages/media/whisper_model.go, SpeechModels(), with their sizes in decimal megabytes, as the app's catalog lists them.
export function readModels(root: string): Model[] {
  const file = "packages/media/whisper_model.go";
  const src = readFileSync(join(root, file), "utf8");
  const entries = src.split('{ID: "').length - 1;
  const models = [...src.matchAll(modelRe)].map((m) => {
    const [, id, speed, accuracy, size, unit, name, recommended] = m;
    const n = Number(size);
    return modelSchema.parse({
      id,
      speed: Number(speed),
      accuracy: Number(accuracy),
      sizeLabel: `${size} ${unit}`,
      // Decimal units, as the app writes sizes: 1.53 GB is 1530 MB.
      mb: Math.round((unit === "GB" ? n * 1000 : n) * 10) / 10,
      file: name,
      recommended: recommended !== undefined,
    });
  });
  if (models.length === 0 || models.length !== entries) {
    throw new Error(`${file}: read ${models.length} of the ${entries} whisper models; the pattern no longer matches the code`);
  }
  if (models.filter((m) => m.recommended).length !== 1) {
    throw new Error(`${file}: want exactly one recommended model`);
  }
  return models;
}

// modelName is how apps/kando/settings/speech.go's SpeechModelNameIn names a model: a literal name, or an app string (by its key) where the name is translated.
export type ModelName = { literal: string } | { appKey: string };

// nameRe is a case of the switch: a literal name, or a string of the app's catalogs, called on the Messages m it is given or, for a function under a scope, as locales.X(cx).
const nameRe = /case "([a-z0-9-]+)":\s*return ("([^"]+)"|locales\.(\w+)\((?:cx)?\)|m\.(\w+)\(\))/g;

// readModelNames reads SpeechModelNameIn, which SpeechModelName calls with the language of its scope, and checks that it names every model.
export function readModelNames(root: string, models: Model[]): Map<string, ModelName> {
  const file = "apps/kando/settings/speech.go";
  const src = readFileSync(join(root, file), "utf8");
  const start = src.indexOf("func SpeechModelNameIn(");
  if (start < 0) {
    throw new Error(`${file}: no func SpeechModelNameIn`);
  }
  const body = src.slice(start, src.indexOf("\n}\n", start));
  const names = new Map<string, ModelName>();
  for (const m of body.matchAll(nameRe)) {
    const [, id, , literal, scoped, given] = m;
    if (!id) {
      continue;
    }
    const fn = scoped ?? given ?? "";
    names.set(id, literal !== undefined ? { literal } : { appKey: fn.charAt(0).toLowerCase() + fn.slice(1) });
  }
  for (const model of models) {
    if (!names.has(model.id)) {
      throw new Error(`${file}: SpeechModelNameIn does not name the whisper model ${model.id}`);
    }
  }
  return names;
}

export interface AppStrings {
  // app is the app's strings by key, the ones with placeholders formatted with the values the page shows.
  app: Record<AppKey, string>;
  // clipSpans are filmClipSpan for each clip of the demo film.
  clipSpans: string[];
  modelNames: Record<string, string>;
  // modelWords are the app's words for a model's speed and accuracy, as Settings › Speech Recognition labels them in settingsSpeechModelStats.
  modelWords: ModelWords;
  // modelStats are settingsSpeechModelStats for each model, by id, with its speed and accuracy as a rating out of ratingSteps and its size as the app writes it: the line the app shows under the model's name.
  modelStats: Record<string, string>;
}

export interface ModelWords {
  speed: string;
  accuracy: string;
}

// statsWords are the labels of settingsSpeechModelStats as formatted: for each of its values, the words between it and the value before it (or the start), which is how every catalog writes it ("Speed {speed}  Accuracy {accuracy}  {size}").
export function statsWords(file: string, format: (values: Record<string, string>) => string): ModelWords {
  const marks = { speed: "\u0001", accuracy: "\u0002", size: "\u0003" };
  const s = format(marks);
  const word = (mark: string): string => {
    const at = s.indexOf(mark);
    const before = s.slice(0, at);
    const from = Math.max(...Object.values(marks).map((m) => before.lastIndexOf(m) + 1));
    return before.slice(from).trim();
  };
  const words = { speed: word(marks.speed), accuracy: word(marks.accuracy) };
  if (words.speed === "" || words.accuracy === "") {
    throw new Error(`${file}: settingsSpeechModelStats names no speed or accuracy before its values: ${JSON.stringify(s)}`);
  }
  return words;
}

// loadAppStrings reads the app catalog of l (apps/kando/locales/<app>.yaml) and formats the strings the page shows. The app writes its strings in ICU MessageFormat, as filmSkipBack's English plural, so they are formatted as the app formats them rather than by replacing names.
export function loadAppStrings(root: string, l: Language, models: readonly Model[], modelNames: Map<string, ModelName>): AppStrings {
  const file = `apps/kando/locales/${l.app}.yaml`;
  const raw = appCatalogSchema.safeParse(readYAML(join(root, file)));
  if (!raw.success) {
    throw issues(file, raw.error);
  }
  const all: Record<string, string> = {};
  for (const [key, v] of Object.entries(raw.data)) {
    const message = appMessage(v);
    if (message !== undefined) {
      all[key] = message;
    }
  }
  const picked = appStringsSchema.safeParse(Object.fromEntries(appKeys.map((k) => [k, all[k]])));
  if (!picked.success) {
    throw issues(file, picked.error);
  }
  const format = (key: string, values: Record<string, string | number>): string => {
    const s = all[key];
    if (s === undefined) {
      throw new Error(`${file}: no key ${key}`);
    }
    const out = new IntlMessageFormat(s, l.app).format(values);
    if (typeof out !== "string") {
      throw new Error(`${file}: ${key} did not format to text`);
    }
    return out;
  };
  const clips = demoLines.length / 2;
  const clipSpans = Array.from({ length: clips }, (_, k) =>
    format("filmClipSpan", { number: k + 1, start: clock(2 * k * demoLineSeconds), end: clock(2 * (k + 1) * demoLineSeconds) }),
  );
  // values are what the page fills in; every other string is formatted with none, which fails on a placeholder the page does not fill.
  const values: Partial<Record<AppKey, Record<string, string | number>>> = {
    filmSkipBack: { seconds: 5 },
    filmSkipForward: { seconds: 5 },
    filmStatusTranslated: { percent: 100 },
    filmClipSpan: { number: 1, start: clock(0), end: clock(2 * demoLineSeconds) },
  };
  const app = Object.fromEntries(appKeys.map((k) => [k, format(k, values[k] ?? {})])) as Record<AppKey, string>;
  const names: Record<string, string> = {};
  for (const [id, name] of modelNames) {
    names[id] = "literal" in name ? name.literal : format(name.appKey, {});
  }
  const stats = (m: Model) =>
    format("settingsSpeechModelStats", { speed: `${m.speed}/${ratingSteps}`, accuracy: `${m.accuracy}/${ratingSteps}`, size: m.sizeLabel });
  const modelWords = statsWords(file, (values) => format("settingsSpeechModelStats", values));
  const modelStats = Object.fromEntries(models.map((m) => [m.id, stats(m)]));
  return { app, clipSpans, modelNames: names, modelWords, modelStats };
}

// appMessage is the text of a catalog entry: the translation itself in a translated catalog, or the message of the source catalog's entry, which also holds its description and placeholders. Anything else, as the words of NSGo's own packages that a catalog translates under their import paths, is no string of the app's.
function appMessage(entry: unknown): string | undefined {
  if (typeof entry === "string") {
    return entry;
  }
  if (typeof entry === "object" && entry !== null && "message" in entry && typeof entry.message === "string") {
    return entry.message;
  }
  return undefined;
}
