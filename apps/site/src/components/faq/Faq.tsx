// Questions, as native disclosures that read complete without script; with it, one is open at a time, and an answer unfolds and folds away instead of appearing and vanishing.
import site from "virtual:kando/site";
import { useRef, type MouseEvent, type SyntheticEvent } from "react";
import { useWords } from "../../i18n/content";
import { faqPairs, issuesURL } from "../../lib/site";
import { fold, hold, unfold } from "../common/motion";

export function Faq() {
  const { t, tf } = useWords();
  const list = useRef<HTMLDivElement>(null);
  // clicked is the question a click has just opened, whose toggle event has then nothing left to do.
  const clicked = useRef<HTMLDetailsElement | null>(null);
  // foldOthers folds away every open answer but opened's.
  const foldOthers = (opened: HTMLDetailsElement) => {
    for (const other of list.current?.querySelectorAll("details.qa") ?? []) {
      if (other !== opened && other instanceof HTMLDetailsElement && other.open) {
        fold(other.querySelector("p"), () => {
          other.open = false;
        });
      }
    }
  };
  // A click on a question's summary opens or closes it here rather than by the disclosure's own action, so that the answer starts folded in the frame it is shown instead of flashing at its full height first; the question clicked stays under the pointer while an answer above it folds away.
  const onSummary = (event: MouseEvent<HTMLElement>) => {
    const summary = event.currentTarget;
    const details = summary.parentElement;
    if (!(details instanceof HTMLDetailsElement)) {
      return;
    }
    event.preventDefault();
    if (details.open) {
      fold(details.querySelector("p"), () => {
        details.open = false;
      });
      return;
    }
    clicked.current = details;
    details.open = true;
    unfold(details.querySelector("p"));
    foldOthers(details);
    hold(summary, 420);
  };
  // A question opened otherwise, as the browser's find in page opens one, closes the others too.
  const onToggle = (event: SyntheticEvent<HTMLDetailsElement>) => {
    const opened = event.currentTarget;
    if (opened === clicked.current) {
      clicked.current = null;
    } else if (opened.open) {
      foldOthers(opened);
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
          {faqPairs.map(([q, a], i) => (
            <details key={q} className="qa" open={i === 0 ? true : undefined} onToggle={onToggle}>
              <summary onClick={onSummary}>
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
