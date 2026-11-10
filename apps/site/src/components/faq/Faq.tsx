// Questions, as native disclosures that read complete without script; with it, one is open at a time.
import site from "virtual:kando/site";
import { useRef, type SyntheticEvent } from "react";
import { useWords, type ContentKey } from "../../i18n/content";
import { issuesURL } from "../../lib/site";

const questions: readonly [ContentKey, ContentKey][] = [
  ["faqPriceQ", "faqPriceA"],
  ["faqSitesQ", "faqSitesA"],
  ["faqSpeechQ", "faqSpeechA"],
  ["faqTranslateQ", "faqTranslateA"],
  ["faqSystemsQ", "faqSystemsA"],
  ["faqLoginQ", "faqLoginA"],
  ["faqExportQ", "faqExportA"],
];

export function Faq() {
  const { t, tf } = useWords();
  const list = useRef<HTMLDivElement>(null);
  const onToggle = (event: SyntheticEvent<HTMLDetailsElement>) => {
    const opened = event.currentTarget;
    if (!opened.open) {
      return;
    }
    for (const other of list.current?.querySelectorAll("details.qa") ?? []) {
      if (other !== opened && other instanceof HTMLDetailsElement) {
        other.open = false;
      }
    }
  };
  return (
    <section className="section" id="faq" aria-labelledby="faq-title">
      <div className="wrap faq-in">
        <div className="faq-head rv">
          <p className="eyebrow">{t("faqEyebrow")}</p>
          <h2 id="faq-title">{t("faqTitle")}</h2>
          <p>
            {t("faqLead") + " "}
            <a href={issuesURL}>{t("faqAsk")}</a>
          </p>
        </div>
        <div className="faq-list" ref={list}>
          {questions.map(([q, a], i) => (
            <details key={q} className="qa" open={i === 0 ? true : undefined} onToggle={onToggle}>
              <summary>
                {t(q)}
                <span className="plus" aria-hidden="true" />
              </summary>
              <p>{a === "faqSitesA" ? tf("faqSitesA", { count: site.total }) : t(a)}</p>
            </details>
          ))}
        </div>
      </div>
    </section>
  );
}
