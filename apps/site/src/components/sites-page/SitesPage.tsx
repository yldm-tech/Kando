// A language's page of every website the app downloads from, the app's whole catalog. Rendered on the server it is a plain table that reads complete without script; once the page runs, a search box filters it as the app's Supported Websites sheet does (catalog.Site.Matches), and only the rows on screen are drawn.
import catalog from "virtual:kando/catalog";
import site from "virtual:kando/site";
import { useHydrated } from "@tanstack/react-router";
import { useState, type ReactNode } from "react";
import { useWords } from "../../i18n/content";
import { Footer } from "../footer/Footer";
import { Header } from "../header/Header";
import { SiteMark } from "../sites-strip/SitesStrip";
import { SitesSearch } from "./SitesSearch";
import { localRows, type Row } from "./rows";

// Classes shared by the table the server renders and the one the search draws.
export const tableClass = "sites-table w-full border-collapse text-[15px]";
const cellClass = "h-11 px-4 text-left shadow-[inset_0_-1px_0_rgba(28,25,22,0.06)]";

export function SitesPage() {
  const { t, tf, lang } = useWords();
  const hydrated = useHydrated();
  const [rows] = useState(() => localRows(catalog, lang, site.siteNames[lang.code] ?? {}));
  return (
    <>
      <Header page="sites" />
      <main id="main" className="relative pt-[132px] pb-[100px] phone:pt-[108px]">
        <div className="pointer-events-none absolute inset-x-0 top-0 h-[520px] overflow-hidden" aria-hidden="true">
          <span className="blob blob-a" />
          <span className="blob blob-b" />
        </div>
        <div className="wrap relative">
          <header className="head mx-auto max-w-[720px]">
            <h1 id="sites-title" className="text-[clamp(34px,4.6vw,56px)] leading-[1.12] font-extrabold tracking-[-0.03em]">
              {t("sitesPageTitle")}
            </h1>
            <p className="mx-auto mt-[18px] max-w-[600px] text-[17px] leading-[1.75] text-ink-2">
              {tf("sitesPageLead", { count: site.total, ytdlp: site.ytdlp })}
            </p>
          </header>
          <div className="mx-auto mt-10 max-w-[880px]">{hydrated ? <SitesSearch rows={rows} /> : <StaticTable rows={rows} />}</div>
        </div>
      </main>
      <Footer page="sites" />
    </>
  );
}

// SitesHead is the table's header row, which stays under the page's header as the list scrolls.
export function SitesHead() {
  const { t } = useWords();
  const head = "sticky top-[64px] z-10 bg-[#fdfbf8]/95 backdrop-blur-sm text-[13px] font-semibold text-ink-3 " + cellClass;
  return (
    <thead>
      <tr aria-rowindex={1}>
        <th scope="col" className={head}>
          {t("sitesColumnName")}
        </th>
        <th scope="col" className={head}>
          {t("sitesColumnDomain")}
        </th>
      </tr>
    </thead>
  );
}

// rowIndex is the row's position among all the table's rows, header first, for a table that renders only some of them (aria-rowindex).
export function SiteRow({ row, rowIndex }: { row: Row; rowIndex?: number }) {
  return (
    <tr className={(row.featured ? "featured " : "") + "transition-colors duration-200 hover:bg-accent/[0.05]"} aria-rowindex={rowIndex}>
      <th scope="row" lang={row.lang} className={cellClass + " font-medium"}>
        <span className="flex items-center gap-3 [&>.initial]:size-[22px] [&>.initial]:text-xs [&>.initial]:opacity-100 [&>img]:size-[22px]">
          <SiteMark tile={row} />
          <span>{row.name}</span>
        </span>
      </th>
      <td className={cellClass + " font-mono text-[13px] text-ink-2"}>{row.domain}</td>
    </tr>
  );
}

// TableCard is the card the table sits in.
export function TableCard({ children }: { children: ReactNode }) {
  return <div className="card overflow-clip rounded-[22px] px-2 pb-2">{children}</div>;
}

// StaticTable is every site, as the server renders the page.
function StaticTable({ rows }: { rows: readonly Row[] }) {
  const { t } = useWords();
  return (
    <TableCard>
      <table className={tableClass} aria-labelledby="sites-title">
        <caption className="sr">{t("sitesSearchLabel")}</caption>
        <SitesHead />
        <tbody>
          {rows.map((row) => (
            <SiteRow key={row.domain} row={row} />
          ))}
        </tbody>
      </table>
    </TableCard>
  );
}
