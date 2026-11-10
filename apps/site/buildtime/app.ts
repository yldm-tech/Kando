// What the page says about the app, read from the app's own source so that the page follows it: the version, the commands and MCP tools, the whisper models, and the app's strings the hero's player shows. Each reader fails the build when its source no longer has the shape it reads.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { IntlMessageFormat } from "intl-messageformat";
import { clock, demoLineSeconds, demoLines, type Language } from "../src/lib/site";
import { issues, readJSON } from "./content";
import { appCatalogSchema, appKeys, appStringsSchema, manifestSchema, modelSchema, type AppKey, type Model } from "./schema";

// appVersion is the version in apps/kando/mygo.json, which the Release workflow tags and publishes.
export function appVersion(root: string): string {
  const res = manifestSchema.safeParse(readJSON(join(root, "apps/kando/mygo.json")));
  if (!res.success) {
    throw issues("apps/kando/mygo.json", res.error);
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

// modelName is how apps/kando/settings/speech.go's SpeechModelName names a model: a literal name, or an app string (by its key) where the name is translated.
export type ModelName = { literal: string } | { appKey: string };

const nameRe = /case "([a-z0-9-]+)":\s*return ("([^"]+)"|locales\.(\w+)\(\))/g;

// readModelNames reads SpeechModelName and checks that it names every model.
export function readModelNames(root: string, models: Model[]): Map<string, ModelName> {
  const file = "apps/kando/settings/speech.go";
  const src = readFileSync(join(root, file), "utf8");
  const start = src.indexOf("func SpeechModelName(");
  if (start < 0) {
    throw new Error(`${file}: no func SpeechModelName`);
  }
  const body = src.slice(start, src.indexOf("\n}\n", start));
  const names = new Map<string, ModelName>();
  for (const m of body.matchAll(nameRe)) {
    const [, id, , literal, fn] = m;
    if (!id) {
      continue;
    }
    names.set(id, literal !== undefined ? { literal } : { appKey: (fn ?? "").charAt(0).toLowerCase() + (fn ?? "").slice(1) });
  }
  for (const model of models) {
    if (!names.has(model.id)) {
      throw new Error(`${file}: SpeechModelName does not name the whisper model ${model.id}`);
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
}

// loadAppStrings reads the app catalog of l (apps/kando/locales/<app>.json) and formats the strings the page shows. The app writes its strings in ICU MessageFormat, as filmSkipBack's English plural, so they are formatted as the app formats them rather than by replacing names.
export function loadAppStrings(root: string, l: Language, modelNames: Map<string, ModelName>): AppStrings {
  const file = `apps/kando/locales/${l.app}.json`;
  const raw = appCatalogSchema.safeParse(readJSON(join(root, file)));
  if (!raw.success) {
    throw issues(file, raw.error);
  }
  const all: Record<string, string> = {};
  for (const [key, v] of Object.entries(raw.data)) {
    if (!key.startsWith("@") && typeof v === "string") {
      all[key] = v;
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
  return { app, clipSpans, modelNames: names };
}
