// The whisper models' sizes as bars, drawn once the page runs, in a chunk of its own; the table beside it is what the server renders. The recommended model's bar is the accent color. onShown is called once the chart is on the page, for its fade.
import { defineChart, barX } from "@tanstack/charts";
import { Chart } from "@tanstack/charts/react";
import { scaleBand } from "@tanstack/charts/scales/band";
import { scaleLinear } from "@tanstack/charts/scales/linear";
import { useEffect, useMemo } from "react";
import site from "virtual:kando/site";
import { useWords } from "../../i18n/content";

export default function ModelsChart({ onShown }: { onShown: () => void }) {
  const { t, content, lang } = useWords();
  const definition = useMemo(() => {
    const nf = new Intl.NumberFormat(lang.tag);
    const rows = site.models.map((m) => ({ id: m.id, name: content.modelNames[m.id] ?? m.id, mb: m.mb, recommended: m.recommended }));
    return defineChart({
      marks: [barX(rows, { y: "name", x: "mb", fill: (r) => (r.recommended ? "#e5651d" : "#f3b05e") })],
      scales: {
        y: { scale: () => scaleBand<string>().padding(0.2) },
        x: { scale: scaleLinear, nice: true, axis: { label: t("modelsAxis"), ticks: { format: (v: number) => nf.format(v) } } },
      },
    });
  }, [t, content, lang]);
  useEffect(onShown, [onShown]);
  return <Chart definition={definition} height={280} initialWidth={640} ariaLabel={t("modelsChartLabel")} />;
}
