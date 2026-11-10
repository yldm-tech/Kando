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
      <p className="mt-5 mb-0 text-center text-sm">
        <a
          className="group inline-flex items-center gap-1.5 rounded-full px-3.5 py-1.5 font-semibold text-accent-ink transition-colors duration-300 hover:bg-accent/[0.08] hover:text-accent-hover"
          href={pagePath(lang.code, "sites")}
        >
          {tf("sitesAll", { count: site.total })}
          <span aria-hidden="true" className="transition-transform duration-300 group-hover:translate-x-0.5">
            →
          </span>
        </a>
      </p>
    </section>
  );
}
