// The models section follows the app's whisper catalog: every model of packages/media/whisper_model.go, with its size in decimal megabytes and the recommended one marked.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { readModels } from "../buildtime/app";
import { attr, builtLangs, pageFile, readOut, repoRoot, section, tags } from "./helpers";

it("reads the seven whisper models with their sizes", () => {
  const src = readFileSync(join(repoRoot, "packages/media/whisper_model.go"), "utf8");
  const models = readModels(repoRoot);
  expect(models.length).toBe(src.split('{ID: "').length - 1);
  expect(models.find((m) => m.id === "medium")?.mb).toBe(1530);
  expect(models.find((m) => m.id === "tiny")?.mb).toBe(77.7);
  expect(models.filter((m) => m.recommended).map((m) => m.id)).toEqual(["large-v3-turbo-q5"]);
});

it("lists every model on each page", () => {
  const models = readModels(repoRoot);
  for (const l of builtLangs()) {
    const table = section(readOut(pageFile(l)), 'class="models-table', "</table>");
    const rows = tags(table, "tr").filter((t) => t.attrs.has("data-mb"));
    expect(
      rows.map((t) => Number(attr(t, "data-mb"))),
      l.code,
    ).toEqual(models.map((m) => m.mb));
    expect(rows.filter((t) => attr(t, "class") === "recommended").length).toBe(1);
  }
});
