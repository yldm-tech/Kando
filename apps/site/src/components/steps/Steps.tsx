// From a link to bilingual subtitles: the four steps of a job.
import { useWords, type ContentKey } from "../../i18n/content";

const steps: readonly { tool: string | null; title: ContentKey; body: ContentKey }[] = [
  { tool: "yt-dlp", title: "stepDownloadTitle", body: "stepDownloadBody" },
  { tool: "whisper.cpp", title: "stepTranscribeTitle", body: "stepTranscribeBody" },
  { tool: null, title: "stepTranslateTitle", body: "stepTranslateBody" },
  { tool: ".srt", title: "stepExportTitle", body: "stepExportBody" },
];

export function Steps() {
  const { t } = useWords();
  return (
    <section className="section" id="how" aria-labelledby="how-title">
      <div className="wrap">
        <header className="head rv">
          <p className="eyebrow">{t("stepsEyebrow")}</p>
          <h2 id="how-title">{t("stepsTitle")}</h2>
        </header>
        <ol className="steps rv">
          {steps.map((step, i) => (
            <li key={step.title} className="card step">
              <p className="step-top">
                <span className="n" aria-hidden="true">
                  {i + 1}
                </span>
                {step.tool ? <code>{step.tool}</code> : <span className="tool">{t("stepTranslateTool")}</span>}
              </p>
              <h3>{t(step.title)}</h3>
              <p>{t(step.body)}</p>
              <span className="bar" aria-hidden="true">
                <i />
              </span>
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}
