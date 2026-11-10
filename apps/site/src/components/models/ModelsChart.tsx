// The whisper models as a bubble chart of what each gives: speed across, accuracy up, and the bubble's area its download size, with the recommended model in the accent color. It is plain SVG the server renders, so it reads without script; it is drawn twice, for a narrow and a wide box, and the stylesheet shows the one that fits its container, because an SVG scaled to fit would scale its labels with it. The labels are placed when the page is rendered: each goes below, above, right or left of its bubble, on one line or two, wherever it first clears the bubbles, the labels placed before it and the chart's edges.
import { useId } from "react";
import { ratingSteps } from "../../lib/site";

export interface ChartModel {
  id: string;
  name: string;
  speed: number;
  accuracy: number;
  mb: number;
  recommended: boolean;
}

// Frame is the geometry of one of the chart's two drawings, in its viewBox's units, which are pixels at the width it is shown at.
interface Frame {
  w: number;
  h: number;
  font: number;
  // rMax is the radius of the largest model's bubble; the others' areas are in proportion to their sizes.
  rMax: number;
  pad: { l: number; r: number; t: number; b: number };
}

export const frames = {
  narrow: { w: 320, h: 320, font: 12, rMax: 24, pad: { l: 24, r: 8, t: 26, b: 46 } },
  wide: { w: 560, h: 340, font: 13, rMax: 30, pad: { l: 30, r: 12, t: 26, b: 46 } },
} as const satisfies Record<string, Frame>;

export interface Box {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

export interface PlacedModel extends ChartModel {
  cx: number;
  cy: number;
  r: number;
  // label is where the model's name is set, and box the room it takes, as estimated.
  label: { x: number; ys: number[]; lines: string[]; anchor: "start" | "middle" | "end"; box: Box };
}

// textWidth estimates the width of s set at size: the server has no fonts to measure with, so each character counts as a share of the size, a full one for Chinese, Japanese and Korean and for full-width punctuation.
export function textWidth(s: string, size: number): number {
  let em = 0;
  for (const ch of s) {
    const cp = ch.codePointAt(0) ?? 0;
    if (cp >= 0x2e80) {
      em += 1;
    } else if (ch === " ") {
      em += 0.3;
    } else if ("()[].,'|il1".includes(ch)) {
      em += 0.34;
    } else if (/[A-Z]/.test(ch)) {
      em += 0.68;
    } else {
      em += 0.58;
    }
  }
  return em * size;
}

// splits are the ways a name can be set: on one line, or on two, broken before its parenthesis (full-width or not) or else at the space nearest its middle.
function splits(name: string): string[][] {
  const out = [[name]];
  const paren = Math.max(name.lastIndexOf(" ("), name.lastIndexOf("（"));
  if (paren > 0) {
    out.push([name.slice(0, paren).trim(), name.slice(paren).trim()]);
    return out;
  }
  const spaces: number[] = [];
  for (let i = name.indexOf(" "); i >= 0; i = name.indexOf(" ", i + 1)) {
    spaces.push(i);
  }
  const mid = spaces.sort((a, b) => Math.abs(a - name.length / 2) - Math.abs(b - name.length / 2))[0];
  if (mid !== undefined) {
    out.push([name.slice(0, mid), name.slice(mid + 1)]);
  }
  return out;
}

function overlap(a: Box, b: Box): number {
  const w = Math.min(a.x1, b.x1) - Math.max(a.x0, b.x0);
  const h = Math.min(a.y1, b.y1) - Math.max(a.y0, b.y0);
  return w > 0 && h > 0 ? w * h : 0;
}

export interface Words {
  speed: string;
  accuracy: string;
}

// layout places the models in frame f: the scales run half a step past the lowest rating any model has and past the top of the scale, so that no bubble sits on an edge. The axes' titles are the words with arrows that point the better way: the vertical one above the plot at its left, the horizontal one under the ticks at its right, and the labels keep clear of them.
export function layout(models: readonly ChartModel[], f: Frame, words: Words) {
  const tickFont = f.font - 1;
  const xTitle = `${words.speed} →`;
  const yTitle = `${words.accuracy} ↑`;
  const titles: Box[] = [
    { x0: 0, y0: 0, x1: textWidth(yTitle, tickFont) + 4, y1: tickFont + 6 },
    { x0: f.w - f.pad.r - textWidth(xTitle, tickFont) - 4, y0: f.h - tickFont - 6, x1: f.w, y1: f.h },
  ];
  const xLo = Math.min(...models.map((m) => m.speed)) - 0.5;
  const yLo = Math.min(...models.map((m) => m.accuracy)) - 0.5;
  const hi = ratingSteps + 0.5;
  const x = (v: number) => f.pad.l + ((v - xLo) / (hi - xLo)) * (f.w - f.pad.l - f.pad.r);
  const y = (v: number) => f.pad.t + ((hi - v) / (hi - yLo)) * (f.h - f.pad.t - f.pad.b);
  const range = (lo: number) => Array.from({ length: ratingSteps - Math.ceil(lo) + 1 }, (_, i) => Math.ceil(lo) + i);
  const maxMB = Math.max(...models.map((m) => m.mb));
  const bubbles = models.map((m) => ({ ...m, cx: x(m.speed), cy: y(m.accuracy), r: f.rMax * Math.sqrt(m.mb / maxMB) }));
  const gap = 4;
  const lineH = f.font * 1.2;
  const bounds: Box = { x0: f.pad.l + 2, y0: 0, x1: f.w, y1: f.h - f.pad.b };
  const taken: Box[] = [...titles, ...bubbles.map((b) => ({ x0: b.cx - b.r, y0: b.cy - b.r, x1: b.cx + b.r, y1: b.cy + b.r }))];
  // The recommended model's label is placed first, so that it gets the best place; then the others from the top left down.
  const order = [...bubbles].sort((a, b) => Number(b.recommended) - Number(a.recommended) || b.accuracy - a.accuracy || a.speed - b.speed);
  const labels = new Map<string, PlacedModel["label"]>();
  for (const b of order) {
    let best: { score: number; label: PlacedModel["label"] } | undefined;
    for (const lines of splits(b.name)) {
      const width = Math.max(...lines.map((s) => textWidth(s, f.font)));
      const height = lines.length * lineH;
      const r = b.r + gap;
      const sides = [
        { anchor: "middle", x: b.cx, top: b.cy + r },
        { anchor: "middle", x: b.cx, top: b.cy - r - height },
        { anchor: "start", x: b.cx + r, top: b.cy - height / 2 },
        { anchor: "end", x: b.cx - r, top: b.cy - height / 2 },
      ] as const;
      for (const side of sides) {
        const x0 = side.anchor === "start" ? side.x : side.anchor === "end" ? side.x - width : side.x - width / 2;
        const box = { x0: x0 - 2, y0: side.top - 1, x1: x0 + width + 2, y1: side.top + height + 1 };
        const outside = (box.x1 - box.x0) * (box.y1 - box.y0) - overlap(box, bounds);
        const score = outside * 4 + taken.reduce((sum, t) => sum + overlap(box, t), 0);
        const label = { x: side.x, ys: lines.map((_, i) => side.top + lineH * (i + 0.5)), lines, anchor: side.anchor, box };
        if (!best || score < best.score) {
          best = { score, label };
        }
        if (score === 0) {
          break;
        }
      }
      if (best?.score === 0) {
        break;
      }
    }
    if (best) {
      taken.push(best.label.box);
      labels.set(b.id, best.label);
    }
  }
  const placed = bubbles.map((b) => {
    const label = labels.get(b.id);
    if (!label) {
      throw new Error(`no place for the label of ${b.id}`);
    }
    return { ...b, label };
  });
  return { placed, bounds, titles, tickFont, xTitle, yTitle, x, y, ticks: { x: range(xLo), y: range(yLo) } };
}

const round = (v: number) => Math.round(v * 10) / 10;

// Drawing is one of the chart's drawings, in frame f.
function Drawing({ models, f, kind, words, label }: { models: readonly ChartModel[]; f: Frame; kind: keyof typeof frames; words: Words; label: string }) {
  const glow = useId();
  const { placed, tickFont, xTitle, yTitle, x, y, ticks } = layout(models, f, words);
  const plot = { x0: f.pad.l, y0: f.pad.t, x1: f.w - f.pad.r, y1: f.h - f.pad.b };
  return (
    <svg
      viewBox={`0 0 ${f.w} ${f.h}`}
      width={f.w}
      height={f.h}
      role="img"
      aria-label={label}
      data-frame={kind}
      className={
        kind === "narrow"
          ? "block h-auto w-full max-w-[400px] overflow-visible font-sans @min-[460px]:hidden"
          : "hidden h-auto w-full max-w-[640px] overflow-visible font-sans @min-[460px]:block"
      }
    >
      <defs>
        <radialGradient id={glow} cx="1" cy="0" r="0.9">
          <stop offset="0" stopColor="#e5651d" stopOpacity="0.12" />
          <stop offset="1" stopColor="#e5651d" stopOpacity="0" />
        </radialGradient>
      </defs>
      <rect x={plot.x0} y={plot.y0} width={plot.x1 - plot.x0} height={plot.y1 - plot.y0} rx="10" fill={`url(#${glow})`} />
      <g aria-hidden="true" className="fill-ink-3 stroke-ink/10" fontSize={tickFont}>
        {ticks.x.map((v) => (
          <g key={`x${v}`}>
            <line x1={round(x(v))} x2={round(x(v))} y1={plot.y0} y2={plot.y1} strokeDasharray="2 4" />
            <text x={round(x(v))} y={plot.y1 + tickFont + 6} textAnchor="middle" stroke="none">
              {v}
            </text>
          </g>
        ))}
        {ticks.y.map((v) => (
          <g key={`y${v}`}>
            <line x1={plot.x0} x2={plot.x1} y1={round(y(v))} y2={round(y(v))} strokeDasharray="2 4" />
            <text x={plot.x0 - 8} y={round(y(v))} textAnchor="end" dominantBaseline="central" stroke="none">
              {v}
            </text>
          </g>
        ))}
        <text x={0} y={tickFont / 2 + 2} dominantBaseline="central" stroke="none" className="fill-ink-2 font-semibold">
          {yTitle}
        </text>
        <text x={f.w - f.pad.r} y={f.h - tickFont / 2 - 2} textAnchor="end" dominantBaseline="central" stroke="none" className="fill-ink-2 font-semibold">
          {xTitle}
        </text>
      </g>
      {placed.map((m) => (
        <g
          key={m.id}
          data-model={m.id}
          data-speed={m.speed}
          data-accuracy={m.accuracy}
          data-mb={m.mb}
          data-cx={round(m.cx)}
          data-cy={round(m.cy)}
          data-r={round(m.r)}
        >
          {m.recommended && (
            <circle cx={round(m.cx)} cy={round(m.cy)} r={round(m.r + 5)} fill="none" strokeWidth="2" className="stroke-accent/35" data-bubble="" />
          )}
          <circle
            cx={round(m.cx)}
            cy={round(m.cy)}
            r={round(m.r)}
            strokeWidth="1.5"
            className={(m.recommended ? "fill-accent stroke-accent-hover" : "fill-[#f3b05e]/70 stroke-[#e8953a]") + " origin-center [transform-box:fill-box]"}
            data-bubble=""
          />
          <text
            fontSize={f.font}
            textAnchor={m.label.anchor}
            dominantBaseline="central"
            className={m.recommended ? "fill-accent-ink font-bold" : "fill-ink-2 font-medium"}
          >
            {m.label.lines.map((line, i) => (
              <tspan key={line} x={round(m.label.x)} y={round(m.label.ys[i] ?? m.cy)}>
                {line}
              </tspan>
            ))}
          </text>
        </g>
      ))}
    </svg>
  );
}

// ModelsChart is the chart in its figure, with caption, the caption saying how to read it.
export function ModelsChart({ models, words, label, caption }: { models: readonly ChartModel[]; words: Words; label: string; caption: string }) {
  return (
    <figure className="models-chart @container mt-6 mb-0">
      <Drawing models={models} f={frames.narrow} kind="narrow" words={words} label={label} />
      <Drawing models={models} f={frames.wide} kind="wide" words={words} label={label} />
      <figcaption className="mt-3 max-w-[520px] text-[13px] leading-relaxed text-ink-3">{caption}</figcaption>
    </figure>
  );
}
