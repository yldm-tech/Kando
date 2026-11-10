// The library: a .kando folder of ordinary files, beside a screenshot of it in the page's language.
import site from "virtual:kando/site";
import { Rich, useWords } from "../../i18n/content";

// shotSizes is the screenshot's width in the layout (.split-in): beside the text from 912 pixels, where the two fit on a line, up to 614 pixels at the widest wrap, and the width of the page below its padding when it wraps under the text.
const shotSizes = "(min-width: 1176px) 614px, (min-width: 912px) 52vw, calc(100vw - 40px)";

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
            {shot && (
              <picture>
                {shot.sources.map((s) => (
                  <source key={s.type} type={s.type} srcSet={s.srcset} sizes={shotSizes} />
                ))}
                <img src={shot.src} width={shot.w} height={shot.h} alt={t("shotLibraryAlt")} loading="lazy" decoding="async" />
              </picture>
            )}
          </div>
        </figure>
      </div>
    </section>
  );
}
