// A language's page of every website the app downloads from, the app's whole catalog. Rendered on the server it is a plain table that reads complete without script; once the page runs, a search box filters it as the app's Supported Websites sheet does (catalog.Site.Matches), and only the rows on screen are drawn.
import catalog from "virtual:kando/catalog";
import site from "virtual:kando/site";
import { useHydrated } from "@tanstack/react-router";
import { useState } from "react";
import { useWords } from "../../i18n/content";
import { Footer } from "../footer/Footer";
import { Header } from "../header/Header";
import { SiteMark } from "../sites-strip/SitesStrip";
import { SitesSearch } from "./SitesSearch";
import { localRows, type Row } from "./rows";

export function SitesPage() {
  const { t, tf, lang } = useWords();
  const hydrated = useHydrated();
  const [rows] = useState(() => localRows(catalog, lang, site.siteNames[lang.code] ?? {}));
  return (
    <>
      <Header page="sites" />
      <main id="main" className="sites-page">
        <div className="wrap">
          <header className="head">
            <h1 id="sites-title">{t("sitesPageTitle")}</h1>
            <p className="lead">{tf("sitesPageLead", { count: site.total, ytdlp: site.ytdlp })}</p>
          </header>
          {hydrated ? <SitesSearch rows={rows} /> : <StaticTable rows={rows} />}
        </div>
      </main>
      <Footer page="sites" />
    </>
  );
}

// SitesHead is the table's header row.
export function SitesHead() {
  const { t } = useWords();
  return (
    <thead>
      <tr>
        <th scope="col">{t("sitesColumnName")}</th>
        <th scope="col">{t("sitesColumnDomain")}</th>
      </tr>
    </thead>
  );
}

export function SiteRow({ row }: { row: Row }) {
  return (
    <tr className={row.featured ? "featured" : undefined}>
      <th scope="row" lang={row.lang}>
        <SiteMark tile={row} />
        <span>{row.name}</span>
      </th>
      <td>{row.domain}</td>
    </tr>
  );
}

// StaticTable is every site, as the server renders the page.
function StaticTable({ rows }: { rows: readonly Row[] }) {
  const { t } = useWords();
  return (
    <table className="sites-table" aria-labelledby="sites-title">
      <caption className="sr">{t("sitesSearchLabel")}</caption>
      <SitesHead />
      <tbody>
        {rows.map((row) => (
          <SiteRow key={row.domain} row={row} />
        ))}
      </tbody>
    </table>
  );
}
