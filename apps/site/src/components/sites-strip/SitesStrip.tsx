// The works-with strip: the catalog's count and its featured sites, scrolling by twice so that the loop has no seam; the second copy is hidden from screen readers.
import site from "virtual:kando/site";
import { useWords } from "../../i18n/content";
import { pagePath } from "../../lib/site";
import type { Tile } from "../../../buildtime/schema";

export function SiteMark({ tile }: { tile: Pick<Tile, "logo" | "initial" | "plate"> }) {
  if (tile.logo) {
    return <img src={tile.logo} width="22" height="22" alt="" loading="lazy" decoding="async" />;
  }
  return (
    <span className={"initial plate-" + (tile.plate ?? 0)} aria-hidden="true">
      {tile.initial}
    </span>
  );
}

export function SitesStrip() {
  const { t, tf, lang } = useWords();
  const tiles = site.tiles[lang.code] ?? [];
  return (
    <section className="sites" aria-labelledby="sites-title">
      <p className="sites-lead" id="sites-title">
        {tf("sitesLead", { count: site.total })}
      </p>
      <div className="marquee">
        <div className="marquee-track">
          <ul className="logos" aria-label={t("sitesStripLabel")}>
            {tiles.map((tile) => (
              <li key={tile.domain} title={tile.name} lang={tile.lang}>
                <SiteMark tile={tile} />
                <span>{tile.name}</span>
              </li>
            ))}
          </ul>
          <ul className="logos" aria-hidden="true">
            {tiles.map((tile) => (
              <li key={tile.domain} lang={tile.lang}>
                <SiteMark tile={tile} />
                <span>{tile.name}</span>
              </li>
            ))}
          </ul>
        </div>
      </div>
      <p className="sites-all">
        <a href={pagePath(lang.code, "sites")}>{tf("sitesAll", { count: site.total })}</a>
      </p>
    </section>
  );
}
