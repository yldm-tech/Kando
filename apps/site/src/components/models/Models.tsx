// The whisper models the app offers, with their sizes, as packages/media/whisper_model.go lists them: a wide card in the details, right under the whisper card. The table reads complete without script; once the page runs, the chart beside it draws the same sizes as bars, its code fetched only as the card nears the screen.
import { useHydrated } from "@tanstack/react-router";
import { animate, inView } from "motion";
import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import site from "virtual:kando/site";
import { useWords } from "../../i18n/content";
import { ease, lessMotion } from "../common/motion";

const ModelsChart = lazy(() => import("./ModelsChart"));

export function Models() {
  const { t, tf, app, content, lang } = useWords();
  const nf = new Intl.NumberFormat(lang.tag);
  const hydrated = useHydrated();
  const chartBox = useRef<HTMLDivElement>(null);
  const [wanted, setWanted] = useState(false);

  useEffect(() => {
    const el = chartBox.current;
    if (!el) {
      return;
    }
    return inView(el, () => setWanted(true), { margin: "0px 0px 600px 0px" });
  }, []);

  const shown = useCallback(() => {
    if (chartBox.current && !lessMotion()) {
      void animate(chartBox.current, { opacity: [0, 1] }, { duration: 0.8, ease });
    }
  }, []);

  return (
    <li
      className="card detail rv col-span-full grid gap-x-12 gap-y-6 rounded-[22px] p-[30px] narrow:p-6 min-[981px]:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]"
      id="models"
      aria-labelledby="models-title"
    >
      <div className="flex min-w-0 flex-col">
        <p className="m-0 text-[13px] font-bold text-accent">{t("modelsEyebrow")}</p>
        <h3 id="models-title" className="mt-2 text-[clamp(22px,2.4vw,28px)] leading-tight font-extrabold tracking-[-0.02em]">
          {tf("modelsTitle", { count: site.models.length })}
        </h3>
        <p className="max-w-[520px]">{t("modelsLead")}</p>
        <div ref={chartBox} className="ts-chart mt-6 hidden min-h-[280px] in-[.js]:block">
          {hydrated && wanted && (
            <Suspense fallback={null}>
              <ModelsChart onShown={shown} />
            </Suspense>
          )}
        </div>
      </div>
      <table className="models-table w-full self-start border-collapse text-[15px]">
        <caption className="mb-3 text-left text-[13px] text-ink-3">{t("modelsTableCaption")}</caption>
        <thead>
          <tr className="shadow-[inset_0_-1px_0_rgba(28,25,22,0.12)]">
            <th scope="col" className="px-3 py-2 text-left text-[13px] font-semibold text-ink-3">
              {t("modelsColumnModel")}
            </th>
            <th scope="col" className="px-3 py-2 text-right text-[13px] font-semibold text-ink-3">
              {t("modelsAxis")}
            </th>
          </tr>
        </thead>
        <tbody>
          {site.models.map((m) => (
            <tr key={m.id} className={m.recommended ? "recommended" : undefined} data-mb={m.mb}>
              <th scope="row" className="px-3 py-[9px] text-left font-medium shadow-[inset_0_-1px_0_rgba(28,25,22,0.07)] [.recommended>&]:text-accent-ink">
                {content.modelNames[m.id] ?? m.id}
                {m.recommended && (
                  <span className="ml-2 rounded-full bg-accent/12 px-2 py-0.5 align-[1px] text-[11px] font-semibold whitespace-nowrap text-accent-ink">
                    {app("settingsSpeechRecommended")}
                  </span>
                )}
              </th>
              <td className="px-3 py-[9px] text-right font-mono text-[13px] text-ink-2 tabular-nums shadow-[inset_0_-1px_0_rgba(28,25,22,0.07)]">
                <data value={String(m.mb)}>{nf.format(m.mb)}</data>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </li>
  );
}
