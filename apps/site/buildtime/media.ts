// The images the dev script scripts/media.ts makes and commits under media/, which the build copies into the site: a link preview image for each language (og/<code>.png, the app icon, the headline and a short line beside a still of the hero's player), and the library screenshot of each language as AVIF and WebP in two widths (shots/<code>/library-<width>.<format>), which the page offers before the PNG. Making them needs a browser and ffmpeg, so they are not made by vp build; instead media/manifest.json records, for each image, a hash of the inputs it was made from, and the tests fail when an image's inputs no longer hash to what the manifest says (staleMedia), naming the command that makes it again.
import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { z } from "zod";
import { languages, type Language, type LangCode } from "../src/lib/site";
import { loadAppStrings, readModelNames, readModels } from "./app";
import { loadContent, readJSON, type Strings } from "./content";
import { langCodeSchema, type AppKey } from "./schema";

// mediaDir is the folder of the committed images, relative to the site's folder.
export const mediaDir = "media";

// mediaCommand makes the images again, run in apps/site.
export const mediaCommand = "vp run media";

// ogTemplate is the HTML page the link preview image is a screenshot of, relative to the site's folder; the script fills in its {{name}} fields.
export const ogTemplate = "scripts/og.html";

// ogSize is the link preview image's size, the 1.91:1 that Open Graph and X's large summary card show uncropped.
export const ogSize = { w: 1200, h: 630 } as const;

// iconFile is the app icon, relative to the repository root; the link preview image shows it.
export const iconFile = "apps/kando/resources/icon.png";

// ogKeys are the content strings the link preview image shows beside the still: the headline with its mark, and the short line under it.
export const ogKeys = ["heroTitle", "heroTitleMark", "ogTagline"] as const;

// stillKeys and stillAppKeys are the content strings and the app's strings the still of the hero's player shows: the film's library and its lines, the first under the picture and the first few at the edge of the side panel, and the player's labels that are drawn as text.
export const stillKeys = ["demoLibrary", "demoCue1", "demoCue2", "demoCue3", "demoCue4", "demoCue5", "demoCue6", "demoCue7", "demoCue8"] as const;
export const stillAppKeys: readonly AppKey[] = ["filmStatusTranslated", "filmModeBilingual", "filmSubtitles", "filmSearchLines"];

// shotWidths are the widths the library screenshot is offered in: half of the PNG's for small screens and one-times displays, and the PNG's own.
export const shotWidths = [652, 1304] as const;
export const shotFormats = ["avif", "webp"] as const;
export type ShotFormat = (typeof shotFormats)[number];

// shotEncoders are the encoders' arguments for an image width wide: ffmpeg's SVT-AV1 for AVIF, and libwebp's cwebp for WebP, as Homebrew's ffmpeg has no WebP encoder. They are part of the shots' inputs, so a change to them makes the images again.
export function shotEncoder(format: ShotFormat, src: string, width: number, out: string): { cmd: string; args: string[] } {
  if (format === "avif") {
    return {
      cmd: "ffmpeg",
      args: [
        "-hide_banner",
        "-loglevel",
        "error",
        "-y",
        "-i",
        src,
        "-vf",
        `scale=${width}:-1:flags=lanczos`,
        "-c:v",
        "libsvtav1",
        "-preset",
        "4",
        "-crf",
        "30",
        "-pix_fmt",
        "yuv420p",
        "-frames:v",
        "1",
        "-f",
        "avif",
        out,
      ],
    };
  }
  return { cmd: "cwebp", args: ["-quiet", "-q", "82", "-m", "6", "-resize", String(width), "0", src, "-o", out] };
}

// shotFile is the path of the library screenshot of code in a width and format, relative to mediaDir and to the built site's shots folder's language folder.
export function shotFile(code: LangCode, width: number, format: ShotFormat): string {
  return `shots/${code}/library-${width}.${format}`;
}

export function ogFile(code: LangCode): string {
  return `og/${code}.png`;
}

export function sha256(data: string | Uint8Array): string {
  return createHash("sha256").update(data).digest("hex");
}

// Inputs are what an image is made from, by name: the text it shows and the hashes of the files it is drawn from.
export type Inputs = Record<string, string>;

// inputsHash is the hash of inputs that the manifest records, independent of their order.
export function inputsHash(inputs: Inputs): string {
  return sha256(JSON.stringify(Object.entries(inputs).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))));
}

// ogInputs are the inputs of a language's link preview image: its strings, the template and the app icon.
export function ogInputs(root: string, siteDir: string, strings: Strings, app: Record<string, string>): Inputs {
  const inputs: Inputs = {
    template: sha256(readFileSync(join(siteDir, ogTemplate))),
    icon: sha256(readFileSync(join(root, iconFile))),
  };
  for (const key of [...ogKeys, ...stillKeys]) {
    const s = strings[key];
    if (s === undefined) {
      throw new Error(`no content key ${key}`);
    }
    inputs[key] = s;
  }
  for (const key of stillAppKeys) {
    const s = app[key];
    if (s === undefined) {
      throw new Error(`no app string ${key}`);
    }
    inputs["app." + key] = s;
  }
  return inputs;
}

// shotInputs are the inputs of a language's library screenshot in a format: the PNG and the encoder's arguments.
export function shotInputs(root: string, code: LangCode, format: ShotFormat): Inputs {
  const src = join(root, "docs/assets", code, "library.png");
  const inputs: Inputs = { png: sha256(readFileSync(src)) };
  for (const width of shotWidths) {
    const enc = shotEncoder(format, "in.png", width, "out");
    inputs["encoder" + width] = [enc.cmd, ...enc.args].join(" ");
  }
  return inputs;
}

const hashSchema = z.string().regex(/^[0-9a-f]{64}$/, "is not a sha256");

// mediaManifestSchema is media/manifest.json: the inputs hash each image was made from, the link preview images by language and the screenshots by language and format.
export const mediaManifestSchema = z.strictObject({
  og: z.partialRecord(langCodeSchema, hashSchema),
  shots: z.partialRecord(langCodeSchema, z.partialRecord(z.enum(shotFormats), hashSchema)),
});
export type MediaManifest = z.infer<typeof mediaManifestSchema>;

export function readMediaManifest(siteDir: string): MediaManifest {
  const path = join(siteDir, mediaDir, "manifest.json");
  if (!existsSync(path)) {
    return { og: {}, shots: {} };
  }
  return mediaManifestSchema.parse(readJSON(path));
}

// MediaLang is what a language's images are made from.
export interface MediaLang {
  lang: Language;
  strings: Strings;
  app: Record<string, string>;
  og: Inputs;
  shots: Record<ShotFormat, Inputs>;
}

// mediaLangs reads the inputs of every language with a content file, as the build reads them.
export function mediaLangs(root: string, siteDir: string, contentDir: string): MediaLang[] {
  const content = loadContent(contentDir);
  const models = readModels(root);
  const modelNames = readModelNames(root, models);
  return languages
    .filter((l) => content.has(l.code))
    .map((l) => {
      const strings = content.get(l.code) ?? {};
      const { app } = loadAppStrings(root, l, models, modelNames);
      return {
        lang: l,
        strings,
        app,
        og: ogInputs(root, siteDir, strings, app),
        shots: { avif: shotInputs(root, l.code, "avif"), webp: shotInputs(root, l.code, "webp") },
      };
    });
}

// staleMedia lists the images of langs that are missing or were made from other inputs than they have now.
export function staleMedia(siteDir: string, langs: readonly MediaLang[]): string[] {
  const manifest = readMediaManifest(siteDir);
  const stale: string[] = [];
  for (const m of langs) {
    const code = m.lang.code;
    if (!existsSync(join(siteDir, mediaDir, ogFile(code))) || manifest.og[code] !== inputsHash(m.og)) {
      stale.push(join(mediaDir, ogFile(code)));
    }
    for (const format of shotFormats) {
      const files = shotWidths.map((w) => join(mediaDir, shotFile(code, w, format)));
      if (files.some((f) => !existsSync(join(siteDir, f))) || manifest.shots[code]?.[format] !== inputsHash(m.shots[format])) {
        stale.push(...files);
      }
    }
  }
  return stale;
}
