// The whisper models the app offers, as packages/media/whisper_model.go lists them: a wide card in the details, right under the whisper card. The chart places each model by its speed and accuracy with its size as the bubble, so that the choice reads at a glance; the table beside it says the same in words, each rating as dots with its value for screen readers, and the recommended model marked. Both are rendered by the server and read complete without script; once the page runs, the bubbles grow into place as the chart scrolls in, unless the reader asked for less motion.
import { animate, inView, stagger } from "motion";
import { useEffect, useRef } from "react";
import site from "virtual:kando/site";
import { useWords } from "../../i18n/content";
import { ratingSteps } from "../../lib/site";
import { ease, lessMotion } from "../common/motion";
import { ModelsChart } from "./ModelsChart";

// Rating is a model's speed or accuracy as filled and empty dots out of ratingSteps, with its value as text for screen readers.
function Rating({ value }: { value: number }) {
  return (
    <>
      <span aria-hidden="true" className="inline-flex gap-[3px] align-middle">
        {Array.from({ length: ratingSteps }, (_, i) => (
          <span key={i} className={"size-[7px] rounded-full " + (i < value ? "bg-accent" : "bg-ink/12")} />
        ))}
      </span>
      <span className="sr-only">{`${value}/${ratingSteps}`}</span>
    </>
  );
}

export function Models() {
  const { t, tf, app, content, lang } = useWords();
  const nf = new Intl.NumberFormat(lang.tag);
  const card = useRef<HTMLLIElement>(null);
  const models = site.models.map((m) => ({ ...m, name: content.modelNames[m.id] ?? m.id }));
  const words = content.modelWords;

  // The bubbles of a chart below the screen when the page hydrates start at nothing and grow to their sizes as it comes into view; a chart already on screen stays as it is, so that nothing the reader sees blinks.
  useEffect(() => {
    const el = card.current?.querySelector<HTMLElement>(".models-chart");
    if (!el || lessMotion() || el.getBoundingClientRect().top < window.innerHeight) {
      return;
    }
    const bubbles = [...el.querySelectorAll<SVGElement>("[data-bubble]")];
    for (const b of bubbles) {
      b.style.transform = "scale(0)";
    }
    const stop = inView(
      el,
      () => {
        void animate(bubbles, { transform: ["scale(0)", "scale(1)"] }, { duration: 0.7, ease, delay: stagger(0.06, { startDelay: 0.2 }) });
      },
      { amount: 0.3 },
    );
    return () => {
      stop();
      for (const b of bubbles) {
        b.style.removeProperty("transform");
      }
    };
  }, []);

  return (
    <li
      ref={card}
      className="card detail rv col-span-full grid gap-x-12 gap-y-6 rounded-[22px] p-[30px] narrow:p-6 min-[981px]:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]"
      id="models"
      aria-labelledby="models-title"
    >
      <div className="flex min-w-0 flex-col">
        <p className="m-0 text-[13px] font-bold text-accent">{t("modelsEyebrow")}</p>
        <h3 id="models-title" className="mt-2 text-[clamp(22px,2.4vw,28px)] leading-tight font-extrabold tracking-[-0.02em]">
          {tf("modelsTitle", { count: site.models.length })}
        </h3>
        <p className="max-w-[520px]">{t("modelsLead")}</p>
        <ModelsChart models={models} words={words} label={t("modelsChartLabel")} caption={t("modelsChartCaption")} />
      </div>
      <table className="models-table w-full self-start border-collapse text-[15px] phone:text-[14px]">
        <caption className="mb-3 text-left text-[13px] text-ink-3">{t("modelsTableCaption")}</caption>
        <thead>
          <tr className="shadow-[inset_0_-1px_0_rgba(28,25,22,0.12)]">
            <th scope="col" className="px-3 py-2 text-left text-[13px] font-semibold text-ink-3 phone:px-1.5">
              {t("modelsColumnModel")}
            </th>
            <th scope="col" className="px-3 py-2 text-left text-[13px] font-semibold whitespace-nowrap text-ink-3 phone:px-1.5">
              {words.speed}
            </th>
            <th scope="col" className="px-3 py-2 text-left text-[13px] font-semibold whitespace-nowrap text-ink-3 phone:px-1.5">
              {words.accuracy}
            </th>
            <th scope="col" className="px-3 py-2 text-right text-[13px] font-semibold whitespace-nowrap text-ink-3 phone:px-1.5 phone:whitespace-normal">
              {t("modelsAxis")}
            </th>
          </tr>
        </thead>
        <tbody>
          {models.map((m) => (
            <tr key={m.id} className={m.recommended ? "recommended" : undefined} data-mb={m.mb}>
              <th
                scope="row"
                className="px-3 py-[9px] text-left font-medium shadow-[inset_0_-1px_0_rgba(28,25,22,0.07)] phone:px-1.5 [.recommended>&]:text-accent-ink"
              >
                {m.name}
                {m.recommended && (
                  <span className="ml-2 inline-block rounded-full bg-accent/12 px-2 py-0.5 align-[1px] text-[11px] font-semibold whitespace-nowrap text-accent-ink">
                    {app("settingsSpeechRecommended")}
                  </span>
                )}
              </th>
              <td className="px-3 py-[9px] whitespace-nowrap shadow-[inset_0_-1px_0_rgba(28,25,22,0.07)] phone:px-1.5" data-speed={m.speed}>
                <Rating value={m.speed} />
              </td>
              <td className="px-3 py-[9px] whitespace-nowrap shadow-[inset_0_-1px_0_rgba(28,25,22,0.07)] phone:px-1.5" data-accuracy={m.accuracy}>
                <Rating value={m.accuracy} />
              </td>
              <td className="px-3 py-[9px] text-right font-mono text-[13px] text-ink-2 tabular-nums shadow-[inset_0_-1px_0_rgba(28,25,22,0.07)] phone:px-1.5">
                <data value={String(m.mb)}>{nf.format(m.mb)}</data>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </li>
  );
}
