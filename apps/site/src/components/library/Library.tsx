// The library: a .kando folder of ordinary files, beside a screenshot of it in the page's language.
import site from "virtual:kando/site";
import { Rich, useWords } from "../../i18n/content";

export function Library() {
  const { t, lang } = useWords();
  const shot = site.shots[lang.code]?.library;
  return (
    <section className="section split" id="library" aria-labelledby="library-title">
      <div className="wrap split-in">
        <div className="split-text rv">
          <p className="eyebrow">{t("libraryEyebrow")}</p>
          <h2 id="library-title">
            <Rich text={t("libraryTitle")} />
          </h2>
          <p className="body">
            <Rich text={t("libraryBody")} />
          </p>
          <ul className="points">
            <li>{t("libraryPoint1")}</li>
            <li>{t("libraryPoint2")}</li>
            <li>{t("libraryPoint3")}</li>
          </ul>
        </div>
        <figure className="split-media rv">
          <div className="win light">
            {shot && <img src={shot.src} width={shot.w} height={shot.h} alt={t("shotLibraryAlt")} loading="lazy" decoding="async" />}
          </div>
        </figure>
      </div>
    </section>
  );
}
