// The shapes of what the build reads from the repository and of the data it hands the pages. Each file read is parsed with one of these zod schemas, so that a source that changes shape fails the build instead of rendering a page with a hole.
import { z } from "zod";
import { langCodes } from "../src/lib/site";

// ---- what the build reads ----

// placeholderRe finds a value a content string names, such as {count}, filled in when the page is rendered.
export const placeholderRe = /\{[a-zA-Z]+\}/g;

export function placeholders(s: string): string[] {
  return [...(s.match(placeholderRe) ?? [])].sort();
}

const nonEmpty = z.string().refine((s) => s.trim() !== "", "is empty");

// contentFileSchema is a content file before it is compared with en.json: a flat object of strings, none empty.
export const contentFileSchema = z.record(z.string(), nonEmpty);

// contentSchema is a content file of the language code checked against en.json, the source: exactly its keys, the same placeholders in each string, ` in pairs, and heroTitleMark inside heroTitle.
export function contentSchema(source: Record<string, string>) {
  const shape: Record<string, z.ZodType<string>> = {};
  for (const key of Object.keys(source)) {
    shape[key] = nonEmpty;
  }
  return z.strictObject(shape).superRefine((strs, ctx) => {
    for (const [key, s] of Object.entries(strs)) {
      const want = placeholders(source[key] ?? "");
      const got = placeholders(s);
      if (want.join() !== got.join()) {
        ctx.addIssue({ code: "custom", path: [key], message: `has placeholders [${got.join(" ")}], en.json has [${want.join(" ")}]` });
      }
      if ((s.split("`").length - 1) % 2 !== 0) {
        ctx.addIssue({ code: "custom", path: [key], message: "has a ` that opens code it never closes" });
      }
    }
    const title = strs["heroTitle"];
    const mark = strs["heroTitleMark"];
    if (title !== undefined && mark !== undefined && !title.includes(mark)) {
      ctx.addIssue({ code: "custom", path: ["heroTitleMark"], message: `${JSON.stringify(mark)} is not in heroTitle ${JSON.stringify(title)}` });
    }
  });
}

// siteNamesSchema is content/sites.<code>.json: the names of the catalog's sites as that language writes them, by domain.
export const siteNamesSchema = z.record(z.string().min(1), nonEmpty);

// manifestSchema is the part of apps/kando/mygo.json the site reads: the version the Release workflow tags and publishes.
export const manifestSchema = z.object({ version: z.string().regex(/^\d+\.\d+\.\d+/, "is not a version") });

// catalogSiteSchema is a website of apps/kando/sites/catalog/sites.json.
export const catalogSiteSchema = z.object({
  name: z.string().min(1),
  domain: z.string().min(1),
  aliases: z.array(z.string()).optional(),
  logo: z
    .string()
    .regex(/^[^/\\]+\.(png|svg|jpg|jpeg|webp|ico|gif)$/i, "is not a file name")
    .optional(),
  featured: z.boolean().optional(),
});

export const catalogSchema = z.object({
  ytdlp: z.string().min(1),
  sites: z.array(catalogSiteSchema).min(1),
});
export type CatalogSite = z.infer<typeof catalogSiteSchema>;

// appCatalogSchema is an app catalog of apps/kando/locales: strings by key, with notes under keys that start with @.
export const appCatalogSchema = z.record(z.string(), z.unknown());

// appKeys are the app's own strings the hero's player and the models section show, so that they say what the app says.
export const appKeys = [
  "filmModeBilingual",
  "filmModeOriginal",
  "filmModeTranslation",
  "filmModeOff",
  "filmPlay",
  "filmPause",
  "filmMute",
  "filmUnmute",
  "filmStatusTranslated",
  "filmSidebar",
  "filmLoopOn",
  "filmSkipBack",
  "filmSkipForward",
  "filmPlaybackPosition",
  "filmVolume",
  "filmLoopLine",
  "filmModeHeading",
  "filmFullScreen",
  "filmSubtitles",
  "filmTabClips",
  "filmTabInfo",
  "filmTabStyle",
  "filmSearchLines",
  "filmClickToJump",
  "filmNoMatch",
  "filmClipsNote",
  "filmClipSpan",
  "filmSource",
  "filmLineOriginal",
  "filmLineTranslation",
  "filmFiles",
  "filmStyleSize",
  "filmStyleOrder",
  "filmOrderTranslationFirst",
  "filmOrderOriginalFirst",
  "settingsSpeechNameLargeV3TurboQ5",
  "settingsSpeechRecommended",
] as const;
export type AppKey = (typeof appKeys)[number];

export const appStringsSchema = z.object(Object.fromEntries(appKeys.map((k) => [k, nonEmpty])) as Record<AppKey, typeof nonEmpty>);

// ---- what the build hands the pages ----

export const langCodeSchema = z.enum(langCodes);

export const shotSchema = z.object({ src: z.string(), w: z.number().int().positive(), h: z.number().int().positive() });
export type Shot = z.infer<typeof shotSchema>;

// pictureSchema is a screenshot offered in smaller formats before its PNG: a <source> of each, by MIME type, with the widths it comes in.
export const pictureSchema = shotSchema.extend({ sources: z.array(z.object({ type: z.string(), srcset: z.string() })).min(1) });

// tileSchema is a featured website of the catalog in the works-with strip: its name after its mark, which is AI Logo's where it has one (siteMarks), the logo of the app's catalog otherwise, or its initial on a colored plate when the catalog has none either, as the app's Supported Websites sheet draws it.
export const tileSchema = z.object({
  name: z.string(),
  domain: z.string(),
  // lang is the language of name when it is not the page's: a name written in Chinese characters on a page in another language is marked zh-Hans, the script of the catalog, so that a screen reader reads it as Chinese.
  lang: z.string().optional(),
  logo: z.string().optional(),
  initial: z.string().optional(),
  // plate is the index of the plate color under the initial, a class plate-<n> in the stylesheet.
  plate: z.number().int().min(0).max(7).optional(),
});
export type Tile = z.infer<typeof tileSchema>;

// brandSchema is a mark the page shows with a name.
export const brandSchema = z.object({ name: z.string(), id: z.string(), src: z.string() });
export type Brand = z.infer<typeof brandSchema>;

// modelSchema is a whisper model of the app's catalog (packages/media/whisper_model.go), as the models section charts it.
export const modelSchema = z.object({
  id: z.string(),
  speed: z.number().int(),
  accuracy: z.number().int(),
  // sizeLabel is the size as the app writes it, such as "1.53 GB"; mb is the same size in decimal megabytes.
  sizeLabel: z.string(),
  mb: z.number().positive(),
  file: z.string(),
  recommended: z.boolean(),
});
export type Model = z.infer<typeof modelSchema>;

// The WebMCP data, read by static/webmcp.js; buildtime/webmcp.ts writes it.
export const downloadSchema = z.object({ platform: z.string(), arch: z.string(), kind: z.string(), name: z.string(), url: z.string() });
export const agentReleaseSchema = z.object({
  version: z.string(),
  page: z.string(),
  checksums: z.string(),
  source: z.string(),
  downloads: z.array(downloadSchema),
  install: z.record(z.string(), z.string()),
});
export const agentLangSchema = z.object({ code: z.string(), tag: z.string(), name: z.string(), page: z.string(), content: z.string() });
export const agentDataSchema = z.object({ lang: z.string(), langs: z.array(agentLangSchema), sites: z.string(), release: agentReleaseSchema });
export type AgentData = z.infer<typeof agentDataSchema>;
export type AgentRelease = z.infer<typeof agentReleaseSchema>;

// missingSchema is 404.html's text in one language.
export const missingSchema = z.object({ code: langCodeSchema, tag: z.string(), title: z.string(), body: z.string(), home: z.string() });

// siteDataSchema is virtual:kando/site, what every page reads.
export const siteDataSchema = z.object({
  version: z.string(),
  // commands is the number of the kando command's commands, and tools the names of its MCP server's tools, in apps/kando/mcpserver's order.
  surface: z.object({ commands: z.number().int().positive(), tools: z.array(z.string()).min(1) }),
  // total is the number of websites in the app's catalog, and ytdlp the yt-dlp version it was made from.
  total: z.number().int().positive(),
  ytdlp: z.string(),
  tiles: z.partialRecord(langCodeSchema, z.array(tileSchema)),
  // siteNames are the names of the catalog's sites as a language writes them, where they differ from the catalog's (content/sites.<code>.json).
  siteNames: z.partialRecord(langCodeSchema, z.record(z.string(), z.string())),
  aiServices: z.array(brandSchema),
  claudeCode: brandSchema,
  shots: z.partialRecord(langCodeSchema, z.object({ library: pictureSchema })),
  // og is each language's link preview image (media/og/<code>.png).
  og: z.partialRecord(langCodeSchema, shotSchema),
  // assets are the static scripts and the WebMCP sites file, linked with a hash of their content in the query so that a changed file is fetched again.
  assets: z.object({ siteJS: z.string(), webmcpJS: z.string(), sitesJSON: z.string() }),
  webmcp: z.partialRecord(langCodeSchema, agentDataSchema),
  models: z.array(modelSchema).min(1),
  missing: z.array(missingSchema),
});
export type SiteData = z.infer<typeof siteDataSchema>;

// contentModuleSchema is virtual:kando/content/<code>: a language's content strings, and the app's strings in that language, already formatted with the values the page shows.
export const contentModuleSchema = z.object({
  code: langCodeSchema,
  strings: z.record(z.string(), z.string()),
  app: z.record(z.string(), z.string()),
  // clipSpans are filmClipSpan for each clip of the demo film; modelNames the whisper models' names, by id.
  clipSpans: z.array(z.string()),
  modelNames: z.record(z.string(), z.string()),
});
export type ContentModule = z.infer<typeof contentModuleSchema>;

// catalogRowSchema is a website of the catalog as the sites page lists it. Its aliases, yt-dlp's names for the site, are only searched, so the page fetches them from webmcp/sites.json when the reader searches.
export const catalogRowSchema = z.object({
  name: z.string(),
  domain: z.string(),
  logo: z.string().optional(),
  initial: z.string().optional(),
  plate: z.number().int().optional(),
  featured: z.boolean(),
});
export type CatalogRow = z.infer<typeof catalogRowSchema>;
