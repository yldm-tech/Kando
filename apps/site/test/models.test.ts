// The models section follows the app's whisper catalog: every model of packages/media/whisper_model.go, placed in the chart by its speed and accuracy with its size as the bubble's area, and listed in the table with the same ratings, its size in decimal megabytes and the recommended one marked, in the app's own words for speed and accuracy.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { loadAppStrings, readModelNames, readModels, statsWords } from "../buildtime/app";
import { loadContent } from "../buildtime/content";
import { frames, layout, type Box } from "../src/components/models/ModelsChart";
import { ratingSteps } from "../src/lib/site";
import { attr, builtLangs, pageFile, readOut, repoRoot, section, siteDir, tags, text } from "./helpers";

it("reads the seven whisper models with their ratings and sizes", () => {
  const src = readFileSync(join(repoRoot, "packages/media/whisper_model.go"), "utf8");
  const models = readModels(repoRoot);
  expect(models.length).toBe(src.split('{ID: "').length - 1);
  expect(models.find((m) => m.id === "medium")?.mb).toBe(1530);
  expect(models.find((m) => m.id === "tiny")).toMatchObject({ speed: 5, accuracy: 2, mb: 77.7 });
  expect(models.find((m) => m.id === "large-v3-turbo-q5")).toMatchObject({ speed: 4, accuracy: 5, mb: 574 });
  expect(models.filter((m) => m.recommended).map((m) => m.id)).toEqual(["large-v3-turbo-q5"]);
  // Two models with the same ratings would sit on one another in the chart.
  expect(new Set(models.map((m) => `${m.speed}/${m.accuracy}`)).size).toBe(models.length);
});

it("takes the words for speed and accuracy from the app's line of stats", () => {
  const models = readModels(repoRoot);
  const names = readModelNames(repoRoot, models);
  const want: Record<string, [string, string]> = {
    en: ["Speed", "Accuracy"],
    "zh-CN": ["速度", "准确"],
    "zh-TW": ["速度", "準確"],
    ja: ["速度", "精度"],
    ko: ["속도", "정확도"],
  };
  for (const l of builtLangs()) {
    const { modelWords, modelStats } = loadAppStrings(repoRoot, l, models, names);
    expect([modelWords.speed, modelWords.accuracy], l.code).toEqual(want[l.code]);
    expect(modelStats["large-v3-turbo-q5"], l.code).toContain(`4/${ratingSteps}`);
    expect(modelStats["large-v3-turbo-q5"], l.code).toContain("574 MB");
  }
  expect(() => statsWords("x.json", (v) => `${v["speed"]} ${v["accuracy"]} ${v["size"]}`)).toThrow(/names no speed or accuracy/);
});

// The labels are placed by estimated widths; in every language and both drawings each must clear the bubbles, the other labels and the axes' titles, and stay inside the chart.
it("places every label clear of the bubbles and the other labels, in every language", () => {
  const models = readModels(repoRoot);
  const names = readModelNames(repoRoot, models);
  const overlaps = (a: Box, b: Box) => Math.min(a.x1, b.x1) > Math.max(a.x0, b.x0) && Math.min(a.y1, b.y1) > Math.max(a.y0, b.y0);
  for (const l of builtLangs()) {
    const app = loadAppStrings(repoRoot, l, models, names);
    const rows = models.map((m) => ({ ...m, name: app.modelNames[m.id] ?? m.id }));
    for (const [kind, f] of Object.entries(frames)) {
      const { placed, bounds, titles } = layout(rows, f, app.modelWords);
      const where = `${l.code} ${kind}`;
      for (const m of placed) {
        const b = m.label.box;
        expect(b.x0 >= bounds.x0 && b.x1 <= bounds.x1 && b.y0 >= bounds.y0 && b.y1 <= bounds.y1, `${where}: ${m.id}'s label leaves the chart`).toBe(true);
        for (const t of titles) {
          expect(overlaps(b, t), `${where}: ${m.id}'s label covers an axis title`).toBe(false);
        }
        for (const o of placed) {
          expect(overlaps(b, { x0: o.cx - o.r, y0: o.cy - o.r, x1: o.cx + o.r, y1: o.cy + o.r }), `${where}: ${m.id}'s label covers ${o.id}'s bubble`).toBe(
            false,
          );
          if (o !== m) {
            expect(overlaps(b, o.label.box), `${where}: ${m.id}'s label covers ${o.id}'s`).toBe(false);
          }
        }
      }
    }
  }
});

it("charts every model by speed and accuracy, sized by its download, on each page", () => {
  const models = readModels(repoRoot);
  const names = readModelNames(repoRoot, models);
  const content = loadContent(join(siteDir, "content"));
  for (const l of builtLangs()) {
    const app = loadAppStrings(repoRoot, l, models, names);
    const strs = content.get(l.code) ?? {};
    const figure = section(readOut(pageFile(l)), 'class="models-chart', "</figure>");
    expect(text(figure.slice(figure.indexOf("<figcaption"))).trim(), l.code).toBe(strs["modelsChartCaption"]);
    const heads = tags(figure, "svg");
    expect(
      heads.map((h) => attr(h, "data-frame")),
      l.code,
    ).toEqual(["narrow", "wide"]);
    for (const head of heads) {
      const svg = figure.slice(head.index, figure.indexOf("</svg>", head.index));
      const where = `${l.code} ${attr(head, "data-frame")}`;
      expect(attr(head, "role"), where).toBe("img");
      expect(attr(head, "aria-label"), where).toBe(strs["modelsChartLabel"]);
      const groups = svg.split(/(?=<g data-model=)/).filter((g) => g.startsWith("<g data-model="));
      const points = groups.map((g) => {
        const t = tags(g, "g")[0];
        if (!t) {
          throw new Error(`${where}: a point without its group`);
        }
        const label = g.slice(g.indexOf("<text"), g.indexOf("</text>"));
        return {
          id: attr(t, "data-model"),
          speed: Number(attr(t, "data-speed")),
          accuracy: Number(attr(t, "data-accuracy")),
          mb: Number(attr(t, "data-mb")),
          cx: Number(attr(t, "data-cx")),
          cy: Number(attr(t, "data-cy")),
          r: Number(attr(t, "data-r")),
          label: text(label).replace(/\s/g, ""),
          accent: g.includes("fill-accent "),
        };
      });
      expect(
        points.map((p) => [p.id, p.speed, p.accuracy, p.mb]),
        where,
      ).toEqual(models.map((m) => [m.id, m.speed, m.accuracy, m.mb]));
      expect(
        points.map((p) => p.label),
        where,
      ).toEqual(models.map((m) => (app.modelNames[m.id] ?? m.id).replace(/\s/g, "")));
      expect(
        points.filter((p) => p.accent).map((p) => p.id),
        where,
      ).toEqual(models.filter((m) => m.recommended).map((m) => m.id));
      for (const a of points) {
        for (const b of points) {
          // Faster is further right and more accurate higher up; the same rating is the same line.
          expect(Math.sign(b.cx - a.cx), `${where}: ${a.id} and ${b.id} across`).toBe(Math.sign(b.speed - a.speed));
          expect(Math.sign(a.cy - b.cy), `${where}: ${a.id} and ${b.id} up`).toBe(Math.sign(b.accuracy - a.accuracy));
        }
      }
      // The bubble's area is in proportion to the download size, to the rounding of its radius.
      const biggest = points.reduce((a, b) => (b.mb > a.mb ? b : a));
      for (const p of points) {
        expect(p.r, `${where}: ${p.id}'s radius`).toBeCloseTo(biggest.r * Math.sqrt(p.mb / biggest.mb), 0);
      }
    }
  }
});

it("lists every model with its ratings and size on each page", () => {
  const models = readModels(repoRoot);
  const names = readModelNames(repoRoot, models);
  for (const l of builtLangs()) {
    const app = loadAppStrings(repoRoot, l, models, names);
    const table = section(readOut(pageFile(l)), 'class="models-table', "</table>");
    const head = table.slice(table.indexOf("<thead"), table.indexOf("</thead>"));
    const headings = head
      .split(/(?=<th[\s>])/)
      .filter((h) => /^<th[\s>]/.test(h))
      .map((h) => text(h).trim());
    expect(headings.slice(1, 3), l.code).toEqual([app.modelWords.speed, app.modelWords.accuracy]);
    const body = table.slice(table.indexOf("<tbody"));
    const rows = body.split(/(?=<tr[\s>])/).filter((r) => r.startsWith("<tr"));
    expect(
      rows.map((r) => Number(attr(tags(r, "tr")[0]!, "data-mb"))),
      l.code,
    ).toEqual(models.map((m) => m.mb));
    expect(rows.filter((r) => attr(tags(r, "tr")[0]!, "class") === "recommended").length, l.code).toBe(1);
    rows.forEach((r, i) => {
      const m = models[i]!;
      const cells = r.split("<td").slice(1);
      expect(cells.length, `${l.code} ${m.id}`).toBe(3);
      const rating = (cell: string, name: string, value: number) => {
        const td = tags("<td" + cell, "td")[0]!;
        expect(attr(td, name), `${l.code} ${m.id} ${name}`).toBe(String(value));
        // The dots are hidden from screen readers, which read the value out of 5 instead.
        expect(text("<td" + cell).trim(), `${l.code} ${m.id} ${name}`).toBe(`${value}/${ratingSteps}`);
        expect(cell.split("bg-accent").length - 1, `${l.code} ${m.id} ${name}'s filled dots`).toBe(value);
        expect(cell.split("bg-ink/12").length - 1, `${l.code} ${m.id} ${name}'s empty dots`).toBe(ratingSteps - value);
      };
      rating(cells[0] ?? "", "data-speed", m.speed);
      rating(cells[1] ?? "", "data-accuracy", m.accuracy);
      expect(text(r.slice(r.indexOf("<th"), r.indexOf("</th>"))).includes(app.modelNames[m.id] ?? m.id), `${l.code} ${m.id}'s name`).toBe(true);
    });
  }
});

// The agents read the same choice: the models item of each language's WebMCP content file gives every model's speed and accuracy out of 5 and its size, in the app's words, and marks the recommended one.
it("gives every model's speed, accuracy and size in the WebMCP content files", () => {
  const models = readModels(repoRoot);
  const names = readModelNames(repoRoot, models);
  for (const l of builtLangs()) {
    const app = loadAppStrings(repoRoot, l, models, names);
    const data = JSON.parse(readOut(`webmcp/${l.code}.json`)) as { features: { items: { title: string; text: string }[] } };
    const item = data.features.items.find((i) => i.title.includes("whisper") && i.text.includes(`/${ratingSteps}`));
    expect(item, l.code).toBeDefined();
    for (const m of models) {
      const name = app.modelNames[m.id] ?? m.id;
      const stats = (app.modelStats[m.id] ?? "").replace(/\s+/g, " ");
      expect(stats, `${l.code} ${m.id}`).toContain(`${app.modelWords.speed} ${m.speed}/${ratingSteps}`);
      expect(stats, `${l.code} ${m.id}`).toContain(`${app.modelWords.accuracy} ${m.accuracy}/${ratingSteps}`);
      const shown = m.recommended ? `${name} (${app.app.settingsSpeechRecommended}): ${stats}` : `${name}: ${stats}`;
      expect(item?.text, `${l.code} ${m.id}`).toContain(shown);
    }
  }
});
