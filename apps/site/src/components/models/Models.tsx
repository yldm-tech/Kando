// The whisper models the app offers, with their sizes, as packages/media/whisper_model.go lists them: a table that reads complete without script, which the chart draws as bars once the page runs.
import { useHydrated } from "@tanstack/react-router";
import site from "virtual:kando/site";
import { useWords } from "../../i18n/content";
import { ModelsChart } from "./ModelsChart";

export function Models() {
  const { t, tf, app, content, lang } = useWords();
  const nf = new Intl.NumberFormat(lang.tag);
  const hydrated = useHydrated();
  return (
    <section className="section" id="models" aria-labelledby="models-title">
      <div className="wrap">
        <header className="head rv">
          <p className="eyebrow">{t("modelsEyebrow")}</p>
          <h2 id="models-title">{tf("modelsTitle", { count: site.models.length })}</h2>
          <p className="body">{t("modelsLead")}</p>
        </header>
        <div className="models rv">
          {hydrated && (
            <div className="models-chart">
              <ModelsChart />
            </div>
          )}
          <table className="models-table">
            <caption>{t("modelsTableCaption")}</caption>
            <thead>
              <tr>
                <th scope="col">{t("modelsColumnModel")}</th>
                <th scope="col">{t("modelsAxis")}</th>
              </tr>
            </thead>
            <tbody>
              {site.models.map((m) => (
                <tr key={m.id} className={m.recommended ? "recommended" : undefined} data-mb={m.mb}>
                  <th scope="row">
                    {content.modelNames[m.id] ?? m.id}
                    {m.recommended && <span className="badge-soft">{app("settingsSpeechRecommended")}</span>}
                  </th>
                  <td>
                    <data value={String(m.mb)}>{nf.format(m.mb)}</data>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}
