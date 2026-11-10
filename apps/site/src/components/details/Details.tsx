// Made for language learners: the nine detail cards, in the order buildtime/webmcp.ts's detailKeys lists them for list_features.
import site from "virtual:kando/site";
import { useWords, type ContentKey } from "../../i18n/content";
import { everyAPIURL } from "../../lib/site";

// cards are the details, each with the mark in its icon: a symbol, or a content string where the mark is a word in the page's language.
const cards: readonly { title: ContentKey; body: ContentKey; icon: { symbol: string } | { mark: ContentKey }; ai?: true }[] = [
  { title: "detailReplayTitle", body: "detailReplayBody", icon: { symbol: "↺" } },
  { title: "detailModesTitle", body: "detailModesBody", icon: { mark: "detailModesMark" } },
  { title: "detailSearchTitle", body: "detailSearchBody", icon: { symbol: "⌕" } },
  { title: "detailWhisperTitle", body: "detailWhisperBody", icon: { symbol: "W" } },
  { title: "detailAITitle", body: "detailAIBody", icon: { symbol: "AI" }, ai: true },
  { title: "detailBrowserTitle", body: "detailBrowserBody", icon: { symbol: "⌂" } },
  { title: "detailUploadTitle", body: "detailUploadBody", icon: { symbol: "☁" } },
  { title: "detailShortcutsTitle", body: "detailShortcutsBody", icon: { symbol: "⌘" } },
  { title: "detailLanguagesTitle", body: "detailLanguagesBody", icon: { mark: "detailLanguagesMark" } },
];

export function Details() {
  const { t } = useWords();
  return (
    <section className="section" id="features" aria-labelledby="features-title">
      <div className="wrap">
        <header className="head rv">
          <p className="eyebrow">{t("detailsEyebrow")}</p>
          <h2 id="features-title">{t("detailsTitle")}</h2>
        </header>
        <ul className="details">
          {cards.map((card) => (
            <li key={card.title} className="card detail rv">
              <span className="ic" aria-hidden="true">
                {"mark" in card.icon ? t(card.icon.mark) : card.icon.symbol}
              </span>
              <h3>{t(card.title)}</h3>
              <p>{t(card.body)}</p>
              {card.ai && (
                <>
                  <ul className="brands">
                    {site.aiServices.map((b) => (
                      <li key={b.id}>
                        <img src={b.src} width="20" height="20" alt={b.name} title={b.name} loading="lazy" decoding="async" />
                      </li>
                    ))}
                  </ul>
                  <p className="more">
                    <a href={everyAPIURL}>
                      {t("detailAILink")} <span aria-hidden="true">→</span>
                    </a>
                  </p>
                </>
              )}
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}
